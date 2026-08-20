package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"pokernode/internal/landlord"
	"pokernode/internal/poker"
)

const (
	gameTypeTexasHoldem = "texas_holdem"
	gameTypeLandlord    = landlord.GameType
)

type gameActionInput struct {
	Action         string          `json:"action"`
	Amount         int64           `json:"amount_cents"`
	Bid            int             `json:"bid"`
	Cards          []landlord.Card `json:"cards"`
	ExpectedTurnID uint64          `json:"expected_turn_id"`
}

type runtimePlayerView struct {
	UserID int64
	Name   string
	Seat   int
	Stack  int64
	Ready  bool
}

type runtimeSnapshot struct {
	ID                   string
	Name                 string
	GameType             string
	SmallBlind           int64
	BigBlind             int64
	BaseStake            int64
	HandID               int64
	Phase                string
	ViewerSeat           int
	Players              []runtimePlayerView
	MaxPlayers           int
	CanLeave             bool
	HandActive           bool
	ActingSeat           int
	ActionTimeoutSeconds int
	ActionDeadlineAt     int64
	TurnID               uint64
}

func newPokerRuntime(table *poker.Table) *tableRuntime {
	return &tableRuntime{table: table}
}

func newLandlordRuntime(table *landlord.Table) *tableRuntime {
	return &tableRuntime{landlord: table}
}

func restoreTableRuntime(data []byte) (*tableRuntime, error) {
	var header struct {
		GameType string `json:"game_type"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("decode table type: %w", err)
	}
	if header.GameType == gameTypeLandlord {
		table, err := landlord.RestoreTable(data)
		if err != nil {
			return nil, err
		}
		return newLandlordRuntime(table), nil
	}
	if header.GameType != "" && header.GameType != gameTypeTexasHoldem {
		return nil, fmt.Errorf("unsupported game type %q", header.GameType)
	}
	table, err := poker.RestoreTable(data)
	if err != nil {
		return nil, err
	}
	return newPokerRuntime(table), nil
}

func (runtime *tableRuntime) gameType() string {
	if runtime.landlord != nil {
		return gameTypeLandlord
	}
	return gameTypeTexasHoldem
}

func (runtime *tableRuntime) snapshot(viewerID int64) any {
	if runtime.landlord != nil {
		return runtime.landlord.Snapshot(viewerID)
	}
	return runtime.table.Snapshot(viewerID)
}

func (runtime *tableRuntime) commonSnapshot(viewerID int64) runtimeSnapshot {
	if runtime.landlord != nil {
		snapshot := runtime.landlord.Snapshot(viewerID)
		players := make([]runtimePlayerView, 0, len(snapshot.Players))
		for _, player := range snapshot.Players {
			players = append(players, runtimePlayerView{UserID: player.UserID, Name: player.Name, Seat: player.Seat, Stack: player.Stack, Ready: player.Ready})
		}
		return runtimeSnapshot{
			ID: snapshot.ID, Name: snapshot.Name, GameType: gameTypeLandlord, BaseStake: snapshot.BaseStake,
			HandID: snapshot.HandID, Phase: string(snapshot.Phase), ViewerSeat: snapshot.ViewerSeat,
			Players: players, MaxPlayers: landlord.MaxSeats, CanLeave: snapshot.CanLeave,
			HandActive: snapshot.Phase == landlord.PhaseBidding || snapshot.Phase == landlord.PhasePlaying,
			ActingSeat: snapshot.ActingSeat, ActionTimeoutSeconds: snapshot.ActionTimeoutSeconds,
			ActionDeadlineAt: snapshot.ActionDeadlineAt, TurnID: snapshot.TurnID,
		}
	}
	snapshot := runtime.table.Snapshot(viewerID)
	players := make([]runtimePlayerView, 0, len(snapshot.Players))
	for _, player := range snapshot.Players {
		players = append(players, runtimePlayerView{UserID: player.UserID, Name: player.Name, Seat: player.Seat, Stack: player.Stack, Ready: player.Ready})
	}
	return runtimeSnapshot{
		ID: snapshot.ID, Name: snapshot.Name, GameType: gameTypeTexasHoldem,
		SmallBlind: snapshot.SmallBlind, BigBlind: snapshot.BigBlind, HandID: snapshot.HandID,
		Phase: string(snapshot.Street), ViewerSeat: snapshot.ViewerSeat, Players: players,
		MaxPlayers: poker.MaxSeats, CanLeave: snapshot.CanLeave,
		HandActive: snapshot.Street == poker.StreetPreflop || snapshot.Street == poker.StreetFlop || snapshot.Street == poker.StreetTurn || snapshot.Street == poker.StreetRiver,
		ActingSeat: snapshot.ActingSeat, ActionTimeoutSeconds: snapshot.ActionTimeoutSeconds,
		ActionDeadlineAt: snapshot.ActionDeadlineAt, TurnID: snapshot.TurnID,
	}
}

func (runtime *tableRuntime) isSeated(userID int64) bool {
	if runtime.landlord != nil {
		return runtime.landlord.IsSeated(userID)
	}
	return runtime.table.IsSeated(userID)
}

func (runtime *tableRuntime) stackFor(userID int64) (int64, bool) {
	if runtime.landlord != nil {
		return runtime.landlord.StackFor(userID)
	}
	return runtime.table.StackFor(userID)
}

func (runtime *tableRuntime) join(userID int64, name string, buyIn int64) (int, error) {
	if runtime.landlord != nil {
		return runtime.landlord.Join(userID, name, buyIn)
	}
	return runtime.table.Join(userID, name, buyIn)
}

func (runtime *tableRuntime) leave(userID int64) (int64, error) {
	if runtime.landlord != nil {
		return runtime.landlord.Leave(userID)
	}
	return runtime.table.Leave(userID)
}

func (runtime *tableRuntime) ready(userID int64) (bool, error) {
	if runtime.landlord != nil {
		return runtime.landlord.Ready(userID)
	}
	return runtime.table.Ready(userID)
}

func (runtime *tableRuntime) act(userID int64, input gameActionInput) error {
	if runtime.landlord != nil {
		return runtime.landlord.Act(userID, landlord.ActionType(input.Action), input.Bid, input.Cards)
	}
	return runtime.table.Act(userID, poker.ActionType(input.Action), input.Amount)
}

func (runtime *tableRuntime) timeout(turnID uint64, now time.Time) (string, bool, error) {
	if runtime.landlord != nil {
		action, applied, err := runtime.landlord.Timeout(turnID, now)
		return string(action), applied, err
	}
	action, applied, err := runtime.table.Timeout(turnID, now)
	return string(action), applied, err
}

func (runtime *tableRuntime) marshalState() ([]byte, error) {
	if runtime.landlord != nil {
		return runtime.landlord.MarshalState()
	}
	return runtime.table.MarshalState()
}

func tableGameAPIError(err error) error {
	if errors.Is(err, landlord.ErrActionTimedOut) {
		return &apiError{Status: 409, Message: "本轮行动时间已结束"}
	}
	if errors.Is(err, landlord.ErrNotYourTurn) || errors.Is(err, landlord.ErrHandInProgress) {
		return &apiError{Status: 409, Message: err.Error()}
	}
	if errors.Is(err, landlord.ErrTableFull) || errors.Is(err, landlord.ErrAlreadySeated) || errors.Is(err, landlord.ErrNotSeated) {
		return &apiError{Status: 409, Message: err.Error()}
	}
	return pokerAPIError(err)
}
