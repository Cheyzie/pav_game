package dtos

type ActionType string

const (
	SendMessageAction ActionType = "send_message"
	ReadyAction       ActionType = "ready"
	LieAction         ActionType = "lie"
	VoteAction        ActionType = "vote"
	ResetAction       ActionType = "reset"
)

type Action struct {
	Type    ActionType `json:"type"`
	Payload *string    `json:"payload,omitempty"`
}
