package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"pokernode/internal/auth"
	"pokernode/internal/mcpserver"
	"pokernode/internal/store"
)

type mcpKeyStatus struct {
	Exists    bool   `json:"exists"`
	Last4     string `json:"last4,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func (s *Server) handleMCPKeyStatus(w http.ResponseWriter, r *http.Request, user store.User) error {
	w.Header().Set("Cache-Control", "no-store")
	key, err := s.store.MCPKeyForUser(r.Context(), user.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"status": mcpKeyStatus{}})
		return nil
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": presentMCPKey(key)})
	return nil
}

func (s *Server) handleCreateMCPKey(w http.ResponseWriter, r *http.Request, user store.User) error {
	w.Header().Set("Cache-Control", "no-store")
	token, hash, last4, err := auth.GenerateMCPKey()
	if err != nil {
		return err
	}
	key, err := s.store.UpsertMCPKey(r.Context(), user.ID, hash, last4)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"mcp_key": token, "status": presentMCPKey(key)})
	return nil
}

func (s *Server) handleDeleteMCPKey(w http.ResponseWriter, r *http.Request, user store.User) error {
	w.Header().Set("Cache-Control", "no-store")
	if err := s.store.DeleteMCPKey(r.Context(), user.ID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func presentMCPKey(key store.MCPKey) mcpKeyStatus {
	return mcpKeyStatus{Exists: true, Last4: key.Last4, CreatedAt: key.CreatedAt}
}

func (s *Server) mcpHTTPHandler(apiHandler http.Handler) http.Handler {
	return mcpserver.NewHTTPHandler(func(ctx context.Context, token string, _ *http.Request) (*mcpserver.HTTPIdentity, error) {
		hash, err := auth.HashMCPKey(token)
		if err != nil {
			return nil, err
		}
		user, err := s.store.UserByMCPKeyHash(ctx, hash)
		if err != nil {
			return nil, err
		}
		if user.Status != "active" {
			return nil, errors.New("MCP key owner is not active")
		}
		sessionToken, _, err := s.sessions.Issue(user.ID, user.Username)
		if err != nil {
			return nil, fmt.Errorf("issue internal MCP session: %w", err)
		}
		httpClient := &http.Client{
			Transport: handlerRoundTripper{handler: apiHandler},
			Timeout:   2 * time.Minute,
		}
		client, err := mcpserver.NewAPIClient(mcpserver.Config{
			BaseURL: "http://pokernode.internal", SessionToken: sessionToken, AllowInsecureHTTP: true,
		}, httpClient)
		if err != nil {
			return nil, err
		}
		return &mcpserver.HTTPIdentity{UserID: strconv.FormatInt(user.ID, 10), Client: client}, nil
	}, s.logger)
}

type handlerRoundTripper struct {
	handler http.Handler
}

func (transport handlerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != "http" || request.URL.Host != "pokernode.internal" {
		return nil, fmt.Errorf("unexpected internal MCP target %s", request.URL.Redacted())
	}
	recorder := httptest.NewRecorder()
	transport.handler.ServeHTTP(recorder, request)
	response := recorder.Result()
	response.Request = request
	return response, nil
}
