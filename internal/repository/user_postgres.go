package repository

import (
	"fmt"

	"github.com/Cheyzie/pav_game/internal/model"
	"github.com/jmoiron/sqlx"
)

const userColumns = "id, username, email"

type UserPostgres struct {
	db *sqlx.DB
}

func NewUserPostgres(db *sqlx.DB) *UserPostgres {
	return &UserPostgres{db: db}
}

func (r *UserPostgres) GetByEmail(email string) (model.User, error) {
	var user model.User
	query := fmt.Sprintf("SELECT %s FROM %s WHERE email = $1;", userColumns, usersTable)
	err := r.db.Get(&user, query, email)

	return user, err
}

func (r *UserPostgres) GetByCredentials(email, password_hash string) (model.User, error) {
	var user model.User
	query := fmt.Sprintf("SELECT %s FROM %s WHERE email = $1 AND password_hash = $2;",
		userColumns, usersTable)
	err := r.db.Get(&user, query, email, password_hash)

	return user, err
}

func (r *UserPostgres) GetByID(id uint) (model.User, error) {
	var user model.User
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = $1;", userColumns, usersTable)
	err := r.db.Get(&user, query, id)

	return user, err
}

func (r *UserPostgres) Create(user model.User) (uint, error) {
	var id uint

	query := fmt.Sprintf("INSERT INTO %s (username, email, password_hash) VALUES ($1, $2, $3) RETURNING id;",
		usersTable)
	row := r.db.QueryRow(query, user.Username, user.Email, user.Password)

	if err := row.Scan(&id); err != nil {
		return 0, err
	}

	return id, nil
}

// Update changes the mutable profile fields. Passwords are not touched here.
func (r *UserPostgres) Update(user *model.User) error {
	query := fmt.Sprintf("UPDATE %s SET username = $2, email = $3 WHERE id = $1;", usersTable)

	res, err := r.db.Exec(query, user.ID, user.Username, user.Email)
	if err != nil {
		return err
	}

	return requireOneRow(res, "user", user.ID)
}

func (r *UserPostgres) UpdatePassword(id uint, passwordHash string) error {
	query := fmt.Sprintf("UPDATE %s SET password_hash = $2 WHERE id = $1;", usersTable)

	res, err := r.db.Exec(query, id, passwordHash)
	if err != nil {
		return err
	}

	return requireOneRow(res, "user", id)
}

func (r *UserPostgres) Delete(id uint) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1;", usersTable)

	res, err := r.db.Exec(query, id)
	if err != nil {
		return err
	}

	return requireOneRow(res, "user", id)
}
