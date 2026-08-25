package dtos

type SignupInput struct {
	Email    string `json:"email" binding:"required" validate:"required,email"`
	Username string `json:"username" binding:"required" validate:"required"`
	Password string `json:"password" binding:"required" validate:"required,min=8"`
}
