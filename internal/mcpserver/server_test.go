package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"pokernode/internal/auth"
	"pokernode/internal/poker"
)

func TestMCPReadStateThenAct(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	actionCalls := 0
	var receivedAction struct {
		Action         poker.ActionType `json:"action"`
		AmountCents    int64            `json:"amount_cents"`
		ExpectedTurnID uint64           `json:"expected_turn_id"`
	}
	snapshot := poker.Snapshot{
		ID: "table-1", Name: "Agent table", Street: poker.StreetFlop, HandID: 7,
		SmallBlind: 50, BigBlind: 100,
		Board:      []poker.Card{{Rank: poker.Ace, Suit: poker.Spades}, {Rank: poker.Ten, Suit: poker.Diamonds}},
		ViewerSeat: 1, ActingSeat: 1, Pot: 450, CurrentBet: 200,
		Players: []poker.PlayerView{{UserID: 11, Name: "agent", Seat: 1, Stack: 4_800, Bet: 200, Cards: []poker.Card{{Rank: poker.King, Suit: poker.Hearts}, {Rank: poker.Queen, Suit: poker.Hearts}}, InHand: true, IsActing: true, LastAction: poker.ActionCall, LastActionAmount: 200}},
		Allowed: poker.AllowedActions{CanAct: true, CanFold: true, CanCall: true, CanRaise: true, ToCall: 200, MinRaiseTo: 400, MaxRaiseTo: 5_000},
		TurnID:  42,
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/me/current-game":
			writeTestJSON(t, w, map[string]any{"active": true, "agent_control_enabled": true, "space_id": "space-1", "table_id": "table-1", "table": snapshot})
		case r.Method == http.MethodGet && r.URL.Path == "/api/spaces/space-1/tables/table-1":
			writeTestJSON(t, w, map[string]any{"type": "table", "table": snapshot})
		case r.Method == http.MethodPost && r.URL.Path == "/api/spaces/space-1/tables/table-1/action":
			mu.Lock()
			defer mu.Unlock()
			actionCalls++
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
	toolsJSON, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}
	if len(toolsJSON) >= 15_000 {
		t.Fatalf("tool catalog is too large: %d bytes", len(toolsJSON))
	}
	t.Logf("compact MCP tool catalog: %d bytes", len(toolsJSON))
	var actTool *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.OutputSchema == nil {
			t.Fatalf("tool %s is missing a structured output schema", tool.Name)
		}
		if tool.Name == "pokernode_act" {
			actTool = tool
		}
	}
	if actTool == nil || !strings.Contains(actTool.Description, "10000 = 100.00 USD") {
		t.Fatalf("act tool does not explain the cents-to-USD scale: %#v", actTool)
	}
	actSchema, err := json.Marshal(actTool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(actSchema), "600 means 6.00 USD") {
		t.Fatalf("act input schema does not explain amount_cents: %s", actSchema)
	}
	if strings.Contains(string(actSchema), "space_id") || strings.Contains(string(actSchema), "table_id") {
		t.Fatalf("active-table action schema repeats redundant table IDs: %s", actSchema)
	}
	currentGame, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pokernode_get_current_game", Arguments: map[string]any{}})
	if err != nil || currentGame.IsError {
		t.Fatalf("get current game: result=%#v err=%v", currentGame, err)
	}
	encoded, err := json.Marshal(currentGame.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var current CurrentGameOutput
	if err := json.Unmarshal(encoded, &current); err != nil {
		t.Fatal(err)
	}
	if !current.Active || !current.AgentControlEnabled || current.SpaceID != "space-1" || current.TableID != "table-1" {
		t.Fatalf("unexpected current game: %#v", current)
	}
	assertCompactText(t, currentGame, encoded)

	waited, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pokernode_wait_for_turn", Arguments: map[string]any{
		"max_wait_seconds": 1,
	}})
	if err != nil || waited.IsError {
		t.Fatalf("wait for turn: result=%#v err=%v", waited, err)
	}
	waitJSON, err := json.Marshal(waited.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var waitOutput WaitOutput
	if err := json.Unmarshal(waitJSON, &waitOutput); err != nil {
		t.Fatal(err)
	}
	if waitOutput.Code != "your_turn" || waitOutput.State == nil || waitOutput.State.Status != "your_turn" || waitOutput.State.TurnID != 42 || len(waitOutput.State.LegalActions) != 3 {
		t.Fatalf("unexpected compact decision state: %#v", waitOutput)
	}
	if waitOutput.State.Money != (MoneySpec{Currency: "USD", Unit: "cent", Scale: 100}) || waitOutput.State.SmallBlindCents != 50 || waitOutput.State.BigBlindCents != 100 || waitOutput.State.PotCents != 450 || waitOutput.State.Players[0].Cards[0] != "Kh" {
		t.Fatalf("decision state lost authoritative cards or money scale: %#v", waitOutput.State)
	}
	assertCompactText(t, waited, waitJSON)
	waitWire, err := json.Marshal(waited)
	if err != nil {
		t.Fatal(err)
	}
	if len(waitWire) >= 1_600 {
		t.Fatalf("decision result exceeded token budget: %d bytes", len(waitWire))
	}
	t.Logf("compact decision result: %d bytes", len(waitWire))
	stale, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pokernode_act", Arguments: map[string]any{
		"action": "raise", "amount_cents": 600, "expected_turn_id": 41,
	}})
	if err != nil || !stale.IsError {
		t.Fatalf("stale MCP action did not return a tool error: result=%#v err=%v", stale, err)
	}
	staleJSON, err := json.Marshal(stale.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var staleOutput MutationOutput
	if err := json.Unmarshal(staleJSON, &staleOutput); err != nil {
		t.Fatal(err)
	}
	if staleOutput.Code != "stale_turn" || staleOutput.OK || !staleOutput.Retryable || staleOutput.CurrentTurnID != 42 || staleOutput.NextTool != "pokernode_wait_for_turn" {
		t.Fatalf("stale action is not recoverable from structured output: %#v", staleOutput)
	}

	action, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pokernode_act", Arguments: map[string]any{
		"action": "raise", "amount_cents": 600, "expected_turn_id": 42,
	}})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if action.IsError {
		t.Fatalf("act returned tool error: %v", action.GetError())
	}
	actionJSON, err := json.Marshal(action.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var actionOutput MutationOutput
	if err := json.Unmarshal(actionJSON, &actionOutput); err != nil {
		t.Fatal(err)
	}
	if !actionOutput.OK || actionOutput.Code != "action_applied" || actionOutput.NextTool != "pokernode_wait_for_turn" {
		t.Fatalf("unexpected compact action receipt: %#v", actionOutput)
	}
	assertCompactText(t, action, actionJSON)
	actionWire, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	if len(actionWire) >= 500 {
		t.Fatalf("action receipt exceeded token budget: %d bytes", len(actionWire))
	}
	t.Logf("compact action receipt: %d bytes", len(actionWire))
	mu.Lock()
	defer mu.Unlock()
	if actionCalls != 1 || receivedAction.Action != poker.ActionRaise || receivedAction.AmountCents != 600 || receivedAction.ExpectedTurnID != 42 {
		t.Fatalf("unexpected action request: %#v", receivedAction)
	}
}

func TestMoneyMetadataDefinesCentsScaleOnce(t *testing.T) {
	t.Parallel()
	if got := usdMoney(); got != (MoneySpec{Currency: "USD", Unit: "cent", Scale: 100}) {
		t.Fatalf("unexpected money metadata: %#v", got)
	}
}

func TestWaitForTurnReturnsKickVoteAndAvoidsReadyBusyLoop(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	requests := 0
	snapshot := poker.Snapshot{
		ID: "table-1", Name: "Agent table", Street: poker.StreetWaiting, ViewerSeat: 1, ActingSeat: -1,
		Players:  []poker.PlayerView{{UserID: 11, Name: "agent", Seat: 1, Stack: 10_000}},
		CanStart: true, CanLeave: true,
	}
	vote := &wireKickVote{
		TargetUserID: 11, TargetName: "agent", InitiatorName: "other", YesCount: 1, RequiredYes: 2,
		ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/me/current-game" {
			mu.Lock()
			responseSnapshot := snapshot
			mu.Unlock()
			writeTestJSON(t, w, map[string]any{"active": true, "agent_control_enabled": true, "space_id": "space-1", "table_id": "table-1", "table": responseSnapshot})
			return
		}
		if r.URL.Path == "/api/spaces/space-1/tables/table-1" {
			mu.Lock()
			requests++
			if vote == nil && snapshot.Players[0].Ready && requests >= 2 {
				snapshot.Street = poker.StreetPreflop
				snapshot.TurnID = 9
				snapshot.ActingSeat = 1
				snapshot.Allowed = poker.AllowedActions{CanAct: true, CanCheck: true}
			}
			responseSnapshot := snapshot
			responseVote := vote
			mu.Unlock()
			writeTestJSON(t, w, map[string]any{"type": "table", "table": responseSnapshot, "kick_vote": responseVote})
			return
		}
		http.NotFound(w, r)
	}))
	defer api.Close()
	client, err := NewAPIClient(Config{BaseURL: api.URL, SessionToken: "test-session"}, api.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, output, err := (&Server{api: client}).waitForTurn(context.Background(), nil, WaitForTurnInput{MaxWaitSeconds: 1})
	if err != nil || result.IsError || output.Code != "kick_vote" || output.State == nil || output.State.KickVote == nil {
		t.Fatalf("kick vote was not surfaced: result=%#v output=%#v err=%v", result, output, err)
	}
	if !output.State.KickVote.TargetIsViewer || !output.State.KickVote.CanCancelByReady || output.NextTool != "pokernode_ready" {
		t.Fatalf("kick vote does not guide the Agent to ready: %#v", output.State.KickVote)
	}

	mu.Lock()
	requests = 0
	mu.Unlock()
	snapshot.Players[0].Ready = true
	vote = nil
	result, output, err = (&Server{api: client}).waitForTurn(context.Background(), nil, WaitForTurnInput{MaxWaitSeconds: 1})
	if err != nil || result.IsError || output.Code != "your_turn" {
		t.Fatalf("ready player did not wait for the next relevant state: result=%#v output=%#v err=%v", result, output, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests < 2 {
		t.Fatalf("ready player returned immediately and could busy-loop; requests=%d", requests)
	}
}

func TestWaitForTurnNotSeatedIsStructuredAndThrottled(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/me/current-game" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(t, w, map[string]any{"active": false, "agent_control_enabled": true})
	}))
	defer api.Close()
	client, err := NewAPIClient(Config{BaseURL: api.URL, SessionToken: "test-session"}, api.Client())
	if err != nil {
		t.Fatal(err)
	}
	waits := newWaitRegistry()
	server := &Server{api: client, waits: waits, identity: "11"}
	result, output, err := server.waitForTurn(context.Background(), nil, WaitForTurnInput{MaxWaitSeconds: 1})
	if err != nil || !result.IsError || output.Code != "not_seated" || output.RetryAfterMS != 5000 || output.NextTool != "pokernode_get_current_game" {
		t.Fatalf("not-seated outcome is not safely recoverable: result=%#v output=%#v err=%v", result, output, err)
	}
	secondResult, second, err := server.waitForTurn(context.Background(), nil, WaitForTurnInput{MaxWaitSeconds: 1})
	if err != nil || !secondResult.IsError || second.Code != "wait_in_progress" || second.RetryAfterMS <= 0 {
		t.Fatalf("wait cooldown did not prevent a tight retry loop: result=%#v output=%#v err=%v", secondResult, second, err)
	}
}

func assertCompactText(t *testing.T, result *mcp.CallToolResult, structured []byte) {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("expected one compact text block, got %#v", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || len(text.Text) == 0 || len(text.Text) > 200 {
		t.Fatalf("unexpected compact text fallback: %#v", result.Content[0])
	}
	if text.Text == string(structured) {
		t.Fatal("structured JSON was duplicated verbatim in text content")
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
