package poker

import (
	"math/rand"
	"testing"

	reference "github.com/chehsunliu/poker"
)

func TestEvaluatorMatchesReferenceLibrary(t *testing.T) {
	rng := rand.New(rand.NewSource(20260818))
	for sample := 0; sample < 5_000; sample++ {
		left := randomSevenCards(rng)
		right := randomSevenCards(rng)
		leftValue, rightValue := Evaluate(left), Evaluate(right)
		leftReference, rightReference := reference.Evaluate(referenceCards(left)), reference.Evaluate(referenceCards(right))

		if got, want := 9-int(leftValue.Category), int(reference.RankClass(leftReference)); got != want {
			t.Fatalf("sample %d category mismatch: got=%d want=%d cards=%v", sample, got, want, left)
		}
		ours := compareUint64(leftValue.Score, rightValue.Score)
		theirs := -compareInt32(leftReference, rightReference)
		if ours != theirs {
			t.Fatalf("sample %d ordering mismatch: left=%v right=%v ours=%d reference=%d", sample, left, right, ours, theirs)
		}
	}
}

func randomSevenCards(rng *rand.Rand) []Card {
	deck := newDeck()
	rng.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	return deck[:7]
}

func referenceCards(cards []Card) []reference.Card {
	result := make([]reference.Card, len(cards))
	for i, card := range cards {
		result[i] = reference.NewCard(card.String())
	}
	return result
}

func compareUint64(left, right uint64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareInt32(left, right int32) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
