package model

import (
	"encoding/json"

	"gorm.io/gorm"

	vai "va_visionai_server/internal/va_interface"
)

// ImageGenerationModel 生图模型表
type ImageGenerationModel struct {
	gorm.Model
	ModelName        string `json:"model_name" gorm:"column:model_name;type:varchar(64);not null"`
	DisplayName      string `json:"display_name" gorm:"column:display_name;type:varchar(128);not null"`
	Description      string `json:"description" gorm:"column:description;type:varchar(500)"`
	Icon             string `json:"icon" gorm:"column:icon;type:varchar(255)"`
	Provider         string `json:"provider" gorm:"column:provider;type:varchar(64)"`
	SupportT2I       bool   `json:"support_t2i" gorm:"column:support_t2i;type:tinyint;not null;default:1"`
	SupportI2I       bool   `json:"support_i2i" gorm:"column:support_i2i;type:tinyint;not null;default:0"`
	CreditPoints     int    `json:"credit_points" gorm:"column:credit_points;type:int;not null;default:0"`
	DefaultWidth     int    `json:"default_width" gorm:"column:default_width;type:int;not null;default:1024"`
	DefaultHeight    int    `json:"default_height" gorm:"column:default_height;type:int;not null;default:1024"`
	SupportQualities   string `json:"support_qualities" gorm:"column:support_qualities;type:varchar(255)"`     // JSON 数组字符串
	QualityCreditRules string `json:"quality_credit_rules" gorm:"column:quality_credit_rules;type:varchar(500);default:'{}'"` // JSON: quality→额外积分映射
	MaxImages          int    `json:"max_images" gorm:"column:max_images;type:int;not null;default:1"`
	MaxInputImages     int    `json:"max_input_images" gorm:"column:max_input_images;type:int;not null;default:0"`
	Enabled          bool   `json:"enabled" gorm:"column:enabled;type:tinyint;not null;default:1"`
	Sort             int    `json:"sort" gorm:"column:sort;type:int;not null;default:0"`
	ProjectID        string `json:"project_id" gorm:"column:project_id;type:varchar(64);not null;default:visionai"`
}

func (ImageGenerationModel) TableName() string {
	return "va_image_generation_model"
}

// ToProto 转换为 Proto 格式
func (m *ImageGenerationModel) ToProto() *vai.ImageGenerationModelInfo {
	// 解析 support_qualities JSON 字符串
	var qualities []string
	if m.SupportQualities != "" {
		_ = json.Unmarshal([]byte(m.SupportQualities), &qualities)
	}

	// 解析 quality_credit_rules JSON 字符串
	qualityCreditRules := make(map[string]int32)
	if m.QualityCreditRules != "" {
		var raw map[string]int32
		if err := json.Unmarshal([]byte(m.QualityCreditRules), &raw); err == nil {
			qualityCreditRules = raw
		}
	}

	return &vai.ImageGenerationModelInfo{
		ModelName:          m.ModelName,
		DisplayName:        m.DisplayName,
		Description:        m.Description,
		Icon:               m.Icon,
		SupportT2I:         m.SupportT2I,
		SupportI2I:         m.SupportI2I,
		CreditPoints:       int32(m.CreditPoints),
		DefaultWidth:       int32(m.DefaultWidth),
		DefaultHeight:      int32(m.DefaultHeight),
		SupportQualities:   qualities,
		MaxImages:          int32(m.MaxImages),
		MaxInputImages:     int32(m.MaxInputImages),
		QualityCreditRules: qualityCreditRules,
	}
}
