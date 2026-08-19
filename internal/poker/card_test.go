package poker

import "testing"

func TestShuffledDeckContainsEveryCardExactlyOnce(t *testing.T) {
	deck, err := shuffledDeck()
	if err != nil {
		t.Fatal(err)
	}
	if len(deck) != 52 {
		t.Fatalf("deck has %d cards, want 52", len(deck))
	}
	seen := make(map[Card]bool, 52)
	for _, card := range deck {
		if card.Rank < Two || card.Rank > Ace || card.Suit < Clubs || card.Suit > Spades {
			t.Fatalf("invalid card in deck: %+v", card)
		}
		if seen[card] {
			t.Fatalf("duplicate card in deck: %s", card)
		}
		seen[card] = true
	}
}
