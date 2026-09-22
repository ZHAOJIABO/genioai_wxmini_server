package picture_workflow_task

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type PictureTaskProcessor struct {
	db *gorm.DB
}

func NewPictureTaskProcessor() *PictureTaskProcessor {
	return &PictureTaskProcessor{
		db: db.GetDB(),
	}
}

func (s *PictureTaskProcessor) Start() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := s.Process(); err != nil {
				zlog.Logger.Error("Failed to process picture tasks", zap.Error(err))
			}
		}
	}()

}

func (s *PictureTaskProcessor) Process() error {
	ctx := context.Background()
	deadline := time.Now().Add(-20 * time.Second)

	result := s.db.WithContext(ctx).
		Model(&model.PictureTask{}).
		Where("created_at < ? AND status < ?",
			deadline,
			int32(vai.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED)).
		Updates(map[string]interface{}{
			"status": int32(vai.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED),
			// "error":  "Task timeout: not completed within 24 hours",
		})

	if result.Error != nil {
		return errors.Wrap(result.Error, "failed to update timed out picture tasks")
	}
	if result.RowsAffected > 0 {
		zlog.Logger.Info("Updated timed out picture tasks",
			zap.Int64("affected_rows", result.RowsAffected))
	}
	return nil
}
