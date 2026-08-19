package poker

import "testing"

func TestEvaluateCategories(t *testing.T) {
	tests := []struct {
		name     string
		cards    []Card
		category HandCategory
	}{
		{"royal flush", cards("As", "Ks", "Qs", "Js", "Ts", "2d", "3c"), StraightFlush},
		{"quads", cards("Ah", "Ad", "As", "Ac", "Kd", "2s", "3c"), FourOfAKind},
		{"full house", cards("Kh", "Kd", "Ks", "2c", "2d", "9s", "8h"), FullHouse},
		{"wheel", cards("As", "2d", "3h", "4c", "5s", "Kd", "Qh"), Straight},
		{"two pair", cards("As", "Ad", "Kh", "Kc", "5s", "3d", "2h"), TwoPair},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Evaluate(test.cards); got.Category != test.category {
				t.Fatalf("category = %v, want %v", got.Category, test.category)
			}
		})
	}
}

func TestEvaluateKicker(t *testing.T) {
	acesKing := Evaluate(cards("As", "Ad", "Kh", "9c", "7s", "4d", "2h"))
	acesQueen := Evaluate(cards("Ac", "Ah", "Qh", "9d", "7c", "4s", "2d"))
	if acesKing.Score <= acesQueen.Score {
		t.Fatal("king kicker should beat queen kicker")
	}
}

func cards(values ...string) []Card {
	result := make([]Card, 0, len(values))
	for _, value := range values {
		rankMap := map[byte]Rank{'2': Two, '3': Three, '4': Four, '5': Five, '6': Six, '7': Seven, '8': Eight, '9': Nine, 'T': Ten, 'J': Jack, 'Q': Queen, 'K': King, 'A': Ace}
		suitMap := map[byte]Suit{'c': Clubs, 'd': Diamonds, 'h': Hearts, 's': Spades}
		result = append(result, Card{Rank: rankMap[value[0]], Suit: suitMap[value[1]]})
	}
	return result
}
