package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

const (
	usersTable        = "users"
	refreshTokenTable = "refresh_tokens"
	promptsTable      = "prompts"
	gamesTable        = "games"
	roundsTable       = "rounds"
	submissionsTable  = "submissions"
)

// ErrNotFound reports that an update or delete matched no row. Reads surface
// sql.ErrNoRows from the driver instead; use errors.Is for either.
var ErrNotFound = errors.New("record not found")

// requireOneRow turns a no-op write into an error, so callers can tell "updated"
// apart from "that id does not exist".
func requireOneRow(res sql.Result, entity string, id uint) error {
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("%s %d: %w", entity, id, ErrNotFound)
	}
	return nil
}

type SqlConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	DBName   string
	SSLMode  string
}

func NewPostgresDB(cfg SqlConfig) (*sqlx.DB, error) {
	db, err := sqlx.Open("postgres", fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Username, cfg.DBName, cfg.Password, cfg.SSLMode))

	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	return db, nil
}
