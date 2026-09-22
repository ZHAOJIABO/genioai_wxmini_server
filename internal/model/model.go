package model

import (
	"gorm.io/gorm"

	vai "va_visionai_server/internal/va_interface"
)

type Model struct {
	gorm.Model
	ModelID            string `json:"model_id" gorm:"column:model_id;type:varchar(64);uniqueIndex:model_unique_index;not null"`
	ModelName          string `json:"model_name" gorm:"column:model_name;type:varchar(64);not null;uniqueIndex:model_unique_index"`
	DeploymentName     string `json:"deployment_name" gorm:"column:deployment_name;type:varchar(64);not null"`
	Display            int    `json:"display" gorm:"column:display;type:tinyint;not null;default:1"`
	IsVision           bool   `json:"is_vision" gorm:"column:is_vision;type:tinyint;not null;default:0"`
	Icon               string `json:"icon" gorm:"column:icon;type:varchar(255);not null"`
	Sort               int    `json:"sort" gorm:"column:sort;type:int;not null;default:0"` // 排序字段，值越大排序越靠前
	DefaultChatModel   bool   `json:"default_chat_model" gorm:"column:default_chat_model;type:tinyint;not null;default:0"`
	DefaultVisionModel bool   `json:"default_vision_model" gorm:"column:default_vision_model;type:tinyint;not null;default:0"`
	Language           string `json:"language" gorm:"column:language;type:varchar(10);not null;uniqueIndex:model_unique_index"`     // 语言字段
	ProjectID          string `json:"project_id" gorm:"column:project_id;type:varchar(64);not null;uniqueIndex:model_unique_index"` // 项目ID
}

func (m *Model) ToProto() *vai.ModelInfo {
	return &vai.ModelInfo{
		ModelName:  m.ModelID,
		ModelLabel: m.ModelName,
		Icon:       m.Icon,
	}
}
