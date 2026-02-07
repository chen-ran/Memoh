package schedule

import "context"

// TriggerPayload describes the parameters passed to the chat side when a schedule triggers.
type TriggerPayload struct {
	ID          string
	Name        string
	Description string
	Pattern     string
	MaxCalls    *int
	Command     string
	OwnerUserID string
}

// Triggerer 负责触发与聊天相关的调度执行。
type Triggerer interface {
	TriggerSchedule(ctx context.Context, botID string, payload TriggerPayload, token string) error
}
