package poker

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	GameType                    = "texas_holdem"
	MaxSeats                    = 8
	DefaultActionTimeoutSeconds = 25
	MinActionTimeoutSeconds     = 5
	MaxActionTimeoutSeconds     = 300
)

type Street string

const (
	StreetWaiting  Street = "waiting"
	StreetPreflop  Street = "preflop"
	StreetFlop     Street = "flop"
	StreetTurn     Street = "turn"
	StreetRiver    Street = "river"
	StreetComplete Street = "complete"
)

type ActionType string

const (
	ActionFold  ActionType = "fold"
	ActionCheck ActionType = "check"
	ActionCall  ActionType = "call"
	ActionBet   ActionType = "bet"
	ActionRaise ActionType = "raise"
	ActionAllIn ActionType = "all_in"
)

var (
	ErrTableFull      = errors.New("table is full")
	ErrAlreadySeated  = errors.New("player is already seated")
	ErrNotSeated      = errors.New("player is not seated")
	ErrHandInProgress = errors.New("a hand is in progress")
	ErrNotYourTurn    = errors.New("it is not your turn")
	ErrActionTimedOut = errors.New("action time has expired")
)

type Player struct {
	UserID             int64      `json:"user_id"`
	Name               string     `json:"name"`
	Seat               int        `json:"seat"`
	Stack              int64      `json:"stack_cents"`
	Bet                int64      `json:"bet_cents"`
	Committed          int64      `json:"committed_cents"`
	Hole               []Card     `json:"hole,omitempty"`
	InHand             bool       `json:"in_hand"`
	Folded             bool       `json:"folded"`
	AllIn              bool       `json:"all_in"`
	Ready              bool       `json:"ready"`
	LastAction         ActionType `json:"last_action,omitempty"`
	LastActionAmount   int64      `json:"last_action_amount_cents,omitempty"`
	LastActionBetLevel int        `json:"last_action_bet_level,omitempty"`
}

type HandResult struct {
	HandID   int64              `json:"hand_id"`
	Pot      int64              `json:"pot_cents"`
	Message  string             `json:"message"`
	Showdown bool               `json:"showdown"`
	Board    []Card             `json:"board,omitempty"`
	Payouts  map[int64]int64    `json:"payouts"`
	Refunds  map[int64]int64    `json:"refunds,omitempty"`
	Players  []HandPlayerResult `json:"players,omitempty"`
}

type HandPlayerResult struct {
	UserID        int64  `json:"user_id"`
	Name          string `json:"name"`
	Seat          int    `json:"seat"`
	Cards         []Card `json:"cards,omitempty"`
	Folded        bool   `json:"folded"`
	StartingStack int64  `json:"starting_stack_cents"`
	Committed     int64  `json:"committed_cents"`
	Payout        int64  `json:"payout_cents"`
	Refund        int64  `json:"refund_cents,omitempty"`
	EndingStack   int64  `json:"ending_stack_cents"`
	Net           int64  `json:"net_cents"`
}

type TableState struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	SmallBlind           int64             `json:"small_blind_cents"`
	BigBlind             int64             `json:"big_blind_cents"`
	HandID               int64             `json:"hand_id"`
	Street               Street            `json:"street"`
	Dealer               int               `json:"dealer"`
	Acting               int               `json:"acting"`
	CurrentBet           int64             `json:"current_bet_cents"`
	MinRaise             int64             `json:"min_raise_cents"`
	Board                []Card            `json:"board"`
	Deck                 []Card            `json:"deck"`
	DeckPos              int               `json:"deck_pos"`
	Seats                [MaxSeats]*Player `json:"seats"`
	Acted                [MaxSeats]bool    `json:"acted"`
	ActedAtBet           [MaxSeats]int64   `json:"acted_at_bet_cents"`
	BetLevel             int               `json:"bet_level"`
	SmallBlindSeat       int               `json:"small_blind_seat"`
	BigBlindSeat         int               `json:"big_blind_seat"`
	PositionsSet         bool              `json:"positions_set,omitempty"`
	ActionTimeoutSeconds int               `json:"action_timeout_seconds"`
	ActionDeadlineAt     int64             `json:"action_deadline_at"`
	TurnID               uint64            `json:"turn_id"`
	LastResult           *HandResult       `json:"last_result,omitempty"`
}

type Table struct {
	mu    sync.RWMutex
	state TableState
	now   func() time.Time
}

type AllowedActions struct {
	CanAct     bool  `json:"can_act"`
	CanFold    bool  `json:"can_fold"`
	CanCheck   bool  `json:"can_check"`
	CanCall    bool  `json:"can_call"`
	CanBet     bool  `json:"can_bet"`
	CanRaise   bool  `json:"can_raise"`
	CanAllIn   bool  `json:"can_all_in"`
	ToCall     int64 `json:"to_call_cents"`
	MinRaiseTo int64 `json:"min_raise_to_cents"`
	MaxRaiseTo int64 `json:"max_raise_to_cents"`
}

type PlayerView struct {
	UserID             int64      `json:"user_id"`
	Name               string     `json:"name"`
	Seat               int        `json:"seat"`
	Stack              int64      `json:"stack_cents"`
	Bet                int64      `json:"bet_cents"`
	Cards              []Card     `json:"cards,omitempty"`
	InHand             bool       `json:"in_hand"`
	Folded             bool       `json:"folded"`
	AllIn              bool       `json:"all_in"`
	Ready              bool       `json:"ready"`
	IsDealer           bool       `json:"is_dealer"`
	IsActing           bool       `json:"is_acting"`
	LastAction         ActionType `json:"last_action,omitempty"`
	LastActionAmount   int64      `json:"last_action_amount_cents,omitempty"`
	LastActionBetLevel int        `json:"last_action_bet_level,omitempty"`
}

type Snapshot struct {
	GameType             string         `json:"game_type"`
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	SmallBlind           int64          `json:"small_blind_cents"`
	BigBlind             int64          `json:"big_blind_cents"`
	HandID               int64          `json:"hand_id"`
	Street               Street         `json:"street"`
	Board                []Card         `json:"board"`
	Pot                  int64          `json:"pot_cents"`
	CurrentBet           int64          `json:"current_bet_cents"`
	BetLevel             int            `json:"bet_level"`
	DealerSeat           int            `json:"dealer_seat"`
	SmallBlindSeat       int            `json:"small_blind_seat"`
	BigBlindSeat         int            `json:"big_blind_seat"`
	ActingSeat           int            `json:"acting_seat"`
	ActionTimeoutSeconds int            `json:"action_timeout_seconds"`
	ActionDeadlineAt     int64          `json:"action_deadline_at"`
	TurnID               uint64         `json:"turn_id"`
	ViewerSeat           int            `json:"viewer_seat"`
	Players              []PlayerView   `json:"players"`
	Allowed              AllowedActions `json:"allowed_actions"`
	CanStart             bool           `json:"can_start"`
	CanLeave             bool           `json:"can_leave"`
	LastResult           *HandResult    `json:"last_result,omitempty"`
}

func NewTable(id, name string, smallBlind, bigBlind int64) *Table {
	return &Table{state: TableState{
		ID: id, Name: name, SmallBlind: smallBlind, BigBlind: bigBlind,
		Street: StreetWaiting, Dealer: -1, Acting: -1, MinRaise: bigBlind,
		SmallBlindSeat: -1, BigBlindSeat: -1, ActionTimeoutSeconds: DefaultActionTimeoutSeconds,
	}, now: time.Now}
}

func RestoreTable(data []byte) (*Table, error) {
	var state TableState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode table state: %w", err)
	}
	if state.ID == "" || state.SmallBlind <= 0 || state.BigBlind < state.SmallBlind {
		return nil, errors.New("invalid persisted table state")
	}
	if state.ActionTimeoutSeconds == 0 {
		state.ActionTimeoutSeconds = DefaultActionTimeoutSeconds
	}
	if state.ActionTimeoutSeconds < MinActionTimeoutSeconds || state.ActionTimeoutSeconds > MaxActionTimeoutSeconds {
		return nil, errors.New("invalid persisted action timeout")
	}
	table := &Table{state: state, now: time.Now}
	if table.handActiveLocked() && table.state.Acting >= 0 {
		if table.state.TurnID == 0 {
			table.state.TurnID = 1
		}
		if table.state.ActionDeadlineAt <= 0 {
			table.state.ActionDeadlineAt = table.now().Add(time.Duration(table.state.ActionTimeoutSeconds) * time.Second).UnixMilli()
		}
	}
	return table, nil
}

func (t *Table) SetActionTimeoutSeconds(seconds int) error {
	if seconds < MinActionTimeoutSeconds || seconds > MaxActionTimeoutSeconds {
		return fmt.Errorf("action timeout must be between %d and %d seconds", MinActionTimeoutSeconds, MaxActionTimeoutSeconds)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.handActiveLocked() {
		return ErrHandInProgress
	}
	t.state.ActionTimeoutSeconds = seconds
	return nil
}

func (t *Table) MarshalState() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return json.Marshal(t.state)
}

func (t *Table) Join(userID int64, name string, buyIn int64) (int, error) {
	if userID <= 0 || name == "" || buyIn <= 0 {
		return -1, errors.New("invalid player or buy-in")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.seatForUserLocked(userID) >= 0 {
		return -1, ErrAlreadySeated
	}
	for seat := range t.state.Seats {
		if t.state.Seats[seat] == nil {
			if !t.handActiveLocked() {
				t.clearReadyLocked()
			}
			t.state.Seats[seat] = &Player{UserID: userID, Name: name, Seat: seat, Stack: buyIn}
			return seat, nil
		}
	}
	return -1, ErrTableFull
}

func (t *Table) Leave(userID int64) (int64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.handActiveLocked() {
		return 0, ErrHandInProgress
	}
	seat := t.seatForUserLocked(userID)
	if seat < 0 {
		return 0, ErrNotSeated
	}
	stack := t.state.Seats[seat].Stack
	t.state.Seats[seat] = nil
	t.clearReadyLocked()
	return stack, nil
}

// Ready marks a funded player as ready and starts the hand once every funded
// player at the table is ready. The returned boolean reports whether a hand
// started.
func (t *Table) Ready(userID int64) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.handActiveLocked() {
		return false, ErrHandInProgress
	}
	seat := t.seatForUserLocked(userID)
	if seat < 0 {
		return false, ErrNotSeated
	}
	if t.state.Seats[seat].Stack <= 0 {
		return false, errors.New("only funded players can ready")
	}
	active := t.playableSeatsLocked()
	if len(active) < 2 {
		return false, errors.New("at least two funded players are required")
	}

	t.state.Seats[seat].Ready = true
	for _, activeSeat := range active {
		if !t.state.Seats[activeSeat].Ready {
			return false, nil
		}
	}
	deck, err := shuffledDeck()
	if err != nil {
		t.state.Seats[seat].Ready = false
		return false, err
	}
	if err := t.startHandLocked(userID, deck); err != nil {
		t.state.Seats[seat].Ready = false
		return false, err
	}
	return true, nil
}

func (t *Table) StartHand(userID int64) error {
	deck, err := shuffledDeck()
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.startHandLocked(userID, deck)
}

func (t *Table) startHandLocked(userID int64, deck []Card) error {
	if t.handActiveLocked() {
		return ErrHandInProgress
	}
	if t.seatForUserLocked(userID) < 0 {
		return ErrNotSeated
	}
	active := t.playableSeatsLocked()
	if len(active) < 2 {
		return errors.New("at least two funded players are required")
	}
	if len(deck) != 52 {
		return errors.New("deck must contain 52 cards")
	}

	t.state.HandID++
	t.state.Street = StreetPreflop
	t.state.Board = nil
	t.state.Deck = append([]Card(nil), deck...)
	t.state.DeckPos = 0
	t.state.CurrentBet = 0
	t.state.MinRaise = t.state.BigBlind
	t.state.BetLevel = 1
	t.state.LastResult = nil
	t.state.Acted = [MaxSeats]bool{}
	t.state.ActedAtBet = [MaxSeats]int64{}
	t.state.Dealer = t.nextPlayableSeatLocked(t.state.Dealer)

	for _, seat := range active {
		player := t.state.Seats[seat]
		player.Ready = false
		player.Bet = 0
		player.Committed = 0
		player.Hole = nil
		player.InHand = true
		player.Folded = false
		player.AllIn = false
		player.LastAction = ""
		player.LastActionAmount = 0
		player.LastActionBetLevel = 0
	}

	for round := 0; round < 2; round++ {
		seat := t.nextInHandSeatLocked(t.state.Dealer)
		for dealt := 0; dealt < len(active); dealt++ {
			player := t.state.Seats[seat]
			player.Hole = append(player.Hole, t.drawLocked())
			seat = t.nextInHandSeatLocked(seat)
		}
	}

	smallBlindSeat := t.nextInHandSeatLocked(t.state.Dealer)
	if len(active) == 2 {
		smallBlindSeat = t.state.Dealer
	}
	bigBlindSeat := t.nextInHandSeatLocked(smallBlindSeat)
	t.state.SmallBlindSeat = smallBlindSeat
	t.state.BigBlindSeat = bigBlindSeat
	t.state.PositionsSet = true
	t.commitLocked(smallBlindSeat, t.state.SmallBlind)
	t.commitLocked(bigBlindSeat, t.state.BigBlind)
	t.state.CurrentBet = max(t.state.Seats[smallBlindSeat].Bet, t.state.Seats[bigBlindSeat].Bet)
	t.setActingLocked(t.nextNeedingActionLocked(bigBlindSeat))
	if t.state.Acting < 0 {
		t.advanceStreetLocked()
	}
	return nil
}

func (t *Table) Act(userID int64, action ActionType, amount int64) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.actLocked(userID, action, amount, false)
}

// Timeout applies the safe automatic action for an expired turn. A stale timer
// is ignored by matching the turn ID captured when that timer was scheduled.
func (t *Table) Timeout(turnID uint64, now time.Time) (ActionType, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.handActiveLocked() || t.state.Acting < 0 || turnID != t.state.TurnID {
		return "", false, nil
	}
	if t.state.ActionDeadlineAt <= 0 || now.UnixMilli() < t.state.ActionDeadlineAt {
		return "", false, nil
	}
	player := t.state.Seats[t.state.Acting]
	if player == nil {
		return "", false, errors.New("acting seat is empty")
	}
	action := ActionFold
	if max(0, t.state.CurrentBet-player.Bet) == 0 {
		action = ActionCheck
	}
	if err := t.actLocked(player.UserID, action, 0, true); err != nil {
		return "", false, err
	}
	return action, true, nil
}

func (t *Table) actLocked(userID int64, action ActionType, amount int64, ignoreDeadline bool) error {
	seat := t.seatForUserLocked(userID)
	if seat < 0 {
		return ErrNotSeated
	}
	if !t.handActiveLocked() {
		return errors.New("no active hand")
	}
	if seat != t.state.Acting {
		return ErrNotYourTurn
	}
	if !ignoreDeadline && t.state.ActionDeadlineAt > 0 && !t.currentTimeLocked().Before(time.UnixMilli(t.state.ActionDeadlineAt)) {
		return ErrActionTimedOut
	}
	player := t.state.Seats[seat]
	toCall := max(0, t.state.CurrentBet-player.Bet)
	betBefore := player.Bet
	currentBetBefore := t.state.CurrentBet

	switch action {
	case ActionFold:
		player.Folded = true
		t.state.Acted[seat] = true
	case ActionCheck:
		if toCall != 0 {
			return errors.New("cannot check while facing a bet")
		}
		t.state.Acted[seat] = true
	case ActionCall:
		if toCall == 0 {
			return errors.New("nothing to call")
		}
		t.commitLocked(seat, toCall)
		t.state.Acted[seat] = true
	case ActionBet:
		if t.state.CurrentBet != 0 {
			return errors.New("use raise while facing a bet")
		}
		if err := t.raiseToLocked(seat, amount); err != nil {
			return err
		}
	case ActionRaise:
		if t.state.CurrentBet == 0 {
			return errors.New("use bet when no wager exists")
		}
		if err := t.raiseToLocked(seat, amount); err != nil {
			return err
		}
	case ActionAllIn:
		if player.Stack <= 0 {
			return errors.New("player is already all-in")
		}
		target := player.Bet + player.Stack
		if target <= t.state.CurrentBet {
			t.commitLocked(seat, player.Stack)
			t.state.Acted[seat] = true
		} else if err := t.raiseToLocked(seat, target); err != nil {
			return err
		}
	default:
		return errors.New("unknown action")
	}
	player.LastAction = action
	player.LastActionAmount = player.Bet - betBefore
	player.LastActionBetLevel = 0
	if player.Bet > currentBetBefore {
		player.LastActionBetLevel = t.state.BetLevel
	}
	t.state.ActedAtBet[seat] = t.state.CurrentBet

	if t.remainingPlayersLocked() == 1 {
		t.finishUncontestedLocked()
		return nil
	}
	if t.bettingRoundCompleteLocked() {
		t.advanceStreetLocked()
		return nil
	}
	t.setActingLocked(t.nextNeedingActionLocked(seat))
	if t.state.Acting < 0 {
		t.advanceStreetLocked()
	}
	return nil
}

func (t *Table) Snapshot(viewerID int64) Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	viewerSeat := t.seatForUserLocked(viewerID)
	smallBlindSeat, bigBlindSeat := t.blindSeatsLocked()
	players := make([]PlayerView, 0, MaxSeats)
	for seat, player := range t.state.Seats {
		if player == nil {
			continue
		}
		view := PlayerView{
			UserID: player.UserID, Name: player.Name, Seat: seat, Stack: player.Stack,
			Bet: player.Bet, InHand: player.InHand, Folded: player.Folded, AllIn: player.AllIn, Ready: player.Ready,
			IsDealer: seat == t.state.Dealer, IsActing: seat == t.state.Acting,
			LastAction: player.LastAction, LastActionAmount: player.LastActionAmount,
			LastActionBetLevel: player.LastActionBetLevel,
		}
		showdown := t.state.LastResult != nil && t.state.LastResult.Showdown && !player.Folded
		if player.UserID == viewerID || showdown {
			view.Cards = append([]Card(nil), player.Hole...)
		}
		players = append(players, view)
	}
	return Snapshot{
		GameType: GameType,
		ID:       t.state.ID, Name: t.state.Name, SmallBlind: t.state.SmallBlind, BigBlind: t.state.BigBlind,
		HandID: t.state.HandID, Street: t.state.Street, Board: append([]Card(nil), t.state.Board...),
		Pot: t.potLocked(), CurrentBet: t.state.CurrentBet, BetLevel: t.state.BetLevel,
		DealerSeat: t.state.Dealer, SmallBlindSeat: smallBlindSeat, BigBlindSeat: bigBlindSeat,
		ActingSeat: t.state.Acting, ActionTimeoutSeconds: t.state.ActionTimeoutSeconds,
		ActionDeadlineAt: t.state.ActionDeadlineAt, TurnID: t.state.TurnID,
		ViewerSeat: viewerSeat, Players: players,
		Allowed: t.allowedActionsLocked(viewerID), CanStart: viewerSeat >= 0 && t.state.Seats[viewerSeat].Stack > 0 && !t.handActiveLocked() && len(t.playableSeatsLocked()) >= 2,
		CanLeave: viewerSeat >= 0 && !t.handActiveLocked(), LastResult: cloneResult(t.state.LastResult, viewerID),
	}
}

func (t *Table) IsSeated(userID int64) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.seatForUserLocked(userID) >= 0
}

func (t *Table) StackFor(userID int64) (int64, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	seat := t.seatForUserLocked(userID)
	if seat < 0 {
		return 0, false
	}
	return t.state.Seats[seat].Stack, true
}

func (t *Table) raiseToLocked(seat int, target int64) error {
	player := t.state.Seats[seat]
	maximum := player.Bet + player.Stack
	if target <= t.state.CurrentBet || target > maximum {
		return errors.New("raise amount is outside the allowed range")
	}
	if target > t.maxRaiseToLocked(seat) {
		return errors.New("raise amount exceeds what an opponent can match")
	}
	if !t.raiseReopenedLocked(seat) {
		return errors.New("betting was not reopened by the incomplete all-in raise")
	}
	raiseSize := target - t.state.CurrentBet
	isAllIn := target == maximum
	minimum := t.state.MinRaise
	if t.state.CurrentBet == 0 {
		minimum = t.state.BigBlind
	}
	if raiseSize < minimum && !isAllIn {
		return fmt.Errorf("minimum raise is %d cents", minimum)
	}
	oldCurrent := t.state.CurrentBet
	t.commitLocked(seat, target-player.Bet)
	t.state.CurrentBet = target
	fullRaise := target-oldCurrent >= minimum
	if fullRaise {
		t.state.BetLevel++
		t.state.MinRaise = target - oldCurrent
		for other, candidate := range t.state.Seats {
			if candidate != nil && candidate.InHand && !candidate.Folded && !candidate.AllIn {
				t.state.Acted[other] = false
			}
		}
	}
	t.state.Acted[seat] = true
	return nil
}

func (t *Table) commitLocked(seat int, requested int64) int64 {
	player := t.state.Seats[seat]
	amount := min(requested, player.Stack)
	player.Stack -= amount
	player.Bet += amount
	player.Committed += amount
	if player.Stack == 0 {
		player.AllIn = true
	}
	return amount
}

func (t *Table) advanceStreetLocked() {
	for _, player := range t.state.Seats {
		if player != nil {
			player.Bet = 0
		}
	}
	t.state.CurrentBet = 0
	t.state.MinRaise = t.state.BigBlind
	t.state.Acted = [MaxSeats]bool{}
	t.state.ActedAtBet = [MaxSeats]int64{}

	switch t.state.Street {
	case StreetPreflop:
		t.dealNextBoardLocked()
		t.state.Street = StreetFlop
	case StreetFlop:
		t.dealNextBoardLocked()
		t.state.Street = StreetTurn
	case StreetTurn:
		t.dealNextBoardLocked()
		t.state.Street = StreetRiver
	case StreetRiver:
		t.finishShowdownLocked()
		return
	default:
		return
	}
	t.state.BetLevel = 0
	for _, player := range t.state.Seats {
		if player != nil {
			player.LastAction = ""
			player.LastActionAmount = 0
			player.LastActionBetLevel = 0
		}
	}

	if t.actionablePlayersLocked() <= 1 {
		for len(t.state.Board) < 5 {
			t.dealNextBoardLocked()
		}
		t.finishShowdownLocked()
		return
	}
	t.setActingLocked(t.nextNeedingActionLocked(t.state.Dealer))
	if t.state.Acting < 0 {
		for len(t.state.Board) < 5 {
			t.dealNextBoardLocked()
		}
		t.finishShowdownLocked()
	}
}

func (t *Table) blindSeatsLocked() (int, int) {
	if t.state.PositionsSet {
		return t.state.SmallBlindSeat, t.state.BigBlindSeat
	}
	if !t.handActiveLocked() || t.state.Dealer < 0 {
		return -1, -1
	}
	smallBlindSeat := t.nextInHandSeatLocked(t.state.Dealer)
	if t.remainingPlayersLocked() == 2 {
		smallBlindSeat = t.state.Dealer
	}
	return smallBlindSeat, t.nextInHandSeatLocked(smallBlindSeat)
}

func (t *Table) finishUncontestedLocked() {
	refunds := t.refundUncalledBetLocked()
	pot := t.potLocked()
	payouts := make(map[int64]int64)
	message := "Hand complete"
	for _, player := range t.state.Seats {
		if player != nil && player.InHand && !player.Folded {
			player.Stack += pot
			payouts[player.UserID] = pot
			message = player.Name + " wins"
			break
		}
	}
	t.state.LastResult = t.handResultLocked(pot, message, false, payouts, refunds)
	t.finishHandLocked()
}

func (t *Table) finishShowdownLocked() {
	refunds := t.refundUncalledBetLocked()
	pot := t.potLocked()
	payouts := t.awardSidePotsLocked()
	winnerNames := make([]string, 0, len(payouts))
	for userID, amount := range payouts {
		for _, player := range t.state.Seats {
			if player != nil && player.UserID == userID {
				player.Stack += amount
				winnerNames = append(winnerNames, player.Name)
				break
			}
		}
	}
	sort.Strings(winnerNames)
	message := "Showdown"
	if len(winnerNames) > 0 {
		message = fmt.Sprintf("%s win at showdown", joinNames(winnerNames))
	}
	t.state.LastResult = t.handResultLocked(pot, message, true, payouts, refunds)
	t.finishHandLocked()
}

func (t *Table) refundUncalledBetLocked() map[int64]int64 {
	highest := int64(0)
	second := int64(0)
	highestSeat := -1
	for seat, player := range t.state.Seats {
		if player == nil || player.Committed <= 0 {
			continue
		}
		switch {
		case player.Committed > highest:
			second = highest
			highest = player.Committed
			highestSeat = seat
		case player.Committed == highest:
			second = highest
			highestSeat = -1
		case player.Committed > second:
			second = player.Committed
		}
	}
	if highestSeat < 0 || highest <= second {
		return nil
	}
	player := t.state.Seats[highestSeat]
	amount := highest - second
	player.Committed -= amount
	player.Bet -= min(player.Bet, amount)
	player.Stack += amount
	return map[int64]int64{player.UserID: amount}
}

func (t *Table) handResultLocked(pot int64, message string, showdown bool, payouts, refunds map[int64]int64) *HandResult {
	players := make([]HandPlayerResult, 0, MaxSeats)
	for seat, player := range t.state.Seats {
		if player == nil || !player.InHand {
			continue
		}
		payout := payouts[player.UserID]
		startingStack := player.Stack + player.Committed - payout
		players = append(players, HandPlayerResult{
			UserID: player.UserID, Name: player.Name, Seat: seat, Cards: append([]Card(nil), player.Hole...),
			Folded: player.Folded, StartingStack: startingStack, Committed: player.Committed,
			Payout: payout, Refund: refunds[player.UserID], EndingStack: player.Stack, Net: player.Stack - startingStack,
		})
	}
	return &HandResult{
		HandID: t.state.HandID, Pot: pot, Message: message, Showdown: showdown,
		Board: append([]Card(nil), t.state.Board...), Payouts: payouts, Refunds: refunds, Players: players,
	}
}

func (t *Table) awardSidePotsLocked() map[int64]int64 {
	levels := make([]int64, 0, MaxSeats)
	seen := map[int64]bool{}
	for _, player := range t.state.Seats {
		if player != nil && player.Committed > 0 && !seen[player.Committed] {
			seen[player.Committed] = true
			levels = append(levels, player.Committed)
		}
	}
	sort.Slice(levels, func(i, j int) bool { return levels[i] < levels[j] })
	payouts := make(map[int64]int64)
	previous := int64(0)
	for _, level := range levels {
		contributors := 0
		eligible := make([]int, 0, MaxSeats)
		for seat, player := range t.state.Seats {
			if player == nil || player.Committed < level {
				continue
			}
			contributors++
			if player.InHand && !player.Folded {
				eligible = append(eligible, seat)
			}
		}
		amount := (level - previous) * int64(contributors)
		previous = level
		if amount == 0 || len(eligible) == 0 {
			continue
		}
		best := uint64(0)
		winners := make([]int, 0, len(eligible))
		for _, seat := range eligible {
			player := t.state.Seats[seat]
			cards := append(append([]Card(nil), player.Hole...), t.state.Board...)
			score := Evaluate(cards).Score
			switch {
			case score > best:
				best = score
				winners = []int{seat}
			case score == best:
				winners = append(winners, seat)
			}
		}
		share := amount / int64(len(winners))
		remainder := amount % int64(len(winners))
		sort.Slice(winners, func(i, j int) bool {
			return seatDistanceLeftOfButton(t.state.Dealer, winners[i]) < seatDistanceLeftOfButton(t.state.Dealer, winners[j])
		})
		for _, seat := range winners {
			award := share
			if remainder > 0 {
				award++
				remainder--
			}
			payouts[t.state.Seats[seat].UserID] += award
		}
	}
	return payouts
}

func (t *Table) finishHandLocked() {
	for _, player := range t.state.Seats {
		if player == nil {
			continue
		}
		player.Bet = 0
		player.Committed = 0
		player.Ready = false
		player.InHand = false
		player.AllIn = false
	}
	t.state.CurrentBet = 0
	t.setActingLocked(-1)
	t.state.Street = StreetComplete
	t.state.Acted = [MaxSeats]bool{}
	t.state.ActedAtBet = [MaxSeats]int64{}
}

func (t *Table) setActingLocked(seat int) {
	t.state.Acting = seat
	t.state.TurnID++
	if seat < 0 {
		t.state.ActionDeadlineAt = 0
		return
	}
	t.state.ActionDeadlineAt = t.currentTimeLocked().Add(time.Duration(t.state.ActionTimeoutSeconds) * time.Second).UnixMilli()
}

func (t *Table) currentTimeLocked() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

func (t *Table) allowedActionsLocked(userID int64) AllowedActions {
	seat := t.seatForUserLocked(userID)
	if seat < 0 || seat != t.state.Acting || !t.handActiveLocked() {
		return AllowedActions{}
	}
	player := t.state.Seats[seat]
	toCall := max(0, t.state.CurrentBet-player.Bet)
	maximum := player.Bet + player.Stack
	maxRaiseTo := t.maxRaiseToLocked(seat)
	minimum := t.state.CurrentBet + t.state.MinRaise
	if t.state.CurrentBet == 0 {
		minimum = t.state.BigBlind
	}
	minimum = min(minimum, maxRaiseTo)
	raiseReopened := t.raiseReopenedLocked(seat)
	canBet := t.state.CurrentBet == 0 && maxRaiseTo > 0 && (maxRaiseTo >= t.state.BigBlind || maxRaiseTo == maximum)
	canRaise := t.state.CurrentBet > 0 && maxRaiseTo > t.state.CurrentBet && raiseReopened && (maxRaiseTo-t.state.CurrentBet >= t.state.MinRaise || maxRaiseTo == maximum)
	return AllowedActions{
		CanAct: true, CanFold: true, CanCheck: toCall == 0, CanCall: toCall > 0,
		CanBet:   canBet,
		CanRaise: canRaise,
		CanAllIn: player.Stack > 0 && (maximum <= t.state.CurrentBet || (maximum <= maxRaiseTo && raiseReopened)), ToCall: min(toCall, player.Stack),
		MinRaiseTo: minimum, MaxRaiseTo: maxRaiseTo,
	}
}

func (t *Table) maxRaiseToLocked(seat int) int64 {
	player := t.state.Seats[seat]
	maximum := player.Bet + player.Stack
	opponentMaximum := int64(0)
	for otherSeat, opponent := range t.state.Seats {
		if otherSeat == seat || opponent == nil || !opponent.InHand || opponent.Folded {
			continue
		}
		opponentMaximum = max(opponentMaximum, opponent.Bet+opponent.Stack)
	}
	return min(maximum, opponentMaximum)
}

func (t *Table) raiseReopenedLocked(seat int) bool {
	if !t.state.Acted[seat] {
		return true
	}
	return t.state.CurrentBet-t.state.ActedAtBet[seat] >= t.state.MinRaise
}

func (t *Table) bettingRoundCompleteLocked() bool {
	for seat, player := range t.state.Seats {
		if player == nil || !player.InHand || player.Folded || player.AllIn {
			continue
		}
		if !t.state.Acted[seat] || player.Bet != t.state.CurrentBet {
			return false
		}
	}
	return true
}

func (t *Table) nextNeedingActionLocked(from int) int {
	for offset := 1; offset <= MaxSeats; offset++ {
		seat := (from + offset + MaxSeats) % MaxSeats
		player := t.state.Seats[seat]
		if player == nil || !player.InHand || player.Folded || player.AllIn {
			continue
		}
		if !t.state.Acted[seat] || player.Bet != t.state.CurrentBet {
			return seat
		}
	}
	return -1
}

func (t *Table) nextPlayableSeatLocked(from int) int {
	for offset := 1; offset <= MaxSeats; offset++ {
		seat := (from + offset + MaxSeats) % MaxSeats
		if player := t.state.Seats[seat]; player != nil && player.Stack > 0 {
			return seat
		}
	}
	return -1
}

func (t *Table) nextInHandSeatLocked(from int) int {
	for offset := 1; offset <= MaxSeats; offset++ {
		seat := (from + offset + MaxSeats) % MaxSeats
		if player := t.state.Seats[seat]; player != nil && player.InHand && !player.Folded {
			return seat
		}
	}
	return -1
}

func (t *Table) playableSeatsLocked() []int {
	seats := make([]int, 0, MaxSeats)
	for seat, player := range t.state.Seats {
		if player != nil && player.Stack > 0 {
			seats = append(seats, seat)
		}
	}
	return seats
}

func (t *Table) clearReadyLocked() {
	for _, player := range t.state.Seats {
		if player != nil {
			player.Ready = false
		}
	}
}

func (t *Table) seatForUserLocked(userID int64) int {
	for seat, player := range t.state.Seats {
		if player != nil && player.UserID == userID {
			return seat
		}
	}
	return -1
}

func (t *Table) handActiveLocked() bool {
	return t.state.Street == StreetPreflop || t.state.Street == StreetFlop || t.state.Street == StreetTurn || t.state.Street == StreetRiver
}

func (t *Table) remainingPlayersLocked() int {
	count := 0
	for _, player := range t.state.Seats {
		if player != nil && player.InHand && !player.Folded {
			count++
		}
	}
	return count
}

func (t *Table) actionablePlayersLocked() int {
	count := 0
	for _, player := range t.state.Seats {
		if player != nil && player.InHand && !player.Folded && !player.AllIn {
			count++
		}
	}
	return count
}

func (t *Table) potLocked() int64 {
	var pot int64
	for _, player := range t.state.Seats {
		if player != nil {
			pot += player.Committed
		}
	}
	return pot
}

func (t *Table) drawLocked() Card {
	card := t.state.Deck[t.state.DeckPos]
	t.state.DeckPos++
	return card
}

func (t *Table) drawBoardLocked(count int) {
	for i := 0; i < count; i++ {
		t.state.Board = append(t.state.Board, t.drawLocked())
	}
}

func (t *Table) dealNextBoardLocked() {
	t.state.DeckPos++ // burn one card before the flop, turn, and river
	count := 1
	if len(t.state.Board) == 0 {
		count = 3
	}
	t.drawBoardLocked(count)
}

func cloneResult(result *HandResult, viewerID int64) *HandResult {
	if result == nil {
		return nil
	}
	clone := *result
	clone.Board = append([]Card(nil), result.Board...)
	clone.Payouts = make(map[int64]int64, len(result.Payouts))
	for userID, amount := range result.Payouts {
		clone.Payouts[userID] = amount
	}
	clone.Refunds = make(map[int64]int64, len(result.Refunds))
	for userID, amount := range result.Refunds {
		clone.Refunds[userID] = amount
	}
	clone.Players = make([]HandPlayerResult, len(result.Players))
	for index, player := range result.Players {
		clone.Players[index] = player
		if player.UserID == viewerID || (result.Showdown && !player.Folded) {
			clone.Players[index].Cards = append([]Card(nil), player.Cards...)
		} else {
			clone.Players[index].Cards = nil
		}
	}
	return &clone
}

func seatDistanceLeftOfButton(button, seat int) int {
	distance := (seat - button + MaxSeats) % MaxSeats
	if distance == 0 {
		return MaxSeats
	}
	return distance
}

func joinNames(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	result := ""
	for i, name := range names {
		if i > 0 {
			result += " & "
		}
		result += name
	}
	return result
}
