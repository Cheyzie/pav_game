package model

type User struct {
	ID       uint   `json:"id,omitempty" db:"id"`
	Username string `json:"username" binding:"required" db:"username"`
	Email    string `json:"email" binding:"required" db:"email"`
	Password string `json:"password,omitempty" binding:"required" db:"password_hash"`
}
