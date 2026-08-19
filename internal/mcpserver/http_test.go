package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHTTPMCPIsolatesPlayersByBearerKey(t *testing.T) {
	t.Parallel()
	clients := map[string]*APIClient{
		"key-alice": fakePlayerAPIClient(t, "alice-channel"),
		"key-bob":   fakePlayerAPIClient(t, "bob-channel"),
	}
	handler := NewHTTPHandler(func(_ context.Context, token string, _ *http.Request) (*HTTPIdentity, error) {
		client := clients[token]
		if client == nil {
			return nil, errors.New("unknown key")
		}
		return &HTTPIdentity{UserID: token, Client: client}, nil
	}, nil)
	server := httptest.NewServer(handler)
	defer server.Close()

	for token, expectedChannel := range map[string]string{"key-alice": "alice-channel", "key-bob": "bob-channel"} {
		t.Run(token, func(t *testing.T) {
			session := connectHTTPMCP(t, server.URL, token)
			defer session.Close()
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pokernode_list_channels", Arguments: map[string]any{}})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			var output ListChannelsOutput
			if err := json.Unmarshal(encoded, &output); err != nil {
				t.Fatal(err)
			}
			if len(output.Channels) != 1 || output.Channels[0].ID != expectedChannel {
				t.Fatalf("key resolved the wrong player view: %#v", output.Channels)
			}
		})
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:             server.URL,
		HTTPClient:           &http.Client{Transport: bearerRoundTripper{token: "invalid-key", base: http.DefaultTransport}},
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "unauthorized-test", Version: "1.0.0"}, nil)
	if session, err := client.Connect(context.Background(), transport, nil); err == nil {
		session.Close()
		t.Fatal("invalid bearer key connected successfully")
	}
}

func fakePlayerAPIClient(t *testing.T, channelID string) *APIClient {
	t.Helper()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/spaces" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []map[string]any{{"id": channelID, "name": channelID, "is_bound": true}}})
	}))
	t.Cleanup(api.Close)
	client, err := NewAPIClient(Config{BaseURL: api.URL, SessionToken: "test-session"}, api.Client())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func connectHTTPMCP(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()
	transport := &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           &http.Client{Transport: bearerRoundTripper{token: token, base: http.DefaultTransport}},
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "http-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (transport bearerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+transport.token)
	return transport.base.RoundTrip(clone)
}
