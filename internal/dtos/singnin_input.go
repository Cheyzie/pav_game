package dtos

type SigninInput struct {
	Email       string `json:"email" binding:"required" validate:"required,email"`
	Password    string `json:"password" binding:"required" validate:"required"`
	SessionName string `json:"session_name" binding:"required" validate:"required"`
}
