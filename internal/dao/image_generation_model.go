package dao

import (
	"context"

	"gorm.io/gorm"

	"va_visionai_server/internal/model"
)

type ImageGenerationModelDao struct {
	db *gorm.DB
}

func NewImageGenerationModelDao(db *gorm.DB) *ImageGenerationModelDao {
	return &ImageGenerationModelDao{db: db}
}

// ListEnabledModels 获取所有启用的生图模型
func (d *ImageGenerationModelDao) ListEnabledModels(ctx context.Context, projectID string) ([]*model.ImageGenerationModel, error) {
	var models []*model.ImageGenerationModel
	err := d.db.WithContext(ctx).
		Where("enabled = ? AND project_id = ?", true, projectID).
		Order("sort DESC, id ASC"). // 按排序字段降序，ID升序
		Find(&models).Error

	return models, err
}

// GetModelByName 根据模型名称获取模型
func (d *ImageGenerationModelDao) GetModelByName(ctx context.Context, modelName string, projectID string) (*model.ImageGenerationModel, error) {
	var m model.ImageGenerationModel
	err := d.db.WithContext(ctx).
		Where("model_name = ? AND project_id = ? AND enabled = ?", modelName, projectID, true).
		First(&m).Error

	return &m, err
}

// ListAllModels 获取所有模型（包括禁用的）
func (d *ImageGenerationModelDao) ListAllModels(ctx context.Context, projectID string) ([]*model.ImageGenerationModel, error) {
	var models []*model.ImageGenerationModel
	err := d.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("sort DESC, id ASC").
		Find(&models).Error

	return models, err
}

// CreateModel 创建模型
func (d *ImageGenerationModelDao) CreateModel(ctx context.Context, m *model.ImageGenerationModel) error {
	return d.db.WithContext(ctx).Create(m).Error
}

// UpdateModel 更新模型
func (d *ImageGenerationModelDao) UpdateModel(ctx context.Context, m *model.ImageGenerationModel) error {
	return d.db.WithContext(ctx).Save(m).Error
}

// DeleteModel 软删除模型
func (d *ImageGenerationModelDao) DeleteModel(ctx context.Context, id uint) error {
	return d.db.WithContext(ctx).Delete(&model.ImageGenerationModel{}, id).Error
}
