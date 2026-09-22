package model

import (
	"encoding/json"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type Prompt struct {
	gorm.Model
	PromptID      string `gorm:"type:varchar(64);uniqueIndex:prompt_id_language_version"`
	Title         string `gorm:"type:varchar(255)"`
	Describe      string `gorm:"type:text"`
	Lead          string `gorm:"type:text"`
	LeadTitle     string `gorm:"type:varchar(255)"`
	Content       string `gorm:"type:text"`
	Kind          string `gorm:"type:varchar(255)"`
	Language      string `gorm:"type:varchar(10);uniqueIndex:prompt_id_language_version"`
	Version       int    `gorm:"type:int;uniqueIndex:prompt_id_language_version"`
	Score         int32  `gorm:"type:int"`
	Display       bool   `gorm:"type:tinyint(1)"`
	Icon          string `gorm:"type:varchar(255)"`
	PhotoQuestion string `gorm:"type:varchar(255)"`
	EnableCamera  bool   `gorm:"type:tinyint(1);default:false"`
	CameraShow    bool   `gorm:"type:tinyint(1);default:false"`
}

type PromptKind struct {
	gorm.Model
	Kind     string `gorm:"type:varchar(255)"`
	Label    string `gorm:"type:varchar(255)"`
	Language string `gorm:"type:varchar(10)"`
	Score    int32  `gorm:"type:int"`
	Display  bool   `gorm:"type:tinyint(1)"`
	IconURL  string `gorm:"type:varchar(255)"`
}

func (p *Prompt) DeepCopy() *Prompt {
	return &Prompt{
		PromptID:  p.PromptID,
		Title:     p.Title,
		Describe:  p.Describe,
		LeadTitle: p.LeadTitle,
		Lead:      p.Lead,
		Kind:      p.Kind,
		Icon:      p.Icon,

		PhotoQuestion: p.PhotoQuestion,
	}
}

func (p *Prompt) ConvertToVAIPrompt() *va_interface.Prompt {
	photoQuestion := []string{}
	err := json.Unmarshal([]byte(p.PhotoQuestion), &photoQuestion)
	if err != nil {
		zlog.Logger.Error("ConvertToVAIPrompt Unmarshal Error", zap.Error(err))
	}
	return &va_interface.Prompt{
		PromptId:      p.PromptID,
		Title:         p.Title,
		Description:   p.Describe,
		LeadTitle:     p.LeadTitle,
		Lead:          p.Lead,
		KindId:        p.Kind,
		Icon:          p.Icon,
		PhotoQuestion: photoQuestion,
		EnableCamera:  p.EnableCamera,
		CameraShow:    p.CameraShow,
	}
}
