package app

import (
	"testing"

	"pokernode/internal/landlord"
	"pokernode/internal/poker"
)

func TestRestoreTableRuntimeDispatchesByGameType(t *testing.T) {
	landlordTable := landlord.NewTable("landlord-1", "斗地主", 100)
	data, err := landlordTable.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := restoreTableRuntime(data)
	if err != nil {
		t.Fatal(err)
	}
	summary := summarizeRuntime(runtime, 0)
	if summary.GameType != gameTypeLandlord || summary.MaxPlayers != landlord.MaxSeats || summary.BaseStake != 100 {
		t.Fatalf("unexpected landlord summary: %+v", summary)
	}

	pokerTable := poker.NewTable("poker-1", "德州", 50, 100)
	data, err = pokerTable.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	runtime, err = restoreTableRuntime(data)
	if err != nil {
		t.Fatal(err)
	}
	if summary = summarizeRuntime(runtime, 0); summary.GameType != gameTypeTexasHoldem || summary.MaxPlayers != poker.MaxSeats {
		t.Fatalf("legacy poker state was not restored as Texas Hold'em: %+v", summary)
	}
}
