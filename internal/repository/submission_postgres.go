package repository

import (
	"context"
	"fmt"

	"github.com/Cheyzie/pav_game/internal/model"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// "text" is quoted because it doubles as a type name in Postgres.
const submissionColumns = `id, game_id, round_id, player_id, "text", nickname, created_at`

const insertSubmission = `INSERT INTO %s (game_id, round_id, player_id, "text", nickname)
	VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at;`

type SubmissionPostgres struct {
	db *sqlx.DB
}

func NewSubmissionPostgres(db *sqlx.DB) *SubmissionPostgres {
	return &SubmissionPostgres{db: db}
}

// Store inserts one submission and fills in its generated ID and timestamp.
func (r *SubmissionPostgres) Store(ctx context.Context, submission *model.Submission) error {
	query := fmt.Sprintf(insertSubmission, submissionsTable)

	row := r.db.QueryRowContext(ctx, query,
		submission.GameID, submission.RoundID, submission.PlayerID, submission.Text, submission.Nickname)

	return row.Scan(&submission.ID, &submission.CreatedAt)
}

// StoreBatch writes a whole round's submissions in one transaction, so a
// partially recorded round is never left behind.
func (r *SubmissionPostgres) StoreBatch(ctx context.Context, submissions []*model.Submission) error {
	if len(submissions) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op once Commit succeeds

	stmt, err := tx.PreparexContext(ctx, fmt.Sprintf(insertSubmission, submissionsTable))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, submission := range submissions {
		row := stmt.QueryRowxContext(ctx,
			submission.GameID, submission.RoundID, submission.PlayerID, submission.Text, submission.Nickname)
		if err := row.Scan(&submission.ID, &submission.CreatedAt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *SubmissionPostgres) GetByID(ctx context.Context, id uint) (model.Submission, error) {
	var submission model.Submission
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = $1;", submissionColumns, submissionsTable)
	err := r.db.GetContext(ctx, &submission, query, id)

	return submission, err
}

func (r *SubmissionPostgres) ListByRoundID(ctx context.Context, roundID uint) ([]model.Submission, error) {
	return r.listBy(ctx, "round_id", roundID)
}

func (r *SubmissionPostgres) ListByGameID(ctx context.Context, gameID uint) ([]model.Submission, error) {
	return r.listBy(ctx, "game_id", gameID)
}

// ListByPlayerID returns everything one player ever submitted, newest last.
func (r *SubmissionPostgres) ListByPlayerID(ctx context.Context, playerID uuid.UUID) ([]model.Submission, error) {
	submissions := make([]model.Submission, 0)
	query := fmt.Sprintf("SELECT %s FROM %s WHERE player_id = $1 ORDER BY created_at, id;",
		submissionColumns, submissionsTable)

	if err := r.db.SelectContext(ctx, &submissions, query, playerID); err != nil {
		return nil, err
	}

	return submissions, nil
}

// listBy is shared by the foreign-key lookups; column is never user input.
func (r *SubmissionPostgres) listBy(ctx context.Context, column string, id uint) ([]model.Submission, error) {
	submissions := make([]model.Submission, 0)
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1 ORDER BY created_at, id;",
		submissionColumns, submissionsTable, column)

	if err := r.db.SelectContext(ctx, &submissions, query, id); err != nil {
		return nil, err
	}

	return submissions, nil
}

func (r *SubmissionPostgres) Delete(ctx context.Context, id uint) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1;", submissionsTable)

	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	return requireOneRow(res, "submission", id)
}
