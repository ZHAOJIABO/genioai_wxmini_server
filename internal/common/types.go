package common

import (
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
)

type EventChannel chan *vai.EventWatchResponse

type LLMProcessParams struct {
	Request         *vai.ChatMessageSendRequest
	StreamServer    vai.ChatService_SendChatMessageStreamServer
	History         []model.MessageHistory
	Output          model.Output
	CancelCh        model.CancelCh
	ImageContent    string
	HiddenReasoning bool
	ThinkingConfig  *ThinkingConfig
}

type ThinkingConfig struct {
	ThinkingBudget  *int32
	IncludeThoughts bool
}
