package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"pokernode/internal/landlord"
	"pokernode/internal/poker"
)

const (
	serverVersion = "0.3.0"
	currencyUSD   = "USD"
	moneyToolHint = " All *_cents values are integer USD cents; divide by 100 for dollars (10000 = 100.00 USD)."
)

type Server struct {
	api      *APIClient
	waits    *waitRegistry
	identity string
}

type MoneySpec struct {
	Currency string `json:"currency" jsonschema:"ISO 4217 currency code."`
	Unit     string `json:"unit" jsonschema:"Unit used by every field ending in _cents."`
	Scale    int    `json:"scale" jsonschema:"Number of cents in one USD; divide *_cents by this value for dollars."`
}

type KickVoteView struct {
	TargetName       string `json:"target_name"`
	InitiatorName    string `json:"initiator_name"`
	YesCount         int    `json:"yes_count"`
	RequiredYes      int    `json:"required_yes"`
	ExpiresAt        int64  `json:"expires_at" jsonschema:"Unix time in milliseconds when the vote expires."`
	TargetIsViewer   bool   `json:"target_is_viewer"`
	CanCancelByReady bool   `json:"can_cancel_by_ready" jsonschema:"True when this player can cancel being removed by calling pokernode_ready before expiry."`
}

type ListChannelsInput struct{}

type ListChannelsOutput struct {
	Channels []channelView `json:"channels" jsonschema:"Channels the configured account has joined."`
}

type ListTablesInput struct {
	SpaceID string `json:"space_id" jsonschema:"Channel ID returned by pokernode_list_channels."`
}

type ListTablesOutput struct {
	Money  MoneySpec      `json:"money"`
	Tables []tableSummary `json:"tables" jsonschema:"Poker tables in the channel. Player rosters are intentionally omitted."`
}

type CurrentGameInput struct{}

type JoinTableInput struct {
	SpaceID    string `json:"space_id" jsonschema:"Channel ID returned by pokernode_list_channels."`
	TableID    string `json:"table_id" jsonschema:"Table ID returned by pokernode_list_tables."`
	BuyInCents int64  `json:"buy_in_cents" jsonschema:"Buy-in in integer USD cents, NOT dollars. From 2000 through 100000; 2000 means 20.00 USD and 10000 means 100.00 USD."`
}

type ActInput struct {
	Action         string   `json:"action" jsonschema:"For Texas Hold'em: fold, check, call, bet, raise, or all_in. For landlord: bid, play, or pass. Use only an allowed action."`
	AmountCents    int64    `json:"amount_cents,omitempty" jsonschema:"Texas Hold'em only. Integer USD cents, NOT dollars. For bet or raise, copy a total target from min_raise_to_cents through max_raise_to_cents; 600 means 6.00 USD, not 600 USD."`
	Bid            int      `json:"bid,omitempty" jsonschema:"Landlord bid from 0 through 3. Zero means no bid."`
	Cards          []string `json:"cards,omitempty" jsonschema:"Landlord cards to play using compact codes such as 3c, Td, 2s, SJ, or BJ."`
	ExpectedTurnID uint64   `json:"expected_turn_id" jsonschema:"Required turn_id from the latest table state. The action is rejected if the turn has changed."`
}

type WaitForTurnInput struct {
	MaxWaitSeconds int `json:"max_wait_seconds,omitempty" jsonschema:"How long to wait, from 1 through 60 seconds. Defaults to 25."`
}

type CurrentGameOutput struct {
	Active              bool   `json:"active" jsonschema:"Whether this player is currently seated at a table."`
	AgentControlEnabled bool   `json:"agent_control_enabled" jsonschema:"Whether the player explicitly handed gameplay control to this Agent."`
	SpaceID             string `json:"space_id,omitempty"`
	TableID             string `json:"table_id,omitempty"`
}

type CompactPlayerView struct {
	Name             string   `json:"name"`
	Seat             int      `json:"seat"`
	StackCents       int64    `json:"stack_cents"`
	BetCents         int64    `json:"bet_cents,omitempty"`
	Cards            []string `json:"cards,omitempty" jsonschema:"Compact card codes."`
	State            string   `json:"state" jsonschema:"One of waiting, ready, active, folded, or all_in."`
	LastAction       string   `json:"last_action,omitempty"`
	LastActionAmount int64    `json:"last_action_amount_cents,omitempty"`
	CardCount        int      `json:"card_count,omitempty"`
	Landlord         bool     `json:"landlord,omitempty"`
	Bid              int      `json:"bid,omitempty"`
}

type DecisionView struct {
	Money            MoneySpec           `json:"money"`
	GameType         string              `json:"game_type"`
	TableID          string              `json:"table_id"`
	Status           string              `json:"status" jsonschema:"One of not_seated, your_turn, waiting, ready_required, or between_hands."`
	HandID           int64               `json:"hand_id"`
	Street           string              `json:"street"`
	SmallBlindCents  int64               `json:"small_blind_cents,omitempty"`
	BigBlindCents    int64               `json:"big_blind_cents,omitempty"`
	BaseStakeCents   int64               `json:"base_stake_cents,omitempty"`
	Board            []string            `json:"board,omitempty"`
	Bottom           []string            `json:"bottom,omitempty"`
	LastPlay         []string            `json:"last_play,omitempty"`
	LastCombination  string              `json:"last_combination,omitempty"`
	LastPlaySeat     *int                `json:"last_play_seat,omitempty"`
	TrickOpen        bool                `json:"trick_open,omitempty"`
	HighestBid       int                 `json:"highest_bid,omitempty"`
	LandlordSeat     *int                `json:"landlord_seat,omitempty"`
	Multiplier       int                 `json:"multiplier,omitempty"`
	PotCents         int64               `json:"pot_cents,omitempty"`
	CurrentBetCents  int64               `json:"current_bet_cents,omitempty"`
	DealerSeat       *int                `json:"dealer_seat,omitempty"`
	SmallBlindSeat   *int                `json:"small_blind_seat,omitempty"`
	BigBlindSeat     *int                `json:"big_blind_seat,omitempty"`
	ActingSeat       int                 `json:"acting_seat"`
	ViewerSeat       int                 `json:"viewer_seat"`
	ActionDeadlineAt int64               `json:"action_deadline_at,omitempty"`
	TurnID           uint64              `json:"turn_id"`
	Players          []CompactPlayerView `json:"players"`
	LegalActions     []string            `json:"legal_actions,omitempty"`
	ToCallCents      int64               `json:"to_call_cents,omitempty"`
	MinRaiseToCents  int64               `json:"min_raise_to_cents,omitempty"`
	MaxRaiseToCents  int64               `json:"max_raise_to_cents,omitempty"`
	MinBid           int                 `json:"min_bid,omitempty"`
	CanReady         bool                `json:"can_ready,omitempty"`
	CanLeave         bool                `json:"can_leave,omitempty"`
	KickVote         *KickVoteView       `json:"kick_vote,omitempty"`
}

type WaitOutput struct {
	Code         string        `json:"code" jsonschema:"your_turn, ready_required, kick_vote, timeout, not_seated, wait_in_progress, or waiting."`
	State        *DecisionView `json:"state,omitempty"`
	TimedOut     bool          `json:"timed_out,omitempty"`
	RetryAfterMS int           `json:"retry_after_ms,omitempty"`
	NextTool     string        `json:"next_tool,omitempty"`
}

type MutationOutput struct {
	OK            bool   `json:"ok"`
	Code          string `json:"code" jsonschema:"Stable machine-readable outcome code."`
	HandID        int64  `json:"hand_id,omitempty"`
	TurnID        uint64 `json:"turn_id,omitempty"`
	Notice        string `json:"notice,omitempty"`
	OperationID   string `json:"operation_id,omitempty"`
	SettledCents  int64  `json:"settled_cents,omitempty" jsonschema:"Integer USD cents; divide by 100 for dollars."`
	Retryable     bool   `json:"retryable,omitempty"`
	CurrentTurnID uint64 `json:"current_turn_id,omitempty"`
	NextTool      string `json:"next_tool,omitempty"`
}

func New(api *APIClient) *mcp.Server {
	return newServer(api, nil)
}

func newServer(api *APIClient, schemaCache *mcp.SchemaCache) *mcp.Server {
	return newServerForIdentity(api, schemaCache, nil, "")
}

func newServerForIdentity(api *APIClient, schemaCache *mcp.SchemaCache, waits *waitRegistry, identity string) *mcp.Server {
	service := &Server{api: api, waits: waits, identity: identity}
	var options *mcp.ServerOptions
	if schemaCache != nil {
		options = &mcp.ServerOptions{SchemaCache: schemaCache}
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "pokernode-mcp", Version: serverVersion}, options)

	mcp.AddTool(server, readOnlyTool("pokernode_list_channels", "List channels available to the configured PokerNode player account."), service.listChannels)
	mcp.AddTool(server, readOnlyTool("pokernode_get_current_game", "Return this player's active table location and hosted-control status. Call pokernode_wait_for_turn for decision state."), service.getCurrentGame)
	mcp.AddTool(server, readOnlyTool("pokernode_list_tables", "List compact table summaries in a channel. Player rosters are omitted."+moneyToolHint), service.listTables)
	mcp.AddTool(server, readOnlyTool("pokernode_wait_for_turn", "Wait on this player's active table for a decision, readiness, removal vote, seat removal, or timeout; returns compact authoritative state."+moneyToolHint), service.waitForTurn)
	mcp.AddTool(server, writeTool("pokernode_join_table", "Buy in and join a table. Returns a small receipt; call pokernode_wait_for_turn next."+moneyToolHint, true), service.joinTable)
	mcp.AddTool(server, writeTool("pokernode_ready", "Mark this player ready at the active table. Returns a small receipt; call pokernode_wait_for_turn next."+moneyToolHint, false), service.ready)
	mcp.AddTool(server, writeTool("pokernode_act", "Take one legal action at the active table using the latest nonzero expected_turn_id. Returns a small receipt; call pokernode_wait_for_turn next."+moneyToolHint, true), service.act)
	mcp.AddTool(server, writeTool("pokernode_leave_table", "Leave the active table between hands and settle the remaining stack back to quota."+moneyToolHint, true), service.leaveTable)
	return server
}

func readOnlyTool(name, description string) *mcp.Tool {
	return &mcp.Tool{
		Name: name, Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(true)},
	}
}

func writeTool(name, description string, destructive bool) *mcp.Tool {
	return &mcp.Tool{
		Name: name, Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: boolPointer(destructive), OpenWorldHint: boolPointer(true)},
	}
}

func boolPointer(value bool) *bool { return &value }

func intPointer(value int) *int { return &value }

func (s *Server) listChannels(ctx context.Context, _ *mcp.CallToolRequest, _ ListChannelsInput) (*mcp.CallToolResult, ListChannelsOutput, error) {
	channels, err := s.api.listChannels(ctx)
	return textResult(fmt.Sprintf("%d channel(s)", len(channels))), ListChannelsOutput{Channels: channels}, err
}

func (s *Server) getCurrentGame(ctx context.Context, _ *mcp.CallToolRequest, _ CurrentGameInput) (*mcp.CallToolResult, CurrentGameOutput, error) {
	current, err := s.api.getCurrentGame(ctx)
	if err != nil {
		return nil, CurrentGameOutput{}, err
	}
	output := CurrentGameOutput{Active: current.Active, AgentControlEnabled: current.AgentControlEnabled, SpaceID: current.SpaceID, TableID: current.TableID}
	return textResult(currentGameSummary(output)), output, nil
}

func (s *Server) activeGame(ctx context.Context) (currentGame, error) {
	current, err := s.api.getCurrentGame(ctx)
	if err != nil {
		return currentGame{}, err
	}
	if !current.Active {
		return currentGame{}, errors.New("the configured player is not seated; call pokernode_get_current_game, then join a table if needed")
	}
	return current, nil
}

func (s *Server) listTables(ctx context.Context, _ *mcp.CallToolRequest, input ListTablesInput) (*mcp.CallToolResult, ListTablesOutput, error) {
	if err := requireIDs(input.SpaceID); err != nil {
		return nil, ListTablesOutput{}, err
	}
	tables, err := s.api.listTables(ctx, input.SpaceID)
	return textResult(fmt.Sprintf("%d compact table summary(s)", len(tables))), ListTablesOutput{Money: usdMoney(), Tables: tables}, err
}

func (s *Server) waitForTurn(ctx context.Context, _ *mcp.CallToolRequest, input WaitForTurnInput) (*mcp.CallToolResult, WaitOutput, error) {
	wait := input.MaxWaitSeconds
	if wait == 0 {
		wait = 25
	}
	if wait < 1 || wait > 60 {
		return nil, WaitOutput{}, errors.New("max_wait_seconds must be between 1 and 60")
	}
	cooldown := time.Duration(0)
	if s.waits != nil && s.identity != "" {
		release, retryAfter, ok := s.waits.acquire(s.identity)
		if !ok {
			output := WaitOutput{Code: "wait_in_progress", RetryAfterMS: retryAfter, NextTool: "pokernode_wait_for_turn"}
			return errorResult("Another wait is active or cooling down; retry after the returned delay."), output, nil
		}
		defer func() { release(cooldown) }()
	}
	current, err := s.api.getCurrentGame(ctx)
	if err != nil {
		return nil, WaitOutput{}, err
	}
	if !current.Active {
		cooldown = 5 * time.Second
		state := decisionView(gameSnapshot{}, nil)
		output := WaitOutput{Code: "not_seated", State: &state, RetryAfterMS: 5000, NextTool: "pokernode_get_current_game"}
		return errorResult("Player is not seated. Stop waiting and call pokernode_get_current_game."), output, nil
	}
	envelope, err := s.api.getTable(ctx, current.SpaceID, current.TableID)
	if err != nil {
		return nil, WaitOutput{}, err
	}
	timer := time.NewTimer(time.Duration(wait) * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		state := decisionView(envelope.Table, envelope.KickVote)
		if snapshotViewerSeat(envelope.Table) < 0 {
			cooldown = 5 * time.Second
			output := WaitOutput{Code: "not_seated", State: &state, RetryAfterMS: 5000, NextTool: "pokernode_get_current_game"}
			return errorResult("Player is no longer seated. Stop waiting and call pokernode_get_current_game."), output, nil
		}
		if state.KickVote != nil && state.KickVote.TargetIsViewer {
			output := WaitOutput{Code: "kick_vote", State: &state, NextTool: "pokernode_ready"}
			return textResult("Removal vote targets this player; call pokernode_ready before it expires."), output, nil
		}
		if snapshotCanAct(envelope.Table) {
			return textResult(decisionSummary(state)), WaitOutput{Code: "your_turn", State: &state, NextTool: "pokernode_act"}, nil
		}
		if !snapshotHandActive(envelope.Table) && snapshotCanReady(envelope.Table) && !snapshotViewerReady(envelope.Table) {
			return textResult("Player must ready for the next hand."), WaitOutput{Code: "ready_required", State: &state, NextTool: "pokernode_ready"}, nil
		}
		select {
		case <-ctx.Done():
			return nil, WaitOutput{}, ctx.Err()
		case <-timer.C:
			cooldown = time.Second
			output := WaitOutput{Code: "timeout", State: &state, TimedOut: true, RetryAfterMS: 1000, NextTool: "pokernode_wait_for_turn"}
			return textResult("No relevant state change before timeout; retry after 1000 ms."), output, nil
		case <-ticker.C:
			envelope, err = s.api.getTable(ctx, current.SpaceID, current.TableID)
			if err != nil {
				return nil, WaitOutput{}, err
			}
		}
	}
}

func (s *Server) joinTable(ctx context.Context, _ *mcp.CallToolRequest, input JoinTableInput) (*mcp.CallToolResult, MutationOutput, error) {
	if err := requireIDs(input.SpaceID, input.TableID); err != nil {
		return nil, MutationOutput{}, err
	}
	if input.BuyInCents < 2_000 || input.BuyInCents > 100_000 {
		return nil, MutationOutput{}, errors.New("buy_in_cents must be between 2000 (20.00 USD) and 100000 (1000.00 USD)")
	}
	table, operationID, err := s.api.joinTable(ctx, input.SpaceID, input.TableID, input.BuyInCents)
	output := mutationOutput("joined", table, "")
	output.OperationID = operationID
	return textResult("Joined table; call pokernode_wait_for_turn next."), output, err
}

func (s *Server) ready(ctx context.Context, _ *mcp.CallToolRequest, _ CurrentGameInput) (*mcp.CallToolResult, MutationOutput, error) {
	current, err := s.activeGame(ctx)
	if err != nil {
		return nil, MutationOutput{}, err
	}
	envelope, err := s.api.ready(ctx, current.SpaceID, current.TableID)
	output := mutationOutput("ready", envelope.Table, envelope.Notice)
	return textResult("Ready accepted; call pokernode_wait_for_turn next."), output, err
}

func (s *Server) act(ctx context.Context, _ *mcp.CallToolRequest, input ActInput) (*mcp.CallToolResult, MutationOutput, error) {
	current, err := s.activeGame(ctx)
	if err != nil {
		return nil, MutationOutput{}, err
	}
	if input.ExpectedTurnID == 0 {
		return nil, MutationOutput{}, errors.New("expected_turn_id is required; call pokernode_wait_for_turn and use its current turn_id")
	}
	if input.ExpectedTurnID != snapshotTurnID(current.Table) {
		output := mutationOutput("stale_turn", current.Table, "")
		output.OK = false
		output.Retryable = true
		output.CurrentTurnID = snapshotTurnID(current.Table)
		output.NextTool = "pokernode_wait_for_turn"
		return errorResult("Turn changed; call pokernode_wait_for_turn and decide from the new state."), output, nil
	}
	request := gameActionRequest{Action: input.Action, Amount: input.AmountCents, Bid: input.Bid, ExpectedTurnID: input.ExpectedTurnID}
	if current.Table.GameType == landlord.GameType {
		if input.Action != string(landlord.ActionBid) && input.Action != string(landlord.ActionPlay) && input.Action != string(landlord.ActionPass) {
			return nil, MutationOutput{}, errors.New("landlord action must be bid, play, or pass")
		}
		if input.Action == string(landlord.ActionBid) && (input.Bid < 0 || input.Bid > 3) {
			return nil, MutationOutput{}, errors.New("bid must be from 0 through 3")
		}
		if input.Action == string(landlord.ActionPlay) {
			if len(input.Cards) == 0 {
				return nil, MutationOutput{}, errors.New("cards is required for a landlord play action")
			}
			request.Cards = make([]landlord.Card, 0, len(input.Cards))
			for _, code := range input.Cards {
				card, parseErr := parseLandlordCard(code)
				if parseErr != nil {
					return nil, MutationOutput{}, parseErr
				}
				request.Cards = append(request.Cards, card)
			}
		}
	} else {
		action := poker.ActionType(input.Action)
		if !validAction(action) {
			return nil, MutationOutput{}, errors.New("Texas Hold'em action must be fold, check, call, bet, raise, or all_in")
		}
		if (action == poker.ActionBet || action == poker.ActionRaise) && input.AmountCents <= 0 {
			return nil, MutationOutput{}, errors.New("amount_cents must be a positive total target for bet or raise")
		}
	}
	envelope, err := s.api.act(ctx, current.SpaceID, current.TableID, request)
	output := mutationOutput("action_applied", envelope.Table, envelope.Notice)
	return textResult("Action applied; call pokernode_wait_for_turn next."), output, err
}

func (s *Server) leaveTable(ctx context.Context, _ *mcp.CallToolRequest, _ CurrentGameInput) (*mcp.CallToolResult, MutationOutput, error) {
	current, err := s.activeGame(ctx)
	if err != nil {
		return nil, MutationOutput{}, err
	}
	table, operationID, settled, err := s.api.leaveTable(ctx, current.SpaceID, current.TableID)
	output := mutationOutput("left", table, "")
	output.OperationID = operationID
	output.SettledCents = settled
	return textResult("Left table and settled the remaining stack in USD cents."), output, err
}

func requireIDs(ids ...string) error {
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return errors.New("required IDs must not be empty")
		}
	}
	return nil
}

func validAction(action poker.ActionType) bool {
	switch action {
	case poker.ActionFold, poker.ActionCheck, poker.ActionCall, poker.ActionBet, poker.ActionRaise, poker.ActionAllIn:
		return true
	default:
		return false
	}
}

func usdMoney() MoneySpec {
	return MoneySpec{Currency: currencyUSD, Unit: "cent", Scale: 100}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func errorResult(text string) *mcp.CallToolResult {
	result := textResult(text)
	result.IsError = true
	return result
}

func currentGameSummary(output CurrentGameOutput) string {
	if !output.Active {
		return fmt.Sprintf("No active table; agent_control_enabled=%t.", output.AgentControlEnabled)
	}
	return fmt.Sprintf("Active table %s; call pokernode_wait_for_turn for compact decision state.", output.TableID)
}

func decisionSummary(state DecisionView) string {
	return fmt.Sprintf("status=%s hand_id=%d turn_id=%d; structured state contains the authoritative details.", state.Status, state.HandID, state.TurnID)
}

func mutationOutput(code string, snapshot gameSnapshot, notice string) MutationOutput {
	return MutationOutput{
		OK: true, Code: code, HandID: snapshotHandID(snapshot), TurnID: snapshotTurnID(snapshot),
		Notice: notice, NextTool: "pokernode_wait_for_turn",
	}
}

func decisionView(snapshot gameSnapshot, vote *wireKickVote) DecisionView {
	if snapshot.Landlord != nil {
		return landlordDecisionView(*snapshot.Landlord, enrichKickVote(vote, snapshot))
	}
	if snapshot.Poker != nil {
		return pokerDecisionView(*snapshot.Poker, enrichKickVote(vote, snapshot))
	}
	return DecisionView{Money: usdMoney(), Status: "not_seated", ViewerSeat: -1}
}

func pokerDecisionView(snapshot poker.Snapshot, vote *KickVoteView) DecisionView {
	players := make([]CompactPlayerView, 0, len(snapshot.Players))
	for _, player := range snapshot.Players {
		players = append(players, CompactPlayerView{
			Name: player.Name, Seat: player.Seat, StackCents: player.Stack, BetCents: player.Bet,
			Cards: pokerCardCodes(player.Cards), State: pokerPlayerState(player), LastAction: string(player.LastAction),
			LastActionAmount: player.LastActionAmount,
		})
	}
	state := DecisionView{
		Money: usdMoney(), GameType: poker.GameType, TableID: snapshot.ID, HandID: snapshot.HandID,
		Street: string(snapshot.Street), SmallBlindCents: snapshot.SmallBlind, BigBlindCents: snapshot.BigBlind,
		Board: pokerCardCodes(snapshot.Board), PotCents: snapshot.Pot,
		CurrentBetCents: snapshot.CurrentBet, DealerSeat: intPointer(snapshot.DealerSeat), SmallBlindSeat: intPointer(snapshot.SmallBlindSeat),
		BigBlindSeat: intPointer(snapshot.BigBlindSeat), ActingSeat: snapshot.ActingSeat, ViewerSeat: snapshot.ViewerSeat,
		ActionDeadlineAt: snapshot.ActionDeadlineAt, TurnID: snapshot.TurnID, Players: players,
		LegalActions: pokerLegalActions(snapshot.Allowed), ToCallCents: snapshot.Allowed.ToCall,
		MinRaiseToCents: snapshot.Allowed.MinRaiseTo, MaxRaiseToCents: snapshot.Allowed.MaxRaiseTo,
		CanReady: snapshot.CanStart, CanLeave: snapshot.CanLeave, KickVote: vote,
	}
	state.Status = decisionStatus(gameSnapshot{GameType: poker.GameType, Poker: &snapshot})
	return state
}

func landlordDecisionView(snapshot landlord.Snapshot, vote *KickVoteView) DecisionView {
	players := make([]CompactPlayerView, 0, len(snapshot.Players))
	active := snapshot.Phase == landlord.PhaseBidding || snapshot.Phase == landlord.PhasePlaying
	for _, player := range snapshot.Players {
		players = append(players, CompactPlayerView{
			Name: player.Name, Seat: player.Seat, StackCents: player.Stack, Cards: landlordCardCodes(player.Cards),
			State: landlordPlayerState(player, active), CardCount: player.CardCount, Landlord: player.Landlord, Bid: player.Bid,
		})
	}
	state := DecisionView{
		Money: usdMoney(), GameType: landlord.GameType, TableID: snapshot.ID, HandID: snapshot.HandID,
		Street: string(snapshot.Phase), BaseStakeCents: snapshot.BaseStake,
		Bottom: landlordCardCodes(snapshot.Bottom), LastPlay: landlordCardCodes(snapshot.LastPlay),
		LastCombination: string(snapshot.LastCombination.Kind), LastPlaySeat: intPointer(snapshot.LastPlaySeat), TrickOpen: snapshot.TrickOpen,
		HighestBid: snapshot.HighestBid, LandlordSeat: &snapshot.LandlordSeat, Multiplier: snapshot.Multiplier,
		ActingSeat: snapshot.ActingSeat, ViewerSeat: snapshot.ViewerSeat, ActionDeadlineAt: snapshot.ActionDeadlineAt,
		TurnID: snapshot.TurnID, Players: players, LegalActions: landlordLegalActions(snapshot.Allowed), MinBid: snapshot.Allowed.MinBid,
		CanReady: snapshot.CanStart, CanLeave: snapshot.CanLeave, KickVote: vote,
	}
	state.Status = decisionStatus(gameSnapshot{GameType: landlord.GameType, Landlord: &snapshot})
	return state
}

func decisionStatus(snapshot gameSnapshot) string {
	if snapshotViewerSeat(snapshot) < 0 {
		return "not_seated"
	}
	if snapshotCanAct(snapshot) {
		return "your_turn"
	}
	if snapshotHandActive(snapshot) {
		return "waiting"
	}
	if snapshotCanReady(snapshot) && !snapshotViewerReady(snapshot) {
		return "ready_required"
	}
	if snapshotViewerReady(snapshot) {
		return "waiting"
	}
	return "between_hands"
}

func pokerPlayerState(player poker.PlayerView) string {
	switch {
	case player.Folded:
		return "folded"
	case player.AllIn:
		return "all_in"
	case player.InHand:
		return "active"
	case player.Ready:
		return "ready"
	default:
		return "waiting"
	}
}

func landlordPlayerState(player landlord.PlayerView, active bool) string {
	if active {
		return "active"
	}
	if player.Ready {
		return "ready"
	}
	return "waiting"
}

func pokerLegalActions(allowed poker.AllowedActions) []string {
	actions := make([]string, 0, 6)
	for _, item := range []struct {
		allowed bool
		name    string
	}{
		{allowed.CanFold, string(poker.ActionFold)}, {allowed.CanCheck, string(poker.ActionCheck)},
		{allowed.CanCall, string(poker.ActionCall)}, {allowed.CanBet, string(poker.ActionBet)},
		{allowed.CanRaise, string(poker.ActionRaise)}, {allowed.CanAllIn, string(poker.ActionAllIn)},
	} {
		if item.allowed {
			actions = append(actions, item.name)
		}
	}
	return actions
}

func landlordLegalActions(allowed landlord.AllowedActions) []string {
	actions := make([]string, 0, 3)
	if allowed.CanBid {
		actions = append(actions, string(landlord.ActionBid))
	}
	if allowed.CanPlay {
		actions = append(actions, string(landlord.ActionPlay))
	}
	if allowed.CanPass {
		actions = append(actions, string(landlord.ActionPass))
	}
	return actions
}

func pokerCardCodes(cards []poker.Card) []string {
	if len(cards) == 0 {
		return nil
	}
	codes := make([]string, 0, len(cards))
	for _, card := range cards {
		codes = append(codes, card.String())
	}
	return codes
}

func landlordCardCodes(cards []landlord.Card) []string {
	if len(cards) == 0 {
		return nil
	}
	codes := make([]string, 0, len(cards))
	for _, card := range cards {
		codes = append(codes, card.String())
	}
	return codes
}

func enrichKickVote(vote *wireKickVote, snapshot gameSnapshot) *KickVoteView {
	if vote == nil {
		return nil
	}
	view := KickVoteView{
		TargetName: vote.TargetName, InitiatorName: vote.InitiatorName, YesCount: vote.YesCount,
		RequiredYes: vote.RequiredYes, ExpiresAt: vote.ExpiresAt,
	}
	view.TargetIsViewer = vote.TargetUserID == snapshotViewerUserID(snapshot)
	view.CanCancelByReady = view.TargetIsViewer && snapshotCanReady(snapshot) && !snapshotViewerReady(snapshot)
	return &view
}

func snapshotViewerUserID(snapshot gameSnapshot) int64 {
	viewerSeat := snapshotViewerSeat(snapshot)
	if snapshot.Landlord != nil {
		for _, player := range snapshot.Landlord.Players {
			if player.Seat == viewerSeat {
				return player.UserID
			}
		}
	}
	if snapshot.Poker != nil {
		for _, player := range snapshot.Poker.Players {
			if player.Seat == viewerSeat {
				return player.UserID
			}
		}
	}
	return 0
}

func snapshotViewerReady(snapshot gameSnapshot) bool {
	viewerSeat := snapshotViewerSeat(snapshot)
	if snapshot.Landlord != nil {
		for _, player := range snapshot.Landlord.Players {
			if player.Seat == viewerSeat {
				return player.Ready
			}
		}
	}
	if snapshot.Poker != nil {
		for _, player := range snapshot.Poker.Players {
			if player.Seat == viewerSeat {
				return player.Ready
			}
		}
	}
	return false
}

func snapshotCanReady(snapshot gameSnapshot) bool {
	if snapshot.Landlord != nil {
		return snapshot.Landlord.CanStart
	}
	return snapshot.Poker != nil && snapshot.Poker.CanStart
}

func snapshotHandID(snapshot gameSnapshot) int64 {
	if snapshot.Landlord != nil {
		return snapshot.Landlord.HandID
	}
	if snapshot.Poker != nil {
		return snapshot.Poker.HandID
	}
	return 0
}

type waitState struct {
	active      bool
	nextAllowed time.Time
}

type waitRegistry struct {
	mu     sync.Mutex
	states map[string]waitState
}

func newWaitRegistry() *waitRegistry {
	return &waitRegistry{states: make(map[string]waitState)}
}

func (registry *waitRegistry) acquire(identity string) (func(time.Duration), int, bool) {
	registry.mu.Lock()
	now := time.Now()
	state := registry.states[identity]
	if state.active || now.Before(state.nextAllowed) {
		retryAfter := max(1, int(time.Until(state.nextAllowed).Milliseconds()))
		if state.active {
			retryAfter = 1000
		}
		registry.mu.Unlock()
		return nil, retryAfter, false
	}
	registry.states[identity] = waitState{active: true}
	registry.mu.Unlock()
	return func(cooldown time.Duration) {
		registry.mu.Lock()
		registry.states[identity] = waitState{nextAllowed: time.Now().Add(cooldown)}
		registry.mu.Unlock()
	}, 0, true
}

func snapshotViewerSeat(snapshot gameSnapshot) int {
	if snapshot.Landlord != nil {
		return snapshot.Landlord.ViewerSeat
	}
	if snapshot.Poker != nil {
		return snapshot.Poker.ViewerSeat
	}
	return -1
}

func snapshotCanAct(snapshot gameSnapshot) bool {
	if snapshot.Landlord != nil {
		return snapshot.Landlord.Allowed.CanAct
	}
	return snapshot.Poker != nil && snapshot.Poker.Allowed.CanAct
}

func snapshotTurnID(snapshot gameSnapshot) uint64 {
	if snapshot.Landlord != nil {
		return snapshot.Landlord.TurnID
	}
	if snapshot.Poker != nil {
		return snapshot.Poker.TurnID
	}
	return 0
}

func snapshotHandActive(snapshot gameSnapshot) bool {
	if snapshot.Landlord != nil {
		return snapshot.Landlord.Phase == landlord.PhaseBidding || snapshot.Landlord.Phase == landlord.PhasePlaying
	}
	if snapshot.Poker == nil {
		return false
	}
	street := snapshot.Poker.Street
	return street == poker.StreetPreflop || street == poker.StreetFlop || street == poker.StreetTurn || street == poker.StreetRiver
}

func parseLandlordCard(code string) (landlord.Card, error) {
	code = strings.TrimSpace(code)
	upper := strings.ToUpper(code)
	if upper == "SJ" {
		return landlord.Card{Rank: landlord.SmallJoker, Suit: landlord.Joker}, nil
	}
	if upper == "BJ" {
		return landlord.Card{Rank: landlord.BigJoker, Suit: landlord.Joker}, nil
	}
	if len(code) < 2 {
		return landlord.Card{}, fmt.Errorf("invalid landlord card code %q", code)
	}
	suitCode := strings.ToLower(code[len(code)-1:])
	rankCode := strings.ToUpper(code[:len(code)-1])
	suits := map[string]landlord.Suit{"c": landlord.Clubs, "d": landlord.Diamonds, "h": landlord.Hearts, "s": landlord.Spades}
	suit, ok := suits[suitCode]
	if !ok {
		return landlord.Card{}, fmt.Errorf("invalid landlord card suit in %q", code)
	}
	ranks := map[string]landlord.Rank{
		"3": landlord.Three, "4": landlord.Four, "5": landlord.Five, "6": landlord.Six,
		"7": landlord.Seven, "8": landlord.Eight, "9": landlord.Nine, "T": landlord.Ten,
		"10": landlord.Ten, "J": landlord.Jack, "Q": landlord.Queen, "K": landlord.King,
		"A": landlord.Ace, "2": landlord.Two,
	}
	rank, ok := ranks[rankCode]
	if !ok {
		return landlord.Card{}, fmt.Errorf("invalid landlord card rank in %q", code)
	}
	return landlord.Card{Rank: rank, Suit: suit}, nil
}
