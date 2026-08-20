package model

type Round struct {
	ID             uint `json:"id" db:"id"`
	GameID         uint `json:"game_id" db:"game_id"`
	PromptID       uint `json:"prompt_id" db:"prompt_id"`
	TruthFinders   uint `json:"truth_finders" db:"truth_finders"`
	TotalGuessers  uint `json:"total_guessers" db:"total_guessers"`
	PlayersPresent uint `json:"players_present" db:"players_present"`
}
