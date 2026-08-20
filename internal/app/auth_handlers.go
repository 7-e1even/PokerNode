package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"pokernode/internal/access"
	"pokernode/internal/auth"
	"pokernode/internal/store"
	"pokernode/internal/wechat"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,24}$`)

const (
	maxAvatarBytes     = 2 << 20
	wechatProviderName = "wechat"
	wechatStateCookie  = "pokernode_wechat_state"
	wechatFlowCookie   = "pokernode_wechat_flow"
	wechatFlowLogin    = "login"
	wechatFlowLink     = "link"
)

type wechatProvider interface {
	AuthorizeURL(redirectURL, state string) string
	Authenticate(context.Context, string) (wechat.Profile, error)
}

type wechatAuth struct {
	redirectURL string
	provider    wechatProvider
}

type authInput struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type credentialsInput struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type profileInput struct {
	DisplayName string `json:"display_name"`
}

type authenticatedUser struct {
	store.User
	Permissions []access.Permission `json:"permissions"`
	WeChatBound bool                `json:"wechat_bound"`
	HasPassword bool                `json:"has_password"`
}

func (s *Server) presentUser(ctx context.Context, user store.User) (authenticatedUser, error) {
	permissions, err := s.permissionsForUser(ctx, user)
	if err != nil {
		return authenticatedUser{}, err
	}
	wechatBound, err := s.store.HasExternalIdentity(ctx, user.ID, wechatProviderName)
	if err != nil {
		return authenticatedUser{}, err
	}
	return authenticatedUser{User: user, Permissions: permissions, WeChatBound: wechatBound, HasPassword: user.HasPassword()}, nil
}

func validateAccountInput(input *authInput) error {
	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !usernamePattern.MatchString(input.Username) {
		return &apiError{Status: http.StatusBadRequest, Message: "用户名需为 3–24 位字母、数字或下划线"}
	}
	if len(input.Password) < 8 || len(input.Password) > 72 {
		return &apiError{Status: http.StatusBadRequest, Message: "密码长度需为 8–72 位"}
	}
	if input.DisplayName == "" {
		input.DisplayName = input.Username
	}
	if len([]rune(input.DisplayName)) > 32 {
		return &apiError{Status: http.StatusBadRequest, Message: "显示名称不能超过 32 个字符"}
	}
	return nil
}

func (s *Server) handlePublicConfig(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.store.RegistrationEnabled(r.Context())
	if err != nil {
		s.writeHandlerError(w, err)
		return
	}
	loginHeroConfig, err := s.store.LoginHeroImageConfig(r.Context())
	if err != nil {
		s.writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"registration_enabled": enabled,
		"wechat_login_enabled": s.wechat != nil,
		"login_hero":           loginHeroImagePayload(loginHeroConfig),
	})
}

func (s *Server) handleLoginHeroImage(w http.ResponseWriter, r *http.Request) {
	asset, err := s.store.LoginHeroImage(r.Context())
	if err != nil {
		s.writeHandlerError(w, err)
		return
	}
	w.Header().Set("Content-Type", asset.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(asset.Data)))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(asset.Data)
}

func loginHeroImageURL(version string) string {
	if version == "" {
		return ""
	}
	return "/api/branding/login-hero?v=" + url.QueryEscape(version)
}

func loginHeroImagePayload(config store.LoginHeroImageConfig) map[string]any {
	return map[string]any{
		"url":        loginHeroImageURL(config.UpdatedAt),
		"position_x": config.PositionX,
		"position_y": config.PositionY,
		"zoom":       config.Zoom,
	}
}

func (s *Server) handleWeChatStart(w http.ResponseWriter, r *http.Request) {
	if err := s.beginWeChatAuth(w, r, wechatFlowLogin); err != nil {
		s.logger.Error("start wechat login", "error", err)
		s.redirectWeChatResult(w, r, wechatFlowLogin, "unavailable")
	}
}

func (s *Server) handleWeChatLink(w http.ResponseWriter, r *http.Request, _ store.User) error {
	if err := s.beginWeChatAuth(w, r, wechatFlowLink); err != nil {
		s.logger.Error("start wechat link", "error", err)
		s.redirectWeChatResult(w, r, wechatFlowLink, "unavailable")
	}
	return nil
}

func (s *Server) beginWeChatAuth(w http.ResponseWriter, r *http.Request, flow string) error {
	if s.wechat == nil {
		return errors.New("wechat login is not configured")
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	state := base64.RawURLEncoding.EncodeToString(random)
	s.setWeChatCookie(w, r, wechatStateCookie, state, 10*time.Minute)
	s.setWeChatCookie(w, r, wechatFlowCookie, flow, 10*time.Minute)
	http.Redirect(w, r, s.wechat.provider.AuthorizeURL(s.wechat.redirectURL, state), http.StatusFound)
	return nil
}

func (s *Server) handleWeChatCallback(w http.ResponseWriter, r *http.Request) {
	flowCookie, _ := r.Cookie(wechatFlowCookie)
	flow := wechatFlowLogin
	if flowCookie != nil && flowCookie.Value == wechatFlowLink {
		flow = wechatFlowLink
	}
	stateCookie, stateErr := r.Cookie(wechatStateCookie)
	state := r.URL.Query().Get("state")
	if stateErr != nil || state == "" || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(state)) != 1 {
		s.clearWeChatCookies(w, r)
		s.redirectWeChatResult(w, r, flow, "invalid_state")
		return
	}
	s.clearWeChatCookies(w, r)
	if s.wechat == nil {
		s.redirectWeChatResult(w, r, flow, "unavailable")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		s.redirectWeChatResult(w, r, flow, "cancelled")
		return
	}
	profile, err := s.wechat.provider.Authenticate(r.Context(), code)
	if err != nil || profile.Subject() == "" {
		s.logger.Error("complete wechat authorization", "error", err)
		s.redirectWeChatResult(w, r, flow, "provider_failed")
		return
	}

	if flow == wechatFlowLink {
		user, err := s.currentUser(r)
		if err != nil {
			s.redirectWeChatResult(w, r, flow, "session_expired")
			return
		}
		if err := s.store.BindExternalIdentity(r.Context(), user.ID, wechatProviderName, profile.Subject(), profile.AvatarURL); err != nil {
			switch {
			case errors.Is(err, store.ErrExternalIdentityBound):
				s.redirectWeChatResult(w, r, flow, "already_bound")
			case errors.Is(err, store.ErrExternalIdentitySet):
				s.redirectWeChatResult(w, r, flow, "account_bound")
			default:
				s.logger.Error("bind wechat identity", "error", err)
				s.redirectWeChatResult(w, r, flow, "failed")
			}
			return
		}
		s.redirectWeChatResult(w, r, flow, "success")
		return
	}

	user, err := s.store.CreateExternalUser(r.Context(), wechatProviderName, profile.Subject(), profile.Nickname, profile.AvatarURL)
	if err != nil {
		if errors.Is(err, store.ErrRegistrationClosed) {
			s.redirectWeChatResult(w, r, flow, "registration_closed")
			return
		}
		s.logger.Error("create wechat user", "error", err)
		s.redirectWeChatResult(w, r, flow, "failed")
		return
	}
	if user.Status != "active" {
		s.redirectWeChatResult(w, r, flow, "disabled")
		return
	}
	s.issueSession(w, r, user)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) setWeChatCookie(w http.ResponseWriter, r *http.Request, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/api/auth/wechat", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil, Expires: time.Now().Add(ttl), MaxAge: int(ttl.Seconds()),
	})
}

func (s *Server) clearWeChatCookies(w http.ResponseWriter, r *http.Request) {
	for _, name := range []string{wechatStateCookie, wechatFlowCookie} {
		s.setWeChatCookie(w, r, name, "", -time.Hour)
	}
}

func (s *Server) redirectWeChatResult(w http.ResponseWriter, r *http.Request, flow, result string) {
	parameter := "wechat_error"
	path := "/"
	if flow == wechatFlowLink {
		parameter = "wechat_link"
		path = "/settings"
	}
	http.Redirect(w, r, path+"?"+parameter+"="+result, http.StatusFound)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var input authInput
	if err := decodeJSON(r, &input); err != nil {
		s.writeHandlerError(w, err)
		return
	}
	if err := validateAccountInput(&input); err != nil {
		s.writeHandlerError(w, err)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		s.writeHandlerError(w, err)
		return
	}
	user, err := s.store.CreateRegisteredUser(r.Context(), input.Username, input.DisplayName, string(hash))
	if err != nil {
		if errors.Is(err, store.ErrRegistrationClosed) {
			writeError(w, &apiError{Status: http.StatusForbidden, Message: "平台已关闭自助注册，请联系管理员创建账号"})
			return
		}
		if store.IsUniqueViolation(err) {
			writeError(w, &apiError{Status: http.StatusConflict, Message: "用户名已存在"})
			return
		}
		s.writeHandlerError(w, err)
		return
	}
	s.issueSession(w, r, user)
	presented, err := s.presentUser(r.Context(), user)
	if err != nil {
		s.writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": presented})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	s.handleLoginForPermission(w, r, "", "")
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	s.handleLoginForPermission(w, r, access.PermissionAdminView, "该账号没有运营后台权限")
}

func (s *Server) handleLoginForPermission(w http.ResponseWriter, r *http.Request, required access.Permission, forbiddenMessage string) {
	var input authInput
	if err := decodeJSON(r, &input); err != nil {
		s.writeHandlerError(w, err)
		return
	}
	user, err := s.store.UserByUsername(r.Context(), strings.TrimSpace(input.Username))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
		writeError(w, &apiError{Status: http.StatusUnauthorized, Message: "用户名或密码错误"})
		return
	}
	if user.Status != "active" {
		writeError(w, &apiError{Status: http.StatusForbidden, Message: "账号已停用，请联系管理员"})
		return
	}
	if required != "" {
		allowed, permissionErr := s.hasPermission(r.Context(), user, required)
		if permissionErr != nil {
			s.writeHandlerError(w, permissionErr)
			return
		}
		if !allowed {
			writeError(w, &apiError{Status: http.StatusForbidden, Message: forbiddenMessage})
			return
		}
	}
	s.issueSession(w, r, user)
	presented, err := s.presentUser(r.Context(), user)
	if err != nil {
		s.writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": presented})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil, Expires: time.Unix(0, 0), MaxAge: -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, user store.User) error {
	presented, err := s.presentUser(r.Context(), user)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": presented})
	return nil
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request, user store.User) error {
	var input profileInput
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.DisplayName == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "显示名称不能为空"}
	}
	if len([]rune(input.DisplayName)) > 32 {
		return &apiError{Status: http.StatusBadRequest, Message: "显示名称不能超过 32 个字符"}
	}
	if err := s.store.UpdateUserProfile(r.Context(), user.ID, input.DisplayName); err != nil {
		return err
	}
	user.DisplayName = input.DisplayName
	presented, err := s.presentUser(r.Context(), user)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": presented})
	return nil
}

func (s *Server) handleUpdateAvatar(w http.ResponseWriter, r *http.Request, user store.User) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+(64<<10))
	if err := r.ParseMultipartForm(maxAvatarBytes); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return &apiError{Status: http.StatusRequestEntityTooLarge, Message: "头像不能超过 2 MB"}
		}
		return &apiError{Status: http.StatusBadRequest, Message: "头像上传格式不正确"}
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, _, err := r.FormFile("avatar")
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "请选择头像文件"}
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAvatarBytes+1))
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return &apiError{Status: http.StatusBadRequest, Message: "头像文件不能为空"}
	}
	if len(data) > maxAvatarBytes {
		return &apiError{Status: http.StatusRequestEntityTooLarge, Message: "头像不能超过 2 MB"}
	}
	contentType := http.DetectContentType(data)
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" && contentType != "image/gif" {
		return &apiError{Status: http.StatusUnsupportedMediaType, Message: "只支持 JPG、PNG、WebP 或 GIF 头像"}
	}
	avatarURL := fmt.Sprintf("/api/users/%d/avatar?v=%d", user.ID, time.Now().UTC().UnixNano())
	if err := s.store.UpdateUserAvatar(r.Context(), user.ID, avatarURL, contentType, data); err != nil {
		return err
	}
	user.AvatarURL = avatarURL
	presented, err := s.presentUser(r.Context(), user)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": presented})
	return nil
}

func (s *Server) handleDeleteAvatar(w http.ResponseWriter, r *http.Request, user store.User) error {
	if err := s.store.ClearUserAvatar(r.Context(), user.ID); err != nil {
		return err
	}
	user.AvatarURL = ""
	presented, err := s.presentUser(r.Context(), user)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": presented})
	return nil
}

func (s *Server) handleUserAvatar(w http.ResponseWriter, r *http.Request, _ store.User) error {
	userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
	if err != nil || userID <= 0 {
		return &apiError{Status: http.StatusBadRequest, Message: "用户 ID 无效"}
	}
	avatar, err := s.store.UserAvatar(r.Context(), userID)
	if err != nil {
		return err
	}
	if len(avatar.Data) > 0 && strings.HasPrefix(avatar.ContentType, "image/") {
		w.Header().Set("Cache-Control", "private, no-cache")
		w.Header().Set("Content-Type", avatar.ContentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(avatar.Data)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(avatar.Data)
		return nil
	}
	if strings.HasPrefix(avatar.URL, "https://") || strings.HasPrefix(avatar.URL, "http://") {
		http.Redirect(w, r, avatar.URL, http.StatusTemporaryRedirect)
		return nil
	}
	return &apiError{Status: http.StatusNotFound, Message: "头像不存在"}
}

func (s *Server) handleUpdateCredentials(w http.ResponseWriter, r *http.Request, user store.User) error {
	var input credentialsInput
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	input.Username = strings.TrimSpace(input.Username)
	if !usernamePattern.MatchString(input.Username) {
		return &apiError{Status: http.StatusBadRequest, Message: "用户名需为 3–24 位字母、数字或下划线"}
	}
	if user.HasPassword() {
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.CurrentPassword)) != nil {
			return &apiError{Status: http.StatusUnauthorized, Message: "当前密码错误"}
		}
	} else if input.NewPassword == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "首次设置登录账号时必须同时设置密码"}
	}
	if input.NewPassword == "" && input.Username == user.Username {
		return &apiError{Status: http.StatusBadRequest, Message: "账号或密码没有变化"}
	}

	passwordHash := user.PasswordHash
	if input.NewPassword != "" {
		if len(input.NewPassword) < 8 || len(input.NewPassword) > 72 {
			return &apiError{Status: http.StatusBadRequest, Message: "新密码长度需为 8–72 位"}
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		passwordHash = string(hash)
	}
	if err := s.store.UpdateUserCredentials(r.Context(), user.ID, input.Username, passwordHash); err != nil {
		if store.IsUniqueViolation(err) {
			return &apiError{Status: http.StatusConflict, Message: "用户名已存在"}
		}
		return err
	}
	user.Username = input.Username
	user.PasswordHash = passwordHash
	s.issueSession(w, r, user)
	presented, err := s.presentUser(r.Context(), user)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": presented})
	return nil
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, user store.User) {
	value, expires, err := s.sessions.Issue(user.ID, user.Username)
	if err != nil {
		panic(err)
	}
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil, Expires: expires, MaxAge: int(time.Until(expires).Seconds()),
	})
}
