package repository

import (
	"fmt"

	"github.com/Cheyzie/pav_game/internal/model"
	"github.com/jmoiron/sqlx"
)

const gameColumns = "id, room_code, final_scores, started_at, ended_at"

type GamePostgres struct {
	db *sqlx.DB
}

func NewGamePostgres(db *sqlx.DB) *GamePostgres {
	return &GamePostgres{db: db}
}

// Store records a finished game and fills in its generated ID.
func (r *GamePostgres) Store(game *model.Game) error {
	query := fmt.Sprintf(`INSERT INTO %s (room_code, final_scores, started_at, ended_at)
		VALUES ($1, $2, $3, $4) RETURNING id;`, gamesTable)

	row := r.db.QueryRow(query, game.RoomCode, game.FinalScores, game.StartedAt, game.EndedAt)

	if err := row.Scan(&game.ID); err != nil {
		return err
	}

	return nil
}

func (r *GamePostgres) GetByID(id uint) (model.Game, error) {
	var game model.Game
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = $1;", gameColumns, gamesTable)
	err := r.db.Get(&game, query, id)

	return game, err
}

// ListByRoomCode returns every game played in a room, most recent first.
func (r *GamePostgres) ListByRoomCode(roomCode string) ([]model.Game, error) {
	games := make([]model.Game, 0)
	query := fmt.Sprintf("SELECT %s FROM %s WHERE room_code = $1 ORDER BY started_at DESC;",
		gameColumns, gamesTable)

	if err := r.db.Select(&games, query, roomCode); err != nil {
		return nil, err
	}

	return games, nil
}

// List pages through games, most recent first.
func (r *GamePostgres) List(limit, offset int) ([]model.Game, error) {
	games := make([]model.Game, 0)
	query := fmt.Sprintf("SELECT %s FROM %s ORDER BY started_at DESC LIMIT $1 OFFSET $2;",
		gameColumns, gamesTable)

	if err := r.db.Select(&games, query, limit, offset); err != nil {
		return nil, err
	}

	return games, nil
}

func (r *GamePostgres) Delete(id uint) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1;", gamesTable)

	res, err := r.db.Exec(query, id)
	if err != nil {
		return err
	}

	return requireOneRow(res, "game", id)
}
