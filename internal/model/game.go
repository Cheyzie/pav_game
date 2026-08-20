package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Game struct {
	ID          uint        `json:"id" db:"id"`
	RoomCode    string      `json:"room_code" db:"room_code"`
	FinalScores FinalScores `json:"final_scores" db:"final_scores"`
	StartedAt   time.Time   `json:"started_at" db:"started_at"`
	EndedAt     time.Time   `json:"ended_at" db:"ended_at"`
}

type FinalScore struct {
	PlayerID    uuid.UUID `json:"player_id"`
	Nickname    string    `json:"nickname"`
	Score       uint      `json:"score"`
	JoinedRound uint      `json:"joined_round"`
	LeftRound   uint      `json:"left_round"`
}

// FinalScores is persisted in the games.final_scores JSONB column, so it
// carries its own encoding rather than relying on the driver.
type FinalScores []FinalScore

func (s FinalScores) Value() (driver.Value, error) {
	if s == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(s)
}

func (s *FinalScores) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("model: cannot scan %T into FinalScores", src)
	}
	return json.Unmarshal(data, s)
}
