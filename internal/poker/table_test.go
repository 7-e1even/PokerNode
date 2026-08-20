package poker

import (
	"encoding/json"
	"math/rand"
	"testing"
	"time"
)

func TestHeadsUpFoldAwardsBlinds(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	if _, err := table.Join(1, "Alice", 10_000); err != nil {
		t.Fatal(err)
	}
	if _, err := table.Join(2, "Bob", 10_000); err != nil {
		t.Fatal(err)
	}
	if err := table.StartHand(1); err != nil {
		t.Fatal(err)
	}
	snapshot := table.Snapshot(1)
	if snapshot.ActingSeat != snapshot.ViewerSeat {
		t.Fatalf("heads-up dealer should act first preflop: acting=%d viewer=%d", snapshot.ActingSeat, snapshot.ViewerSeat)
	}
	if snapshot.SmallBlindSeat != snapshot.DealerSeat || snapshot.BigBlindSeat == snapshot.DealerSeat {
		t.Fatalf("heads-up dealer must post the small blind: dealer=%d small=%d big=%d", snapshot.DealerSeat, snapshot.SmallBlindSeat, snapshot.BigBlindSeat)
	}
	if err := table.Act(1, ActionFold, 0); err != nil {
		t.Fatal(err)
	}
	alice, _ := table.StackFor(1)
	bob, _ := table.StackFor(2)
	if alice != 9_950 || bob != 10_050 {
		t.Fatalf("unexpected stacks Alice=%d Bob=%d", alice, bob)
	}
	if table.Snapshot(1).Street != StreetComplete {
		t.Fatal("hand should be complete")
	}
}

func TestAllFundedPlayersMustReadyBeforeHandStarts(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	_, _ = table.Join(1, "Alice", 10_000)
	_, _ = table.Join(2, "Bob", 10_000)
	_, _ = table.Join(3, "Cara", 10_000)

	started, err := table.Ready(1)
	if err != nil || started {
		t.Fatalf("first ready should wait: started=%v err=%v", started, err)
	}
	started, err = table.Ready(2)
	if err != nil || started {
		t.Fatalf("second ready should wait: started=%v err=%v", started, err)
	}
	if snapshot := table.Snapshot(1); snapshot.Street != StreetWaiting || !snapshot.Players[0].Ready || !snapshot.Players[1].Ready {
		t.Fatalf("table should remain waiting and expose ready players: %+v", snapshot)
	}

	started, err = table.Ready(3)
	if err != nil || !started {
		t.Fatalf("last ready should start the hand: started=%v err=%v", started, err)
	}
	snapshot := table.Snapshot(1)
	if snapshot.Street != StreetPreflop {
		t.Fatalf("expected preflop after everyone readied, got %s", snapshot.Street)
	}
	for _, player := range snapshot.Players {
		if player.Ready {
			t.Fatalf("ready state should reset when the hand starts: %+v", player)
		}
	}
}

func TestHandCompletionClearsReadyState(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	_, _ = table.Join(1, "Alice", 10_000)
	_, _ = table.Join(2, "Bob", 10_000)
	if err := table.StartHand(1); err != nil {
		t.Fatal(err)
	}

	// A completed hand must clear even a stale ready flag restored mid-hand.
	for _, player := range table.state.Seats {
		if player != nil {
			player.Ready = true
		}
	}
	actor := table.Snapshot(1).Players[table.Snapshot(1).ActingSeat].UserID
	if err := table.Act(actor, ActionFold, 0); err != nil {
		t.Fatal(err)
	}
	for _, player := range table.Snapshot(1).Players {
		if player.Ready {
			t.Fatalf("ready state should reset when the hand completes: %+v", player)
		}
	}
}

func TestSeatChangesClearReadyState(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	_, _ = table.Join(1, "Alice", 10_000)
	_, _ = table.Join(2, "Bob", 10_000)
	if _, err := table.Ready(1); err != nil {
		t.Fatal(err)
	}
	if _, err := table.Join(3, "Cara", 10_000); err != nil {
		t.Fatal(err)
	}
	for _, player := range table.Snapshot(1).Players {
		if player.Ready {
			t.Fatalf("joining should clear existing readiness: %+v", player)
		}
	}
}

func TestPlayerCanJoinOpenSeatDuringHandForNextDeal(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	_, _ = table.Join(1, "Alice", 10_000)
	_, _ = table.Join(2, "Bob", 10_000)
	if err := table.StartHand(1); err != nil {
		t.Fatal(err)
	}
	before := table.Snapshot(1)
	if _, err := table.Join(3, "Cara", 10_000); err != nil {
		t.Fatalf("joining an open seat during a hand: %v", err)
	}
	after := table.Snapshot(3)
	if after.ViewerSeat < 0 || after.Street != before.Street || after.ActingSeat != before.ActingSeat {
		t.Fatalf("joining should reserve a seat without changing the active hand: before=%+v after=%+v", before, after)
	}
	var joined PlayerView
	for _, player := range after.Players {
		if player.UserID == 3 {
			joined = player
		}
	}
	if joined.InHand || len(joined.Cards) != 0 || after.CanStart || after.CanLeave {
		t.Fatalf("new player must wait for the next hand: player=%+v snapshot=%+v", joined, after)
	}

	actorID := int64(0)
	for _, player := range after.Players {
		if player.Seat == after.ActingSeat {
			actorID = player.UserID
		}
	}
	if err := table.Act(actorID, ActionFold, 0); err != nil {
		t.Fatal(err)
	}
	if started, err := table.Ready(3); err != nil || started {
		t.Fatalf("joined player should be able to ready for the next hand: started=%v err=%v", started, err)
	}
}

func TestActionTimeoutFoldsWhenFacingBet(t *testing.T) {
	current := time.Unix(1_800_000_000, 0)
	table := NewTable("main", "Test", 50, 100)
	table.now = func() time.Time { return current }
	if err := table.SetActionTimeoutSeconds(17); err != nil {
		t.Fatal(err)
	}
	_, _ = table.Join(1, "Alice", 10_000)
	_, _ = table.Join(2, "Bob", 10_000)
	if err := table.StartHand(1); err != nil {
		t.Fatal(err)
	}
	snapshot := table.Snapshot(1)
	if snapshot.ActionTimeoutSeconds != 17 || snapshot.ActionDeadlineAt != current.Add(17*time.Second).UnixMilli() || snapshot.TurnID == 0 {
		t.Fatalf("unexpected timeout state: %+v", snapshot)
	}
	if action, applied, err := table.Timeout(snapshot.TurnID, current.Add(16*time.Second)); err != nil || applied || action != "" {
		t.Fatalf("turn must not expire early: action=%s applied=%v err=%v", action, applied, err)
	}
	current = current.Add(17 * time.Second)
	action, applied, err := table.Timeout(snapshot.TurnID, current)
	if err != nil || !applied || action != ActionFold {
		t.Fatalf("expected automatic fold: action=%s applied=%v err=%v", action, applied, err)
	}
	if table.Snapshot(1).Street != StreetComplete {
		t.Fatal("heads-up timeout fold should finish the hand")
	}
}

func TestActionTimeoutChecksWhenNothingToCall(t *testing.T) {
	current := time.Unix(1_800_000_000, 0)
	table := NewTable("main", "Test", 50, 100)
	table.now = func() time.Time { return current }
	_, _ = table.Join(1, "Alice", 10_000)
	_, _ = table.Join(2, "Bob", 10_000)
	if err := table.StartHand(1); err != nil {
		t.Fatal(err)
	}
	oldTurnID := table.Snapshot(1).TurnID
	if err := table.Act(1, ActionCall, 0); err != nil {
		t.Fatal(err)
	}
	snapshot := table.Snapshot(2)
	if !snapshot.Allowed.CanCheck {
		t.Fatalf("big blind should be able to check: %+v", snapshot.Allowed)
	}
	if _, applied, err := table.Timeout(oldTurnID, current.Add(time.Hour)); err != nil || applied {
		t.Fatalf("stale turn timer must be ignored: applied=%v err=%v", applied, err)
	}
	current = time.UnixMilli(snapshot.ActionDeadlineAt)
	action, applied, err := table.Timeout(snapshot.TurnID, current)
	if err != nil || !applied || action != ActionCheck {
		t.Fatalf("expected automatic check: action=%s applied=%v err=%v", action, applied, err)
	}
	after := table.Snapshot(2)
	if after.Street != StreetFlop || after.TurnID == snapshot.TurnID || after.ActionDeadlineAt <= snapshot.ActionDeadlineAt {
		t.Fatalf("automatic check should advance with a fresh deadline: before=%+v after=%+v", snapshot, after)
	}
}

func TestActionAfterDeadlineIsRejected(t *testing.T) {
	current := time.Unix(1_800_000_000, 0)
	table := NewTable("main", "Test", 50, 100)
	table.now = func() time.Time { return current }
	_, _ = table.Join(1, "Alice", 10_000)
	_, _ = table.Join(2, "Bob", 10_000)
	if err := table.StartHand(1); err != nil {
		t.Fatal(err)
	}
	current = time.UnixMilli(table.Snapshot(1).ActionDeadlineAt)
	if err := table.Act(1, ActionFold, 0); err != ErrActionTimedOut {
		t.Fatalf("expected ErrActionTimedOut, got %v", err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	_, _ = table.Join(1, "Alice", 10_000)
	_, _ = table.Join(2, "Bob", 10_000)
	if err := table.StartHand(1); err != nil {
		t.Fatal(err)
	}
	data, err := table.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreTable(data)
	if err != nil {
		t.Fatal(err)
	}
	before := table.Snapshot(1)
	after := restored.Snapshot(1)
	if before.HandID != after.HandID || before.Street != after.Street || before.Pot != after.Pot || before.ActionTimeoutSeconds != after.ActionTimeoutSeconds || before.ActionDeadlineAt != after.ActionDeadlineAt || before.TurnID != after.TurnID || len(after.Players) != 2 {
		t.Fatalf("restored state differs: before=%+v after=%+v", before, after)
	}
}

func TestRestoreLegacyActiveStateAddsActionDeadline(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	_, _ = table.Join(1, "Alice", 10_000)
	_, _ = table.Join(2, "Bob", 10_000)
	if err := table.StartHand(1); err != nil {
		t.Fatal(err)
	}
	data, err := table.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	delete(legacy, "action_timeout_seconds")
	delete(legacy, "action_deadline_at")
	delete(legacy, "turn_id")
	data, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreTable(data)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := restored.Snapshot(1)
	if snapshot.ActionTimeoutSeconds != DefaultActionTimeoutSeconds || snapshot.ActionDeadlineAt <= time.Now().UnixMilli() || snapshot.TurnID == 0 {
		t.Fatalf("legacy active table did not receive a fresh timeout: %+v", snapshot)
	}
}

func TestSnapshotExposesPositionsAndBetLevels(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	_, _ = table.Join(1, "Alice", 10_000)
	_, _ = table.Join(2, "Bob", 10_000)
	_, _ = table.Join(3, "Cara", 10_000)
	if err := table.StartHand(1); err != nil {
		t.Fatal(err)
	}

	snapshot := table.Snapshot(1)
	if snapshot.DealerSeat != 0 || snapshot.SmallBlindSeat != 1 || snapshot.BigBlindSeat != 2 {
		t.Fatalf("unexpected positions: dealer=%d small=%d big=%d", snapshot.DealerSeat, snapshot.SmallBlindSeat, snapshot.BigBlindSeat)
	}
	if snapshot.BetLevel != 1 {
		t.Fatalf("blinds should open preflop at level 1, got %d", snapshot.BetLevel)
	}

	if err := table.Act(1, ActionRaise, 200); err != nil {
		t.Fatal(err)
	}
	if snapshot = table.Snapshot(1); snapshot.BetLevel != 2 {
		t.Fatalf("opening raise should move to level 2, got %d", snapshot.BetLevel)
	}
	if err := table.Act(2, ActionRaise, 300); err != nil {
		t.Fatal(err)
	}
	snapshot = table.Snapshot(1)
	if snapshot.BetLevel != 3 {
		t.Fatalf("re-raise should be exposed as a 3-bet, got level %d", snapshot.BetLevel)
	}
	var bob PlayerView
	for _, player := range snapshot.Players {
		if player.UserID == 2 {
			bob = player
		}
	}
	if bob.LastAction != ActionRaise || bob.LastActionBetLevel != 3 || bob.LastActionAmount != 250 {
		t.Fatalf("unexpected last action: %+v", bob)
	}

	if err := table.Act(3, ActionRaise, 400); err != nil {
		t.Fatal(err)
	}
	snapshot = table.Snapshot(1)
	if snapshot.BetLevel != 4 {
		t.Fatalf("second re-raise should be exposed as a 4-bet, got level %d", snapshot.BetLevel)
	}
}

func TestSidePotAwardsCorrectWinners(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	table.state.Street = StreetRiver
	table.state.Board = cards("2c", "3d", "4h", "9s", "Jd")
	table.state.Seats[0] = &Player{UserID: 1, Name: "Alice", Seat: 0, Hole: cards("As", "Ad"), InHand: true, Committed: 300}
	table.state.Seats[1] = &Player{UserID: 2, Name: "Bob", Seat: 1, Hole: cards("Ks", "Kh"), InHand: true, Committed: 600}
	table.state.Seats[2] = &Player{UserID: 3, Name: "Cara", Seat: 2, Hole: cards("Qs", "Qh"), InHand: true, Committed: 600}
	payouts := table.awardSidePotsLocked()
	if payouts[1] != 900 || payouts[2] != 600 {
		t.Fatalf("unexpected payouts: %#v", payouts)
	}
}

func TestUncalledAllInIsRefundedInsteadOfMarkedAsWinnings(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	table.state.HandID = 29
	table.state.Street = StreetRiver
	table.state.Dealer = 1
	table.state.Board = cards("3s", "Tc", "2d", "Js", "8d")
	table.state.Seats[0] = &Player{UserID: 1, Name: "admin123", Seat: 0, Hole: cards("7c", "Jc"), InHand: true, AllIn: true, Committed: 1_000}
	table.state.Seats[1] = &Player{UserID: 2, Name: "l4zily", Seat: 1, Hole: cards("8h", "3c"), Stack: 842, InHand: true, AllIn: true, Committed: 158}

	table.finishShowdownLocked()
	result := table.state.LastResult
	if result == nil {
		t.Fatal("showdown result was not recorded")
	}
	if result.Pot != 316 || result.Payouts[2] != 316 || len(result.Payouts) != 1 {
		t.Fatalf("only the contestable pot should be awarded to l4zily: %+v", result)
	}
	if result.Refunds[1] != 842 || len(result.Refunds) != 1 {
		t.Fatalf("uncalled excess must be recorded as a refund: %+v", result.Refunds)
	}
	admin, _ := table.StackFor(1)
	winner, _ := table.StackFor(2)
	if admin != 842 || winner != 1_158 {
		t.Fatalf("unexpected final stacks admin=%d l4zily=%d", admin, winner)
	}
	if result.Message != "l4zily win at showdown" {
		t.Fatalf("refunded player must not be announced as a winner: %q", result.Message)
	}
	if len(result.Players) != 2 || result.Players[0].Net != -158 || result.Players[1].Net != 158 {
		t.Fatalf("unexpected participant ledger: %+v", result.Players)
	}
}

func TestCannotRaiseBeyondOpponentsEffectiveStack(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	table.state.Street = StreetFlop
	table.state.CurrentBet = 158
	table.state.MinRaise = 100
	table.state.Acting = 0
	table.state.Seats[0] = &Player{UserID: 1, Name: "Deep", Seat: 0, Stack: 1_000, InHand: true}
	table.state.Seats[1] = &Player{UserID: 2, Name: "Short", Seat: 1, Bet: 158, Committed: 158, InHand: true, AllIn: true}

	allowed := table.Snapshot(1).Allowed
	if !allowed.CanCall || allowed.CanRaise || allowed.CanAllIn || allowed.MaxRaiseTo != 158 {
		t.Fatalf("deep stack should only be able to call or fold: %+v", allowed)
	}
	if err := table.Act(1, ActionRaise, 200); err == nil {
		t.Fatal("server accepted a raise no opponent could match")
	}
	if err := table.Act(1, ActionAllIn, 0); err == nil {
		t.Fatal("server accepted an unmatched all-in")
	}
}

func TestIncompleteAllInDoesNotReopenRaise(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	table.state.Street = StreetFlop
	table.state.CurrentBet = 150
	table.state.MinRaise = 100
	table.state.Acting = 0
	table.state.Seats[0] = &Player{UserID: 1, Name: "Alice", Seat: 0, Stack: 1_000, Bet: 100, Committed: 100, InHand: true}
	table.state.Seats[1] = &Player{UserID: 2, Name: "Bob", Seat: 1, Bet: 150, Committed: 150, InHand: true, AllIn: true}
	table.state.Acted[0] = true
	table.state.ActedAtBet[0] = 100

	allowed := table.Snapshot(1).Allowed
	if allowed.CanRaise || allowed.CanAllIn || !allowed.CanCall {
		t.Fatalf("short all-in should allow call but not reopen raise: %+v", allowed)
	}
	if err := table.Act(1, ActionRaise, 250); err == nil {
		t.Fatal("server must reject a raise after an incomplete all-in")
	}
}

func TestCumulativeShortAllInsReopenRaise(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	table.state.Street = StreetFlop
	table.state.CurrentBet = 200
	table.state.MinRaise = 100
	table.state.Acting = 0
	table.state.Seats[0] = &Player{UserID: 1, Name: "Alice", Seat: 0, Stack: 1_000, Bet: 100, Committed: 100, InHand: true}
	table.state.Seats[1] = &Player{UserID: 2, Name: "Bob", Seat: 1, Bet: 200, Committed: 200, InHand: true, AllIn: true}
	table.state.Seats[2] = &Player{UserID: 3, Name: "Cara", Seat: 2, Stack: 1_000, Bet: 200, Committed: 200, InHand: true}
	table.state.Acted[0] = true
	table.state.ActedAtBet[0] = 100

	allowed := table.Snapshot(1).Allowed
	if !allowed.CanRaise || !allowed.CanAllIn {
		t.Fatalf("a cumulative full raise must reopen betting: %+v", allowed)
	}
}

func TestOddChipGoesToFirstWinnerLeftOfButton(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	table.state.Dealer = 0
	table.state.Street = StreetRiver
	table.state.Board = cards("As", "Ks", "Qs", "Js", "Ts")
	table.state.Seats[0] = &Player{UserID: 1, Name: "Alice", Seat: 0, Hole: cards("2c", "3d"), InHand: true, Committed: 1}
	table.state.Seats[1] = &Player{UserID: 2, Name: "Bob", Seat: 1, Hole: cards("4c", "5d"), InHand: true, Committed: 1}
	table.state.Seats[2] = &Player{UserID: 3, Name: "Cara", Seat: 2, Hole: cards("6c", "7d"), InHand: true, Folded: true, Committed: 1}

	payouts := table.awardSidePotsLocked()
	if payouts[1] != 1 || payouts[2] != 2 {
		t.Fatalf("odd chip should go to the first winner left of the button: %#v", payouts)
	}
}

func TestBurnsBeforeFlopTurnAndRiver(t *testing.T) {
	table := NewTable("main", "Test", 50, 100)
	_, _ = table.Join(1, "Alice", 50)
	_, _ = table.Join(2, "Bob", 100)
	deck := newDeck()
	table.mu.Lock()
	err := table.startHandLocked(1, deck)
	table.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	board := table.Snapshot(1).Board
	want := []Card{deck[5], deck[6], deck[7], deck[9], deck[11]}
	if len(board) != len(want) {
		t.Fatalf("board has %d cards, want %d", len(board), len(want))
	}
	for i := range want {
		if board[i] != want[i] {
			t.Fatalf("board[%d] = %s, want %s", i, board[i], want[i])
		}
	}
}

func TestRandomEightMaxHandsTerminateAndConserveChips(t *testing.T) {
	rng := rand.New(rand.NewSource(20260818))
	for hand := 0; hand < 1_000; hand++ {
		table := NewTable("main", "Test", 50, 100)
		var total int64
		for player := int64(1); player <= MaxSeats; player++ {
			stack := int64(200 + rng.Intn(9_801))
			total += stack
			if _, err := table.Join(player, "Player", stack); err != nil {
				t.Fatal(err)
			}
		}
		deck := newDeck()
		rng.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
		table.mu.Lock()
		err := table.startHandLocked(1, deck)
		table.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}

		for step := 0; step < 200; step++ {
			snapshot := table.Snapshot(0)
			if snapshot.Street == StreetComplete {
				break
			}
			var actorID int64
			for _, player := range snapshot.Players {
				if player.Seat == snapshot.ActingSeat {
					actorID = player.UserID
					break
				}
			}
			if actorID == 0 {
				t.Fatalf("hand %d has no actor in active street %s", hand, snapshot.Street)
			}
			allowed := table.Snapshot(actorID).Allowed
			action, amount := chooseTestAction(rng, allowed)
			if err := table.Act(actorID, action, amount); err != nil {
				t.Fatalf("hand %d step %d action %s amount %d: %v", hand, step, action, amount, err)
			}
		}
		if snapshot := table.Snapshot(0); snapshot.Street != StreetComplete {
			t.Fatalf("hand %d did not terminate", hand)
		}
		var after int64
		for player := int64(1); player <= MaxSeats; player++ {
			stack, ok := table.StackFor(player)
			if !ok {
				t.Fatalf("player %d disappeared", player)
			}
			after += stack
		}
		if after != total {
			t.Fatalf("hand %d changed total chips: before=%d after=%d", hand, total, after)
		}
	}
}

func chooseTestAction(rng *rand.Rand, allowed AllowedActions) (ActionType, int64) {
	roll := rng.Intn(100)
	if allowed.CanCheck && roll < 45 {
		return ActionCheck, 0
	}
	if allowed.CanCall && roll < 55 {
		return ActionCall, 0
	}
	if allowed.CanRaise && roll < 70 {
		return ActionRaise, allowed.MinRaiseTo
	}
	if allowed.CanBet && roll < 70 {
		return ActionBet, allowed.MinRaiseTo
	}
	if allowed.CanAllIn && roll < 80 {
		return ActionAllIn, 0
	}
	if allowed.CanCall && roll < 90 {
		return ActionCall, 0
	}
	if allowed.CanCheck {
		return ActionCheck, 0
	}
	return ActionFold, 0
}
