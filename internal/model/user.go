package model

type User struct {
	ID       uint   `json:"id,omitempty" db:"id"`
	Username string `json:"username" db:"username"`
	Email    string `json:"email" db:"email"`
	Password string `json:"-" db:"password_hash"`
}
