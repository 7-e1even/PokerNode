package landlord

import "testing"

func TestWaitingSnapshotUsesEmptyCardArrays(t *testing.T) {
	snapshot := NewTable("landlord", "斗地主", 100).Snapshot(1)
	if snapshot.Bottom == nil || snapshot.LastPlay == nil {
		t.Fatalf("waiting snapshot must use empty card arrays: bottom=%v last_play=%v", snapshot.Bottom, snapshot.LastPlay)
	}
}

func TestThreeReadyPlayersStartBidding(t *testing.T) {
	table := NewTable("landlord", "斗地主", 100)
	for userID := int64(1); userID <= 3; userID++ {
		if _, err := table.Join(userID, "player", 10_000); err != nil {
			t.Fatal(err)
		}
	}
	for userID := int64(1); userID <= 3; userID++ {
		if _, err := table.Ready(userID); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := table.Snapshot(1)
	if snapshot.Phase != PhaseBidding || snapshot.ActingSeat < 0 {
		t.Fatalf("unexpected start state: %+v", snapshot)
	}
	if len(snapshot.Players[0].Cards) != 17 {
		t.Fatalf("viewer has %d cards, want 17", len(snapshot.Players[0].Cards))
	}
	for _, player := range snapshot.Players[1:] {
		if len(player.Cards) != 0 || player.CardCount != 17 {
			t.Fatalf("opponent cards leaked: %+v", player)
		}
	}
}

func TestPlayerCanReadyBeforeTableFills(t *testing.T) {
	table := NewTable("landlord", "斗地主", 100)
	if _, err := table.Join(1, "Alice", 10_000); err != nil {
		t.Fatal(err)
	}
	if snapshot := table.Snapshot(1); !snapshot.CanStart {
		t.Fatal("a seated player should be able to ready before the table fills")
	}
	if started, err := table.Ready(1); err != nil || started {
		t.Fatalf("early ready should wait for more players: started=%v err=%v", started, err)
	}

	for userID := int64(2); userID <= 3; userID++ {
		if _, err := table.Join(userID, "player", 10_000); err != nil {
			t.Fatal(err)
		}
	}
	if snapshot := table.Snapshot(1); !snapshot.Players[0].Ready {
		t.Fatal("joining players should not clear an existing ready state")
	}
	if started, err := table.Ready(2); err != nil || started {
		t.Fatalf("second ready should wait: started=%v err=%v", started, err)
	}
	if started, err := table.Ready(3); err != nil || !started {
		t.Fatalf("last ready should start the hand: started=%v err=%v", started, err)
	}
}

func TestReadyPlayerCanCancelBeforeHandStarts(t *testing.T) {
	table := NewTable("landlord", "斗地主", 100)
	for userID := int64(1); userID <= 3; userID++ {
		if _, err := table.Join(userID, "player", 10_000); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := table.Ready(1); err != nil {
		t.Fatal(err)
	}
	if _, err := table.Ready(1); err != nil {
		t.Fatal(err)
	}
	if player := table.Snapshot(1).Players[0]; player.Ready {
		t.Fatalf("player should be unready after toggling readiness: %+v", player)
	}
}

func TestSnapshotTracksCardsRemainingOutsideViewerHand(t *testing.T) {
	table := readyTable(t)
	snapshot := table.Snapshot(1)
	total := 0
	for _, count := range snapshot.RemainingCounts {
		total += count
	}
	if total != 37 {
		t.Fatalf("remaining counter totals %d cards, want 37", total)
	}
	if spectator := table.Snapshot(0); len(spectator.RemainingCounts) != 0 {
		t.Fatalf("spectator must not receive a card counter: %v", spectator.RemainingCounts)
	}

	actor := snapshot.Players[snapshot.ActingSeat].UserID
	if err := table.Act(actor, ActionBid, 3, nil); err != nil {
		t.Fatal(err)
	}
	actorSnapshot := table.Snapshot(actor)
	played := actorSnapshot.Players[actorSnapshot.ViewerSeat].Cards[0]
	farmerID := int64(1)
	if farmerID == actor {
		farmerID = 2
	}
	before := table.Snapshot(farmerID)
	if err := table.Act(actor, ActionPlay, 0, []Card{played}); err != nil {
		t.Fatal(err)
	}
	after := table.Snapshot(farmerID)
	if after.RemainingCounts[int(played.Rank)] != before.RemainingCounts[int(played.Rank)]-1 {
		t.Fatalf("rank %d counter did not decrease after play: before=%d after=%d", played.Rank, before.RemainingCounts[int(played.Rank)], after.RemainingCounts[int(played.Rank)])
	}
}

func TestBidThreeAssignsLandlordAndBottomCards(t *testing.T) {
	table := readyTable(t)
	snapshot := table.Snapshot(1)
	actor := snapshot.Players[snapshot.ActingSeat].UserID
	if err := table.Act(actor, ActionBid, 3, nil); err != nil {
		t.Fatal(err)
	}
	snapshot = table.Snapshot(actor)
	if snapshot.Phase != PhasePlaying || snapshot.LandlordSeat != snapshot.ViewerSeat || len(snapshot.Bottom) != 3 {
		t.Fatalf("unexpected landlord state: %+v", snapshot)
	}
	if len(snapshot.Players[snapshot.ViewerSeat].Cards) != 20 {
		t.Fatalf("landlord has %d cards, want 20", len(snapshot.Players[snapshot.ViewerSeat].Cards))
	}
}

func TestPlayingMustFollowCombination(t *testing.T) {
	table := readyTable(t)
	snapshot := table.Snapshot(1)
	actor := snapshot.Players[snapshot.ActingSeat].UserID
	if err := table.Act(actor, ActionBid, 3, nil); err != nil {
		t.Fatal(err)
	}
	snapshot = table.Snapshot(actor)
	card := snapshot.Players[snapshot.ViewerSeat].Cards[0]
	if err := table.Act(actor, ActionPlay, 0, []Card{card}); err != nil {
		t.Fatal(err)
	}
	next := table.Snapshot(0)
	nextActor := next.Players[next.ActingSeat].UserID
	if err := table.Act(nextActor, ActionPlay, 0, cards(Three, Three)); err == nil {
		t.Fatal("pair should not beat a single")
	}
}

func TestLandlordWinSettlesZeroSumAndCapsLosses(t *testing.T) {
	table := NewTable("landlord", "斗地主", 100)
	table.state.Phase = PhasePlaying
	table.state.HandID = 1
	table.state.Acting = 0
	table.state.LandlordSeat = 0
	table.state.HighestBid = 1
	table.state.Multiplier = 1
	table.state.Seats[0] = &Player{UserID: 1, Name: "landlord", Seat: 0, Stack: 100, Hand: cards(Three), Ready: true, Landlord: true, Plays: 1}
	table.state.Seats[1] = &Player{UserID: 2, Name: "farmer-1", Seat: 1, Stack: 100, Hand: cards(Four), Ready: true}
	table.state.Seats[2] = &Player{UserID: 3, Name: "farmer-2", Seat: 2, Stack: 200, Hand: cards(Five), Ready: true}

	if err := table.Act(1, ActionPlay, 0, cards(Three)); err != nil {
		t.Fatal(err)
	}
	snapshot := table.Snapshot(1)
	if snapshot.Phase != PhaseComplete || snapshot.LastResult == nil {
		t.Fatalf("hand did not complete: %+v", snapshot)
	}
	if snapshot.LastResult.Multiplier != 2 || snapshot.LastResult.Stake != 200 {
		t.Fatalf("spring multiplier was not applied: %+v", snapshot.LastResult)
	}
	stacks := map[int64]int64{}
	total := int64(0)
	for _, player := range snapshot.Players {
		if player.Ready {
			t.Fatalf("player %d remained ready after the hand completed", player.UserID)
		}
		stacks[player.UserID] = player.Stack
		total += player.Stack
	}
	if total != 400 || stacks[1] != 400 || stacks[2] != 0 || stacks[3] != 0 {
		t.Fatalf("unexpected capped settlement: stacks=%v total=%d", stacks, total)
	}
}

func readyTable(t *testing.T) *Table {
	t.Helper()
	table := NewTable("landlord", "斗地主", 100)
	for userID := int64(1); userID <= 3; userID++ {
		if _, err := table.Join(userID, "player", 10_000); err != nil {
			t.Fatal(err)
		}
	}
	for userID := int64(1); userID <= 3; userID++ {
		if _, err := table.Ready(userID); err != nil {
			t.Fatal(err)
		}
	}
	return table
}
