package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"pokernode/internal/newapi"
	"pokernode/internal/poker"
	"pokernode/internal/secure"
	"pokernode/internal/store"
)

type spaceInput struct {
	Name        string `json:"name"`
	BaseURL     string `json:"newapi_base_url"`
	AdminToken  string `json:"admin_token"`
	QuotaPerUSD int64  `json:"quota_per_usd"`
}

type bindInput struct {
	Token string `json:"token"`
}

type accountBindingView struct {
	Space      store.Space  `json:"space"`
	Membership store.Member `json:"membership"`
}

func (s *Server) handleListAccountBindings(w http.ResponseWriter, r *http.Request, user store.User) error {
	spaces, err := s.store.SpacesForUser(r.Context(), user.ID)
	if err != nil {
		return err
	}
	bindings := make([]accountBindingView, 0, len(spaces))
	for _, space := range spaces {
		member, err := s.store.Member(r.Context(), space.ID, user.ID)
		if err != nil {
			return err
		}
		bindings = append(bindings, accountBindingView{Space: space, Membership: member})
	}
	writeJSON(w, http.StatusOK, map[string]any{"bindings": bindings})
	return nil
}

func (s *Server) handleListSpaces(w http.ResponseWriter, r *http.Request, user store.User) error {
	spaces, err := s.spacesForActor(r.Context(), user)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"spaces": spaces})
	return nil
}

func (s *Server) handleLobbyLeaderboard(w http.ResponseWriter, r *http.Request, user store.User) error {
	entries, err := s.store.LobbyLeaderboard(r.Context(), user.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"leaderboard": entries})
	return nil
}

func (s *Server) handleCreateSpace(w http.ResponseWriter, r *http.Request, user store.User) error {
	var input spaceInput
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	space, err := s.validatedSpace(r, input, user.ID)
	if err != nil {
		return err
	}
	if err := s.store.CreateSpace(r.Context(), space); err != nil {
		return err
	}
	defaultTable := poker.NewTable(mainTableID, "新手桌", 50, 100)
	if err := s.persistTable(r.Context(), space.ID, mainTableID, defaultTable); err != nil {
		return err
	}
	s.tablesMu.Lock()
	s.tables[tableRoomKey(space.ID, mainTableID)] = &tableRuntime{table: defaultTable}
	s.tablesMu.Unlock()
	space.IsOwner = true
	space.CanManage = true
	warning := ""
	if boundSpace, err := s.autoProvisionMember(r.Context(), space, user); err != nil {
		s.logger.Warn("automatic New API user provisioning failed", "space_id", space.ID, "user_id", user.ID, "error", err)
		warning = "频道已创建，但自动创建 New API 玩家账号失败；进入牌桌后可手动绑定个人凭证"
	} else {
		space = boundSpace
	}
	writeJSON(w, http.StatusCreated, map[string]any{"space": space, "warning": warning})
	return nil
}

func (s *Server) handleJoinSpace(w http.ResponseWriter, r *http.Request, user store.User) error {
	var input struct {
		InviteCode string `json:"invite_code"`
	}
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	code := strings.ToUpper(strings.TrimSpace(input.InviteCode))
	if code == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "请输入频道邀请码"}
	}
	space, err := s.store.JoinSpace(r.Context(), code, user.ID)
	if err != nil {
		if err == store.ErrNotFound {
			return &apiError{Status: http.StatusNotFound, Message: "邀请码无效"}
		}
		return err
	}
	warning := ""
	if boundSpace, err := s.autoProvisionMember(r.Context(), space, user); err != nil {
		s.logger.Warn("automatic New API user provisioning failed", "space_id", space.ID, "user_id", user.ID, "error", err)
		warning = "已加入频道，但自动创建 New API 玩家账号失败；进入牌桌后可手动绑定个人凭证"
	} else {
		space = boundSpace
	}
	writeJSON(w, http.StatusOK, map[string]any{"space": space, "warning": warning})
	return nil
}

func (s *Server) handleGetSpace(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, err := s.spaceForActor(r.Context(), r.PathValue("spaceID"), user)
	if err != nil {
		return err
	}
	member, err := s.store.Member(r.Context(), space.ID, user.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if err == nil && member.NewAPIUserID == 0 {
		provisioned, provisionErr := s.autoProvisionMember(r.Context(), space, user)
		if provisionErr != nil {
			s.logger.Warn("automatic New API user reprovisioning failed", "space_id", space.ID, "user_id", user.ID, "error", provisionErr)
		} else {
			space = provisioned
			member, err = s.store.Member(r.Context(), space.ID, user.ID)
			if err != nil {
				return err
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"space": space, "membership": member})
	return nil
}

func (s *Server) handleUpdateSpaceConnection(w http.ResponseWriter, r *http.Request, user store.User) error {
	spaceID := r.PathValue("spaceID")
	space, err := s.spaceForActor(r.Context(), spaceID, user)
	if err != nil {
		return err
	}
	if !space.CanManage {
		return &apiError{Status: http.StatusForbidden, Message: "只有频道管理员可以修改连接"}
	}
	var input spaceInput
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	input.Name = space.Name
	validated, err := s.validatedSpace(r, input, user.ID)
	if err != nil {
		return err
	}
	if err := s.store.UpdateSpaceConnectionManaged(r.Context(), spaceID, validated.BaseURL, validated.AdminTokenEnc, validated.AdminTokenLast4,
		validated.AdminNewAPIUserID, validated.AdminNewAPIRole, validated.QuotaPerUSD); err != nil {
		return err
	}
	updated, err := s.spaceForActor(r.Context(), spaceID, user)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"space": updated})
	return nil
}

func (s *Server) handleBindMember(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, err := s.store.SpaceForUser(r.Context(), r.PathValue("spaceID"), user.ID)
	if err != nil {
		return err
	}
	seated, err := s.userSeatedInSpace(r.Context(), space.ID, user.ID)
	if err != nil {
		return err
	}
	if seated {
		return &apiError{Status: http.StatusConflict, Message: "请先从当前频道的牌桌离桌并完成结算，再更换绑定账号"}
	}
	var input bindInput
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	input.Token = strings.TrimSpace(input.Token)
	if input.Token == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "请输入当前频道的 System Access Token"}
	}
	newUser, err := s.newAPI.Self(r.Context(), space.BaseURL, input.Token)
	if err != nil {
		return &apiError{Status: http.StatusBadGateway, Message: "个人凭证验证失败，请检查频道地址和 Token"}
	}
	if newUser.Status != 1 {
		return &apiError{Status: http.StatusForbidden, Message: "该 New API 用户已被禁用"}
	}
	if space.AdminNewAPIRole < 100 && newUser.Role >= space.AdminNewAPIRole {
		return &apiError{Status: http.StatusBadRequest, Message: "频道管理凭证无权调整该 New API 用户的余额"}
	}
	encrypted, err := s.cipher.Encrypt(input.Token)
	if err != nil {
		return err
	}
	member := store.Member{
		SpaceID: space.ID, UserID: user.ID, NewAPIUserID: newUser.ID, NewAPIUsername: newUser.Username,
		NewAPIDisplay: newUser.DisplayName, NewAPIRole: newUser.Role, UserTokenEnc: encrypted, UserTokenLast4: secure.LastFour(input.Token),
	}
	if err := s.store.BindMember(r.Context(), member); err != nil {
		if store.IsUniqueViolation(err) {
			return &apiError{Status: http.StatusConflict, Message: "该 New API 用户已绑定到频道内的其他账号"}
		}
		return err
	}
	member, err = s.store.Member(r.Context(), space.ID, user.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"membership": member, "balance": balanceView(newUser.Quota, space.QuotaPerUSD)})
	return nil
}

func (s *Server) userSeatedInSpace(ctx context.Context, spaceID string, userID int64) (bool, error) {
	tableIDs, err := s.store.TableStateIDs(ctx, spaceID)
	if err != nil {
		return false, err
	}
	for _, tableID := range tableIDs {
		runtime, err := s.runtimeForTable(ctx, spaceID, tableID)
		if err != nil {
			return false, err
		}
		if runtime.table.IsSeated(userID) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) handleBalance(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, member, token, err := s.memberCredentials(r, user.ID)
	if err != nil {
		return err
	}
	newUser, err := s.newAPI.Self(r.Context(), space.BaseURL, token)
	if err != nil {
		return &apiError{Status: http.StatusBadGateway, Message: "读取 New API 余额失败，请检查频道连接和个人凭证"}
	}
	if newUser.ID != member.NewAPIUserID {
		return &apiError{Status: http.StatusConflict, Message: "个人凭证对应的 New API 用户已发生变化，请重新绑定"}
	}
	writeJSON(w, http.StatusOK, map[string]any{"balance": balanceView(newUser.Quota, space.QuotaPerUSD), "newapi_user": newUser})
	return nil
}

func (s *Server) handleOperations(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, err := s.store.SpaceForUser(r.Context(), r.PathValue("spaceID"), user.ID)
	if err != nil {
		return err
	}
	operations, err := s.store.WalletOperations(r.Context(), space.ID, user.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"operations": operations})
	return nil
}

func (s *Server) handleChannelLeaderboard(w http.ResponseWriter, r *http.Request, user store.User) error {
	space, err := s.store.SpaceForUser(r.Context(), r.PathValue("spaceID"), user.ID)
	if err != nil {
		return err
	}
	entries, err := s.store.ChannelLeaderboard(r.Context(), space.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"leaderboard": entries})
	return nil
}

func (s *Server) validatedSpace(r *http.Request, input spaceInput, ownerID int64) (store.Space, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 40 {
		return store.Space{}, &apiError{Status: http.StatusBadRequest, Message: "频道名称需为 1–40 个字符"}
	}
	baseURL, err := newapi.NormalizeBaseURL(input.BaseURL)
	if err != nil {
		return store.Space{}, &apiError{Status: http.StatusBadRequest, Message: err.Error()}
	}
	token := strings.TrimSpace(input.AdminToken)
	if token == "" {
		return store.Space{}, &apiError{Status: http.StatusBadRequest, Message: "请输入管理员 System Access Token"}
	}
	admin, err := s.newAPI.Self(r.Context(), baseURL, token)
	if err != nil {
		return store.Space{}, &apiError{Status: http.StatusBadGateway, Message: "管理员凭证验证失败，请检查 New API 地址和 Token"}
	}
	if admin.Status != 1 || admin.Role < 10 {
		return store.Space{}, &apiError{Status: http.StatusForbidden, Message: "该凭证不是可用的 New API 管理员凭证"}
	}
	quotaPerUSD := input.QuotaPerUSD
	if quotaPerUSD == 0 {
		quotaPerUSD = newapi.DefaultQuotaPerUSD
	}
	if quotaPerUSD <= 0 || quotaPerUSD%100 != 0 {
		return store.Space{}, &apiError{Status: http.StatusBadRequest, Message: "每美元 quota 必须是可被 100 整除的正整数"}
	}
	encrypted, err := s.cipher.Encrypt(token)
	if err != nil {
		return store.Space{}, err
	}
	invite, err := inviteCode()
	if err != nil {
		return store.Space{}, err
	}
	return store.Space{
		ID: uuid.NewString(), Name: name, InviteCode: invite, OwnerUserID: ownerID, BaseURL: baseURL,
		AdminTokenEnc: encrypted, AdminTokenLast4: secure.LastFour(token), AdminNewAPIUserID: admin.ID,
		AdminNewAPIRole: admin.Role, QuotaPerUSD: quotaPerUSD, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *Server) memberCredentials(r *http.Request, userID int64) (store.Space, store.Member, string, error) {
	space, err := s.store.SpaceForUser(r.Context(), r.PathValue("spaceID"), userID)
	if err != nil {
		return store.Space{}, store.Member{}, "", err
	}
	member, err := s.store.Member(r.Context(), space.ID, userID)
	if err != nil {
		return store.Space{}, store.Member{}, "", err
	}
	if member.NewAPIUserID == 0 || member.UserTokenEnc == "" {
		return store.Space{}, store.Member{}, "", &apiError{Status: http.StatusPreconditionRequired, Message: "请先绑定当前频道的个人 System Access Token"}
	}
	token, err := s.cipher.Decrypt(member.UserTokenEnc)
	if err != nil {
		return store.Space{}, store.Member{}, "", err
	}
	return space, member, token, nil
}

func (s *Server) adminToken(space store.Space) (string, error) {
	return s.cipher.Decrypt(space.AdminTokenEnc)
}

func (s *Server) autoProvisionMember(ctx context.Context, space store.Space, user store.User) (store.Space, error) {
	member, err := s.store.Member(ctx, space.ID, user.ID)
	if err != nil {
		return store.Space{}, err
	}
	if member.NewAPIUserID > 0 && member.UserTokenEnc != "" {
		return s.store.SpaceForUser(ctx, space.ID, user.ID)
	}

	adminToken, err := s.adminToken(space)
	if err != nil {
		return store.Space{}, err
	}
	username, password := provisionedNewAPIIdentity(space.ID, user.ID, adminToken)
	newUser, token, err := s.newAPI.ProvisionUser(ctx, space.BaseURL, adminToken, username, password, user.DisplayName)
	if err != nil {
		return store.Space{}, err
	}
	if newUser.Status != 1 {
		return store.Space{}, fmt.Errorf("自动创建的 New API 用户状态不可用")
	}
	if space.AdminNewAPIRole < 100 && newUser.Role >= space.AdminNewAPIRole {
		return store.Space{}, fmt.Errorf("频道管理凭证无权管理自动创建的 New API 用户")
	}
	encrypted, err := s.cipher.Encrypt(token)
	if err != nil {
		return store.Space{}, err
	}
	if err := s.store.BindMember(ctx, store.Member{
		SpaceID: space.ID, UserID: user.ID, NewAPIUserID: newUser.ID, NewAPIUsername: newUser.Username,
		NewAPIDisplay: newUser.DisplayName, NewAPIRole: newUser.Role, UserTokenEnc: encrypted, UserTokenLast4: secure.LastFour(token),
	}); err != nil {
		return store.Space{}, err
	}
	return s.store.SpaceForUser(ctx, space.ID, user.ID)
}

func provisionedNewAPIIdentity(spaceID string, userID int64, adminToken string) (string, string) {
	identity := fmt.Sprintf("%s:%d", spaceID, userID)
	digest := sha256.Sum256([]byte(identity))
	username := fmt.Sprintf("pn_%x", digest[:8])
	mac := hmac.New(sha256.New, []byte(adminToken))
	_, _ = mac.Write([]byte("pokernode:newapi-user:" + identity))
	password := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))[:20]
	return username, password
}

func balanceView(quota, quotaPerUSD int64) map[string]int64 {
	perCent := quotaPerUSD / 100
	cents := int64(0)
	if perCent > 0 {
		cents = quota / perCent
	}
	return map[string]int64{"cents": cents, "quota": quota, "quota_per_usd": quotaPerUSD}
}

func inviteCode() (string, error) {
	data := make([]byte, 6)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(data), nil
}
