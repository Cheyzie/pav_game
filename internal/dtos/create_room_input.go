package dtos

type CreateRoomInput struct {
	PromptsWrittenIn string `json:"prompts_written_in" validate:"oneof=ua en"`
}
