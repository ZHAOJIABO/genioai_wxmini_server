package event_handlers

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type ViewPictureHandler struct {
	pictureTaskDao *dao.PictureTaskDao
}

func NewViewPictureHandler(pictureTaskDao *dao.PictureTaskDao) *ViewPictureHandler {
	return &ViewPictureHandler{pictureTaskDao: pictureTaskDao}
}

func (h *ViewPictureHandler) HandleEvent(ctx context.Context, event *model.UserEvent) error {
	zlog.LogWithContext(ctx).Info("handling view picture event",
		zap.String(constants.CtxEventType, event.EventType))
	var viewPictureEvent vai.ViewPictureEventData
	err := json.Unmarshal(event.EventData, &viewPictureEvent)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to unmarshal view picture event",
			zap.Error(err))
		return err
	}
	pictureID := viewPictureEvent.GetPictureTaskId()

	task, err := h.pictureTaskDao.GetTask(ctx, pictureID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get task",
			zap.String("pictureID", pictureID),
			zap.Error(err))
		return err
	}

	newViewCount := task.ViewCount + 1
	if task.ViewCountSeed == 0 {
		task.ViewCountSeed = time.Now().UnixNano()
	}
	fakeViewCount := utils.LogarithmicCalculationWithSeed(int(newViewCount), task.ViewCountSeed)
	err = h.pictureTaskDao.UpdateTaskCounts(ctx, pictureID, newViewCount, int32(fakeViewCount), task.ViewCountSeed)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to update task counts",
			zap.String("pictureID", pictureID),
			zap.Error(err))
		return err
	}

	return nil
}

func (h *ViewPictureHandler) EventType() []string {
	return []string{
		vai.UserEventType_USER_EVENT_TYPE_VIEW_PICTURE.String(),
	}
}
