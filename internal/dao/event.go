package dao

import (
	"context"

	"github.com/pkg/errors"
	"gorm.io/gorm"

	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
)

type EventDao struct {
	db *gorm.DB
}

func NewEventDao() *EventDao {
	return &EventDao{
		db: db.GetDB(),
	}
}

// CreateEvent 创建事件记录
func (d *EventDao) CreateEvent(ctx context.Context, event *model.UserEvent) error {
	return errors.Wrap(
		d.db.WithContext(ctx).Create(event).Error,
		"create event",
	)
}
