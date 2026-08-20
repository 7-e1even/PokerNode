package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"pokernode/internal/landlord"
	"pokernode/internal/poker"
)

const serverVersion = "0.1.0"

type Server struct {
	api *APIClient
}

type ListChannelsInput struct{}

type ListChannelsOutput struct {
	Channels []channelView `json:"channels" jsonschema:"Channels the configured account has joined."`
}

type ListTablesInput struct {
	SpaceID string `json:"space_id" jsonschema:"Channel ID returned by pokernode_list_channels."`
}

type ListTablesOutput struct {
	Tables []tableSummary `json:"tables" jsonschema:"Poker tables in the channel."`
}

type CurrentGameInput struct{}

type TableInput struct {
	SpaceID string `json:"space_id" jsonschema:"Channel ID returned by pokernode_list_channels."`
	TableID string `json:"table_id" jsonschema:"Table ID returned by pokernode_list_tables."`
}

type JoinTableInput struct {
	SpaceID    string `json:"space_id" jsonschema:"Channel ID returned by pokernode_list_channels."`
	TableID    string `json:"table_id" jsonschema:"Table ID returned by pokernode_list_tables."`
	BuyInCents int64  `json:"buy_in_cents" jsonschema:"Buy-in in integer cents, from 2000 through 100000."`
}

type ActInput struct {
	SpaceID        string   `json:"space_id" jsonschema:"Channel ID returned by pokernode_list_channels."`
	TableID        string   `json:"table_id" jsonschema:"Table ID returned by pokernode_list_tables."`
	Action         string   `json:"action" jsonschema:"For Texas Hold'em: fold, check, call, bet, raise, or all_in. For landlord: bid, play, or pass. Use only an allowed action."`
	AmountCents    int64    `json:"amount_cents,omitempty" jsonschema:"Texas Hold'em only. For bet or raise, the total target bet in cents."`
	Bid            int      `json:"bid,omitempty" jsonschema:"Landlord bid from 0 through 3. Zero means no bid."`
	Cards          []string `json:"cards,omitempty" jsonschema:"Landlord cards to play using compact codes such as 3c, Td, 2s, SJ, or BJ."`
	ExpectedTurnID uint64   `json:"expected_turn_id" jsonschema:"Required turn_id from the latest table state. The action is rejected if the turn has changed."`
}

type WaitForTurnInput struct {
	SpaceID        string `json:"space_id" jsonschema:"Channel ID returned by pokernode_list_channels."`
	TableID        string `json:"table_id" jsonschema:"Table ID returned by pokernode_list_tables."`
	MaxWaitSeconds int    `json:"max_wait_seconds,omitempty" jsonschema:"How long to wait, from 1 through 60 seconds. Defaults to 25."`
}

type CardView struct {
	Code string `json:"code" jsonschema:"Compact notation such as As, Td, 3c, SJ, or BJ."`
	Rank int    `json:"rank" jsonschema:"Numeric game rank. Landlord uses 15 for two and 16/17 for the jokers."`
	Suit string `json:"suit" jsonschema:"One of clubs, diamonds, hearts, spades, or joker."`
}

type PlayerView struct {
	UserID           int64      `json:"user_id"`
	Name             string     `json:"name"`
	Seat             int        `json:"seat"`
	StackCents       int64      `json:"stack_cents"`
	BetCents         int64      `json:"bet_cents"`
	Cards            []CardView `json:"cards,omitempty" jsonschema:"The viewer's hole cards, plus non-folded opponents' cards only after a showdown."`
	InHand           bool       `json:"in_hand"`
	Folded           bool       `json:"folded"`
	AllIn            bool       `json:"all_in"`
	Ready            bool       `json:"ready"`
	IsDealer         bool       `json:"is_dealer"`
	IsActing         bool       `json:"is_acting"`
	LastAction       string     `json:"last_action,omitempty"`
	LastActionAmount int64      `json:"last_action_amount_cents,omitempty"`
	CardCount        int        `json:"card_count,omitempty"`
	Landlord         bool       `json:"landlord,omitempty"`
	Bid              int        `json:"bid,omitempty"`
}

type AllowedActionsView struct {
	CanAct          bool  `json:"can_act"`
	CanFold         bool  `json:"can_fold"`
	CanCheck        bool  `json:"can_check"`
	CanCall         bool  `json:"can_call"`
	CanBet          bool  `json:"can_bet"`
	CanRaise        bool  `json:"can_raise"`
	CanAllIn        bool  `json:"can_all_in"`
	ToCallCents     int64 `json:"to_call_cents"`
	MinRaiseToCents int64 `json:"min_raise_to_cents" jsonschema:"Minimum total target for a bet or raise."`
	MaxRaiseToCents int64 `json:"max_raise_to_cents" jsonschema:"Maximum total target for a bet or raise."`
	CanBid          bool  `json:"can_bid,omitempty"`
	MinBid          int   `json:"min_bid,omitempty"`
	CanPlay         bool  `json:"can_play,omitempty"`
	CanPass         bool  `json:"can_pass,omitempty"`
}

type PayoutView struct {
	UserID      int64 `json:"user_id"`
	AmountCents int64 `json:"amount_cents"`
}

type HandResultView struct {
	HandID   int64        `json:"hand_id"`
	PotCents int64        `json:"pot_cents"`
	Message  string       `json:"message"`
	Showdown bool         `json:"showdown"`
	Payouts  []PayoutView `json:"payouts"`
	Winner   string       `json:"winner,omitempty"`
	Bid      int          `json:"bid,omitempty"`
	Multiple int          `json:"multiplier,omitempty"`
	Stake    int64        `json:"stake_cents,omitempty"`
}

type TableView struct {
	GameType             string             `json:"game_type"`
	ID                   string             `json:"id"`
	Name                 string             `json:"name"`
	SmallBlindCents      int64              `json:"small_blind_cents,omitempty"`
	BigBlindCents        int64              `json:"big_blind_cents,omitempty"`
	BaseStakeCents       int64              `json:"base_stake_cents,omitempty"`
	HandID               int64              `json:"hand_id"`
	Street               string             `json:"street"`
	Board                []CardView         `json:"board"`
	Bottom               []CardView         `json:"bottom,omitempty"`
	LastPlay             []CardView         `json:"last_play,omitempty"`
	LastCombination      string             `json:"last_combination,omitempty"`
	HighestBid           int                `json:"highest_bid,omitempty"`
	LandlordSeat         *int               `json:"landlord_seat,omitempty"`
	Multiplier           int                `json:"multiplier,omitempty"`
	PotCents             int64              `json:"pot_cents"`
	CurrentBetCents      int64              `json:"current_bet_cents"`
	DealerSeat           int                `json:"dealer_seat"`
	SmallBlindSeat       int                `json:"small_blind_seat"`
	BigBlindSeat         int                `json:"big_blind_seat"`
	ActingSeat           int                `json:"acting_seat"`
	ActionTimeoutSeconds int                `json:"action_timeout_seconds"`
	ActionDeadlineAt     int64              `json:"action_deadline_at" jsonschema:"Unix time in milliseconds when the current action expires."`
	TurnID               uint64             `json:"turn_id"`
	ViewerSeat           int                `json:"viewer_seat" jsonschema:"The configured account's seat, or -1 when not seated."`
	Players              []PlayerView       `json:"players"`
	AllowedActions       AllowedActionsView `json:"allowed_actions"`
	CanReady             bool               `json:"can_ready" jsonschema:"Whether the configured account may mark itself ready."`
	CanLeave             bool               `json:"can_leave"`
	LastResult           *HandResultView    `json:"last_result,omitempty"`
}

type TableOutput struct {
	Table        TableView `json:"table"`
	Notice       string    `json:"notice,omitempty"`
	OperationID  string    `json:"operation_id,omitempty"`
	SettledCents int64     `json:"settled_cents,omitempty"`
	WaitTimedOut bool      `json:"wait_timed_out,omitempty"`
}

type CurrentGameOutput struct {
	Active              bool       `json:"active" jsonschema:"Whether this player is currently seated at a table."`
	AgentControlEnabled bool       `json:"agent_control_enabled" jsonschema:"Whether the player explicitly handed gameplay control to this Agent."`
	SpaceID             string     `json:"space_id,omitempty"`
	TableID             string     `json:"table_id,omitempty"`
	Table               *TableView `json:"table,omitempty"`
}

func New(api *APIClient) *mcp.Server {
	return newServer(api, nil)
}

func newServer(api *APIClient, schemaCache *mcp.SchemaCache) *mcp.Server {
	service := &Server{api: api}
	var options *mcp.ServerOptions
	if schemaCache != nil {
		options = &mcp.ServerOptions{SchemaCache: schemaCache}
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "pokernode-mcp", Version: serverVersion}, options)

	mcp.AddTool(server, readOnlyTool("pokernode_list_channels", "List channels available to the configured PokerNode player account."), service.listChannels)
	mcp.AddTool(server, readOnlyTool("pokernode_get_current_game", "Return the single table where this player is currently seated, across all channels and game types."), service.getCurrentGame)
	mcp.AddTool(server, readOnlyTool("pokernode_list_tables", "List game tables in a channel, including game type, seats, stakes, and whether this account is seated."), service.listTables)
	mcp.AddTool(server, readOnlyTool("pokernode_get_table", "Read the current table from this player's perspective. Private hole cards are visible only when PokerNode permits them."), service.getTable)
	mcp.AddTool(server, readOnlyTool("pokernode_wait_for_turn", "Wait until this player can act, the hand ends, or the timeout expires, then return the latest table state."), service.waitForTurn)
	mcp.AddTool(server, writeTool("pokernode_join_table", "Buy in and join a table as the configured player. This moves quota into the poker stack.", true), service.joinTable)
	mcp.AddTool(server, writeTool("pokernode_ready", "Mark the configured seated player ready. A hand starts automatically when every funded player is ready.", false), service.ready)
	mcp.AddTool(server, writeTool("pokernode_act", "Take one legal game action. Read game_type and allowed_actions first. Landlord play uses compact card codes in cards.", true), service.act)
	mcp.AddTool(server, writeTool("pokernode_leave_table", "Leave between hands and settle the remaining stack back to the configured player's quota balance.", true), service.leaveTable)
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

func (s *Server) listChannels(ctx context.Context, _ *mcp.CallToolRequest, _ ListChannelsInput) (*mcp.CallToolResult, ListChannelsOutput, error) {
	channels, err := s.api.listChannels(ctx)
	return nil, ListChannelsOutput{Channels: channels}, err
}

func (s *Server) getCurrentGame(ctx context.Context, _ *mcp.CallToolRequest, _ CurrentGameInput) (*mcp.CallToolResult, CurrentGameOutput, error) {
	current, err := s.api.getCurrentGame(ctx)
	if err != nil {
		return nil, CurrentGameOutput{}, err
	}
	output := CurrentGameOutput{Active: current.Active, AgentControlEnabled: current.AgentControlEnabled, SpaceID: current.SpaceID, TableID: current.TableID}
	if current.Active {
		table := tableOutput(current.Table, "").Table
		output.Table = &table
	}
	return nil, output, nil
}

func (s *Server) listTables(ctx context.Context, _ *mcp.CallToolRequest, input ListTablesInput) (*mcp.CallToolResult, ListTablesOutput, error) {
	if err := requireIDs(input.SpaceID); err != nil {
		return nil, ListTablesOutput{}, err
	}
	tables, err := s.api.listTables(ctx, input.SpaceID)
	return nil, ListTablesOutput{Tables: tables}, err
}

func (s *Server) getTable(ctx context.Context, _ *mcp.CallToolRequest, input TableInput) (*mcp.CallToolResult, TableOutput, error) {
	if err := requireIDs(input.SpaceID, input.TableID); err != nil {
		return nil, TableOutput{}, err
	}
	envelope, err := s.api.getTable(ctx, input.SpaceID, input.TableID)
	return nil, tableOutput(envelope.Table, envelope.Notice), err
}

func (s *Server) waitForTurn(ctx context.Context, _ *mcp.CallToolRequest, input WaitForTurnInput) (*mcp.CallToolResult, TableOutput, error) {
	if err := requireIDs(input.SpaceID, input.TableID); err != nil {
		return nil, TableOutput{}, err
	}
	wait := input.MaxWaitSeconds
	if wait == 0 {
		wait = 25
	}
	if wait < 1 || wait > 60 {
		return nil, TableOutput{}, errors.New("max_wait_seconds must be between 1 and 60")
	}
	timer := time.NewTimer(time.Duration(wait) * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		envelope, err := s.api.getTable(ctx, input.SpaceID, input.TableID)
		if err != nil {
			return nil, TableOutput{}, err
		}
		if snapshotViewerSeat(envelope.Table) < 0 {
			return nil, TableOutput{}, errors.New("the configured player is not seated at this table; call pokernode_join_table first")
		}
		if snapshotCanAct(envelope.Table) || !snapshotHandActive(envelope.Table) {
			return nil, tableOutput(envelope.Table, envelope.Notice), nil
		}
		select {
		case <-ctx.Done():
			return nil, TableOutput{}, ctx.Err()
		case <-timer.C:
			output := tableOutput(envelope.Table, envelope.Notice)
			output.WaitTimedOut = true
			return nil, output, nil
		case <-ticker.C:
		}
	}
}

func (s *Server) joinTable(ctx context.Context, _ *mcp.CallToolRequest, input JoinTableInput) (*mcp.CallToolResult, TableOutput, error) {
	if err := requireIDs(input.SpaceID, input.TableID); err != nil {
		return nil, TableOutput{}, err
	}
	if input.BuyInCents < 2_000 || input.BuyInCents > 100_000 {
		return nil, TableOutput{}, errors.New("buy_in_cents must be between 2000 and 100000")
	}
	table, operationID, err := s.api.joinTable(ctx, input.SpaceID, input.TableID, input.BuyInCents)
	output := tableOutput(table, "")
	output.OperationID = operationID
	return nil, output, err
}

func (s *Server) ready(ctx context.Context, _ *mcp.CallToolRequest, input TableInput) (*mcp.CallToolResult, TableOutput, error) {
	if err := requireIDs(input.SpaceID, input.TableID); err != nil {
		return nil, TableOutput{}, err
	}
	envelope, err := s.api.ready(ctx, input.SpaceID, input.TableID)
	return nil, tableOutput(envelope.Table, envelope.Notice), err
}

func (s *Server) act(ctx context.Context, _ *mcp.CallToolRequest, input ActInput) (*mcp.CallToolResult, TableOutput, error) {
	if err := requireIDs(input.SpaceID, input.TableID); err != nil {
		return nil, TableOutput{}, err
	}
	current, err := s.api.getTable(ctx, input.SpaceID, input.TableID)
	if err != nil {
		return nil, TableOutput{}, err
	}
	if input.ExpectedTurnID == 0 {
		return nil, TableOutput{}, errors.New("expected_turn_id is required; call pokernode_get_table or pokernode_wait_for_turn and use its current turn_id")
	}
	if input.ExpectedTurnID != snapshotTurnID(current.Table) {
		return nil, tableOutput(current.Table, ""), errors.New("the turn has changed; use the returned current table state and decide again")
	}
	request := gameActionRequest{Action: input.Action, Amount: input.AmountCents, Bid: input.Bid, ExpectedTurnID: input.ExpectedTurnID}
	if current.Table.GameType == landlord.GameType {
		if input.Action != string(landlord.ActionBid) && input.Action != string(landlord.ActionPlay) && input.Action != string(landlord.ActionPass) {
			return nil, TableOutput{}, errors.New("landlord action must be bid, play, or pass")
		}
		if input.Action == string(landlord.ActionBid) && (input.Bid < 0 || input.Bid > 3) {
			return nil, TableOutput{}, errors.New("bid must be from 0 through 3")
		}
		if input.Action == string(landlord.ActionPlay) {
			if len(input.Cards) == 0 {
				return nil, TableOutput{}, errors.New("cards is required for a landlord play action")
			}
			request.Cards = make([]landlord.Card, 0, len(input.Cards))
			for _, code := range input.Cards {
				card, parseErr := parseLandlordCard(code)
				if parseErr != nil {
					return nil, TableOutput{}, parseErr
				}
				request.Cards = append(request.Cards, card)
			}
		}
	} else {
		action := poker.ActionType(input.Action)
		if !validAction(action) {
			return nil, TableOutput{}, errors.New("Texas Hold'em action must be fold, check, call, bet, raise, or all_in")
		}
		if (action == poker.ActionBet || action == poker.ActionRaise) && input.AmountCents <= 0 {
			return nil, TableOutput{}, errors.New("amount_cents must be a positive total target for bet or raise")
		}
	}
	envelope, err := s.api.act(ctx, input.SpaceID, input.TableID, request)
	return nil, tableOutput(envelope.Table, envelope.Notice), err
}

func (s *Server) leaveTable(ctx context.Context, _ *mcp.CallToolRequest, input TableInput) (*mcp.CallToolResult, TableOutput, error) {
	if err := requireIDs(input.SpaceID, input.TableID); err != nil {
		return nil, TableOutput{}, err
	}
	table, operationID, settled, err := s.api.leaveTable(ctx, input.SpaceID, input.TableID)
	output := tableOutput(table, "")
	output.OperationID = operationID
	output.SettledCents = settled
	return nil, output, err
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

func tableOutput(snapshot gameSnapshot, notice string) TableOutput {
	if snapshot.Landlord != nil {
		return landlordTableOutput(*snapshot.Landlord, notice)
	}
	if snapshot.Poker == nil {
		return TableOutput{Notice: notice}
	}
	return pokerTableOutput(*snapshot.Poker, notice)
}

func pokerTableOutput(snapshot poker.Snapshot, notice string) TableOutput {
	players := make([]PlayerView, 0, len(snapshot.Players))
	for _, player := range snapshot.Players {
		players = append(players, PlayerView{
			UserID: player.UserID, Name: player.Name, Seat: player.Seat, StackCents: player.Stack,
			BetCents: player.Bet, Cards: cardViews(player.Cards), InHand: player.InHand,
			Folded: player.Folded, AllIn: player.AllIn, Ready: player.Ready, IsDealer: player.IsDealer,
			IsActing: player.IsActing, LastAction: string(player.LastAction), LastActionAmount: player.LastActionAmount,
		})
	}
	allowed := snapshot.Allowed
	view := TableView{
		GameType: poker.GameType, ID: snapshot.ID, Name: snapshot.Name, SmallBlindCents: snapshot.SmallBlind, BigBlindCents: snapshot.BigBlind,
		HandID: snapshot.HandID, Street: string(snapshot.Street), Board: cardViews(snapshot.Board), PotCents: snapshot.Pot,
		CurrentBetCents: snapshot.CurrentBet, DealerSeat: snapshot.DealerSeat, SmallBlindSeat: snapshot.SmallBlindSeat,
		BigBlindSeat: snapshot.BigBlindSeat, ActingSeat: snapshot.ActingSeat, ActionTimeoutSeconds: snapshot.ActionTimeoutSeconds,
		ActionDeadlineAt: snapshot.ActionDeadlineAt, TurnID: snapshot.TurnID, ViewerSeat: snapshot.ViewerSeat,
		Players: players, CanReady: snapshot.CanStart, CanLeave: snapshot.CanLeave,
		AllowedActions: AllowedActionsView{
			CanAct: allowed.CanAct, CanFold: allowed.CanFold, CanCheck: allowed.CanCheck, CanCall: allowed.CanCall,
			CanBet: allowed.CanBet, CanRaise: allowed.CanRaise, CanAllIn: allowed.CanAllIn, ToCallCents: allowed.ToCall,
			MinRaiseToCents: allowed.MinRaiseTo, MaxRaiseToCents: allowed.MaxRaiseTo,
		},
	}
	if snapshot.LastResult != nil {
		payouts := make([]PayoutView, 0, len(snapshot.LastResult.Payouts))
		for userID, amount := range snapshot.LastResult.Payouts {
			payouts = append(payouts, PayoutView{UserID: userID, AmountCents: amount})
		}
		sort.Slice(payouts, func(i, j int) bool { return payouts[i].UserID < payouts[j].UserID })
		view.LastResult = &HandResultView{
			HandID: snapshot.LastResult.HandID, PotCents: snapshot.LastResult.Pot, Message: snapshot.LastResult.Message,
			Showdown: snapshot.LastResult.Showdown, Payouts: payouts,
		}
	}
	return TableOutput{Table: view, Notice: notice}
}

func landlordTableOutput(snapshot landlord.Snapshot, notice string) TableOutput {
	players := make([]PlayerView, 0, len(snapshot.Players))
	for _, player := range snapshot.Players {
		players = append(players, PlayerView{
			UserID: player.UserID, Name: player.Name, Seat: player.Seat, StackCents: player.Stack,
			Cards: landlordCardViews(player.Cards), CardCount: player.CardCount, Ready: player.Ready,
			Landlord: player.Landlord, Bid: player.Bid, IsActing: player.IsActing,
		})
	}
	allowed := snapshot.Allowed
	view := TableView{
		GameType: landlord.GameType, ID: snapshot.ID, Name: snapshot.Name, BaseStakeCents: snapshot.BaseStake,
		HandID: snapshot.HandID, Street: string(snapshot.Phase), Bottom: landlordCardViews(snapshot.Bottom),
		LastPlay: landlordCardViews(snapshot.LastPlay), LastCombination: string(snapshot.LastCombination.Kind),
		HighestBid: snapshot.HighestBid, LandlordSeat: &snapshot.LandlordSeat, Multiplier: snapshot.Multiplier,
		ActingSeat: snapshot.ActingSeat, ActionTimeoutSeconds: snapshot.ActionTimeoutSeconds,
		ActionDeadlineAt: snapshot.ActionDeadlineAt, TurnID: snapshot.TurnID, ViewerSeat: snapshot.ViewerSeat,
		Players: players, CanReady: snapshot.CanStart, CanLeave: snapshot.CanLeave,
		AllowedActions: AllowedActionsView{
			CanAct: allowed.CanAct, CanBid: allowed.CanBid, MinBid: allowed.MinBid,
			CanPlay: allowed.CanPlay, CanPass: allowed.CanPass,
		},
	}
	if snapshot.LastResult != nil {
		payouts := make([]PayoutView, 0, len(snapshot.LastResult.Payouts))
		for userID, amount := range snapshot.LastResult.Payouts {
			payouts = append(payouts, PayoutView{UserID: userID, AmountCents: amount})
		}
		sort.Slice(payouts, func(i, j int) bool { return payouts[i].UserID < payouts[j].UserID })
		view.LastResult = &HandResultView{
			HandID: snapshot.LastResult.HandID, Message: snapshot.LastResult.Message, Payouts: payouts,
			Winner: snapshot.LastResult.Winner, Bid: snapshot.LastResult.Bid,
			Multiple: snapshot.LastResult.Multiplier, Stake: snapshot.LastResult.Stake,
		}
	}
	return TableOutput{Table: view, Notice: notice}
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

func cardViews(cards []poker.Card) []CardView {
	if len(cards) == 0 {
		return nil
	}
	views := make([]CardView, 0, len(cards))
	for _, card := range cards {
		views = append(views, CardView{Code: card.String(), Rank: int(card.Rank), Suit: suitName(card.Suit)})
	}
	return views
}

func landlordCardViews(cards []landlord.Card) []CardView {
	if len(cards) == 0 {
		return nil
	}
	views := make([]CardView, 0, len(cards))
	for _, card := range cards {
		views = append(views, CardView{Code: card.String(), Rank: int(card.Rank), Suit: landlordSuitName(card.Suit)})
	}
	return views
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

func suitName(suit poker.Suit) string {
	switch suit {
	case poker.Clubs:
		return "clubs"
	case poker.Diamonds:
		return "diamonds"
	case poker.Hearts:
		return "hearts"
	case poker.Spades:
		return "spades"
	default:
		return fmt.Sprintf("unknown_%d", suit)
	}
}

func landlordSuitName(suit landlord.Suit) string {
	switch suit {
	case landlord.Clubs:
		return "clubs"
	case landlord.Diamonds:
		return "diamonds"
	case landlord.Hearts:
		return "hearts"
	case landlord.Spades:
		return "spades"
	case landlord.Joker:
		return "joker"
	default:
		return fmt.Sprintf("unknown_%d", suit)
	}
}
