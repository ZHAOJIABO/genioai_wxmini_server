package dao

import (
	"gorm.io/gorm"

	"va_visionai_server/internal/model"
)

type PromptDao struct {
	db *gorm.DB
}

func NewPromptDao(db *gorm.DB) *PromptDao {
	return &PromptDao{db: db}
}

func (p *PromptDao) CreatePrompt(promptID, content, describe, lead, language, title, kind string, version int, display bool, enableCamera bool) (*model.Prompt, error) {
	prompt := &model.Prompt{
		PromptID:     promptID,
		Version:      version,
		Content:      content,
		Describe:     describe,
		Lead:         lead,
		Language:     language,
		Title:        title,
		Kind:         kind,
		EnableCamera: enableCamera,
	}

	err := p.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("prompt_id =? AND version <?", promptID, version).Delete(&model.Prompt{}).Error; err != nil {
			return err
		}
		if err := tx.Create(prompt).Error; err != nil {
			return err
		}
		return nil
	})
	return prompt, err
}

func (p *PromptDao) GetPrompt(promptID, language string) (*model.Prompt, error) {
	var prompt model.Prompt
	err := p.db.Where("prompt_id = ? and language = ?", promptID, language).Where("deleted_at is null").First(&prompt).Error
	return &prompt, err
}

func (p *PromptDao) GetPromptBatch(promptIDs []string, language string) ([]*model.Prompt, error) {
	var prompt []*model.Prompt
	err := p.db.Where("prompt_id in ? and language = ?", promptIDs, language).Where("deleted_at is null").Order("score desc").Find(&prompt).Error
	return prompt, err
}

func (p *PromptDao) ListPrompt(kind, language string) ([]*model.Prompt, error) {
	var prompts []*model.Prompt
	tx := p.db.Where("deleted_at is null and display is true")
	if kind != "" {
		tx = tx.Where("kind =?", kind)
	}
	if language != "" {
		tx = tx.Where("language =?", language)
	}

	err := tx.Order("score desc").Find(&prompts).Error
	return prompts, err
}

func (p *PromptDao) ListQuickPrompt(promptIDs []string, lang string) ([]*model.Prompt, error) {
	filterPromptKind := []string{"Popular", "default"}
	var prompts []*model.Prompt
	tx := p.db.Where("deleted_at is null and display is true").Where("prompt_id in ? and language = ? and kind not in ?", promptIDs, lang, filterPromptKind).
		Order("score desc").
		Limit(5)

	err := tx.Find(&prompts).Error
	return prompts, err
}

func (p *PromptDao) ListCameraPrompt(kind, language string) ([]*model.Prompt, error) {
	var prompts []*model.Prompt
	tx := p.db.Where("deleted_at is null AND display = true AND camera_show = true")
	if kind != "" {
		tx = tx.Where("kind = ?", kind)
	}
	if language != "" {
		tx = tx.Where("language = ?", language)
	}

	err := tx.Order("score desc").Find(&prompts).Error
	return prompts, err
}

// GetAstroPrompt 获取astro的prompt
func (p *PromptDao) GetAstroPrompt(promptID, language string) (*model.Prompt, error) {
	var prompt model.Prompt
	err := p.db.Table("st_constellation_prompt").Select("prompt AS content").Where("prompt_id = ? and language = ?", promptID, language).Where("deleted_at is null").First(&prompt).Error
	return &prompt, err
}

// GetGoCalPrompt 获取gocal的prompt
func (p *PromptDao) GetGoCalPrompt(promptID, language string) (*model.Prompt, error) {
	var prompt model.Prompt
	err := p.db.Table("st_gocal_prompt").Select("prompt AS content").Where("prompt_id = ? and language = ?", promptID, language).Where("deleted_at is null").First(&prompt).Error
	return &prompt, err
}
