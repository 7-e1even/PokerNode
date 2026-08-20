package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"pokernode/internal/auth"
	"pokernode/internal/secure"
)

func TestMCPKeyLifecycleAndHTTPTransport(t *testing.T) {
	database := openTestDatabase(t)
	cipher, err := secure.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{12}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions("test-session-secret-that-is-long-enough", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	appServer := httptest.NewServer(NewServer(database, cipher, sessions, logger).Handler(filepath.Join(t.TempDir(), "missing-web")))
	defer appServer.Close()

	browser := newTestClient(t)
	requestJSON(t, browser, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{
		"username": "mcp_player", "display_name": "MCP Player", "password": "password-1",
	}, http.StatusCreated, nil)
	requestJSON(t, browser, http.MethodPut, appServer.URL+"/api/me/agent-control", map[string]bool{"enabled": true}, http.StatusConflict, nil)

	var created struct {
		Key    string       `json:"mcp_key"`
		Status mcpKeyStatus `json:"status"`
	}
	requestJSON(t, browser, http.MethodPost, appServer.URL+"/api/me/mcp-key", nil, http.StatusCreated, &created)
	if !strings.HasPrefix(created.Key, auth.MCPKeyPrefix) || !created.Status.Exists || created.Status.Last4 == "" {
		t.Fatalf("MCP key was not created correctly: %#v", created)
	}
	directMCP := &http.Client{Transport: applicationBearerRoundTripper{token: created.Key, base: http.DefaultTransport}}
	requestJSON(t, directMCP, http.MethodGet, appServer.URL+"/api/spaces", nil, http.StatusOK, nil)
	requestJSON(t, directMCP, http.MethodPut, appServer.URL+"/api/me/agent-control", map[string]bool{"enabled": true}, http.StatusUnauthorized, nil)
	requestJSON(t, browser, http.MethodPut, appServer.URL+"/api/me/agent-control", map[string]bool{"enabled": true}, http.StatusOK, nil)
	var control struct {
		Enabled bool `json:"enabled"`
	}
	requestJSON(t, browser, http.MethodGet, appServer.URL+"/api/me/agent-control", nil, http.StatusOK, &control)
	if !control.Enabled {
		t.Fatal("Agent control was not enabled")
	}

	var status struct {
		Key    string       `json:"mcp_key"`
		Status mcpKeyStatus `json:"status"`
	}
	requestJSON(t, browser, http.MethodGet, appServer.URL+"/api/me/mcp-key", nil, http.StatusOK, &status)
	if status.Key != "" || !status.Status.Exists || status.Status.Last4 != created.Status.Last4 {
		t.Fatalf("status endpoint leaked or lost key metadata: %#v", status)
	}

	oldSession, err := connectApplicationMCP(appServer.URL+"/mcp", created.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer oldSession.Close()
	if result, err := oldSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pokernode_list_channels", Arguments: map[string]any{}}); err != nil || result.IsError {
		t.Fatalf("created MCP key could not call a tool: result=%#v err=%v", result, err)
	}

	var rotated struct {
		Key    string       `json:"mcp_key"`
		Status mcpKeyStatus `json:"status"`
	}
	requestJSON(t, browser, http.MethodPost, appServer.URL+"/api/me/mcp-key", nil, http.StatusCreated, &rotated)
	if rotated.Key == created.Key || rotated.Status.Last4 != rotated.Key[len(rotated.Key)-4:] {
		t.Fatalf("rotation did not replace the MCP key: before=%#v after=%#v", created, rotated)
	}
	if _, err := oldSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pokernode_list_channels", Arguments: map[string]any{}}); err == nil {
		t.Fatal("rotated MCP key remained authorized")
	}

	newSession, err := connectApplicationMCP(appServer.URL+"/mcp", rotated.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer newSession.Close()
	if result, err := newSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pokernode_list_channels", Arguments: map[string]any{}}); err != nil || result.IsError {
		t.Fatalf("rotated MCP key could not call a tool: result=%#v err=%v", result, err)
	}

	requestJSON(t, browser, http.MethodDelete, appServer.URL+"/api/me/mcp-key", nil, http.StatusNoContent, nil)
	if _, err := newSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pokernode_list_channels", Arguments: map[string]any{}}); err == nil {
		t.Fatal("revoked MCP key remained authorized")
	}
	requestJSON(t, browser, http.MethodGet, appServer.URL+"/api/me/mcp-key", nil, http.StatusOK, &status)
	if status.Status.Exists {
		t.Fatalf("revoked MCP key still appears active: %#v", status.Status)
	}
	requestJSON(t, browser, http.MethodGet, appServer.URL+"/api/me/agent-control", nil, http.StatusOK, &control)
	if control.Enabled {
		t.Fatal("revoking the MCP key did not return control to the user")
	}
}

func connectApplicationMCP(endpoint, token string) (*mcp.ClientSession, error) {
	transport := &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           &http.Client{Transport: applicationBearerRoundTripper{token: token, base: http.DefaultTransport}},
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "pokernode-app-test", Version: "1.0.0"}, nil)
	return client.Connect(context.Background(), transport, nil)
}

type applicationBearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (transport applicationBearerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+transport.token)
	return transport.base.RoundTrip(clone)
}
