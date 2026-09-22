package dao

import (
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/zlog"
)

// PromptKindDao 定义了一个 PromptKindDao 结构体
type PromptKindDao struct {
	db *gorm.DB
}

// NewPromptKindDao 创建一个新的 PromptKindDao 实例
func NewPromptKindDao() *PromptKindDao {
	return &PromptKindDao{db: db.GetDB()}
}

// CreatePromptKind 在数据库中创建一个新的 PromptKind
func (p *PromptKindDao) CreatePromptKind(promptKind *model.PromptKind) error {
	return p.db.Create(promptKind).Error
}

// GetPromptKind 根据 ID 获取一个 PromptKind
func (p *PromptKindDao) GetPromptKind(kind uint) (*model.PromptKind, error) {
	var promptKind model.PromptKind
	err := p.db.Where("kind = ?", kind).First(&promptKind).Error
	return &promptKind, err
}

// SavePromptKind 更新一个 PromptKind
func (p *PromptKindDao) SavePromptKind(promptKind *model.PromptKind) error {
	if promptKind.Kind == "" {
		zlog.Logger.Error("PromptKind kind is Required")
		return constants.ERR_INVALID_PARAM
	}
	return p.db.Where("kind = ?", promptKind.Kind).Save(promptKind).Error
}

// DeletePromptKind 根据 ID 删除一个 PromptKind
func (p *PromptKindDao) DeletePromptKind(kind uint) error {
	return p.db.Where("kind = ?", kind).Delete(&model.PromptKind{}).Error
}

// ListPromptKinds 获取所有 PromptKinds
func (p *PromptKindDao) ListPromptKinds(lang string) ([]*model.PromptKind, error) {
	var promptKinds []*model.PromptKind
	tx := p.db.Where("display is true and deleted_at is null ")
	if lang != "" {
		tx.Where("language = ?", lang)
	}
	tx.Order("score asc")
	err := tx.Find(&promptKinds).Error
	if err != nil {
		zlog.Logger.Error("ListPromptKinds Error", zap.Error(err))
		return nil, constants.ERR_REQUEST_FAILED
	}

	return promptKinds, err
}
