package app

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"pokernode/internal/access"
	"pokernode/internal/auth"
	"pokernode/internal/newapi"
	"pokernode/internal/poker"
	"pokernode/internal/realtime"
	"pokernode/internal/secure"
	"pokernode/internal/store"
)

type Server struct {
	store                   *store.Store
	cipher                  *secure.Cipher
	sessions                *auth.Sessions
	newAPI                  *newapi.Client
	wechat                  *wechatAuth
	hub                     *realtime.Hub
	logger                  *slog.Logger
	websocketOriginPatterns []string

	tablesMu  sync.Mutex
	tables    map[string]*tableRuntime
	balanceMu sync.Mutex
}

type tableRuntime struct {
	mu          sync.Mutex
	table       *poker.Table
	deleted     bool
	timer       *time.Timer
	timerTurnID uint64
	kickVote    *kickVote
	kickedUntil map[int64]time.Time
}

type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return e.Message }

type ServerOption func(*Server)

func WithWeChat(redirectURL string, provider wechatProvider) ServerOption {
	return func(server *Server) {
		server.wechat = &wechatAuth{redirectURL: redirectURL, provider: provider}
	}
}

func WithWebSocketOrigins(patterns ...string) ServerOption {
	return func(server *Server) {
		for _, pattern := range patterns {
			if pattern = strings.TrimSpace(pattern); pattern != "" {
				server.websocketOriginPatterns = append(server.websocketOriginPatterns, pattern)
			}
		}
	}
}

func NewServer(database *store.Store, cipher *secure.Cipher, sessions *auth.Sessions, logger *slog.Logger, options ...ServerOption) *Server {
	server := &Server{
		store: database, cipher: cipher, sessions: sessions, newAPI: newapi.NewClient(),
		hub: realtime.NewHub(), logger: logger, tables: make(map[string]*tableRuntime),
	}
	for _, option := range options {
		option(server)
	}
	return server
}

func (s *Server) Handler(webRoot string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReadiness)
	mux.HandleFunc("GET /api/config", s.handlePublicConfig)
	mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("GET /api/auth/wechat/start", s.handleWeChatStart)
	mux.HandleFunc("GET /api/auth/wechat/link", s.withUser(s.handleWeChatLink))
	mux.HandleFunc("GET /api/auth/wechat/callback", s.handleWeChatCallback)
	mux.HandleFunc("POST /api/admin/auth/login", s.handleAdminLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/me", s.withUser(s.handleMe))
	mux.HandleFunc("PATCH /api/me/credentials", s.withUser(s.handleUpdateCredentials))
	mux.HandleFunc("GET /api/account-bindings", s.withUser(s.handleListAccountBindings))
	mux.HandleFunc("GET /api/spaces", s.withUser(s.handleListSpaces))
	mux.HandleFunc("POST /api/spaces", s.withUser(s.handleCreateSpace))
	mux.HandleFunc("POST /api/spaces/join", s.withUser(s.handleJoinSpace))
	mux.HandleFunc("GET /api/spaces/{spaceID}", s.withUser(s.handleGetSpace))
	mux.HandleFunc("PUT /api/spaces/{spaceID}/connection", s.withUser(s.handleUpdateSpaceConnection))
	mux.HandleFunc("POST /api/spaces/{spaceID}/bind", s.withUser(s.handleBindMember))
	mux.HandleFunc("GET /api/spaces/{spaceID}/balance", s.withUser(s.handleBalance))
	mux.HandleFunc("GET /api/spaces/{spaceID}/managed-balances", s.withUser(s.handleManagedBalances))
	mux.HandleFunc("POST /api/spaces/{spaceID}/managed-balances/{userID}/adjust", s.withUser(s.handleAdjustManagedBalance))
	mux.HandleFunc("GET /api/spaces/{spaceID}/operations", s.withUser(s.handleOperations))
	mux.HandleFunc("GET /api/spaces/{spaceID}/leaderboard", s.withUser(s.handleChannelLeaderboard))
	mux.HandleFunc("GET /api/spaces/{spaceID}/tables", s.withUser(s.handleListTables))
	mux.HandleFunc("POST /api/spaces/{spaceID}/tables", s.withUser(s.handleCreateTable))
	mux.HandleFunc("GET /api/spaces/{spaceID}/tables/{tableID}", s.withUser(s.handleTable))
	mux.HandleFunc("DELETE /api/spaces/{spaceID}/tables/{tableID}", s.withUser(s.handleDeleteTable))
	mux.HandleFunc("POST /api/spaces/{spaceID}/tables/{tableID}/join", s.withUser(s.handleTableJoin))
	mux.HandleFunc("POST /api/spaces/{spaceID}/tables/{tableID}/leave", s.withUser(s.handleTableLeave))
	mux.HandleFunc("POST /api/spaces/{spaceID}/tables/{tableID}/ready", s.withUser(s.handleTableReady))
	mux.HandleFunc("POST /api/spaces/{spaceID}/tables/{tableID}/kick-vote", s.withUser(s.handleTableKickVote))
	// Keep /start as a compatibility alias; it now records readiness rather than starting unilaterally.
	mux.HandleFunc("POST /api/spaces/{spaceID}/tables/{tableID}/start", s.withUser(s.handleTableReady))
	mux.HandleFunc("POST /api/spaces/{spaceID}/tables/{tableID}/action", s.withUser(s.handleTableAction))
	mux.HandleFunc("GET /api/spaces/{spaceID}/tables/{tableID}/ws", s.withUser(s.handleWebSocket))
	// Legacy routes keep existing installations on the default table.
	mux.HandleFunc("GET /api/spaces/{spaceID}/table", s.withUser(s.handleTable))
	mux.HandleFunc("POST /api/spaces/{spaceID}/table/join", s.withUser(s.handleTableJoin))
	mux.HandleFunc("POST /api/spaces/{spaceID}/table/leave", s.withUser(s.handleTableLeave))
	mux.HandleFunc("POST /api/spaces/{spaceID}/table/ready", s.withUser(s.handleTableReady))
	mux.HandleFunc("POST /api/spaces/{spaceID}/table/kick-vote", s.withUser(s.handleTableKickVote))
	mux.HandleFunc("POST /api/spaces/{spaceID}/table/start", s.withUser(s.handleTableReady))
	mux.HandleFunc("POST /api/spaces/{spaceID}/table/action", s.withUser(s.handleTableAction))
	mux.HandleFunc("GET /api/spaces/{spaceID}/ws", s.withUser(s.handleWebSocket))
	mux.HandleFunc("GET /api/admin/overview", s.withPermission(access.PermissionAdminView, s.handleAdminOverview))
	mux.HandleFunc("POST /api/admin/users", s.withPermission(access.PermissionUsersManage, s.handleAdminCreateUser))
	mux.HandleFunc("PATCH /api/admin/users/{userID}", s.withPermission(access.PermissionUsersManage, s.handleAdminUpdateUser))
	mux.HandleFunc("DELETE /api/admin/users/{userID}", s.withPermission(access.PermissionUsersManage, s.handleAdminDeleteUser))
	mux.HandleFunc("PUT /api/admin/rankings/{userID}", s.withUser(s.handleAdminRankingVisibility))
	mux.HandleFunc("GET /api/admin/roles", s.withPermission(access.PermissionRolesManage, s.handleAdminRoles))
	mux.HandleFunc("POST /api/admin/roles", s.withPermission(access.PermissionRolesManage, s.handleAdminCreateRole))
	mux.HandleFunc("PATCH /api/admin/roles/{roleKey}", s.withPermission(access.PermissionRolesManage, s.handleAdminUpdateRole))
	mux.HandleFunc("DELETE /api/admin/roles/{roleKey}", s.withPermission(access.PermissionRolesManage, s.handleAdminDeleteRole))
	mux.HandleFunc("PUT /api/admin/settings/registration", s.withPermission(access.PermissionRegistrationManage, s.handleAdminRegistration))
	mux.Handle("/", staticHandler(webRoot))
	return s.recover(mux)
}

type userHandler func(http.ResponseWriter, *http.Request, store.User) error

func (s *Server) withUser(next userHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.currentUser(r)
		if err != nil {
			writeError(w, &apiError{Status: http.StatusUnauthorized, Message: "请先登录"})
			return
		}
		if err := next(w, r, user); err != nil {
			s.writeHandlerError(w, err)
		}
	}
}

func (s *Server) withPermission(permission access.Permission, next userHandler) http.HandlerFunc {
	return s.withUser(func(w http.ResponseWriter, r *http.Request, user store.User) error {
		allowed, err := s.hasPermission(r.Context(), user, permission)
		if err != nil {
			return err
		}
		if !allowed {
			return &apiError{Status: http.StatusForbidden, Message: "当前账号没有此操作权限"}
		}
		return next(w, r, user)
	})
}

func (s *Server) currentUser(r *http.Request) (store.User, error) {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		return store.User{}, err
	}
	claims, err := s.sessions.Parse(cookie.Value)
	if err != nil {
		return store.User{}, err
	}
	user, err := s.store.UserByID(r.Context(), claims.UserID)
	if err != nil {
		return store.User{}, err
	}
	if user.Status != "active" {
		return store.User{}, errors.New("user disabled")
	}
	return user, nil
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("request panic", "method", r.Method, "path", r.URL.Path, "error", recovered)
				writeError(w, &apiError{Status: http.StatusInternalServerError, Message: "服务器内部错误"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeHandlerError(w http.ResponseWriter, err error) {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		writeError(w, apiErr)
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, &apiError{Status: http.StatusNotFound, Message: "未找到该资源"})
		return
	}
	s.logger.Error("request failed", "error", err)
	writeError(w, &apiError{Status: http.StatusInternalServerError, Message: "服务器内部错误"})
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "请求格式不正确"}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err *apiError) {
	writeJSON(w, err.Status, map[string]string{"error": err.Message})
}

func staticHandler(root string) http.Handler {
	root = filepath.Clean(root)
	fileServer := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		index := filepath.Join(root, "index.html")
		if _, err := os.Stat(index); err != nil {
			http.Error(w, "PokerNode frontend has not been built", http.StatusServiceUnavailable)
			return
		}
		http.ServeFile(w, r, index)
	})
}
