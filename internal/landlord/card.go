package landlord

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
)

type Suit uint8

const (
	Clubs Suit = iota
	Diamonds
	Hearts
	Spades
	Joker
)

type Rank uint8

const (
	Three Rank = iota + 3
	Four
	Five
	Six
	Seven
	Eight
	Nine
	Ten
	Jack
	Queen
	King
	Ace
	Two
	SmallJoker
	BigJoker
)

type Card struct {
	Rank Rank `json:"rank"`
	Suit Suit `json:"suit"`
}

func (c Card) String() string {
	if c.Rank == SmallJoker {
		return "SJ"
	}
	if c.Rank == BigJoker {
		return "BJ"
	}
	ranks := map[Rank]string{Ten: "T", Jack: "J", Queen: "Q", King: "K", Ace: "A", Two: "2"}
	rank := ranks[c.Rank]
	if rank == "" {
		rank = fmt.Sprintf("%d", c.Rank)
	}
	suits := [...]string{"c", "d", "h", "s"}
	if int(c.Suit) >= len(suits) {
		return rank + "?"
	}
	return rank + suits[c.Suit]
}

func validCard(card Card) bool {
	if card.Rank == SmallJoker || card.Rank == BigJoker {
		return card.Suit == Joker
	}
	return card.Rank >= Three && card.Rank <= Two && card.Suit >= Clubs && card.Suit <= Spades
}

func newDeck() []Card {
	deck := make([]Card, 0, 54)
	for suit := Clubs; suit <= Spades; suit++ {
		for rank := Three; rank <= Two; rank++ {
			deck = append(deck, Card{Rank: rank, Suit: suit})
		}
	}
	return append(deck, Card{Rank: SmallJoker, Suit: Joker}, Card{Rank: BigJoker, Suit: Joker})
}

func shuffledDeck() ([]Card, error) {
	deck := newDeck()
	for index := len(deck) - 1; index > 0; index-- {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(index+1)))
		if err != nil {
			return nil, fmt.Errorf("shuffle deck: %w", err)
		}
		other := int(randomIndex.Int64())
		deck[index], deck[other] = deck[other], deck[index]
	}
	return deck, nil
}

func sortCards(cards []Card) {
	sort.Slice(cards, func(left, right int) bool {
		if cards[left].Rank == cards[right].Rank {
			return cards[left].Suit < cards[right].Suit
		}
		return cards[left].Rank < cards[right].Rank
	})
}
