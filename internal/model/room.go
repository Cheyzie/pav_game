package model

import (
	"time"

	"github.com/Cheyzie/pav_game/internal/dtos"
)

type GameState string

const (
	LobbyState    GameState = "lobby"
	LieState      GameState = "lie"
	VotingState   GameState = "voting"
	ResultState   GameState = "result"
	FinishedState GameState = "finished"
)

type Room struct {
	Id               uint
	HostID           uint
	PromptsWrittenIn string
	Code             string
	Done             chan struct{}
	UsedPromptIDs    []uint
	State            GameState
	StateTransition  chan GameState
	Messages         chan []byte
	MessageLog       []dtos.Message
	Timers           map[GameState][]*time.Timer
	Round            uint
	Prompt           *Prompt
	// Answers holds the shuffled candidates published when voting opens, so a
	// client connecting mid-phase is shown the same list as everyone else.
	Answers []string
	// PhaseEndsAt is when the current phase's timer fires. Zero when the phase
	// is not on a clock.
	PhaseEndsAt time.Time
	Players     []*Player
}
