package model

import "database/sql"

// PopupNotification 首页弹窗提醒
type PopupNotification struct {
	BaseModel
	// 标题
	Title string `gorm:"column:title;type:varchar(255)"`
	// 内容(支持富文本)
	Content string `gorm:"column:content;type:text"`
	// 图片URL
	ImageURL string `gorm:"column:image_url;type:varchar(500)"`
	// 跳转链接
	LinkURL string `gorm:"column:link_url;type:varchar(500)"`
	// 弹窗类型(announcement, promotion, update 等)
	NotificationType string `gorm:"column:notification_type;type:varchar(50)"`
	// 目标平台(all / ios / android)
	Platform string `gorm:"column:platform;type:varchar(20);default:all"`
	// 排序权重，越大越靠前
	Priority int `gorm:"column:priority;type:int;default:0"`
	// 状态 1=启用 0=禁用
	Status int `gorm:"column:status;type:tinyint(1);default:1"`
	// 生效开始时间(可为空，空表示立即生效)
	StartTime sql.NullTime `gorm:"column:start_time"`
	// 生效结束时间(可为空，空表示永久生效)
	EndTime sql.NullTime `gorm:"column:end_time"`
}
