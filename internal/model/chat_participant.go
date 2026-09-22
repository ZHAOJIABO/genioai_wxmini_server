package model

import (
	"gorm.io/gorm"
)

type ParticipantType string

const (
	ParticipantTypeUser          ParticipantType = "user"
	ParticipantTypeCustomProfile ParticipantType = "custom_profile"
)

// ChatParticipant 关联表，用于管理聊天与参与者（用户或档案）的多对多关系
type ChatParticipant struct {
	gorm.Model
	ChatID          string          `gorm:"type:varchar(64);not null;uniqueIndex:idx_chat_participant_unique,priority:1"`
	ParticipantID   string          `gorm:"type:varchar(64);not null;index;uniqueIndex:idx_chat_participant_unique,priority:2"`
	ParticipantType ParticipantType `gorm:"type:varchar(20);not null;uniqueIndex:idx_chat_participant_unique,priority:3"`
}
