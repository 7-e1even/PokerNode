package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"pokernode/internal/access"
	"pokernode/internal/store"
)

const maxLoginHeroImageBytes = 5 << 20

type adminUserInput struct {
	Username        string   `json:"username"`
	DisplayName     string   `json:"display_name"`
	Password        string   `json:"password"`
	Role            string   `json:"role"`
	ManagedSpaceIDs []string `json:"managed_space_ids"`
}

type adminUserUpdate struct {
	Username        string    `json:"username"`
	DisplayName     string    `json:"display_name"`
	Password        string    `json:"password"`
	Role            string    `json:"role"`
	Status          string    `json:"status"`
	ManagedSpaceIDs *[]string `json:"managed_space_ids"`
}

type adminRankingVisibilityInput struct {
	Hidden bool `json:"hidden"`
}

type adminLoginHeroPlacementInput struct {
	PositionX float64 `json:"position_x"`
	PositionY float64 `json:"position_y"`
	Zoom      float64 `json:"zoom"`
}

type adminWeChatSettingsInput struct {
	AppID       string `json:"app_id"`
	AppSecret   string `json:"app_secret"`
	RedirectURI string `json:"redirect_uri"`
	Enabled     bool   `json:"enabled"`
}

type adminWeChatSettingsView struct {
	AppID               string `json:"app_id"`
	RedirectURI         string `json:"redirect_uri"`
	Enabled             bool   `json:"enabled"`
	Configured          bool   `json:"configured"`
	AppSecretConfigured bool   `json:"app_secret_configured"`
	Source              string `json:"source"`
}

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request, actor store.User) error {
	permissions, err := s.permissionsForUser(r.Context(), actor)
	if err != nil {
		return err
	}
	permissionSet := make(map[access.Permission]struct{}, len(permissions))
	for _, permission := range permissions {
		permissionSet[permission] = struct{}{}
	}
	users := make([]store.User, 0)
	if _, canReadUsers := permissionSet[access.PermissionUsersRead]; canReadUsers {
		users, err = s.store.Users(r.Context())
		if err != nil {
			return err
		}
	} else if _, canManageUsers := permissionSet[access.PermissionUsersManage]; canManageUsers {
		users, err = s.store.Users(r.Context())
		if err != nil {
			return err
		}
	}
	registrationEnabled, err := s.store.RegistrationEnabled(r.Context())
	if err != nil {
		return err
	}
	loginHeroConfig, err := s.store.LoginHeroImageConfig(r.Context())
	if err != nil {
		return err
	}
	wechatSettings, err := s.adminWeChatSettings(r.Context())
	if err != nil {
		return err
	}
	spaces := make([]store.AdminSpaceSummary, 0)
	platformCounts := store.AdminPlatformCounts{}
	if actor.Role == string(access.RoleSuperAdmin) {
		spaces, platformCounts, err = s.store.AdminSpaces(r.Context())
	} else {
		_, canManageChannels := permissionSet[access.PermissionChannelsManage]
		_, canManageBalances := permissionSet[access.PermissionBalancesManage]
		if canManageChannels || canManageBalances {
			spaces, platformCounts, err = s.store.AdminSpacesForUser(r.Context(), actor.ID, true)
		}
	}
	if err != nil {
		return err
	}
	roles, err := s.store.Roles(r.Context())
	if err != nil {
		return err
	}
	counts := map[string]int{"total": len(users)}
	for _, user := range users {
		counts[user.Role]++
		if user.Status == "active" {
			counts["active"]++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users":                users,
		"counts":               counts,
		"spaces":               spaces,
		"platform_counts":      platformCounts,
		"registration_enabled": registrationEnabled,
		"login_hero":           loginHeroImagePayload(loginHeroConfig),
		"wechat_login":         wechatSettings,
		"permissions":          permissions,
		"roles":                roles,
		"permission_catalog":   access.Catalog(),
	})
	return nil
}

func (s *Server) handleAdminRankingVisibility(w http.ResponseWriter, r *http.Request, actor store.User) error {
	if actor.Role != string(access.RoleSuperAdmin) {
		return &apiError{Status: http.StatusForbidden, Message: "只有超级管理员可以管理排名展示"}
	}
	userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
	if err != nil || userID <= 0 {
		return &apiError{Status: http.StatusBadRequest, Message: "用户 ID 无效"}
	}
	var input adminRankingVisibilityInput
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	if err := s.store.SetUserRankingHidden(r.Context(), userID, input.Hidden); err != nil {
		return err
	}
	user, err := s.store.UserByID(r.Context(), userID)
	if err != nil {
		return err
	}
	user.ManagedSpaceIDs, err = s.store.ManagedSpaceIDs(r.Context(), userID)
	if err != nil {
		return err
	}
	user.JoinedSpaceIDs, err = s.store.JoinedSpaceIDs(r.Context(), userID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
	return nil
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var input adminUserInput
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	account := authInput{Username: input.Username, DisplayName: input.DisplayName, Password: input.Password}
	if err := validateAccountInput(&account); err != nil {
		return err
	}
	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = string(access.RolePlayer)
	}
	roleRecord, err := s.store.RoleByKey(r.Context(), role)
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "无效的用户角色"}
	}
	canManageRoles, err := s.hasPermission(r.Context(), actor, access.PermissionRolesManage)
	if err != nil {
		return err
	}
	if !canManageRoles && role != string(access.RolePlayer) {
		return &apiError{Status: http.StatusForbidden, Message: "运营账号只能创建玩家"}
	}
	if role == string(access.RoleSuperAdmin) && actor.Role != string(access.RoleSuperAdmin) {
		return &apiError{Status: http.StatusForbidden, Message: "只有超级管理员可以创建超级管理员账号"}
	}
	if len(input.ManagedSpaceIDs) > 0 {
		if !canManageRoles {
			return &apiError{Status: http.StatusForbidden, Message: "当前账号不能分配频道管理范围"}
		}
		if !roleAllowsChannelAssignments(roleRecord) {
			return &apiError{Status: http.StatusBadRequest, Message: "该角色没有频道或余额管理权限"}
		}
		return &apiError{Status: http.StatusBadRequest, Message: "新账号尚未加入频道，请创建账号并加入频道后再分配管理范围"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(account.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user, err := s.store.CreateUser(r.Context(), account.Username, account.DisplayName, string(hash), role)
	if err != nil {
		if store.IsUniqueViolation(err) {
			return &apiError{Status: http.StatusConflict, Message: "用户名已存在"}
		}
		return err
	}
	if err := s.store.SetManagedSpaces(r.Context(), user.ID, input.ManagedSpaceIDs); err != nil {
		return err
	}
	user, err = s.store.UserByID(r.Context(), user.ID)
	if err != nil {
		return err
	}
	user.ManagedSpaceIDs, err = s.store.ManagedSpaceIDs(r.Context(), user.ID)
	if err != nil {
		return err
	}
	user.JoinedSpaceIDs, err = s.store.JoinedSpaceIDs(r.Context(), user.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
	return nil
}

func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
	if err != nil || userID <= 0 {
		return &apiError{Status: http.StatusBadRequest, Message: "用户 ID 无效"}
	}
	if userID == actor.ID {
		return &apiError{Status: http.StatusBadRequest, Message: "不能在用户管理中修改当前登录账号"}
	}
	target, err := s.store.UserByID(r.Context(), userID)
	if err != nil {
		return err
	}
	target.ManagedSpaceIDs, err = s.store.ManagedSpaceIDs(r.Context(), target.ID)
	if err != nil {
		return err
	}
	target.JoinedSpaceIDs, err = s.store.JoinedSpaceIDs(r.Context(), target.ID)
	if err != nil {
		return err
	}
	var input adminUserUpdate
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	username := strings.TrimSpace(input.Username)
	if username == "" {
		username = target.Username
	}
	if !usernamePattern.MatchString(username) {
		return &apiError{Status: http.StatusBadRequest, Message: "用户名需为 3–24 位字母、数字或下划线"}
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = target.DisplayName
	}
	if len([]rune(displayName)) > 32 {
		return &apiError{Status: http.StatusBadRequest, Message: "显示名称不能超过 32 个字符"}
	}
	passwordHash := target.PasswordHash
	if input.Password != "" {
		if len(input.Password) < 8 || len(input.Password) > 72 {
			return &apiError{Status: http.StatusBadRequest, Message: "密码长度需为 8–72 位"}
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		passwordHash = string(hash)
	}
	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = target.Role
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = target.Status
	}
	roleRecord, err := s.store.RoleByKey(r.Context(), role)
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "无效的用户角色"}
	}
	if status != "active" && status != "disabled" {
		return &apiError{Status: http.StatusBadRequest, Message: "无效的账号状态"}
	}
	canManageRoles, err := s.hasPermission(r.Context(), actor, access.PermissionRolesManage)
	if err != nil {
		return err
	}
	if !canManageRoles {
		if target.Role != string(access.RolePlayer) || role != target.Role {
			return &apiError{Status: http.StatusForbidden, Message: "运营账号只能管理玩家账号，且不能修改角色"}
		}
	}
	if (target.Role == string(access.RoleSuperAdmin) || role == string(access.RoleSuperAdmin)) &&
		actor.Role != string(access.RoleSuperAdmin) {
		return &apiError{Status: http.StatusForbidden, Message: "只有超级管理员可以管理超级管理员账号"}
	}
	managedSpaceIDs := target.ManagedSpaceIDs
	if input.ManagedSpaceIDs != nil {
		if !canManageRoles {
			return &apiError{Status: http.StatusForbidden, Message: "当前账号不能分配频道管理范围"}
		}
		managedSpaceIDs = *input.ManagedSpaceIDs
	}
	if !roleAllowsChannelAssignments(roleRecord) {
		managedSpaceIDs = nil
	}
	if err := s.store.ValidateSpaceIDs(r.Context(), managedSpaceIDs); err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "包含不存在的频道"}
	}
	if err := s.store.ValidateManagedSpaceIDs(r.Context(), userID, managedSpaceIDs); err != nil {
		if errors.Is(err, store.ErrSpaceMembershipRequired) {
			return &apiError{Status: http.StatusBadRequest, Message: "只能分配该用户已经加入的频道"}
		}
		return err
	}
	if err := s.store.UpdateUser(r.Context(), userID, username, displayName, passwordHash, role, status); err != nil {
		if errors.Is(err, store.ErrLastSuperAdmin) {
			return &apiError{Status: http.StatusConflict, Message: "至少需要保留一名正常状态的超级管理员"}
		}
		if store.IsUniqueViolation(err) {
			return &apiError{Status: http.StatusConflict, Message: "用户名已存在"}
		}
		return err
	}
	if err := s.store.SetManagedSpaces(r.Context(), userID, managedSpaceIDs); err != nil {
		if errors.Is(err, store.ErrSpaceMembershipRequired) {
			return &apiError{Status: http.StatusBadRequest, Message: "只能分配该用户已经加入的频道"}
		}
		return err
	}
	updated, err := s.store.UserByID(r.Context(), userID)
	if err != nil {
		return err
	}
	updated.ManagedSpaceIDs, err = s.store.ManagedSpaceIDs(r.Context(), userID)
	if err != nil {
		return err
	}
	updated.JoinedSpaceIDs, err = s.store.JoinedSpaceIDs(r.Context(), userID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
	return nil
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
	if err != nil || userID <= 0 {
		return &apiError{Status: http.StatusBadRequest, Message: "用户 ID 无效"}
	}
	if userID == actor.ID {
		return &apiError{Status: http.StatusBadRequest, Message: "不能删除当前登录账号"}
	}
	target, err := s.store.UserByID(r.Context(), userID)
	if err != nil {
		return err
	}
	if r.URL.Query().Get("force") == "true" {
		if actor.Role != string(access.RoleSuperAdmin) {
			return &apiError{Status: http.StatusForbidden, Message: "只有超级管理员可以强制删除账号"}
		}
		return s.forceDeleteUser(w, r, actor, target)
	}
	canManageRoles, err := s.hasPermission(r.Context(), actor, access.PermissionRolesManage)
	if err != nil {
		return err
	}
	if !canManageRoles && target.Role != string(access.RolePlayer) {
		return &apiError{Status: http.StatusForbidden, Message: "运营账号只能删除玩家账号"}
	}
	if target.Role == string(access.RoleSuperAdmin) && actor.Role != string(access.RoleSuperAdmin) {
		return &apiError{Status: http.StatusForbidden, Message: "只有超级管理员可以删除超级管理员账号"}
	}
	if err := s.store.DeleteUser(r.Context(), userID); err != nil {
		switch {
		case errors.Is(err, store.ErrLastSuperAdmin):
			return &apiError{Status: http.StatusConflict, Message: "至少需要保留一名正常状态的超级管理员"}
		case errors.Is(err, store.ErrUserReferenced):
			return &apiError{Status: http.StatusConflict, Message: "该账号拥有频道或钱包流水，需保留审计记录；可以停用账号"}
		default:
			return err
		}
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) forceDeleteUser(w http.ResponseWriter, r *http.Request, actor, target store.User) error {
	spaceID, tableID, seated, err := s.userTableSeat(r.Context(), target.ID)
	if err != nil {
		return err
	}
	if seated {
		return &apiError{Status: http.StatusConflict, Message: fmt.Sprintf("该账号仍在牌桌 %s/%s 中，请先完成结算并清空座位", spaceID, tableID)}
	}

	bindings, err := s.store.NewAPIUserBindings(r.Context(), target.ID)
	if err != nil {
		return err
	}
	failures := make([]string, 0)
	type bindingGroup struct {
		label      string
		candidates []store.NewAPIUserBinding
	}
	groups := make([]bindingGroup, 0, len(bindings))
	groupIndex := make(map[string]int, len(bindings))
	for _, binding := range bindings {
		key := fmt.Sprintf("%s\x00%d", binding.BaseURL, binding.NewAPIUserID)
		index, ok := groupIndex[key]
		if !ok {
			index = len(groups)
			groupIndex[key] = index
			groups = append(groups, bindingGroup{label: binding.SpaceName})
		} else {
			groups[index].label += " / " + binding.SpaceName
		}
		groups[index].candidates = append(groups[index].candidates, binding)
	}
	for _, group := range groups {
		failure := "管理员凭证不可用"
		deleted := false
		for _, binding := range group.candidates {
			adminToken, decryptErr := s.cipher.Decrypt(binding.AdminTokenEnc)
			if decryptErr != nil {
				s.logger.Error("decrypt New API credential for forced user deletion", "space_id", binding.SpaceID, "user_id", target.ID, "error", decryptErr)
				continue
			}
			if deleteErr := s.newAPI.DeleteUser(r.Context(), binding.BaseURL, adminToken, binding.NewAPIUserID); deleteErr != nil {
				s.logger.Warn("delete New API user during forced account deletion", "space_id", binding.SpaceID, "user_id", target.ID, "newapi_user_id", binding.NewAPIUserID, "error", deleteErr)
				failure = deleteErr.Error()
				continue
			}
			deleted = true
			break
		}
		if !deleted {
			failures = append(failures, group.label+"："+failure)
		}
	}
	if len(failures) > 0 {
		return &apiError{Status: http.StatusBadGateway, Message: "New API 账号删除失败，本地账号未删除：" + strings.Join(failures, "；")}
	}

	if err := s.store.ForceDeleteUser(r.Context(), target.ID, actor.ID); err != nil {
		if errors.Is(err, store.ErrLastSuperAdmin) {
			return &apiError{Status: http.StatusConflict, Message: "至少需要保留一名正常状态的超级管理员"}
		}
		s.logger.Error("finalize forced local user deletion", "user_id", target.ID, "actor_user_id", actor.ID, "error", err)
		return &apiError{Status: http.StatusInternalServerError, Message: "New API 账号已删除，但本地账号注销失败；请检查服务日志后重试"}
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) userTableSeat(ctx context.Context, userID int64) (string, string, bool, error) {
	states, err := s.store.TableStates(ctx)
	if err != nil {
		return "", "", false, err
	}
	for _, state := range states {
		runtime, err := restoreTableRuntime(state.Data)
		if err != nil {
			return "", "", false, err
		}
		if runtime.isSeated(userID) {
			return state.SpaceID, state.TableID, true, nil
		}
	}
	return "", "", false, nil
}

func roleAllowsChannelAssignments(role store.Role) bool {
	for _, permission := range role.Permissions {
		if permission == string(access.PermissionChannelsManage) || permission == string(access.PermissionBalancesManage) {
			return true
		}
	}
	return false
}

func (s *Server) handleAdminRegistration(w http.ResponseWriter, r *http.Request, _ store.User) error {
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	if err := s.store.SetRegistrationEnabled(r.Context(), input.Enabled); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]bool{"registration_enabled": input.Enabled})
	return nil
}

func (s *Server) adminWeChatSettings(ctx context.Context) (adminWeChatSettingsView, error) {
	settings, err := s.store.WeChatSettings(ctx)
	if err == nil {
		secretConfigured := settings.AppSecretEnc != ""
		return adminWeChatSettingsView{
			AppID: settings.AppID, RedirectURI: settings.RedirectURI, Enabled: settings.Enabled,
			Configured:          settings.AppID != "" && settings.RedirectURI != "" && secretConfigured,
			AppSecretConfigured: secretConfigured, Source: "database",
		}, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return adminWeChatSettingsView{}, err
	}
	wechatAuth := s.currentWeChat()
	if wechatAuth == nil {
		return adminWeChatSettingsView{Source: "none"}, nil
	}
	return adminWeChatSettingsView{
		AppID: wechatAuth.appID, RedirectURI: wechatAuth.redirectURL, Enabled: true,
		Configured:          wechatAuth.appID != "" && wechatAuth.redirectURL != "" && wechatAuth.secretConfigured,
		AppSecretConfigured: wechatAuth.secretConfigured, Source: wechatAuth.source,
	}, nil
}

func (s *Server) handleAdminWeChatSettings(w http.ResponseWriter, r *http.Request, actor store.User) error {
	if actor.Role != string(access.RoleSuperAdmin) {
		return &apiError{Status: http.StatusForbidden, Message: "只有超级管理员可以修改微信登录配置"}
	}
	var input adminWeChatSettingsInput
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	input.AppID = strings.TrimSpace(input.AppID)
	input.AppSecret = strings.TrimSpace(input.AppSecret)
	input.RedirectURI = strings.TrimSpace(input.RedirectURI)
	if len(input.AppID) > 128 || len(input.AppSecret) > 256 || len(input.RedirectURI) > 2048 {
		return &apiError{Status: http.StatusBadRequest, Message: "微信登录配置长度无效"}
	}
	if input.RedirectURI != "" {
		redirectURL, err := url.Parse(input.RedirectURI)
		if err != nil || (redirectURL.Scheme != "http" && redirectURL.Scheme != "https") || redirectURL.Host == "" || redirectURL.User != nil || redirectURL.Fragment != "" || redirectURL.RawQuery != "" || redirectURL.Path != "/api/auth/wechat/callback" {
			return &apiError{Status: http.StatusBadRequest, Message: "回调地址必须是以 /api/auth/wechat/callback 结尾的完整 HTTP(S) 地址"}
		}
	}
	existing, err := s.store.WeChatSettings(r.Context())
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	secretEnc := existing.AppSecretEnc
	if input.AppSecret != "" {
		secretEnc, err = s.cipher.Encrypt(input.AppSecret)
		if err != nil {
			return err
		}
	}
	if input.Enabled && (input.AppID == "" || input.RedirectURI == "" || secretEnc == "") {
		return &apiError{Status: http.StatusBadRequest, Message: "启用微信登录前请填写 AppID、AppSecret 和回调地址"}
	}
	settings := store.WeChatSettings{AppID: input.AppID, AppSecretEnc: secretEnc, RedirectURI: input.RedirectURI, Enabled: input.Enabled}
	if err := s.store.SetWeChatSettings(r.Context(), settings); err != nil {
		return err
	}
	if err := s.reloadStoredWeChat(r.Context()); err != nil {
		return err
	}
	view, err := s.adminWeChatSettings(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"wechat_login": view})
	return nil
}

func (s *Server) handleAdminLoginHeroImage(w http.ResponseWriter, r *http.Request, actor store.User) error {
	if actor.Role != string(access.RoleSuperAdmin) {
		return &apiError{Status: http.StatusForbidden, Message: "只有超级管理员可以修改登录页宣传图"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginHeroImageBytes+(1<<20))
	if err := r.ParseMultipartForm(maxLoginHeroImageBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return &apiError{Status: http.StatusRequestEntityTooLarge, Message: "图片不能超过 5 MB"}
		}
		return &apiError{Status: http.StatusBadRequest, Message: "图片上传格式无效"}
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, _, err := r.FormFile("image")
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "请选择要上传的图片"}
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxLoginHeroImageBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxLoginHeroImageBytes {
		return &apiError{Status: http.StatusRequestEntityTooLarge, Message: "图片不能超过 5 MB"}
	}
	contentType := http.DetectContentType(data)
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" {
		return &apiError{Status: http.StatusUnsupportedMediaType, Message: "仅支持 JPEG、PNG 或 WebP 图片"}
	}
	updatedAt, err := s.store.SetLoginHeroImage(r.Context(), contentType, data)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"login_hero": loginHeroImagePayload(store.LoginHeroImageConfig{UpdatedAt: updatedAt, PositionX: 50, PositionY: 50, Zoom: 1})})
	return nil
}

func (s *Server) handleAdminUpdateLoginHeroPlacement(w http.ResponseWriter, r *http.Request, actor store.User) error {
	if actor.Role != string(access.RoleSuperAdmin) {
		return &apiError{Status: http.StatusForbidden, Message: "只有超级管理员可以修改登录页宣传图"}
	}
	var input adminLoginHeroPlacementInput
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	if input.PositionX < 0 || input.PositionX > 100 || input.PositionY < 0 || input.PositionY > 100 || input.Zoom < 1 || input.Zoom > 3 {
		return &apiError{Status: http.StatusBadRequest, Message: "图片位置或缩放比例无效"}
	}
	config, err := s.store.UpdateLoginHeroImagePlacement(r.Context(), input.PositionX, input.PositionY, input.Zoom)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"login_hero": loginHeroImagePayload(config)})
	return nil
}

func (s *Server) handleAdminDeleteLoginHeroImage(w http.ResponseWriter, r *http.Request, actor store.User) error {
	if actor.Role != string(access.RoleSuperAdmin) {
		return &apiError{Status: http.StatusForbidden, Message: "只有超级管理员可以修改登录页宣传图"}
	}
	if err := s.store.DeleteLoginHeroImage(r.Context()); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
