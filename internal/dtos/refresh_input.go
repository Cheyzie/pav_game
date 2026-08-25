package dtos

type RefreshInput struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}
