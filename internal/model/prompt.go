package model

import "time"

type Prompt struct {
	ID               uint       `json:"id" db:"id"`
	UserID           uint       `json:"user_id" db:"user_id"`
	WrittenIn        string     `json:"written_in" db:"written_in"`
	Question         string     `json:"question" db:"question"`
	Truth            string     `json:"truth" db:"truth"`
	Category         string     `json:"category" db:"category"`
	TimesUsed        uint       `json:"times_used" db:"times_used"`
	GuessedCorrectly uint       `json:"guessed_correctly" db:"guessed_correctly"`
	BlockedAt        *time.Time `json:"blocked_at,omitempty" db:"blocked_at"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}
