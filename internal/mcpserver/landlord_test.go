package mcpserver

import (
	"testing"

	"pokernode/internal/landlord"
)

func TestLandlordDecisionOutputAndCardCodes(t *testing.T) {
	snapshot := landlord.Snapshot{
		GameType: landlord.GameType, ID: "landlord-1", Name: "Agent 斗地主", BaseStake: 100,
		Phase: landlord.PhasePlaying, HandID: 2, ActingSeat: 0, LandlordSeat: 0,
		Bottom:  []landlord.Card{{Rank: landlord.SmallJoker, Suit: landlord.Joker}},
		Players: []landlord.PlayerView{{UserID: 1, Name: "agent", Seat: 0, Stack: 10_000, CardCount: 1, Landlord: true, IsActing: true, Cards: []landlord.Card{{Rank: landlord.BigJoker, Suit: landlord.Joker}}}},
		Allowed: landlord.AllowedActions{CanAct: true, CanPlay: true}, ViewerSeat: 0,
	}
	output := decisionView(gameSnapshot{GameType: landlord.GameType, Landlord: &snapshot}, nil)
	if output.GameType != landlord.GameType || len(output.LegalActions) != 1 || output.LegalActions[0] != "play" || output.Players[0].Cards[0] != "BJ" || output.Bottom[0] != "SJ" {
		t.Fatalf("unexpected landlord MCP output: %+v", output)
	}
	if output.Money != (MoneySpec{Currency: "USD", Unit: "cent", Scale: 100}) || output.BaseStakeCents != 100 || output.Players[0].StackCents != 10_000 {
		t.Fatalf("landlord money metadata does not preserve the cents scale: %+v", output)
	}
	card, err := parseLandlordCard("Td")
	if err != nil || card.Rank != landlord.Ten || card.Suit != landlord.Diamonds {
		t.Fatalf("parse Td: card=%+v err=%v", card, err)
	}
}
