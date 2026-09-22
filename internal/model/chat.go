package model

import (
	"time"

	"gorm.io/gorm"

	vai "va_visionai_server/internal/va_interface"
)

// Chat表
type Chat struct {
	// 自增ID
	Id int64
	// 项目ID
	ProjectID string `gorm:"type:varchar(64)"`
	// ChatID
	ChatID string `gorm:"type:varchar(64)"`
	// Chat主题Title
	Title string
	// Chat主题Image，一般是第一个图片，
	Image string
	// 归属UserId
	UserID string `gorm:"type:varchar(64)"`
	// 状态，0正常，1删除
	Status int
	// CreateTime
	CreateTime int64
	// 最后一次对话时间
	LastTime *time.Time
	// 对话使用的promptID
	PromptID string
	// 是否是视频对话
	IsVideo bool
}

type ChatTaskInfo struct {
	ChatID       string
	LastChatTime *time.Time
}

type UserPersonalChat struct {
	gorm.Model

	ChatID          string
	UserID          string
	UserDocID       string
	TargetUserDocID string
	Title           string
}

func (c *Chat) ChatToGRPC() *vai.Chat {
	var lastTime string
	if c.LastTime != nil {
		lastTime = c.LastTime.Format("2006-01-02 15:04:05")
	}
	var createTime string
	if c.CreateTime > 0 {
		createTime = time.Unix(c.CreateTime, 0).Format("2006-01-02 15:04:05")
	}

	return &vai.Chat{
		ChatId:     c.ChatID,
		Title:      c.Title,
		CreateTime: createTime,
		Image:      c.Image,
		LastTime:   lastTime,
		// Prompt: &vai.Prompt{
		// 	PromptId: c.PromptID,
		// },
	}
}
