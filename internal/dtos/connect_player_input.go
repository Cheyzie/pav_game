package dtos

type ConnectPlayerInput struct {
	Token string `json:"token"`
	Code  string `json:"code"`
}
