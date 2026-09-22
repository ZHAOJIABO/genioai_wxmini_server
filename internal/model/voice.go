package model

import (
	"database/sql"

	"gorm.io/gorm"
)

type Voice struct {
	gorm.Model

	ProjectID      string `gorm:"type:varchar(100)"`
	UserID         string `gorm:"type:varchar(100)"`
	FileHash       string `gorm:"type:varchar(128)"`
	FileType       string `gorm:"type:varchar(16)"`
	FileUrl        string `gorm:"type:varchar(255)"`
	Content        string `gorm:"type:text"`
	Duration       uint   `gorm:"type:tinyint;UNSIGNED"`
	ParseStartTime sql.NullTime
	ParseEndTime   sql.NullTime
	Status         uint   `gorm:"type:tinyint;UNSIGNED"`
	TaskID         string `gorm:"type:varchar(128)"`
	TraceId        string `gorm:"type:varchar(100)"`
	ErrMsg         string `gorm:"type:text"`
}

type TTS struct {
	gorm.Model

	ChatID   string `gorm:"type:varchar(100)"`
	MsgID    string `gorm:"type:varchar(100)"`
	FileType string `gorm:"type:varchar(16)"`
	FileHash string `gorm:"type:varchar(100)"`
	FileUrl  string `gorm:"type:varchar(255)"`
}
