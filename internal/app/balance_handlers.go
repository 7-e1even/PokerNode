package app

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"pokernode/internal/access"
	"pokernode/internal/newapi"
	"pokernode/internal/store"
)

type managedBalanceMember struct {
	UserID           int64            `json:"user_id"`
	PokerDisplayName string           `json:"poker_display_name"`
	NewAPIUserID     int64            `json:"newapi_user_id,omitempty"`
	NewAPIUsername   string           `json:"newapi_username,omitempty"`
	NewAPIDisplay    string           `json:"newapi_display_name,omitempty"`
	Bound            bool             `json:"bound"`
	Balance          map[string]int64 `json:"balance,omitempty"`
	Error            string           `json:"error,omitempty"`
}

func (s *Server) handleManagedBalances(w http.ResponseWriter, r *http.Request, actor store.User) error {
	space, err := s.managedBalanceSpace(r, actor)
	if err != nil {
		return err
	}
	members, err := s.store.Members(r.Context(), space.ID)
	if err != nil {
		return err
	}
	adminToken, err := s.adminToken(space)
	if err != nil {
		return err
	}
	maxAdjustmentCents, err := newapi.MaxCentsForQuota(space.QuotaPerUSD)
	if err != nil {
		return err
	}

	result := make([]managedBalanceMember, 0, len(members))
	for _, member := range members {
		item := managedBalanceItem(member)
		if !item.Bound {
			result = append(result, item)
			continue
		}
		newUser, err := s.newAPI.User(r.Context(), space.BaseURL, adminToken, member.NewAPIUserID)
		if err != nil {
			item.Error = "暂时无法读取余额"
		} else {
			item.Balance = balanceView(newUser.Quota, space.QuotaPerUSD)
		}
		result = append(result, item)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"space":   map[string]any{"id": space.ID, "name": space.Name, "quota_per_usd": space.QuotaPerUSD, "max_adjustment_cents": maxAdjustmentCents},
		"members": result,
	})
	return nil
}

func (s *Server) handleAdjustManagedBalance(w http.ResponseWriter, r *http.Request, actor store.User) error {
	space, err := s.managedBalanceSpace(r, actor)
	if err != nil {
		return err
	}
	targetUserID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
	if err != nil || targetUserID <= 0 {
		return &apiError{Status: http.StatusBadRequest, Message: "成员编号无效"}
	}
	member, err := s.store.Member(r.Context(), space.ID, targetUserID)
	if err != nil {
		return err
	}
	if member.NewAPIUserID <= 0 {
		return &apiError{Status: http.StatusPreconditionRequired, Message: "该成员尚未绑定 New API 账号"}
	}

	var input struct {
		Direction   string `json:"direction"`
		AmountCents int64  `json:"amount_cents"`
		Reason      string `json:"reason"`
	}
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	input.Direction = strings.TrimSpace(input.Direction)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Direction != "add" && input.Direction != "subtract" {
		return &apiError{Status: http.StatusBadRequest, Message: "请选择增加或扣减余额"}
	}
	if input.AmountCents <= 0 {
		return &apiError{Status: http.StatusBadRequest, Message: "调整金额必须大于 0"}
	}
	if utf8.RuneCountInString(input.Reason) < 2 || utf8.RuneCountInString(input.Reason) > 200 {
		return &apiError{Status: http.StatusBadRequest, Message: "调整原因需填写 2 到 200 个字符"}
	}
	maxAdjustmentCents, err := newapi.MaxCentsForQuota(space.QuotaPerUSD)
	if err != nil {
		return err
	}
	if input.AmountCents > maxAdjustmentCents {
		return &apiError{Status: http.StatusBadRequest, Message: fmt.Sprintf("调整金额过大，单次最多为 %s", formatUSDCents(maxAdjustmentCents))}
	}
	quota, err := newapi.CentsToQuota(input.AmountCents, space.QuotaPerUSD)
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "调整金额超出可用范围"}
	}
	adminToken, err := s.adminToken(space)
	if err != nil {
		return err
	}

	// Keep the balance check and the corresponding New API adjustment together.
	s.balanceMu.Lock()
	defer s.balanceMu.Unlock()
	newUser, err := s.newAPI.User(r.Context(), space.BaseURL, adminToken, member.NewAPIUserID)
	if err != nil {
		return &apiError{Status: http.StatusBadGateway, Message: "读取该成员的 New API 余额失败"}
	}
	if newUser.Status != 1 {
		return &apiError{Status: http.StatusConflict, Message: "该成员的 New API 账号当前不可用"}
	}
	if space.AdminNewAPIRole < 100 && newUser.Role >= space.AdminNewAPIRole {
		return &apiError{Status: http.StatusForbidden, Message: "频道管理凭证无权调整该 New API 账号"}
	}
	add := input.Direction == "add"
	if !add && newUser.Quota < quota {
		availableCents := newUser.Quota / (space.QuotaPerUSD / 100)
		return &apiError{Status: http.StatusConflict, Message: fmt.Sprintf("扣减金额不能超过该成员当前余额 %s", formatUSDCents(availableCents))}
	}

	kind := "manual_debit"
	if add {
		kind = "manual_credit"
	}
	operation := store.WalletOperation{
		ID: uuid.NewString(), SpaceID: space.ID, UserID: member.UserID, NewAPIUserID: member.NewAPIUserID,
		ActorUserID: actor.ID, Kind: kind, Cents: input.AmountCents, Quota: quota, Note: input.Reason, Status: "pending",
	}
	if err := s.store.CreateWalletOperation(r.Context(), operation); err != nil {
		return err
	}
	if err := s.newAPI.AdjustQuota(r.Context(), space.BaseURL, adminToken, member.NewAPIUserID, quota, add); err != nil {
		_ = s.store.UpdateWalletOperation(r.Context(), operation.ID, "manual_review", err.Error())
		return &apiError{Status: http.StatusBadGateway, Message: "New API 未确认余额调整，请根据操作记录人工核对"}
	}
	if err := s.store.UpdateWalletOperation(r.Context(), operation.ID, "completed", ""); err != nil {
		s.logger.Error("mark manual balance operation complete", "operation_id", operation.ID, "error", err)
	}
	updated, err := s.newAPI.User(r.Context(), space.BaseURL, adminToken, member.NewAPIUserID)
	if err != nil {
		updated = newUser
		if add {
			updated.Quota += quota
		} else {
			updated.Quota -= quota
		}
	}
	item := managedBalanceItem(member)
	item.Balance = balanceView(updated.Quota, space.QuotaPerUSD)
	operation.Status = "completed"
	writeJSON(w, http.StatusOK, map[string]any{"member": item, "operation": operation})
	return nil
}

func formatUSDCents(cents int64) string {
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

func (s *Server) managedBalanceSpace(r *http.Request, actor store.User) (store.Space, error) {
	space, err := s.store.SpaceByID(r.Context(), r.PathValue("spaceID"))
	if err != nil {
		return store.Space{}, err
	}
	if space.OwnerUserID == actor.ID {
		return space, nil
	}
	allowed, err := s.canManageAssignedSpace(r.Context(), actor, space.ID, access.PermissionBalancesManage)
	if err != nil {
		return store.Space{}, err
	}
	if !allowed {
		return store.Space{}, &apiError{Status: http.StatusForbidden, Message: "你没有该频道的余额管理权限"}
	}
	return space, nil
}

func managedBalanceItem(member store.Member) managedBalanceMember {
	return managedBalanceMember{
		UserID: member.UserID, PokerDisplayName: member.PokerDisplayName,
		NewAPIUserID: member.NewAPIUserID, NewAPIUsername: member.NewAPIUsername, NewAPIDisplay: member.NewAPIDisplay,
		Bound: member.NewAPIUserID > 0,
	}
}
