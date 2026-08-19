package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"pokernode/internal/newapi"
	"pokernode/internal/poker"
	"pokernode/internal/store"
)

const (
	mainTableID = "main"
	minBuyIn    = int64(2_000)
	maxBuyIn    = int64(100_000)
)

type tableEnvelope struct {
	Type  string         `json:"type"`
	Table poker.Snapshot `json:"table"`
}

type tableSummary struct {
	ID                   string       `json:"id"`
	Name                 string       `json:"name"`
	SmallBlind           int64        `json:"small_blind_cents"`
	BigBlind             int64        `json:"big_blind_cents"`
	ActionTimeoutSeconds int          `json:"action_timeout_seconds"`
	PlayerCount          int          `json:"player_count"`
	MaxPlayers           int          `json:"max_players"`
	HandID               int64        `json:"hand_id"`
	Street               poker.Street `json:"street"`
	ViewerSeated         bool         `json:"viewer_seated"`
	Players              []tableSeat  `json:"players"`
}

type tableSeat struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Seat   int    `json:"seat"`
	Stack  int64  `json:"stack_cents"`
}

func (s *Server) handleListTables(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, err := s.spaceForActor(r.Context(), r.PathValue("spaceID"), user)
	if err != nil {
		return err
	}
	ids, err := s.store.TableStateIDs(r.Context(), space.ID)
	if err != nil {
		return err
	}
	tables := make([]tableSummary, 0, len(ids))
	for _, tableID := range ids {
		runtime, err := s.runtimeForTable(r.Context(), space.ID, tableID)
		if err != nil {
			return err
		}
		tables = append(tables, summarizeTable(runtime.table.Snapshot(user.ID)))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
	return nil
}

func (s *Server) handleCreateTable(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, err := s.spaceForActor(r.Context(), r.PathValue("spaceID"), user)
	if err != nil {
		return err
	}
	if !space.CanManage {
		return &apiError{Status: http.StatusForbidden, Message: "只有频道管理员可以创建牌桌"}
	}
	var input struct {
		Name                 string `json:"name"`
		SmallBlind           int64  `json:"small_blind_cents"`
		BigBlind             int64  `json:"big_blind_cents"`
		ActionTimeoutSeconds *int   `json:"action_timeout_seconds"`
	}
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 40 {
		return &apiError{Status: http.StatusBadRequest, Message: "牌桌名称需为 1–40 个字符"}
	}
	if input.SmallBlind <= 0 || input.BigBlind < input.SmallBlind || input.BigBlind > 100_000 {
		return &apiError{Status: http.StatusBadRequest, Message: "盲注设置不正确"}
	}
	actionTimeoutSeconds := poker.DefaultActionTimeoutSeconds
	if input.ActionTimeoutSeconds != nil {
		actionTimeoutSeconds = *input.ActionTimeoutSeconds
	}
	if actionTimeoutSeconds < poker.MinActionTimeoutSeconds || actionTimeoutSeconds > poker.MaxActionTimeoutSeconds {
		return &apiError{Status: http.StatusBadRequest, Message: "行动时限需为 5–300 秒"}
	}
	tableID := uuid.NewString()
	table := poker.NewTable(tableID, input.Name, input.SmallBlind, input.BigBlind)
	if err := table.SetActionTimeoutSeconds(actionTimeoutSeconds); err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: err.Error()}
	}
	if err := s.persistTable(r.Context(), space.ID, tableID, table); err != nil {
		return err
	}
	s.tablesMu.Lock()
	s.tables[tableRoomKey(space.ID, tableID)] = &tableRuntime{table: table}
	s.tablesMu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{"table": summarizeTable(table.Snapshot(user.ID))})
	return nil
}

func (s *Server) handleDeleteTable(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, err := s.spaceForActor(r.Context(), r.PathValue("spaceID"), user)
	if err != nil {
		return err
	}
	if !space.CanManage {
		return &apiError{Status: http.StatusForbidden, Message: "只有频道管理员可以删除牌桌"}
	}
	tableID := tableIDFromRequest(r)
	runtime, err := s.runtimeForTable(r.Context(), space.ID, tableID)
	if err != nil {
		return err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.deleted {
		return store.ErrNotFound
	}
	if len(runtime.table.Snapshot(user.ID).Players) > 0 {
		return &apiError{Status: http.StatusConflict, Message: "牌桌还有玩家，请先让所有玩家结算离桌"}
	}
	runtime.deleted = true
	if err := s.store.DeleteTableState(r.Context(), space.ID, tableID); err != nil {
		runtime.deleted = false
		return err
	}
	if runtime.timer != nil {
		runtime.timer.Stop()
		runtime.timer = nil
	}
	runtime.timerTurnID = 0
	s.tablesMu.Lock()
	key := tableRoomKey(space.ID, tableID)
	if s.tables[key] == runtime {
		delete(s.tables, key)
	}
	s.tablesMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) handleTable(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, err := s.spaceForActor(r.Context(), r.PathValue("spaceID"), user)
	if err != nil {
		return err
	}
	runtime, err := s.runtimeForTable(r.Context(), space.ID, tableIDFromRequest(r))
	if err != nil {
		return err
	}
	if err := lockTableRuntime(runtime); err != nil {
		return err
	}
	defer runtime.mu.Unlock()
	writeJSON(w, http.StatusOK, tableEnvelope{Type: "table", Table: runtime.table.Snapshot(user.ID)})
	return nil
}

func (s *Server) handleTableJoin(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, member, memberToken, err := s.memberCredentials(r, user.ID)
	if err != nil {
		return err
	}
	var input struct {
		BuyIn int64 `json:"buy_in_cents"`
	}
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	if input.BuyIn < minBuyIn || input.BuyIn > maxBuyIn {
		return &apiError{Status: http.StatusBadRequest, Message: "买入金额需在 $20–$1,000 之间"}
	}
	quota, err := newapi.CentsToQuota(input.BuyIn, space.QuotaPerUSD)
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: err.Error()}
	}
	tableID := tableIDFromRequest(r)
	runtime, err := s.runtimeForTable(r.Context(), space.ID, tableID)
	if err != nil {
		return err
	}
	if err := lockTableRuntime(runtime); err != nil {
		return err
	}
	defer runtime.mu.Unlock()
	if runtime.table.IsSeated(user.ID) {
		return &apiError{Status: http.StatusConflict, Message: "你已经在牌桌上"}
	}
	current, err := s.newAPI.Self(r.Context(), space.BaseURL, memberToken)
	if err != nil {
		return &apiError{Status: http.StatusBadGateway, Message: "买入前验证个人余额失败，请检查频道连接和个人凭证"}
	}
	if current.ID != member.NewAPIUserID || current.Status != 1 {
		return &apiError{Status: http.StatusConflict, Message: "个人凭证已失效或用户发生变化，请重新绑定"}
	}
	if current.Quota < quota {
		return &apiError{Status: http.StatusPaymentRequired, Message: "New API 余额不足"}
	}
	adminToken, err := s.adminToken(space)
	if err != nil {
		return err
	}
	operationID := uuid.NewString()
	operation := store.WalletOperation{ID: operationID, SpaceID: space.ID, TableID: tableID, UserID: user.ID, NewAPIUserID: member.NewAPIUserID, Kind: "buy_in", Cents: input.BuyIn, Quota: quota, Status: "pending"}
	if err := s.store.CreateWalletOperation(r.Context(), operation); err != nil {
		return err
	}
	if err := s.newAPI.AdjustQuota(r.Context(), space.BaseURL, adminToken, member.NewAPIUserID, quota, false); err != nil {
		_ = s.store.UpdateWalletOperation(r.Context(), operationID, "manual_review", err.Error())
		return &apiError{Status: http.StatusBadGateway, Message: "New API 扣款未确认，已停止自动重试，请频道管理员核对操作 " + operationID}
	}
	if _, err := runtime.table.Join(user.ID, user.DisplayName, input.BuyIn); err != nil {
		refundErr := s.newAPI.AdjustQuota(r.Context(), space.BaseURL, adminToken, member.NewAPIUserID, quota, true)
		status := "compensated"
		message := err.Error()
		if refundErr != nil {
			status = "manual_review"
			message += "; refund: " + refundErr.Error()
		}
		_ = s.store.UpdateWalletOperation(r.Context(), operationID, status, message)
		return pokerAPIError(err)
	}
	if err := s.persistTable(r.Context(), space.ID, tableID, runtime.table); err != nil {
		_, _ = runtime.table.Leave(user.ID)
		refundErr := s.newAPI.AdjustQuota(r.Context(), space.BaseURL, adminToken, member.NewAPIUserID, quota, true)
		status := "compensated"
		message := err.Error()
		if refundErr != nil {
			status = "manual_review"
			message += "; refund: " + refundErr.Error()
		}
		_ = s.store.UpdateWalletOperation(r.Context(), operationID, status, message)
		return err
	}
	if err := s.store.UpdateWalletOperation(r.Context(), operationID, "completed", ""); err != nil {
		s.logger.Error("mark buy-in operation complete", "operation_id", operationID, "error", err)
	}
	s.broadcast(space.ID, tableID, runtime)
	writeJSON(w, http.StatusOK, map[string]any{"operation_id": operationID, "table": runtime.table.Snapshot(user.ID)})
	return nil
}

func (s *Server) handleTableLeave(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, member, _, err := s.memberCredentials(r, user.ID)
	if err != nil {
		return err
	}
	tableID := tableIDFromRequest(r)
	runtime, err := s.runtimeForTable(r.Context(), space.ID, tableID)
	if err != nil {
		return err
	}
	if err := lockTableRuntime(runtime); err != nil {
		return err
	}
	defer runtime.mu.Unlock()
	stack, seated := runtime.table.StackFor(user.ID)
	if !seated {
		return &apiError{Status: http.StatusConflict, Message: "你不在牌桌上"}
	}
	if !runtime.table.Snapshot(user.ID).CanLeave {
		return &apiError{Status: http.StatusConflict, Message: "一手牌进行中，暂时不能离桌"}
	}
	if stack == 0 {
		if _, err := runtime.table.Leave(user.ID); err != nil {
			return pokerAPIError(err)
		}
		if err := s.persistTable(r.Context(), space.ID, tableID, runtime.table); err != nil {
			return err
		}
		s.broadcast(space.ID, tableID, runtime)
		writeJSON(w, http.StatusOK, map[string]any{"settled_cents": 0, "table": runtime.table.Snapshot(user.ID)})
		return nil
	}
	quota, err := newapi.CentsToQuota(stack, space.QuotaPerUSD)
	if err != nil {
		return err
	}
	adminToken, err := s.adminToken(space)
	if err != nil {
		return err
	}
	operationID := uuid.NewString()
	operation := store.WalletOperation{ID: operationID, SpaceID: space.ID, TableID: tableID, UserID: user.ID, NewAPIUserID: member.NewAPIUserID, Kind: "cash_out", Cents: stack, Quota: quota, Status: "pending"}
	if err := s.store.CreateWalletOperation(r.Context(), operation); err != nil {
		return err
	}
	if err := s.newAPI.AdjustQuota(r.Context(), space.BaseURL, adminToken, member.NewAPIUserID, quota, true); err != nil {
		_ = s.store.UpdateWalletOperation(r.Context(), operationID, "manual_review", err.Error())
		return &apiError{Status: http.StatusBadGateway, Message: "New API 结算未确认，玩家仍保留在牌桌，请频道管理员核对操作 " + operationID}
	}
	settled, err := runtime.table.Leave(user.ID)
	if err != nil {
		_ = s.store.UpdateWalletOperation(r.Context(), operationID, "manual_review", "balance credited but local leave failed: "+err.Error())
		return pokerAPIError(err)
	}
	if err := s.persistTable(r.Context(), space.ID, tableID, runtime.table); err != nil {
		_ = s.store.UpdateWalletOperation(r.Context(), operationID, "manual_review", "balance credited but local persistence failed: "+err.Error())
		return err
	}
	if err := s.store.UpdateWalletOperation(r.Context(), operationID, "completed", ""); err != nil {
		s.logger.Error("mark cash-out operation complete", "operation_id", operationID, "error", err)
	}
	s.broadcast(space.ID, tableID, runtime)
	writeJSON(w, http.StatusOK, map[string]any{"operation_id": operationID, "settled_cents": settled, "table": runtime.table.Snapshot(user.ID)})
	return nil
}

func (s *Server) handleTableReady(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, err := s.store.SpaceForUser(r.Context(), r.PathValue("spaceID"), user.ID)
	if err != nil {
		return err
	}
	tableID := tableIDFromRequest(r)
	runtime, err := s.runtimeForTable(r.Context(), space.ID, tableID)
	if err != nil {
		return err
	}
	if err := lockTableRuntime(runtime); err != nil {
		return err
	}
	defer runtime.mu.Unlock()
	if _, err := runtime.table.Ready(user.ID); err != nil {
		return pokerAPIError(err)
	}
	if err := s.persistTable(r.Context(), space.ID, tableID, runtime.table); err != nil {
		return err
	}
	s.syncTableTimerLocked(space.ID, tableID, runtime)
	s.broadcast(space.ID, tableID, runtime)
	writeJSON(w, http.StatusOK, tableEnvelope{Type: "table", Table: runtime.table.Snapshot(user.ID)})
	return nil
}

func (s *Server) handleTableAction(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, err := s.store.SpaceForUser(r.Context(), r.PathValue("spaceID"), user.ID)
	if err != nil {
		return err
	}
	var input struct {
		Action poker.ActionType `json:"action"`
		Amount int64            `json:"amount_cents"`
	}
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	tableID := tableIDFromRequest(r)
	runtime, err := s.runtimeForTable(r.Context(), space.ID, tableID)
	if err != nil {
		return err
	}
	if err := lockTableRuntime(runtime); err != nil {
		return err
	}
	defer runtime.mu.Unlock()
	if err := runtime.table.Act(user.ID, input.Action, input.Amount); err != nil {
		return pokerAPIError(err)
	}
	if err := s.persistTable(r.Context(), space.ID, tableID, runtime.table); err != nil {
		return err
	}
	s.syncTableTimerLocked(space.ID, tableID, runtime)
	s.broadcast(space.ID, tableID, runtime)
	writeJSON(w, http.StatusOK, tableEnvelope{Type: "table", Table: runtime.table.Snapshot(user.ID)})
	return nil
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, err := s.spaceForActor(r.Context(), r.PathValue("spaceID"), user)
	if err != nil {
		return err
	}
	tableID := tableIDFromRequest(r)
	runtime, err := s.runtimeForTable(r.Context(), space.ID, tableID)
	if err != nil {
		return err
	}
	if err := s.hub.Serve(tableRoomKey(space.ID, tableID), user.ID, func(viewerID int64) any {
		return tableEnvelope{Type: "table", Table: runtime.table.Snapshot(viewerID)}
	}, s.websocketOriginPatterns, w, r); err != nil {
		s.logger.Warn("accept realtime connection", "origin", r.Header.Get("Origin"), "host", r.Host, "error", err)
	}
	return nil
}

func lockTableRuntime(runtime *tableRuntime) error {
	runtime.mu.Lock()
	if runtime.deleted {
		runtime.mu.Unlock()
		return store.ErrNotFound
	}
	return nil
}

func (s *Server) runtimeForTable(ctx context.Context, spaceID, tableID string) (*tableRuntime, error) {
	s.tablesMu.Lock()
	defer s.tablesMu.Unlock()
	key := tableRoomKey(spaceID, tableID)
	if runtime := s.tables[key]; runtime != nil {
		return runtime, nil
	}
	data, err := s.store.LoadTableState(ctx, spaceID, tableID)
	if err != nil {
		return nil, err
	}
	table, err := poker.RestoreTable(data)
	if err != nil {
		return nil, err
	}
	runtime := &tableRuntime{table: table}
	s.tables[key] = runtime
	runtime.mu.Lock()
	s.syncTableTimerLocked(spaceID, tableID, runtime)
	runtime.mu.Unlock()
	return runtime, nil
}

func (s *Server) persistTable(ctx context.Context, spaceID, tableID string, table *poker.Table) error {
	data, err := table.MarshalState()
	if err != nil {
		return err
	}
	return s.store.SaveTableState(ctx, spaceID, tableID, data)
}

func (s *Server) broadcast(spaceID, tableID string, runtime *tableRuntime) {
	s.hub.Broadcast(tableRoomKey(spaceID, tableID), func(viewerID int64) any {
		return tableEnvelope{Type: "table", Table: runtime.table.Snapshot(viewerID)}
	})
}

func (s *Server) syncTableTimerLocked(spaceID, tableID string, runtime *tableRuntime) {
	if runtime.timer != nil {
		runtime.timer.Stop()
		runtime.timer = nil
	}
	runtime.timerTurnID = 0
	snapshot := runtime.table.Snapshot(0)
	if snapshot.ActingSeat < 0 || snapshot.ActionDeadlineAt <= 0 || snapshot.TurnID == 0 {
		return
	}
	delay := time.Until(time.UnixMilli(snapshot.ActionDeadlineAt))
	if delay < 0 {
		delay = 0
	}
	runtime.timerTurnID = snapshot.TurnID
	runtime.timer = time.AfterFunc(delay, func() {
		s.handleTableTimeout(spaceID, tableID, runtime, snapshot.TurnID)
	})
}

func (s *Server) handleTableTimeout(spaceID, tableID string, runtime *tableRuntime, turnID uint64) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.deleted || runtime.timerTurnID != turnID {
		return
	}
	runtime.timer = nil
	runtime.timerTurnID = 0
	action, applied, err := runtime.table.Timeout(turnID, time.Now())
	if err != nil {
		s.logger.Error("apply table action timeout", "space_id", spaceID, "table_id", tableID, "turn_id", turnID, "error", err)
		return
	}
	if !applied {
		s.syncTableTimerLocked(spaceID, tableID, runtime)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.persistTable(ctx, spaceID, tableID, runtime.table); err != nil {
		s.logger.Error("persist table action timeout", "space_id", spaceID, "table_id", tableID, "turn_id", turnID, "action", action, "error", err)
	}
	s.broadcast(spaceID, tableID, runtime)
	s.syncTableTimerLocked(spaceID, tableID, runtime)
}

func tableIDFromRequest(r *http.Request) string {
	if tableID := r.PathValue("tableID"); tableID != "" {
		return tableID
	}
	return mainTableID
}

func tableRoomKey(spaceID, tableID string) string {
	return spaceID + ":" + tableID
}

func summarizeTable(snapshot poker.Snapshot) tableSummary {
	players := make([]tableSeat, 0, len(snapshot.Players))
	for _, player := range snapshot.Players {
		players = append(players, tableSeat{
			UserID: player.UserID,
			Name:   player.Name,
			Seat:   player.Seat,
			Stack:  player.Stack,
		})
	}
	return tableSummary{
		ID: snapshot.ID, Name: snapshot.Name, SmallBlind: snapshot.SmallBlind, BigBlind: snapshot.BigBlind,
		ActionTimeoutSeconds: snapshot.ActionTimeoutSeconds,
		PlayerCount:          len(snapshot.Players), MaxPlayers: poker.MaxSeats, HandID: snapshot.HandID, Street: snapshot.Street,
		ViewerSeated: snapshot.ViewerSeat >= 0, Players: players,
	}
}

func pokerAPIError(err error) error {
	if errors.Is(err, poker.ErrActionTimedOut) {
		return &apiError{Status: http.StatusConflict, Message: "本轮行动时间已结束"}
	}
	status := http.StatusBadRequest
	if errors.Is(err, poker.ErrHandInProgress) || errors.Is(err, poker.ErrAlreadySeated) || errors.Is(err, poker.ErrNotYourTurn) {
		status = http.StatusConflict
	}
	return &apiError{Status: status, Message: err.Error()}
}
