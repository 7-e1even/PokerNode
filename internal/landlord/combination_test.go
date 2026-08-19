package landlord

import "testing"

func TestClassifyStandardCombinations(t *testing.T) {
	tests := []struct {
		name  string
		cards []Card
		kind  CombinationKind
	}{
		{"rocket", cards(SmallJoker, BigJoker), CombinationRocket},
		{"bomb", cards(Seven, Seven, Seven, Seven), CombinationBomb},
		{"straight", cards(Three, Four, Five, Six, Seven), CombinationStraight},
		{"pairs", cards(Three, Three, Four, Four, Five, Five), CombinationConsecutivePairs},
		{"plane", cards(Three, Three, Three, Four, Four, Four), CombinationPlane},
		{"plane singles", cards(Three, Three, Three, Four, Four, Four, Seven, Eight), CombinationPlaneSingles},
		{"plane pairs", cards(Three, Three, Three, Four, Four, Four, Seven, Seven, Eight, Eight), CombinationPlanePairs},
		{"four singles", cards(Six, Six, Six, Six, Nine, Ten), CombinationFourSingles},
		{"four pairs", cards(Six, Six, Six, Six, Nine, Nine, Ten, Ten), CombinationFourPairs},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			combination, err := Classify(test.cards)
			if err != nil {
				t.Fatal(err)
			}
			if combination.Kind != test.kind {
				t.Fatalf("kind = %s, want %s", combination.Kind, test.kind)
			}
		})
	}
}

func TestCombinationComparison(t *testing.T) {
	low, _ := Classify(cards(Three, Four, Five, Six, Seven))
	high, _ := Classify(cards(Four, Five, Six, Seven, Eight))
	bomb, _ := Classify(cards(Nine, Nine, Nine, Nine))
	rocket, _ := Classify(cards(SmallJoker, BigJoker))
	if !Beats(high, low) || !Beats(bomb, high) || !Beats(rocket, bomb) {
		t.Fatal("expected higher straight, bomb, and rocket to win in order")
	}
	short, _ := Classify(cards(Three, Four, Five, Six, Seven, Eight))
	if Beats(short, low) {
		t.Fatal("straights with different lengths must not compare")
	}
}

func cards(ranks ...Rank) []Card {
	cards := make([]Card, len(ranks))
	seen := make(map[Rank]int)
	for index, rank := range ranks {
		suit := Suit(seen[rank] % 4)
		if rank == SmallJoker || rank == BigJoker {
			suit = Joker
		}
		cards[index] = Card{Rank: rank, Suit: suit}
		seen[rank]++
	}
	return cards
}
