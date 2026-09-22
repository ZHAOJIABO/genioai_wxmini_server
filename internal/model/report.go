package model

import (
	"gorm.io/gorm"

	vai "va_visionai_server/internal/va_interface"
)

type MessageReportInfo struct {
	gorm.Model
	UserID    string            `gorm:"type:varchar(128)"`
	MessageID string            `gorm:"uniqueIndex;type:varchar(128)"`
	ChatID    string            `gorm:"type:varchar(128)"`
	Quality   vai.LLMMsgQuality `gorm:"type:tinyint;UNSIGNED"`
}

type UserFeedbackRecord struct {
	gorm.Model

	ProjectID       string `gorm:"type:varchar(128)"`
	UserID          string `gorm:"type:varchar(128)"`
	FeedbackType    string `gorm:"type:varchar(128)"`
	FeedbackContent string `gorm:"type:text"`
	ScreenshotUrls  string `gorm:"type:text;"`
	HeaderInfo      string `gorm:"type:text"`
	Mail            string `gorm:"type:varchar(128)"`
}
