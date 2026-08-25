package repository

import (
	"context"
	"fmt"

	"github.com/Cheyzie/pav_game/internal/model"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

const promptColumns = "id, user_id, written_in, question, truth, category, times_used, guessed_correctly, blocked_at, created_at, updated_at"

type PromptPostgres struct {
	db *sqlx.DB
}

func NewPromptPostgres(db *sqlx.DB) *PromptPostgres {
	return &PromptPostgres{db: db}
}

func (r *PromptPostgres) Store(prompt *model.Prompt) error {
	query := fmt.Sprintf(`INSERT INTO %s (user_id, written_in, question, truth, category )
		VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at;`, promptsTable)

	row := r.db.QueryRow(query, prompt.UserID, prompt.WrittenIn, prompt.Question, prompt.Truth, prompt.Category)

	if err := row.Scan(&prompt.ID, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
		return err
	}

	return nil
}

// GetRand picks a prompt the room has not seen yet.
func (r *PromptPostgres) GetRand(writtenIn string, usedID []uint) (model.Prompt, error) {
	var prompt model.Prompt
	query := fmt.Sprintf("SELECT %s FROM %s WHERE blocked_at IS NULL AND written_in=$1 AND id != ALL($2) ORDER BY random() LIMIT 1;",
		promptColumns, promptsTable)

	err := r.db.Get(&prompt, query, writtenIn, pq.Array(usedID))

	return prompt, err
}

func (r *PromptPostgres) CountByWrittenIn(writtenIn string) (uint, error) {
	var count uint
	query := fmt.Sprintf("SELECT count(*) FROM %s WHERE blocked_at IS NULL AND written_in=$1;",
		promptsTable)

	err := r.db.Get(&count, query, writtenIn)

	return count, err
}

func (r *PromptPostgres) CountByUser(userID uint) (uint, error) {
	var count uint
	query := fmt.Sprintf("SELECT count(*) FROM %s WHERE user_id = $1;",
		promptsTable)

	err := r.db.Get(&count, query, userID)

	return count, err
}

func (r *PromptPostgres) GetCategories(writtenIn string) ([]model.Category, error) {
	categories := make([]model.Category, 0)
	query := fmt.Sprintf("SELECT category, count(*) as prompts_count FROM %s WHERE written_in = $1 GROUP BY category ORDER BY count(*) DESC;",
		promptsTable)

	err := r.db.Select(&categories, query, writtenIn)

	return categories, err
}

func (r *PromptPostgres) GetByID(id uint) (model.Prompt, error) {
	var prompt model.Prompt
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = $1;", promptColumns, promptsTable)
	err := r.db.Get(&prompt, query, id)

	return prompt, err
}

func (r *PromptPostgres) List(category string, limit, offset int) ([]model.Prompt, error) {
	prompts := make([]model.Prompt, 0)
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE ($1 = '' OR category = $1)
		ORDER BY id LIMIT $2 OFFSET $3;`, promptColumns, promptsTable)

	if err := r.db.Select(&prompts, query, category, limit, offset); err != nil {
		return nil, err
	}

	return prompts, nil
}

func (r *PromptPostgres) IncrementTimesUsed(ctx context.Context, id uint, guessedCorrectly uint) error {
	query := fmt.Sprintf("UPDATE %s SET times_used = times_used + 1, guessed_correctly = guessed_correctly + $1, updated_at = NOW() WHERE id = $2;",
		promptsTable)

	res, err := r.db.ExecContext(ctx, query, guessedCorrectly, id)
	if err != nil {
		return err
	}

	return requireOneRow(res, "prompt", id)
}

func (r *PromptPostgres) Update(prompt *model.Prompt) error {
	query := fmt.Sprintf(`UPDATE %s SET question = $2, truth = $3, category = $4, written_in = $5,
		updated_at = NOW() WHERE id = $1 RETURNING updated_at;`, promptsTable)

	row := r.db.QueryRow(query, prompt.ID, prompt.Question, prompt.Truth, prompt.Category, prompt.WrittenIn)

	return row.Scan(&prompt.UpdatedAt)
}

func (r *PromptPostgres) Delete(id uint) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1;", promptsTable)

	res, err := r.db.Exec(query, id)
	if err != nil {
		return err
	}

	return requireOneRow(res, "prompt", id)
}
