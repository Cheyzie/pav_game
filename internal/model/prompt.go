package model

import "time"

type Prompt struct {
	ID         uint      `json:"id" db:"id"`
	Situation  string    `json:"situation" db:"situation"`
	Truth      string    `json:"truth" db:"truth"`
	Category   string    `json:"category" db:"category"`
	IsFallback bool      `json:"is_fallback" db:"is_fallback"`
	TimesUsed  uint      `json:"times_used" db:"times_used"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}
