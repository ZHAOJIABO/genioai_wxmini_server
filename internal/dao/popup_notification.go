package dao

import (
	"context"
	"time"

	"gorm.io/gorm"

	"va_visionai_server/internal/model"
)

type PopupNotificationDao struct {
	db *gorm.DB
}

func NewPopupNotificationDao(db *gorm.DB) *PopupNotificationDao {
	return &PopupNotificationDao{db: db}
}

// GetActiveNotifications 查询当前生效的弹窗提醒列表
// status=1、当前时间在 start_time~end_time 范围内，按 priority DESC 排序
func (d *PopupNotificationDao) GetActiveNotifications(ctx context.Context, platform string) ([]*model.PopupNotification, error) {
	var notifications []*model.PopupNotification
	now := time.Now()

	query := d.db.WithContext(ctx).
		Where("status = ?", 1).
		Where("(start_time IS NULL OR start_time <= ?)", now).
		Where("(end_time IS NULL OR end_time >= ?)", now)

	if platform != "" && platform != "all" {
		query = query.Where("platform IN ?", []string{"all", platform})
	}

	err := query.Order("priority DESC").Find(&notifications).Error
	return notifications, err
}
