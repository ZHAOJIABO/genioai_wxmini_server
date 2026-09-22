package event_handlers

import (
	"context"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/zlog"
)

type DefaultHandler struct{}

func NewDefaultHandler() *DefaultHandler {
	return &DefaultHandler{}
}

func (h *DefaultHandler) HandleEvent(ctx context.Context, event *model.UserEvent) error {
	zlog.LogWithContext(ctx).Info("handling default event",
		zap.String(constants.CtxEventType, event.EventType),
		zap.Any("UserEvent", event))
	return nil
}

func (h *DefaultHandler) EventType() []string {
	return []string{
		"default",
	}
}
