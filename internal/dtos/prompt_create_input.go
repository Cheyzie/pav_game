package dtos

type PromptCreateInput struct {
	WrittenIn string `json:"written_in" validate:"oneof=ua en"`
	Question  string `json:"question" validate:"required"`
	Truth     string `json:"truth" validate:"required"`
	Category  string `json:"category" validate:"required"`
}
