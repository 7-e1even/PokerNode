package mcpserver

import (
	"testing"

	"pokernode/internal/landlord"
)

func TestLandlordTableOutputAndCardCodes(t *testing.T) {
	snapshot := landlord.Snapshot{
		GameType: landlord.GameType, ID: "landlord-1", Name: "Agent 斗地主", BaseStake: 100,
		Phase: landlord.PhasePlaying, HandID: 2, ActingSeat: 0, LandlordSeat: 0,
		Bottom:  []landlord.Card{{Rank: landlord.SmallJoker, Suit: landlord.Joker}},
		Players: []landlord.PlayerView{{UserID: 1, Name: "agent", Seat: 0, Stack: 10_000, CardCount: 1, Landlord: true, IsActing: true, Cards: []landlord.Card{{Rank: landlord.BigJoker, Suit: landlord.Joker}}}},
		Allowed: landlord.AllowedActions{CanAct: true, CanPlay: true}, ViewerSeat: 0,
	}
	output := tableOutput(gameSnapshot{GameType: landlord.GameType, Landlord: &snapshot}, "")
	if output.Table.GameType != landlord.GameType || !output.Table.AllowedActions.CanPlay || output.Table.Players[0].Cards[0].Code != "BJ" || output.Table.Bottom[0].Code != "SJ" {
		t.Fatalf("unexpected landlord MCP output: %+v", output.Table)
	}
	card, err := parseLandlordCard("Td")
	if err != nil || card.Rank != landlord.Ten || card.Suit != landlord.Diamonds {
		t.Fatalf("parse Td: card=%+v err=%v", card, err)
	}
}
