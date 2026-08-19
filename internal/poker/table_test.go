package poker

import (
	"math/rand"
	"testing"
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
	if before.HandID != after.HandID || before.Street != after.Street || before.Pot != after.Pot || len(after.Players) != 2 {
		t.Fatalf("restored state differs: before=%+v after=%+v", before, after)
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
