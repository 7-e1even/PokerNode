package poker

import "sort"

type HandCategory uint8

const (
	HighCard HandCategory = iota
	OnePair
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
)

type HandValue struct {
	Category HandCategory `json:"category"`
	Score    uint64       `json:"score"`
}

func Evaluate(cards []Card) HandValue {
	if len(cards) < 5 {
		return HandValue{}
	}
	best := HandValue{}
	for a := 0; a < len(cards)-4; a++ {
		for b := a + 1; b < len(cards)-3; b++ {
			for c := b + 1; c < len(cards)-2; c++ {
				for d := c + 1; d < len(cards)-1; d++ {
					for e := d + 1; e < len(cards); e++ {
						value := evaluateFive([5]Card{cards[a], cards[b], cards[c], cards[d], cards[e]})
						if value.Score > best.Score {
							best = value
						}
					}
				}
			}
		}
	}
	return best
}

func evaluateFive(cards [5]Card) HandValue {
	counts := make(map[Rank]int, 5)
	ranks := make([]int, 0, 5)
	flush := true
	for i, card := range cards {
		counts[card.Rank]++
		ranks = append(ranks, int(card.Rank))
		if i > 0 && card.Suit != cards[0].Suit {
			flush = false
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ranks)))
	straightHigh := straightHighCard(ranks)

	type group struct {
		count int
		rank  int
	}
	groups := make([]group, 0, len(counts))
	for rank, count := range counts {
		groups = append(groups, group{count: count, rank: int(rank)})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].count != groups[j].count {
			return groups[i].count > groups[j].count
		}
		return groups[i].rank > groups[j].rank
	})

	switch {
	case flush && straightHigh > 0:
		return makeValue(StraightFlush, straightHigh)
	case groups[0].count == 4:
		return makeValue(FourOfAKind, groups[0].rank, groups[1].rank)
	case groups[0].count == 3 && groups[1].count == 2:
		return makeValue(FullHouse, groups[0].rank, groups[1].rank)
	case flush:
		return makeValue(Flush, ranks...)
	case straightHigh > 0:
		return makeValue(Straight, straightHigh)
	case groups[0].count == 3:
		kickers := []int{groups[0].rank}
		for _, g := range groups[1:] {
			kickers = append(kickers, g.rank)
		}
		return makeValue(ThreeOfAKind, kickers...)
	case groups[0].count == 2 && groups[1].count == 2:
		return makeValue(TwoPair, groups[0].rank, groups[1].rank, groups[2].rank)
	case groups[0].count == 2:
		kickers := []int{groups[0].rank}
		for _, g := range groups[1:] {
			kickers = append(kickers, g.rank)
		}
		return makeValue(OnePair, kickers...)
	default:
		return makeValue(HighCard, ranks...)
	}
}

func straightHighCard(sortedRanks []int) int {
	seen := make(map[int]bool, len(sortedRanks))
	unique := make([]int, 0, len(sortedRanks))
	for _, rank := range sortedRanks {
		if !seen[rank] {
			seen[rank] = true
			unique = append(unique, rank)
		}
	}
	if seen[int(Ace)] {
		unique = append(unique, 1)
	}
	for i := 0; i+4 < len(unique); i++ {
		if unique[i]-unique[i+4] == 4 {
			return unique[i]
		}
	}
	return 0
}

func makeValue(category HandCategory, ranks ...int) HandValue {
	score := uint64(category)
	for i := 0; i < 5; i++ {
		score *= 15
		if i < len(ranks) {
			score += uint64(ranks[i])
		}
	}
	return HandValue{Category: category, Score: score}
}
