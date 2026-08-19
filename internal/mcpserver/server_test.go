package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"pokernode/internal/auth"
	"pokernode/internal/poker"
)

func TestMCPReadStateThenAct(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var receivedAction struct {
		Action      poker.ActionType `json:"action"`
		AmountCents int64            `json:"amount_cents"`
	}
	snapshot := poker.Snapshot{
		ID: "table-1", Name: "Agent table", Street: poker.StreetFlop, HandID: 7,
		Board:      []poker.Card{{Rank: poker.Ace, Suit: poker.Spades}, {Rank: poker.Ten, Suit: poker.Diamonds}},
		ViewerSeat: 1, ActingSeat: 1, Pot: 450, CurrentBet: 200,
		Players: []poker.PlayerView{{UserID: 11, Name: "agent", Seat: 1, Stack: 4_800, Bet: 200, Cards: []poker.Card{{Rank: poker.King, Suit: poker.Hearts}, {Rank: poker.Queen, Suit: poker.Hearts}}, InHand: true, IsActing: true}},
		Allowed: poker.AllowedActions{CanAct: true, CanFold: true, CanCall: true, CanRaise: true, ToCall: 200, MinRaiseTo: 400, MaxRaiseTo: 5_000},
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "test-session", Path: "/"})
			writeTestJSON(t, w, map[string]any{"user": map[string]any{"id": 11}})
			return
		}
		if cookie, err := r.Cookie(auth.CookieName); err != nil || cookie.Value != "test-session" {
			w.WriteHeader(http.StatusUnauthorized)
			writeTestJSON(t, w, map[string]string{"error": "login required"})
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/spaces/space-1/tables/table-1":
			writeTestJSON(t, w, map[string]any{"type": "table", "table": snapshot})
		case r.Method == http.MethodPost && r.URL.Path == "/api/spaces/space-1/tables/table-1/action":
			mu.Lock()
			defer mu.Unlock()
			if err := json.NewDecoder(r.Body).Decode(&receivedAction); err != nil {
				t.Errorf("decode action: %v", err)
			}
			snapshot.Allowed = poker.AllowedActions{}
			writeTestJSON(t, w, map[string]any{"type": "table", "table": snapshot})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	apiClient, err := NewAPIClient(Config{BaseURL: api.URL, Username: "agent", Password: "secret"}, api.Client())
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := New(apiClient)
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 8 {
		t.Fatalf("expected 8 PokerNode tools, got %d", len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		if tool.OutputSchema == nil {
			t.Fatalf("tool %s is missing a structured output schema", tool.Name)
		}
	}

	state, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pokernode_get_table", Arguments: map[string]any{"space_id": "space-1", "table_id": "table-1"}})
	if err != nil {
		t.Fatalf("get table: %v", err)
	}
	if state.IsError {
		t.Fatalf("get table returned tool error: %v", state.GetError())
	}
	encoded, err := json.Marshal(state.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output TableOutput
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	got := output.Table.Players[0].Cards[0].Code
	if !output.Table.AllowedActions.CanAct || got != "Kh" {
		t.Fatalf("unexpected player view: %#v", output.Table)
	}

	action, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pokernode_act", Arguments: map[string]any{
		"space_id": "space-1", "table_id": "table-1", "action": "raise", "amount_cents": 600,
	}})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if action.IsError {
		t.Fatalf("act returned tool error: %v", action.GetError())
	}
	mu.Lock()
	defer mu.Unlock()
	if receivedAction.Action != poker.ActionRaise || receivedAction.AmountCents != 600 {
		t.Fatalf("unexpected action request: %#v", receivedAction)
	}
}

func TestConfigRejectsRemotePlainHTTP(t *testing.T) {
	t.Parallel()
	err := (Config{BaseURL: "http://poker.example", Username: "agent", Password: "secret"}).Validate()
	if err == nil {
		t.Fatal("expected remote plain HTTP to be rejected")
	}
	if err := (Config{BaseURL: "http://poker.example", Username: "agent", Password: "secret", AllowInsecureHTTP: true}).Validate(); err != nil {
		t.Fatalf("explicit private-network opt-in should be accepted: %v", err)
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode test response: %v", err)
	}
}
