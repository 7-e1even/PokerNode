package landlord

import (
	"errors"
	"sort"
)

type CombinationKind string

const (
	CombinationSingle           CombinationKind = "single"
	CombinationPair             CombinationKind = "pair"
	CombinationTriple           CombinationKind = "triple"
	CombinationTripleSingle     CombinationKind = "triple_single"
	CombinationTriplePair       CombinationKind = "triple_pair"
	CombinationStraight         CombinationKind = "straight"
	CombinationConsecutivePairs CombinationKind = "consecutive_pairs"
	CombinationPlane            CombinationKind = "plane"
	CombinationPlaneSingles     CombinationKind = "plane_singles"
	CombinationPlanePairs       CombinationKind = "plane_pairs"
	CombinationFourSingles      CombinationKind = "four_singles"
	CombinationFourPairs        CombinationKind = "four_pairs"
	CombinationBomb             CombinationKind = "bomb"
	CombinationRocket           CombinationKind = "rocket"
)

type Combination struct {
	Kind     CombinationKind `json:"kind"`
	MainRank Rank            `json:"main_rank"`
	Length   int             `json:"length"`
	Chain    int             `json:"chain"`
}

var ErrInvalidCombination = errors.New("这些牌不能组成合法牌型")

func Classify(cards []Card) (Combination, error) {
	if len(cards) == 0 {
		return Combination{}, ErrInvalidCombination
	}
	counts := make(map[Rank]int)
	for _, card := range cards {
		if !validCard(card) {
			return Combination{}, ErrInvalidCombination
		}
		counts[card.Rank]++
	}
	ranks := sortedRanks(counts)
	combo := Combination{Length: len(cards)}

	if len(cards) == 2 && counts[SmallJoker] == 1 && counts[BigJoker] == 1 {
		combo.Kind, combo.MainRank = CombinationRocket, BigJoker
		return combo, nil
	}
	if len(cards) == 4 && len(ranks) == 1 {
		combo.Kind, combo.MainRank = CombinationBomb, ranks[0]
		return combo, nil
	}
	if len(cards) == 1 {
		combo.Kind, combo.MainRank = CombinationSingle, ranks[0]
		return combo, nil
	}
	if len(cards) == 2 && len(ranks) == 1 {
		combo.Kind, combo.MainRank = CombinationPair, ranks[0]
		return combo, nil
	}
	if len(cards) == 3 && len(ranks) == 1 {
		combo.Kind, combo.MainRank = CombinationTriple, ranks[0]
		return combo, nil
	}
	if len(cards) == 4 {
		if rank, ok := rankWithCount(counts, 3); ok {
			combo.Kind, combo.MainRank = CombinationTripleSingle, rank
			return combo, nil
		}
	}
	if len(cards) == 5 {
		if triple, ok := rankWithCount(counts, 3); ok {
			if _, pair := rankWithCount(counts, 2); pair {
				combo.Kind, combo.MainRank = CombinationTriplePair, triple
				return combo, nil
			}
		}
	}
	if len(cards) >= 5 && len(ranks) == len(cards) && consecutive(ranks) && ranks[len(ranks)-1] <= Ace {
		combo.Kind, combo.MainRank, combo.Chain = CombinationStraight, ranks[len(ranks)-1], len(ranks)
		return combo, nil
	}
	if len(cards) >= 6 && len(cards)%2 == 0 && allCounts(counts, 2) && consecutive(ranks) && ranks[len(ranks)-1] <= Ace {
		combo.Kind, combo.MainRank, combo.Chain = CombinationConsecutivePairs, ranks[len(ranks)-1], len(ranks)
		return combo, nil
	}
	if combo, ok := classifyPlane(cards, counts); ok {
		return combo, nil
	}
	if len(cards) == 6 {
		if rank, ok := rankWithCount(counts, 4); ok {
			combo.Kind, combo.MainRank = CombinationFourSingles, rank
			return combo, nil
		}
	}
	if len(cards) == 8 {
		if rank, ok := rankWithCount(counts, 4); ok {
			remaining := copyCounts(counts)
			delete(remaining, rank)
			if len(remaining) == 2 && allCounts(remaining, 2) {
				combo.Kind, combo.MainRank = CombinationFourPairs, rank
				return combo, nil
			}
		}
	}
	return Combination{}, ErrInvalidCombination
}

func Beats(next, previous Combination) bool {
	if next.Kind == CombinationRocket {
		return previous.Kind != CombinationRocket
	}
	if previous.Kind == CombinationRocket {
		return false
	}
	if next.Kind == CombinationBomb && previous.Kind != CombinationBomb {
		return true
	}
	if previous.Kind == CombinationBomb && next.Kind != CombinationBomb {
		return false
	}
	return next.Kind == previous.Kind && next.Length == previous.Length && next.Chain == previous.Chain && next.MainRank > previous.MainRank
}

func classifyPlane(cards []Card, counts map[Rank]int) (Combination, bool) {
	wingSize := 0
	kind := CombinationPlane
	switch {
	case len(cards) >= 6 && len(cards)%3 == 0:
		wingSize = 0
		kind = CombinationPlane
	case len(cards) >= 8 && len(cards)%4 == 0:
		wingSize = 1
		kind = CombinationPlaneSingles
	case len(cards) >= 10 && len(cards)%5 == 0:
		wingSize = 2
		kind = CombinationPlanePairs
	default:
		return Combination{}, false
	}
	chain := len(cards) / (3 + wingSize)
	if chain < 2 {
		return Combination{}, false
	}
	tripleRanks := make([]Rank, 0)
	for rank, count := range counts {
		if count == 3 && rank <= Ace {
			tripleRanks = append(tripleRanks, rank)
		}
	}
	sort.Slice(tripleRanks, func(left, right int) bool { return tripleRanks[left] < tripleRanks[right] })
	for start := 0; start+chain <= len(tripleRanks); start++ {
		main := tripleRanks[start : start+chain]
		if !consecutive(main) {
			continue
		}
		remaining := copyCounts(counts)
		valid := true
		for _, rank := range main {
			if remaining[rank] != 3 {
				valid = false
				break
			}
			delete(remaining, rank)
		}
		if !valid {
			continue
		}
		switch wingSize {
		case 0:
			valid = len(remaining) == 0
		case 1:
			wingCards := 0
			for _, count := range remaining {
				wingCards += count
			}
			valid = wingCards == chain
		case 2:
			valid = len(remaining) == chain && allCounts(remaining, 2)
		}
		if valid {
			return Combination{Kind: kind, MainRank: main[len(main)-1], Length: len(cards), Chain: chain}, true
		}
	}
	return Combination{}, false
}

func sortedRanks(counts map[Rank]int) []Rank {
	ranks := make([]Rank, 0, len(counts))
	for rank := range counts {
		ranks = append(ranks, rank)
	}
	sort.Slice(ranks, func(left, right int) bool { return ranks[left] < ranks[right] })
	return ranks
}

func consecutive(ranks []Rank) bool {
	for index := 1; index < len(ranks); index++ {
		if ranks[index] != ranks[index-1]+1 {
			return false
		}
	}
	return true
}

func rankWithCount(counts map[Rank]int, wanted int) (Rank, bool) {
	for rank, count := range counts {
		if count == wanted {
			return rank, true
		}
	}
	return 0, false
}

func allCounts(counts map[Rank]int, wanted int) bool {
	for _, count := range counts {
		if count != wanted {
			return false
		}
	}
	return true
}

func copyCounts(counts map[Rank]int) map[Rank]int {
	copy := make(map[Rank]int, len(counts))
	for rank, count := range counts {
		copy[rank] = count
	}
	return copy
}
