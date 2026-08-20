package repository

import (
	"fmt"

	"github.com/Cheyzie/pav_game/internal/model"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

const promptColumns = "id, situation, truth, category, is_fallback, times_used, created_at, updated_at"

type PromptPostgres struct {
	db *sqlx.DB
}

func NewPromptPostgres(db *sqlx.DB) *PromptPostgres {
	return &PromptPostgres{db: db}
}

func (r *PromptPostgres) Store(prompt *model.Prompt) error {
	query := fmt.Sprintf(`INSERT INTO %s (situation, truth, category, is_fallback)
		VALUES ($1, $2, $3, $4) RETURNING id, created_at, updated_at;`, promptsTable)

	row := r.db.QueryRow(query, prompt.Situation, prompt.Truth, prompt.Category, prompt.IsFallback)

	if err := row.Scan(&prompt.ID, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
		return err
	}

	return nil
}

// GetRand picks a prompt the room has not seen yet.
func (r *PromptPostgres) GetRand(usedID []uint) (model.Prompt, error) {
	var prompt model.Prompt
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id != ALL($1) ORDER BY random() LIMIT 1;",
		promptColumns, promptsTable)

	err := r.db.Get(&prompt, query, pq.Array(usedID))

	return prompt, err
}

func (r *PromptPostgres) GetByID(id uint) (model.Prompt, error) {
	var prompt model.Prompt
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = $1;", promptColumns, promptsTable)
	err := r.db.Get(&prompt, query, id)

	return prompt, err
}

// List pages through prompts, optionally narrowed to one category. An empty
// category returns every prompt.
func (r *PromptPostgres) List(category string, limit, offset int) ([]model.Prompt, error) {
	prompts := make([]model.Prompt, 0)
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE ($1 = '' OR category = $1)
		ORDER BY id LIMIT $2 OFFSET $3;`, promptColumns, promptsTable)

	if err := r.db.Select(&prompts, query, category, limit, offset); err != nil {
		return nil, err
	}

	return prompts, nil
}

// IncrementTimesUsed bumps the usage counter after a prompt has been played.
// Nothing writes times_used otherwise.
func (r *PromptPostgres) IncrementTimesUsed(id uint) error {
	query := fmt.Sprintf("UPDATE %s SET times_used = times_used + 1, updated_at = NOW() WHERE id = $1;",
		promptsTable)

	res, err := r.db.Exec(query, id)
	if err != nil {
		return err
	}

	return requireOneRow(res, "prompt", id)
}

func (r *PromptPostgres) Update(prompt *model.Prompt) error {
	query := fmt.Sprintf(`UPDATE %s SET situation = $2, truth = $3, category = $4, is_fallback = $5,
		updated_at = NOW() WHERE id = $1 RETURNING updated_at;`, promptsTable)

	row := r.db.QueryRow(query, prompt.ID, prompt.Situation, prompt.Truth, prompt.Category, prompt.IsFallback)

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
