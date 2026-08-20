package repository

import (
	"context"
	"fmt"

	"github.com/Cheyzie/pav_game/internal/model"
	"github.com/jmoiron/sqlx"
)

const refreshTokenColumns = "id, token, user_id, session_name, ip_address, expires_at, created_at"

type RefreshTokenPostgres struct {
	db *sqlx.DB
}

func NewRefreshTokenPostgres(db *sqlx.DB) *RefreshTokenPostgres {
	return &RefreshTokenPostgres{
		db: db,
	}
}

// Store upserts the token for a (user, session) pair, so signing in again from
// the same session replaces the old token rather than accumulating rows.
func (r *RefreshTokenPostgres) Store(ctx context.Context, token *model.RefreshToken) error {
	query := fmt.Sprintf(`INSERT INTO %s (token, user_id, session_name, ip_address, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, session_name)
		DO UPDATE SET token = EXCLUDED.token, ip_address = EXCLUDED.ip_address, expires_at = EXCLUDED.expires_at
		RETURNING id, created_at;`, refreshTokenTable)

	row := r.db.QueryRowContext(ctx, query,
		token.Token, token.UserID, token.SessionName, token.IpAddress, token.ExpiresAt)

	if err := row.Scan(&token.ID, &token.CreatedAt); err != nil {
		return err
	}

	return nil
}

func (r *RefreshTokenPostgres) Get(ctx context.Context, token string) (*model.RefreshToken, error) {
	tokenEntity := new(model.RefreshToken)

	query := fmt.Sprintf("SELECT %s FROM %s WHERE token = $1;", refreshTokenColumns, refreshTokenTable)
	err := r.db.GetContext(ctx, tokenEntity, query, token)

	if err != nil {
		return nil, err
	}

	return tokenEntity, nil
}

func (r *RefreshTokenPostgres) ListByUserID(ctx context.Context, userID uint) ([]*model.RefreshToken, error) {
	tokenEntity := make([]*model.RefreshToken, 0, 1)

	query := fmt.Sprintf(`SELECT id, user_id, session_name, ip_address, expires_at, created_at
		FROM %s WHERE user_id = $1;`, refreshTokenTable)
	err := r.db.SelectContext(ctx, &tokenEntity, query, userID)

	if err != nil {
		return nil, err
	}

	return tokenEntity, nil
}

func (r *RefreshTokenPostgres) Delete(ctx context.Context, userID, tokenID uint) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE user_id = $1 AND id = $2;", refreshTokenTable)
	_, err := r.db.ExecContext(ctx, query, userID, tokenID)

	if err != nil {
		return err
	}

	return nil
}

// DeleteByToken revokes a single token, for sign-out on the current session.
func (r *RefreshTokenPostgres) DeleteByToken(ctx context.Context, token string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE token = $1;", refreshTokenTable)
	_, err := r.db.ExecContext(ctx, query, token)

	return err
}

// DeleteAllByUserID revokes every session a user has, for "sign out everywhere"
// and for password changes.
func (r *RefreshTokenPostgres) DeleteAllByUserID(ctx context.Context, userID uint) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE user_id = $1;", refreshTokenTable)
	_, err := r.db.ExecContext(ctx, query, userID)

	return err
}

// DeleteExpired clears out tokens that are already past their expiry and
// reports how many rows went away.
func (r *RefreshTokenPostgres) DeleteExpired(ctx context.Context) (int64, error) {
	query := fmt.Sprintf("DELETE FROM %s WHERE expires_at < NOW();", refreshTokenTable)

	res, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}
