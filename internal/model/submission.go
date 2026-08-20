package model

import (
	"time"

	"github.com/google/uuid"
)

type Submission struct {
	ID        uint      `json:"id" db:"id"`
	GameID    uint      `json:"game_id" db:"game_id"`
	RoundID   uint      `json:"round_id" db:"round_id"`
	PlayerID  uuid.UUID `json:"player_id" db:"player_id"`
	Text      string    `json:"text" db:"text"`
	Nickname  string    `json:"nickname" db:"nickname"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
