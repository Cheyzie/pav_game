package model

import (
	"github.com/google/uuid"
)

type Player struct {
	ID          uuid.UUID
	UserId      uint
	IsReady     bool
	Nickname    string
	Token       string
	Score       uint
	ScoreDiff   uint
	JoinedRound uint
	LeavedRound uint
	Answer      *string
	Vote        *string
	MessageBus  chan<- []byte
}

type PlayerResult struct {
	Nickname  string  `json:"nickname"`
	Score     uint    `json:"score"`
	ScoreDiff uint    `json:"score_diff"`
	Answer    *string `json:"answer"`
	Vote      *string `json:"vote"`
}

type FinalResult struct {
	Nickname string `json:"nickname"`
	Score    uint   `json:"score"`
	Position uint   `json:"position"`
}

// PlayerState is the per-player view sent in a room snapshot.
type PlayerState struct {
	UserID    uint   `json:"user_id"`
	Nickname  string `json:"nickname"`
	Score     uint   `json:"score"`
	ScoreDiff uint   `json:"score_diff"`
	IsReady   bool   `json:"is_ready"`
	Connected bool   `json:"connected"`
}
