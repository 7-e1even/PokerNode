package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"pokernode/internal/landlord"
	"pokernode/internal/newapi"
	"pokernode/internal/poker"
	"pokernode/internal/store"
)

const (
	mainTableID = "main"
	minBuyIn    = int64(2_000)
	maxBuyIn    = int64(100_000)

	kickVoteDuration   = 60 * time.Second
	kickRejoinCooldown = 5 * time.Minute
)

type tableEnvelope struct {
	Type     string         `json:"type"`
	Table    poker.Snapshot `json:"table"`
	KickVote *kickVoteView  `json:"kick_vote"`
	Notice   string         `json:"notice,omitempty"`
}

type landlordTableEnvelope struct {
	Type     string            `json:"type"`
	Table    landlord.Snapshot `json:"table"`
	KickVote *kickVoteView     `json:"kick_vote"`
	Notice   string            `json:"notice,omitempty"`
}

type kickVote struct {
	TargetUserID    int64
	TargetName      string
	InitiatorUserID int64
	InitiatorName   string
	EligibleVoters  map[int64]struct{}
	YesVoters       map[int64]struct{}
	NoVoters        map[int64]struct{}
	RequiredYes     int
	ExpiresAt       time.Time
}

type kickVoteView struct {
	TargetUserID  int64  `json:"target_user_id"`
	TargetName    string `json:"target_name"`
	InitiatorName string `json:"initiator_name"`
	YesCount      int    `json:"yes_count"`
	NoCount       int    `json:"no_count"`
	RequiredYes   int    `json:"required_yes"`
	EligibleCount int    `json:"eligible_count"`
	ExpiresAt     int64  `json:"expires_at"`
	ViewerVote    string `json:"viewer_vote,omitempty"`
	CanVote       bool   `json:"can_vote"`
}

type tableSummary struct {
	ID                   string      `json:"id"`
	Name                 string      `json:"name"`
	GameType             string      `json:"game_type"`
	SmallBlind           int64       `json:"small_blind_cents,omitempty"`
	BigBlind             int64       `json:"big_blind_cents,omitempty"`
	BaseStake            int64       `json:"base_stake_cents,omitempty"`
	ActionTimeoutSeconds int         `json:"action_timeout_seconds"`
	PlayerCount          int         `json:"player_count"`
	MaxPlayers           int         `json:"max_players"`
	HandID               int64       `json:"hand_id"`
	Street               string      `json:"street"`
	ViewerSeated         bool        `json:"viewer_seated"`
	Players              []tableSeat `json:"players"`
}

type tableSeat struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Seat   int    `json:"seat"`
	Stack  int64  `json:"stack_cents"`
	Ready  bool   `json:"ready"`
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
		tables = append(tables, summarizeRuntime(runtime, user.ID))
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
		GameType             string `json:"game_type"`
		Name                 string `json:"name"`
		SmallBlind           int64  `json:"small_blind_cents"`
		BigBlind             int64  `json:"big_blind_cents"`
		BaseStake            int64  `json:"base_stake_cents"`
		ActionTimeoutSeconds *int   `json:"action_timeout_seconds"`
	}
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 40 {
		return &apiError{Status: http.StatusBadRequest, Message: "牌桌名称需为 1–40 个字符"}
	}
	if input.GameType == "" {
		input.GameType = gameTypeTexasHoldem
	}
	if input.GameType != gameTypeTexasHoldem && input.GameType != gameTypeLandlord {
		return &apiError{Status: http.StatusBadRequest, Message: "暂不支持这种游戏类型"}
	}
	if input.GameType == gameTypeTexasHoldem && (input.SmallBlind <= 0 || input.BigBlind < input.SmallBlind || input.BigBlind > 100_000) {
		return &apiError{Status: http.StatusBadRequest, Message: "盲注设置不正确"}
	}
	if input.GameType == gameTypeLandlord && (input.BaseStake <= 0 || input.BaseStake > 100_000) {
		return &apiError{Status: http.StatusBadRequest, Message: "底分需在 $0.01–$1,000 之间"}
	}
	actionTimeoutSeconds := poker.DefaultActionTimeoutSeconds
	if input.ActionTimeoutSeconds != nil {
		actionTimeoutSeconds = *input.ActionTimeoutSeconds
	}
	if actionTimeoutSeconds < poker.MinActionTimeoutSeconds || actionTimeoutSeconds > poker.MaxActionTimeoutSeconds {
		return &apiError{Status: http.StatusBadRequest, Message: "行动时限需为 5–300 秒"}
	}
	tableID := uuid.NewString()
	var runtime *tableRuntime
	if input.GameType == gameTypeLandlord {
		table := landlord.NewTable(tableID, input.Name, input.BaseStake)
		if err := table.SetActionTimeoutSeconds(actionTimeoutSeconds); err != nil {
			return &apiError{Status: http.StatusBadRequest, Message: err.Error()}
		}
		runtime = newLandlordRuntime(table)
	} else {
		table := poker.NewTable(tableID, input.Name, input.SmallBlind, input.BigBlind)
		if err := table.SetActionTimeoutSeconds(actionTimeoutSeconds); err != nil {
			return &apiError{Status: http.StatusBadRequest, Message: err.Error()}
		}
		runtime = newPokerRuntime(table)
	}
	if err := s.persistRuntime(r.Context(), space.ID, tableID, runtime); err != nil {
		return err
	}
	s.tablesMu.Lock()
	s.tables[tableRoomKey(space.ID, tableID)] = runtime
	s.tablesMu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{"table": summarizeRuntime(runtime, user.ID)})
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
	players := runtime.commonSnapshot(user.ID).Players
	if len(players) > 0 && r.URL.Query().Get("force") != "true" {
		return &apiError{Status: http.StatusConflict, Message: "牌桌还有玩家；管理员可选择结算全部玩家后强制删除"}
	}
	if len(players) > 0 {
		if _, _, err := s.settleAllPlayersLocked(r.Context(), space, tableID, runtime, user.ID, "管理员强制删除牌桌"); err != nil {
			s.broadcast(space.ID, tableID, runtime)
			return err
		}
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
	s.hub.Broadcast(tableRoomKey(space.ID, tableID), func(int64) any {
		return map[string]any{"type": "table_deleted", "table_id": tableID}
	})
	s.tablesMu.Lock()
	key := tableRoomKey(space.ID, tableID)
	if s.tables[key] == runtime {
		delete(s.tables, key)
	}
	s.tablesMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) handleClearTable(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, err := s.spaceForActor(r.Context(), r.PathValue("spaceID"), user)
	if err != nil {
		return err
	}
	if !space.CanManage {
		return &apiError{Status: http.StatusForbidden, Message: "只有频道管理员可以清空牌桌"}
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
	settledPlayers, settledCents, err := s.settleAllPlayersLocked(r.Context(), space, tableID, runtime, user.ID, "管理员强制清空牌桌")
	if err != nil {
		s.broadcast(space.ID, tableID, runtime)
		return err
	}
	s.broadcast(space.ID, tableID, runtime)
	writeJSON(w, http.StatusOK, map[string]any{
		"settled_players": settledPlayers,
		"settled_cents":   settledCents,
		"table":           summarizeRuntime(runtime, user.ID),
	})
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
	writeJSON(w, http.StatusOK, s.tableEnvelopeLocked(runtime, user.ID))
	return nil
}

func (s *Server) handleCurrentGame(w http.ResponseWriter, r *http.Request, user store.User) error {
	agentControlEnabled, err := s.store.AgentControlEnabled(r.Context(), user.ID)
	if err != nil {
		return err
	}
	spaceID, tableID, seated, err := s.userSeatedTableGlobally(r.Context(), user.ID)
	if err != nil {
		return err
	}
	if !seated {
		writeJSON(w, http.StatusOK, map[string]bool{"active": false, "agent_control_enabled": agentControlEnabled})
		return nil
	}
	data, err := s.store.LoadTableState(r.Context(), spaceID, tableID)
	if err != nil {
		return err
	}
	runtime, err := restoreTableRuntime(data)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active": true, "agent_control_enabled": agentControlEnabled,
		"space_id": spaceID, "table_id": tableID, "table": runtime.snapshot(user.ID),
	})
	return nil
}

func (s *Server) handleTableJoin(w http.ResponseWriter, r *http.Request, user store.User) error {
	if err := s.requireGameplayControl(r, user.ID); err != nil {
		return err
	}
	space, member, memberToken, err := s.memberCredentials(r, user.ID)
	if err != nil {
		return err
	}
	tableID := tableIDFromRequest(r)
	finishJoin, err := s.reserveTableJoin(r.Context(), space.ID, tableID, user.ID)
	if err != nil {
		return err
	}
	committed := false
	defer func() { finishJoin(committed) }()
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
	runtime, err := s.runtimeForTable(r.Context(), space.ID, tableID)
	if err != nil {
		return err
	}
	if err := lockTableRuntime(runtime); err != nil {
		return err
	}
	defer runtime.mu.Unlock()
	clearExpiredKickVoteLocked(runtime)
	if runtime.kickVote != nil {
		return &apiError{Status: http.StatusConflict, Message: "牌桌正在进行移出投票，请等待投票结束"}
	}
	if until := runtime.kickedUntil[user.ID]; time.Now().Before(until) {
		return &apiError{Status: http.StatusConflict, Message: "你刚被投票移出，5 分钟后才能重新入座"}
	} else if !until.IsZero() {
		delete(runtime.kickedUntil, user.ID)
	}
	if runtime.isSeated(user.ID) {
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
	if _, err := runtime.join(user.ID, user.DisplayName, input.BuyIn); err != nil {
		refundErr := s.newAPI.AdjustQuota(r.Context(), space.BaseURL, adminToken, member.NewAPIUserID, quota, true)
		status := "compensated"
		message := err.Error()
		if refundErr != nil {
			status = "manual_review"
			message += "; refund: " + refundErr.Error()
		}
		_ = s.store.UpdateWalletOperation(r.Context(), operationID, status, message)
		return tableGameAPIError(err)
	}
	if err := s.persistRuntime(r.Context(), space.ID, tableID, runtime); err != nil {
		_, _ = runtime.leave(user.ID)
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
	committed = true
	if err := s.store.ActivateTableSeat(r.Context(), user.ID, space.ID, tableID); err != nil {
		s.logger.Error("activate global table seat", "user_id", user.ID, "space_id", space.ID, "table_id", tableID, "error", err)
	}
	if err := s.store.UpdateWalletOperation(r.Context(), operationID, "completed", ""); err != nil {
		s.logger.Error("mark buy-in operation complete", "operation_id", operationID, "error", err)
	}
	s.broadcast(space.ID, tableID, runtime)
	writeJSON(w, http.StatusOK, map[string]any{"operation_id": operationID, "table": runtime.snapshot(user.ID)})
	return nil
}

func (s *Server) reserveTableJoin(ctx context.Context, spaceID, tableID string, userID int64) (func(bool), error) {
	s.tableJoinMu.Lock()
	if _, exists := s.tableJoinInFlight[userID]; exists {
		s.tableJoinMu.Unlock()
		return nil, &apiError{Status: http.StatusConflict, Message: "另一个入座请求正在处理中"}
	}
	s.tableJoinInFlight[userID] = struct{}{}
	s.tableJoinMu.Unlock()
	cleanupInFlight := func() {
		s.tableJoinMu.Lock()
		delete(s.tableJoinInFlight, userID)
		s.tableJoinMu.Unlock()
	}

	seatedSpaceID, seatedTableID, seated, err := s.userSeatedTableGlobally(ctx, userID)
	if err != nil {
		cleanupInFlight()
		return nil, err
	}
	if seated {
		cleanupInFlight()
		if seatedSpaceID == spaceID && seatedTableID == tableID {
			return nil, &apiError{Status: http.StatusConflict, Message: "你已经在牌桌上"}
		}
		return nil, &apiError{Status: http.StatusConflict, Message: fmt.Sprintf("你已经在其他牌局中（频道 %s，牌桌 %s），请先离桌", seatedSpaceID, seatedTableID)}
	}
	existing, err := s.store.ReserveActiveTableSeat(ctx, userID, spaceID, tableID)
	if errors.Is(err, store.ErrActiveTableSeat) && existing.Status == "active" {
		cleanupInFlight()
		if existing.SpaceID == spaceID && existing.TableID == tableID {
			return nil, &apiError{Status: http.StatusConflict, Message: "你已经在牌桌上"}
		}
		return nil, &apiError{Status: http.StatusConflict, Message: fmt.Sprintf("你已经在其他牌局中（频道 %s，牌桌 %s），请先离桌", existing.SpaceID, existing.TableID)}
	}
	if err != nil {
		cleanupInFlight()
		if errors.Is(err, store.ErrActiveTableSeat) {
			return nil, &apiError{Status: http.StatusConflict, Message: "另一个入座请求正在处理中，请稍后重试"}
		}
		return nil, err
	}
	return func(committed bool) {
		if !committed {
			cleanupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.store.ReleaseTableSeat(cleanupContext, userID, spaceID, tableID); err != nil {
				s.logger.Error("release failed table reservation", "user_id", userID, "space_id", spaceID, "table_id", tableID, "error", err)
			}
		}
		cleanupInFlight()
	}, nil
}

func (s *Server) handleTableLeave(w http.ResponseWriter, r *http.Request, user store.User) error {
	if err := s.requireGameplayControl(r, user.ID); err != nil {
		return err
	}
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
	stack, seated := runtime.stackFor(user.ID)
	if !seated {
		return &apiError{Status: http.StatusConflict, Message: "你不在牌桌上"}
	}
	if !runtime.commonSnapshot(user.ID).CanLeave {
		return &apiError{Status: http.StatusConflict, Message: "一手牌进行中，暂时不能离桌"}
	}
	if stack == 0 {
		if _, err := runtime.leave(user.ID); err != nil {
			return tableGameAPIError(err)
		}
		if err := s.persistRuntime(r.Context(), space.ID, tableID, runtime); err != nil {
			return err
		}
		if err := s.store.ReleaseTableSeat(r.Context(), user.ID, space.ID, tableID); err != nil {
			s.logger.Error("release global table seat", "user_id", user.ID, "space_id", space.ID, "table_id", tableID, "error", err)
		}
		runtime.kickVote = nil
		s.broadcast(space.ID, tableID, runtime)
		writeJSON(w, http.StatusOK, map[string]any{"settled_cents": 0, "table": runtime.snapshot(user.ID)})
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
	settled, err := runtime.leave(user.ID)
	if err != nil {
		_ = s.store.UpdateWalletOperation(r.Context(), operationID, "manual_review", "balance credited but local leave failed: "+err.Error())
		return tableGameAPIError(err)
	}
	if err := s.persistRuntime(r.Context(), space.ID, tableID, runtime); err != nil {
		_ = s.store.UpdateWalletOperation(r.Context(), operationID, "manual_review", "balance credited but local persistence failed: "+err.Error())
		return err
	}
	if err := s.store.ReleaseTableSeat(r.Context(), user.ID, space.ID, tableID); err != nil {
		s.logger.Error("release global table seat", "user_id", user.ID, "space_id", space.ID, "table_id", tableID, "error", err)
	}
	runtime.kickVote = nil
	if err := s.store.UpdateWalletOperation(r.Context(), operationID, "completed", ""); err != nil {
		s.logger.Error("mark cash-out operation complete", "operation_id", operationID, "error", err)
	}
	s.broadcast(space.ID, tableID, runtime)
	writeJSON(w, http.StatusOK, map[string]any{"operation_id": operationID, "settled_cents": settled, "table": runtime.snapshot(user.ID)})
	return nil
}

func (s *Server) handleTableReady(w http.ResponseWriter, r *http.Request, user store.User) error {
	if err := s.requireGameplayControl(r, user.ID); err != nil {
		return err
	}
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
	if _, err := runtime.ready(user.ID); err != nil {
		return tableGameAPIError(err)
	}
	if err := s.persistRuntime(r.Context(), space.ID, tableID, runtime); err != nil {
		return err
	}
	if runtime.kickVote != nil {
		target := runtime.commonSnapshot(runtime.kickVote.TargetUserID)
		targetReady := false
		for _, player := range target.Players {
			if player.UserID == runtime.kickVote.TargetUserID {
				targetReady = player.Ready
				break
			}
		}
		if !target.CanLeave || targetReady {
			runtime.kickVote = nil
		}
	}
	s.syncTableTimerLocked(space.ID, tableID, runtime)
	s.broadcast(space.ID, tableID, runtime)
	writeJSON(w, http.StatusOK, s.tableEnvelopeLocked(runtime, user.ID))
	return nil
}

func (s *Server) handleTableKickVote(w http.ResponseWriter, r *http.Request, user store.User) error {
	if err := s.requireGameplayControl(r, user.ID); err != nil {
		return err
	}
	space, err := s.store.SpaceForUser(r.Context(), r.PathValue("spaceID"), user.ID)
	if err != nil {
		return err
	}
	var input struct {
		Action       string `json:"action"`
		TargetUserID int64  `json:"target_user_id"`
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
	clearExpiredKickVoteLocked(runtime)

	switch input.Action {
	case "start":
		if runtime.kickVote != nil {
			return &apiError{Status: http.StatusConflict, Message: "已有一项移出投票正在进行"}
		}
		vote, err := newKickVote(runtime.commonSnapshot(user.ID), user.ID, input.TargetUserID)
		if err != nil {
			return err
		}
		runtime.kickVote = vote
	case "approve", "reject":
		vote := runtime.kickVote
		if vote == nil {
			return &apiError{Status: http.StatusConflict, Message: "当前没有进行中的移出投票"}
		}
		if _, eligible := vote.EligibleVoters[user.ID]; !eligible {
			return &apiError{Status: http.StatusForbidden, Message: "你不能参与这项投票"}
		}
		if _, voted := vote.YesVoters[user.ID]; voted {
			return &apiError{Status: http.StatusConflict, Message: "你已经投过票了"}
		}
		if _, voted := vote.NoVoters[user.ID]; voted {
			return &apiError{Status: http.StatusConflict, Message: "你已经投过票了"}
		}
		if input.Action == "approve" {
			vote.YesVoters[user.ID] = struct{}{}
		} else {
			vote.NoVoters[user.ID] = struct{}{}
		}
	default:
		return &apiError{Status: http.StatusBadRequest, Message: "投票操作不正确"}
	}

	vote := runtime.kickVote
	notice := "投票已提交"
	if len(vote.YesVoters) >= vote.RequiredYes {
		runtime.kickVote = nil
		settled, err := s.cashOutPlayerLocked(r.Context(), space, tableID, runtime, vote.TargetUserID, vote.InitiatorUserID, "经牌桌投票移出")
		if err != nil {
			s.broadcast(space.ID, tableID, runtime)
			return err
		}
		if runtime.kickedUntil == nil {
			runtime.kickedUntil = make(map[int64]time.Time)
		}
		runtime.kickedUntil[vote.TargetUserID] = time.Now().Add(kickRejoinCooldown)
		notice = fmt.Sprintf("投票通过，已将 %s 移出并结算 %s", vote.TargetName, formatCents(settled))
	} else {
		remaining := len(vote.EligibleVoters) - len(vote.YesVoters) - len(vote.NoVoters)
		if len(vote.YesVoters)+remaining < vote.RequiredYes {
			runtime.kickVote = nil
			notice = "同意票不足，移出投票未通过"
		}
	}

	s.broadcast(space.ID, tableID, runtime)
	writeJSON(w, http.StatusOK, s.tableEnvelopeWithNoticeLocked(runtime, user.ID, notice))
	return nil
}

func newKickVote(snapshot runtimeSnapshot, initiatorUserID, targetUserID int64) (*kickVote, error) {
	if !snapshot.CanLeave {
		return nil, &apiError{Status: http.StatusConflict, Message: "只有等待开局时的在桌玩家可以发起投票"}
	}
	var initiator, target *runtimePlayerView
	eligible := make(map[int64]struct{})
	for index := range snapshot.Players {
		player := &snapshot.Players[index]
		if player.UserID == initiatorUserID {
			initiator = player
		}
		if player.UserID == targetUserID {
			target = player
		}
		if player.UserID != targetUserID && player.Stack > 0 {
			eligible[player.UserID] = struct{}{}
		}
	}
	if initiator == nil || initiator.Stack <= 0 {
		return nil, &apiError{Status: http.StatusForbidden, Message: "只有有筹码的在桌玩家可以发起投票"}
	}
	if target == nil || target.UserID == initiatorUserID {
		return nil, &apiError{Status: http.StatusBadRequest, Message: "请选择其他在桌玩家"}
	}
	if target.Stack <= 0 || target.Ready {
		return nil, &apiError{Status: http.StatusConflict, Message: "只能对尚未准备的有筹码玩家发起投票"}
	}
	if _, ok := eligible[initiatorUserID]; !ok {
		return nil, &apiError{Status: http.StatusForbidden, Message: "你不能发起这项投票"}
	}
	yes := map[int64]struct{}{initiatorUserID: {}}
	return &kickVote{
		TargetUserID: target.UserID, TargetName: target.Name,
		InitiatorUserID: initiator.UserID, InitiatorName: initiator.Name,
		EligibleVoters: eligible, YesVoters: yes, NoVoters: make(map[int64]struct{}),
		RequiredYes: len(eligible)/2 + 1, ExpiresAt: time.Now().Add(kickVoteDuration),
	}, nil
}

func (s *Server) cashOutPlayerLocked(ctx context.Context, space store.Space, tableID string, runtime *tableRuntime, targetUserID, actorUserID int64, note string) (int64, error) {
	stack, seated := runtime.stackFor(targetUserID)
	if !seated || !runtime.commonSnapshot(targetUserID).CanLeave {
		return 0, &apiError{Status: http.StatusConflict, Message: "目标玩家已经离桌或牌局已经开始"}
	}
	if stack == 0 {
		if _, err := runtime.leave(targetUserID); err != nil {
			return 0, tableGameAPIError(err)
		}
		if err := s.persistRuntime(ctx, space.ID, tableID, runtime); err != nil {
			return 0, err
		}
		if err := s.store.ReleaseTableSeat(ctx, targetUserID, space.ID, tableID); err != nil {
			s.logger.Error("release global table seat", "user_id", targetUserID, "space_id", space.ID, "table_id", tableID, "error", err)
		}
		return 0, nil
	}
	member, err := s.store.Member(ctx, space.ID, targetUserID)
	if err != nil {
		return 0, err
	}
	quota, err := newapi.CentsToQuota(stack, space.QuotaPerUSD)
	if err != nil {
		return 0, err
	}
	adminToken, err := s.adminToken(space)
	if err != nil {
		return 0, err
	}
	operationID := uuid.NewString()
	operation := store.WalletOperation{
		ID: operationID, SpaceID: space.ID, TableID: tableID, UserID: targetUserID,
		NewAPIUserID: member.NewAPIUserID, ActorUserID: actorUserID, Kind: "cash_out",
		Cents: stack, Quota: quota, Note: note, Status: "pending",
	}
	if err := s.store.CreateWalletOperation(ctx, operation); err != nil {
		return 0, err
	}
	if err := s.newAPI.AdjustQuota(ctx, space.BaseURL, adminToken, member.NewAPIUserID, quota, true); err != nil {
		_ = s.store.UpdateWalletOperation(ctx, operationID, "manual_review", err.Error())
		return 0, &apiError{Status: http.StatusBadGateway, Message: "余额结算未确认，玩家仍保留在牌桌，请频道管理员核对操作 " + operationID}
	}
	settled, err := runtime.leave(targetUserID)
	if err != nil {
		_ = s.store.UpdateWalletOperation(ctx, operationID, "manual_review", "balance credited but local leave failed: "+err.Error())
		return 0, tableGameAPIError(err)
	}
	if err := s.persistRuntime(ctx, space.ID, tableID, runtime); err != nil {
		_ = s.store.UpdateWalletOperation(ctx, operationID, "manual_review", "balance credited but local persistence failed: "+err.Error())
		return 0, err
	}
	if err := s.store.ReleaseTableSeat(ctx, targetUserID, space.ID, tableID); err != nil {
		s.logger.Error("release global table seat", "user_id", targetUserID, "space_id", space.ID, "table_id", tableID, "error", err)
	}
	if err := s.store.UpdateWalletOperation(ctx, operationID, "completed", ""); err != nil {
		s.logger.Error("mark vote kick cash-out complete", "operation_id", operationID, "error", err)
	}
	return settled, nil
}

func (s *Server) settleAllPlayersLocked(ctx context.Context, space store.Space, tableID string, runtime *tableRuntime, actorUserID int64, note string) (int, int64, error) {
	snapshot := runtime.commonSnapshot(actorUserID)
	if snapshot.HandActive {
		return 0, 0, &apiError{Status: http.StatusConflict, Message: "一手牌正在进行，请等待本手结束后再清理牌桌"}
	}
	runtime.kickVote = nil
	settledPlayers := 0
	settledCents := int64(0)
	for _, player := range snapshot.Players {
		settled, err := s.cashOutPlayerLocked(ctx, space, tableID, runtime, player.UserID, actorUserID, note)
		if err != nil {
			return settledPlayers, settledCents, settlementProgressError(settledPlayers, err)
		}
		settledPlayers++
		settledCents += settled
	}
	return settledPlayers, settledCents, nil
}

func settlementProgressError(settledPlayers int, err error) error {
	if settledPlayers == 0 {
		return err
	}
	message := fmt.Sprintf("已结算并移出 %d 名玩家，后续结算已停止：%s", settledPlayers, err.Error())
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return &apiError{Status: apiErr.Status, Message: message}
	}
	return errors.New(message)
}

func formatCents(cents int64) string {
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

func (s *Server) handleTableAction(w http.ResponseWriter, r *http.Request, user store.User) error {
	if err := s.requireGameplayControl(r, user.ID); err != nil {
		return err
	}
	space, err := s.store.SpaceForUser(r.Context(), r.PathValue("spaceID"), user.ID)
	if err != nil {
		return err
	}
	var input gameActionInput
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
	if input.ExpectedTurnID == 0 {
		return &apiError{Status: http.StatusBadRequest, Message: "expected_turn_id 必须是刚读取到的当前轮次"}
	}
	current := runtime.commonSnapshot(user.ID)
	if input.ExpectedTurnID != current.TurnID {
		return &apiError{Status: http.StatusConflict, Message: "牌局轮次已经变化；请重新读取牌桌状态后再行动"}
	}
	if err := runtime.act(user.ID, input); err != nil {
		return tableGameAPIError(err)
	}
	if err := s.persistRuntime(r.Context(), space.ID, tableID, runtime); err != nil {
		return err
	}
	s.syncTableTimerLocked(space.ID, tableID, runtime)
	s.broadcast(space.ID, tableID, runtime)
	writeJSON(w, http.StatusOK, s.tableEnvelopeLocked(runtime, user.ID))
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
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return s.tableEnvelopeLocked(runtime, viewerID)
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
	runtime, err := restoreTableRuntime(data)
	if err != nil {
		return nil, err
	}
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

func (s *Server) persistRuntime(ctx context.Context, spaceID, tableID string, runtime *tableRuntime) error {
	data, err := runtime.marshalState()
	if err != nil {
		return err
	}
	return s.store.SaveTableState(ctx, spaceID, tableID, data)
}

func (s *Server) broadcast(spaceID, tableID string, runtime *tableRuntime) {
	s.hub.Broadcast(tableRoomKey(spaceID, tableID), func(viewerID int64) any {
		return s.tableEnvelopeLocked(runtime, viewerID)
	})
}

func (s *Server) tableEnvelopeLocked(runtime *tableRuntime, viewerID int64) any {
	return s.tableEnvelopeWithNoticeLocked(runtime, viewerID, "")
}

func (s *Server) tableEnvelopeWithNoticeLocked(runtime *tableRuntime, viewerID int64, notice string) any {
	clearExpiredKickVoteLocked(runtime)
	vote := kickVoteForViewer(runtime.kickVote, viewerID)
	if runtime.landlord != nil {
		return landlordTableEnvelope{Type: "table", Table: runtime.landlord.Snapshot(viewerID), KickVote: vote, Notice: notice}
	}
	return tableEnvelope{Type: "table", Table: runtime.table.Snapshot(viewerID), KickVote: vote, Notice: notice}
}

func clearExpiredKickVoteLocked(runtime *tableRuntime) {
	if runtime.kickVote != nil && !time.Now().Before(runtime.kickVote.ExpiresAt) {
		runtime.kickVote = nil
	}
}

func kickVoteForViewer(vote *kickVote, viewerID int64) *kickVoteView {
	if vote == nil {
		return nil
	}
	view := &kickVoteView{
		TargetUserID: vote.TargetUserID, TargetName: vote.TargetName, InitiatorName: vote.InitiatorName,
		YesCount: len(vote.YesVoters), NoCount: len(vote.NoVoters), RequiredYes: vote.RequiredYes,
		EligibleCount: len(vote.EligibleVoters), ExpiresAt: vote.ExpiresAt.UnixMilli(),
	}
	_, eligible := vote.EligibleVoters[viewerID]
	if _, voted := vote.YesVoters[viewerID]; voted {
		view.ViewerVote = "approve"
	} else if _, voted := vote.NoVoters[viewerID]; voted {
		view.ViewerVote = "reject"
	} else {
		view.CanVote = eligible
	}
	return view
}

func (s *Server) syncTableTimerLocked(spaceID, tableID string, runtime *tableRuntime) {
	if runtime.timer != nil {
		runtime.timer.Stop()
		runtime.timer = nil
	}
	runtime.timerTurnID = 0
	snapshot := runtime.commonSnapshot(0)
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
	action, applied, err := runtime.timeout(turnID, time.Now())
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
	if err := s.persistRuntime(ctx, spaceID, tableID, runtime); err != nil {
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

func summarizeRuntime(runtime *tableRuntime, viewerID int64) tableSummary {
	snapshot := runtime.commonSnapshot(viewerID)
	players := make([]tableSeat, 0, len(snapshot.Players))
	for _, player := range snapshot.Players {
		players = append(players, tableSeat{
			UserID: player.UserID,
			Name:   player.Name,
			Seat:   player.Seat,
			Stack:  player.Stack,
			Ready:  player.Ready,
		})
	}
	return tableSummary{
		ID: snapshot.ID, Name: snapshot.Name, GameType: snapshot.GameType,
		SmallBlind: snapshot.SmallBlind, BigBlind: snapshot.BigBlind, BaseStake: snapshot.BaseStake,
		ActionTimeoutSeconds: snapshot.ActionTimeoutSeconds,
		PlayerCount:          len(snapshot.Players), MaxPlayers: snapshot.MaxPlayers, HandID: snapshot.HandID, Street: snapshot.Phase,
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
