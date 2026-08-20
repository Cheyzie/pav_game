package repository

import (
	"context"
	"fmt"

	"github.com/Cheyzie/pav_game/internal/model"
	"github.com/jmoiron/sqlx"
)

const roundColumns = "id, game_id, prompt_id, truth_finders, total_guessers, players_present"

type RoundPostgres struct {
	db *sqlx.DB
}

func NewRoundPostgres(db *sqlx.DB) *RoundPostgres {
	return &RoundPostgres{db: db}
}

// Store inserts a round and fills in its generated ID.
func (r *RoundPostgres) Store(ctx context.Context, round *model.Round) error {
	query := fmt.Sprintf(`INSERT INTO %s (game_id, prompt_id, truth_finders, total_guessers, players_present)
		VALUES ($1, $2, $3, $4, $5) RETURNING id;`, roundsTable)

	row := r.db.QueryRowContext(ctx, query,
		round.GameID, round.PromptID, round.TruthFinders, round.TotalGuessers, round.PlayersPresent)

	return row.Scan(&round.ID)
}

func (r *RoundPostgres) GetByID(ctx context.Context, id uint) (model.Round, error) {
	var round model.Round
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = $1;", roundColumns, roundsTable)
	err := r.db.GetContext(ctx, &round, query, id)

	return round, err
}

// ListByGameID returns a game's rounds in the order they were played.
func (r *RoundPostgres) ListByGameID(ctx context.Context, gameID uint) ([]model.Round, error) {
	rounds := make([]model.Round, 0)
	query := fmt.Sprintf("SELECT %s FROM %s WHERE game_id = $1 ORDER BY id;", roundColumns, roundsTable)

	if err := r.db.SelectContext(ctx, &rounds, query, gameID); err != nil {
		return nil, err
	}

	return rounds, nil
}

// UpdateStats writes back the tallies collected while the round was played.
func (r *RoundPostgres) UpdateStats(ctx context.Context, round *model.Round) error {
	query := fmt.Sprintf(`UPDATE %s SET truth_finders = $2, total_guessers = $3, players_present = $4
		WHERE id = $1;`, roundsTable)

	res, err := r.db.ExecContext(ctx, query,
		round.ID, round.TruthFinders, round.TotalGuessers, round.PlayersPresent)
	if err != nil {
		return err
	}

	return requireOneRow(res, "round", round.ID)
}

func (r *RoundPostgres) Delete(ctx context.Context, id uint) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1;", roundsTable)

	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	return requireOneRow(res, "round", id)
}
