package model

type Category struct {
	Category     string `json:"category" db:"category"`
	PromptsCount uint   `json:"prompts_count" db:"prompts_count"`
}
