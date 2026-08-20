package landlord

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	GameType                    = "landlord"
	MaxSeats                    = 3
	DefaultActionTimeoutSeconds = 25
	MinActionTimeoutSeconds     = 5
	MaxActionTimeoutSeconds     = 300
)

type Phase string

const (
	PhaseWaiting  Phase = "waiting"
	PhaseBidding  Phase = "bidding"
	PhasePlaying  Phase = "playing"
	PhaseComplete Phase = "complete"
)

type ActionType string

const (
	ActionBid  ActionType = "bid"
	ActionPlay ActionType = "play"
	ActionPass ActionType = "pass"
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
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	Seat     int    `json:"seat"`
	Stack    int64  `json:"stack_cents"`
	Hand     []Card `json:"hand,omitempty"`
	Ready    bool   `json:"ready"`
	Landlord bool   `json:"landlord"`
	Bid      int    `json:"bid"`
	Plays    int    `json:"plays"`
}

type HandResult struct {
	HandID     int64           `json:"hand_id"`
	Winner     string          `json:"winner"`
	WinnerSeat int             `json:"winner_seat"`
	Bid        int             `json:"bid"`
	Multiplier int             `json:"multiplier"`
	Stake      int64           `json:"stake_cents"`
	Message    string          `json:"message"`
	Payouts    map[int64]int64 `json:"payouts"`
}

type TableState struct {
	GameType             string            `json:"game_type"`
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	BaseStake            int64             `json:"base_stake_cents"`
	HandID               int64             `json:"hand_id"`
	Phase                Phase             `json:"phase"`
	Dealer               int               `json:"dealer"`
	Acting               int               `json:"acting"`
	Seats                [MaxSeats]*Player `json:"seats"`
	Bottom               []Card            `json:"bottom"`
	HighestBid           int               `json:"highest_bid"`
	HighestBidder        int               `json:"highest_bidder"`
	BidCount             int               `json:"bid_count"`
	LandlordSeat         int               `json:"landlord_seat"`
	LastPlay             []Card            `json:"last_play"`
	LastCombination      Combination       `json:"last_combination"`
	LastPlaySeat         int               `json:"last_play_seat"`
	TrickOpen            bool              `json:"trick_open"`
	PassCount            int               `json:"pass_count"`
	Multiplier           int               `json:"multiplier"`
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

type PlayerView struct {
	UserID    int64  `json:"user_id"`
	Name      string `json:"name"`
	Seat      int    `json:"seat"`
	Stack     int64  `json:"stack_cents"`
	Cards     []Card `json:"cards,omitempty"`
	CardCount int    `json:"card_count"`
	Ready     bool   `json:"ready"`
	Landlord  bool   `json:"landlord"`
	Bid       int    `json:"bid"`
	IsActing  bool   `json:"is_acting"`
}

type AllowedActions struct {
	CanAct  bool `json:"can_act"`
	CanBid  bool `json:"can_bid"`
	MinBid  int  `json:"min_bid"`
	CanPlay bool `json:"can_play"`
	CanPass bool `json:"can_pass"`
}

type Snapshot struct {
	GameType             string         `json:"game_type"`
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	BaseStake            int64          `json:"base_stake_cents"`
	HandID               int64          `json:"hand_id"`
	Phase                Phase          `json:"phase"`
	ActingSeat           int            `json:"acting_seat"`
	LandlordSeat         int            `json:"landlord_seat"`
	HighestBid           int            `json:"highest_bid"`
	Bottom               []Card         `json:"bottom"`
	LastPlay             []Card         `json:"last_play"`
	LastCombination      Combination    `json:"last_combination"`
	LastPlaySeat         int            `json:"last_play_seat"`
	TrickOpen            bool           `json:"trick_open"`
	Multiplier           int            `json:"multiplier"`
	ActionTimeoutSeconds int            `json:"action_timeout_seconds"`
	ActionDeadlineAt     int64          `json:"action_deadline_at"`
	TurnID               uint64         `json:"turn_id"`
	ViewerSeat           int            `json:"viewer_seat"`
	Players              []PlayerView   `json:"players"`
	RemainingCounts      map[int]int    `json:"remaining_counts"`
	Allowed              AllowedActions `json:"allowed_actions"`
	CanStart             bool           `json:"can_start"`
	CanLeave             bool           `json:"can_leave"`
	LastResult           *HandResult    `json:"last_result,omitempty"`
}

func NewTable(id, name string, baseStake int64) *Table {
	return &Table{state: TableState{
		GameType: GameType, ID: id, Name: name, BaseStake: baseStake, Phase: PhaseWaiting,
		Dealer: -1, Acting: -1, HighestBidder: -1, LandlordSeat: -1, LastPlaySeat: -1,
		Multiplier: 1, ActionTimeoutSeconds: DefaultActionTimeoutSeconds,
	}, now: time.Now}
}

func RestoreTable(data []byte) (*Table, error) {
	var state TableState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode landlord table state: %w", err)
	}
	if state.GameType != GameType || state.ID == "" || state.BaseStake <= 0 {
		return nil, errors.New("invalid persisted landlord table state")
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
	if t.handActiveLocked() {
		return -1, ErrHandInProgress
	}
	if t.seatForUserLocked(userID) >= 0 {
		return -1, ErrAlreadySeated
	}
	for seat := range t.state.Seats {
		if t.state.Seats[seat] == nil {
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
	if t.state.Seats[seat].Ready {
		t.state.Seats[seat].Ready = false
		return false, nil
	}
	t.state.Seats[seat].Ready = true
	if len(t.playableSeatsLocked()) != MaxSeats {
		return false, nil
	}
	for _, player := range t.state.Seats {
		if player == nil || !player.Ready {
			return false, nil
		}
	}
	if err := t.startHandLocked(); err != nil {
		t.state.Seats[seat].Ready = false
		return false, err
	}
	return true, nil
}

func (t *Table) Act(userID int64, action ActionType, bid int, cards []Card) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.actLocked(userID, action, bid, cards, false)
}

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
	action := ActionPass
	var cards []Card
	if t.state.Phase == PhaseBidding {
		action = ActionBid
	} else if !t.state.TrickOpen || t.state.LastPlaySeat == t.state.Acting {
		action = ActionPlay
		cards = []Card{player.Hand[0]}
	}
	if err := t.actLocked(player.UserID, action, 0, cards, true); err != nil {
		return "", false, err
	}
	return action, true, nil
}

func (t *Table) Snapshot(viewerID int64) Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	viewerSeat := t.seatForUserLocked(viewerID)
	players := make([]PlayerView, 0, MaxSeats)
	for seat, player := range t.state.Seats {
		if player == nil {
			continue
		}
		view := PlayerView{UserID: player.UserID, Name: player.Name, Seat: seat, Stack: player.Stack, CardCount: len(player.Hand), Ready: player.Ready, Landlord: player.Landlord, Bid: player.Bid, IsActing: seat == t.state.Acting}
		if player.UserID == viewerID || t.state.Phase == PhaseComplete {
			view.Cards = append([]Card(nil), player.Hand...)
		}
		players = append(players, view)
	}
	bottom := make([]Card, 0, len(t.state.Bottom))
	if t.state.LandlordSeat >= 0 || t.state.Phase == PhaseComplete {
		bottom = append(bottom, t.state.Bottom...)
	}
	remainingCounts := make(map[int]int)
	if viewerSeat >= 0 && t.state.Phase != PhaseWaiting {
		for seat, player := range t.state.Seats {
			if player == nil || seat == viewerSeat {
				continue
			}
			for _, card := range player.Hand {
				remainingCounts[int(card.Rank)]++
			}
		}
		if t.state.LandlordSeat < 0 {
			for _, card := range t.state.Bottom {
				remainingCounts[int(card.Rank)]++
			}
		}
	}
	return Snapshot{
		GameType: GameType, ID: t.state.ID, Name: t.state.Name, BaseStake: t.state.BaseStake,
		HandID: t.state.HandID, Phase: t.state.Phase, ActingSeat: t.state.Acting, LandlordSeat: t.state.LandlordSeat,
		HighestBid: t.state.HighestBid, Bottom: bottom, LastPlay: append([]Card{}, t.state.LastPlay...), LastCombination: t.state.LastCombination,
		LastPlaySeat: t.state.LastPlaySeat, TrickOpen: t.state.TrickOpen, Multiplier: t.state.Multiplier,
		ActionTimeoutSeconds: t.state.ActionTimeoutSeconds, ActionDeadlineAt: t.state.ActionDeadlineAt, TurnID: t.state.TurnID,
		ViewerSeat: viewerSeat, Players: players, RemainingCounts: remainingCounts, Allowed: t.allowedActionsLocked(viewerID),
		CanStart: viewerSeat >= 0 && t.state.Seats[viewerSeat].Stack > 0 && !t.handActiveLocked(),
		CanLeave: viewerSeat >= 0 && !t.handActiveLocked(), LastResult: cloneResult(t.state.LastResult),
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

func (t *Table) startHandLocked() error {
	deck, err := shuffledDeck()
	if err != nil {
		return err
	}
	return t.dealLocked(deck)
}

func (t *Table) dealLocked(deck []Card) error {
	if len(deck) != 54 {
		return errors.New("landlord deck must contain 54 cards")
	}
	if len(t.playableSeatsLocked()) != MaxSeats {
		return errors.New("斗地主需要正好三名有筹码的玩家")
	}
	t.state.HandID++
	t.state.Phase = PhaseBidding
	t.state.Bottom = append([]Card(nil), deck[51:]...)
	t.state.HighestBid = 0
	t.state.HighestBidder = -1
	t.state.BidCount = 0
	t.state.LandlordSeat = -1
	t.state.LastPlay = nil
	t.state.LastCombination = Combination{}
	t.state.LastPlaySeat = -1
	t.state.TrickOpen = false
	t.state.PassCount = 0
	t.state.Multiplier = 1
	t.state.LastResult = nil
	t.state.Dealer = t.nextSeatLocked(t.state.Dealer)
	for seat, player := range t.state.Seats {
		player.Ready = false
		player.Hand = nil
		player.Landlord = false
		player.Bid = 0
		player.Plays = 0
		for index := seat; index < 51; index += MaxSeats {
			player.Hand = append(player.Hand, deck[index])
		}
		sortCards(player.Hand)
	}
	t.setActingLocked(t.state.Dealer)
	return nil
}

func (t *Table) actLocked(userID int64, action ActionType, bid int, cards []Card, ignoreDeadline bool) error {
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
	if t.state.Phase == PhaseBidding {
		if action == ActionPass {
			action = ActionBid
			bid = 0
		}
		if action != ActionBid {
			return errors.New("叫分阶段只能叫分或不叫")
		}
		return t.bidLocked(seat, bid)
	}
	if action == ActionPass {
		return t.passLocked(seat)
	}
	if action != ActionPlay {
		return errors.New("出牌阶段只能出牌或不出")
	}
	return t.playLocked(seat, cards)
}

func (t *Table) bidLocked(seat, bid int) error {
	if bid < 0 || bid > 3 || (bid > 0 && bid <= t.state.HighestBid) {
		return errors.New("叫分必须是不叫，或高于当前叫分的 1–3 分")
	}
	t.state.Seats[seat].Bid = bid
	t.state.BidCount++
	if bid > t.state.HighestBid {
		t.state.HighestBid = bid
		t.state.HighestBidder = seat
	}
	if bid == 3 || t.state.BidCount == MaxSeats {
		if t.state.HighestBidder < 0 {
			deck, err := shuffledDeck()
			if err != nil {
				return err
			}
			return t.dealLocked(deck)
		}
		t.finalizeLandlordLocked()
		return nil
	}
	t.setActingLocked(t.nextSeatLocked(seat))
	return nil
}

func (t *Table) finalizeLandlordLocked() {
	seat := t.state.HighestBidder
	t.state.LandlordSeat = seat
	player := t.state.Seats[seat]
	player.Landlord = true
	player.Hand = append(player.Hand, t.state.Bottom...)
	sortCards(player.Hand)
	t.state.Phase = PhasePlaying
	t.state.TrickOpen = false
	t.state.PassCount = 0
	t.setActingLocked(seat)
}

func (t *Table) playLocked(seat int, cards []Card) error {
	if len(cards) == 0 {
		return ErrInvalidCombination
	}
	player := t.state.Seats[seat]
	if !containsCards(player.Hand, cards) {
		return errors.New("所选牌不在你的手牌中")
	}
	combination, err := Classify(cards)
	if err != nil {
		return err
	}
	if t.state.TrickOpen && t.state.LastPlaySeat != seat && !Beats(combination, t.state.LastCombination) {
		return errors.New("所选牌型不能压过上一手")
	}
	player.Hand = removeCards(player.Hand, cards)
	player.Plays++
	t.state.LastPlay = append([]Card(nil), cards...)
	sortCards(t.state.LastPlay)
	t.state.LastCombination = combination
	t.state.LastPlaySeat = seat
	t.state.TrickOpen = true
	t.state.PassCount = 0
	if combination.Kind == CombinationBomb || combination.Kind == CombinationRocket {
		t.state.Multiplier *= 2
	}
	if len(player.Hand) == 0 {
		t.finishLocked(seat)
		return nil
	}
	t.setActingLocked(t.nextSeatLocked(seat))
	return nil
}

func (t *Table) passLocked(seat int) error {
	if !t.state.TrickOpen || t.state.LastPlaySeat == seat {
		return errors.New("你是本轮首家，不能不出")
	}
	t.state.PassCount++
	if t.state.PassCount >= MaxSeats-1 {
		leader := t.state.LastPlaySeat
		t.state.TrickOpen = false
		t.state.PassCount = 0
		t.setActingLocked(leader)
		return nil
	}
	t.setActingLocked(t.nextSeatLocked(seat))
	return nil
}

func (t *Table) finishLocked(winnerSeat int) {
	landlordWon := winnerSeat == t.state.LandlordSeat
	if landlordWon {
		farmersPlayed := 0
		for seat, player := range t.state.Seats {
			if seat != t.state.LandlordSeat {
				farmersPlayed += player.Plays
			}
		}
		if farmersPlayed == 0 {
			t.state.Multiplier *= 2
		}
	} else if t.state.Seats[t.state.LandlordSeat].Plays == 1 {
		t.state.Multiplier *= 2
	}
	unit := t.state.BaseStake * int64(t.state.HighestBid) * int64(t.state.Multiplier)
	payouts := make(map[int64]int64, MaxSeats)
	landlord := t.state.Seats[t.state.LandlordSeat]
	if landlordWon {
		for seat, farmer := range t.state.Seats {
			if seat == t.state.LandlordSeat {
				continue
			}
			paid := min(unit, farmer.Stack)
			farmer.Stack -= paid
			landlord.Stack += paid
			payouts[farmer.UserID] -= paid
			payouts[landlord.UserID] += paid
		}
	} else {
		available := min(landlord.Stack, unit*2)
		farmers := make([]*Player, 0, 2)
		for seat, farmer := range t.state.Seats {
			if seat != t.state.LandlordSeat {
				farmers = append(farmers, farmer)
			}
		}
		payments := []int64{available / 2, available - available/2}
		for index, farmer := range farmers {
			paid := min(unit, payments[index])
			landlord.Stack -= paid
			farmer.Stack += paid
			payouts[landlord.UserID] -= paid
			payouts[farmer.UserID] += paid
		}
	}
	winner := "农民"
	if landlordWon {
		winner = "地主"
	}
	t.state.LastResult = &HandResult{
		HandID: t.state.HandID, Winner: winner, WinnerSeat: winnerSeat, Bid: t.state.HighestBid,
		Multiplier: t.state.Multiplier, Stake: unit, Message: fmt.Sprintf("%s获胜，%d 分 × %d 倍", winner, t.state.HighestBid, t.state.Multiplier), Payouts: payouts,
	}
	for _, player := range t.state.Seats {
		player.Ready = false
	}
	t.state.Phase = PhaseComplete
	t.state.Acting = -1
	t.state.ActionDeadlineAt = 0
}

func (t *Table) allowedActionsLocked(viewerID int64) AllowedActions {
	seat := t.seatForUserLocked(viewerID)
	if seat < 0 || seat != t.state.Acting || !t.handActiveLocked() {
		return AllowedActions{}
	}
	allowed := AllowedActions{CanAct: true}
	if t.state.Phase == PhaseBidding {
		allowed.CanBid = t.state.HighestBid < 3
		allowed.MinBid = t.state.HighestBid + 1
		allowed.CanPass = true
		return allowed
	}
	allowed.CanPlay = true
	allowed.CanPass = t.state.TrickOpen && t.state.LastPlaySeat != seat
	return allowed
}

func (t *Table) setActingLocked(seat int) {
	t.state.Acting = seat
	t.state.TurnID++
	t.state.ActionDeadlineAt = t.currentTimeLocked().Add(time.Duration(t.state.ActionTimeoutSeconds) * time.Second).UnixMilli()
}

func (t *Table) nextSeatLocked(seat int) int {
	for offset := 1; offset <= MaxSeats; offset++ {
		next := (seat + offset + MaxSeats) % MaxSeats
		if t.state.Seats[next] != nil && t.state.Seats[next].Stack > 0 {
			return next
		}
	}
	return -1
}

func (t *Table) seatForUserLocked(userID int64) int {
	for seat, player := range t.state.Seats {
		if player != nil && player.UserID == userID {
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

func (t *Table) handActiveLocked() bool {
	return t.state.Phase == PhaseBidding || t.state.Phase == PhasePlaying
}

func (t *Table) clearReadyLocked() {
	for _, player := range t.state.Seats {
		if player != nil {
			player.Ready = false
		}
	}
}

func (t *Table) currentTimeLocked() time.Time {
	if t.now == nil {
		return time.Now()
	}
	return t.now()
}

func containsCards(hand, selected []Card) bool {
	counts := make(map[Card]int, len(hand))
	for _, card := range hand {
		counts[card]++
	}
	for _, card := range selected {
		if counts[card] == 0 {
			return false
		}
		counts[card]--
	}
	return true
}

func removeCards(hand, selected []Card) []Card {
	remove := make(map[Card]int, len(selected))
	for _, card := range selected {
		remove[card]++
	}
	remaining := make([]Card, 0, len(hand)-len(selected))
	for _, card := range hand {
		if remove[card] > 0 {
			remove[card]--
			continue
		}
		remaining = append(remaining, card)
	}
	return remaining
}

func cloneResult(result *HandResult) *HandResult {
	if result == nil {
		return nil
	}
	copy := *result
	copy.Payouts = make(map[int64]int64, len(result.Payouts))
	for userID, amount := range result.Payouts {
		copy.Payouts[userID] = amount
	}
	return &copy
}
