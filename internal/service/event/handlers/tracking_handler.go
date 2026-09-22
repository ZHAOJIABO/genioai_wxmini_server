package event_handlers

import (
	"context"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
)

type TrackingHandler struct {
	trackingDao *dao.TrackingDao
}

func NewTrackingHandler(trackingDao *dao.TrackingDao) *TrackingHandler {
	return &TrackingHandler{
		trackingDao: trackingDao,
	}
}

func (h *TrackingHandler) HandleEvent(ctx context.Context, event *model.UserEvent) error {
	return nil
}

func (h *TrackingHandler) EventType() []string {
	return []string{}
}
