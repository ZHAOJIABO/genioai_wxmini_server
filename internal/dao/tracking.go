package dao

import (
	"context"

	"github.com/pkg/errors"
	"gorm.io/gorm"

	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
)

type TrackingDao struct {
	db *gorm.DB
}

func NewTrackingDao() *TrackingDao {
	return &TrackingDao{
		db: db.GetDB(),
	}
}

// CreateTrackingEvent 创建事件记录
func (d *TrackingDao) CreateTrackingEvent(ctx context.Context, event *model.TrackingEvent) error {
	return errors.Wrap(
		d.db.WithContext(ctx).Create(event).Error,
		"create tracking event",
	)
}
