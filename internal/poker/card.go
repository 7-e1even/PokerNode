package poker

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

type Suit uint8

const (
	Clubs Suit = iota
	Diamonds
	Hearts
	Spades
)

type Rank uint8

const (
	Two Rank = iota + 2
	Three
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
)

type Card struct {
	Rank Rank `json:"rank"`
	Suit Suit `json:"suit"`
}

func (c Card) String() string {
	ranks := map[Rank]string{Ten: "T", Jack: "J", Queen: "Q", King: "K", Ace: "A"}
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

func newDeck() []Card {
	deck := make([]Card, 0, 52)
	for suit := Clubs; suit <= Spades; suit++ {
		for rank := Two; rank <= Ace; rank++ {
			deck = append(deck, Card{Rank: rank, Suit: suit})
		}
	}
	return deck
}

func shuffledDeck() ([]Card, error) {
	deck := newDeck()
	for i := len(deck) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return nil, fmt.Errorf("shuffle deck: %w", err)
		}
		j := int(n.Int64())
		deck[i], deck[j] = deck[j], deck[i]
	}
	return deck, nil
}
