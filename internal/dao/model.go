package dao

import (
	"context"

	"gorm.io/gorm"

	"va_visionai_server/internal/model"
)

type ModelDao struct {
	db *gorm.DB
}

func NewModelDao(db *gorm.DB) *ModelDao {
	return &ModelDao{db: db}
}

// ListModelsWithLang returns all enabled models filtered by language
func (d *ModelDao) ListModelsWithLang(ctx context.Context, projectID string, language string) ([]*model.Model, error) {
	var models []*model.Model
	query := d.db.WithContext(ctx).Where("display = ?", 1)

	if language != "" {
		query = query.Where("language = ?", language)
	}

	if projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}

	if err := query.Order("sort DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	return models, nil
}

func (d *ModelDao) GetModel(ctx context.Context, modelID string) (*model.Model, error) {
	return d.GetModelWithLang(ctx, modelID, "")
}

// GetModelWithLang 根据模型ID和语言获取模型
func (d *ModelDao) GetModelWithLang(ctx context.Context, modelID string, language string) (*model.Model, error) {
	var model model.Model
	query := d.db.WithContext(ctx).Where("model_id = ?", modelID)

	if language != "" {
		query = query.Where("language = ?", language)
	}

	if err := query.First(&model).Error; err != nil {
		return nil, err
	}
	return &model, nil
}

// GetDefaultModels 获取默认的聊天和视觉模型
func (d *ModelDao) GetDefaultModels(ctx context.Context) (*model.Model, *model.Model, error) {
	return d.GetDefaultModelsWithLang(ctx, "")
}

// GetDefaultModelsWithLang 根据语言获取默认的聊天和视觉模型
func (d *ModelDao) GetDefaultModelsWithLang(ctx context.Context, language string) (*model.Model, *model.Model, error) {
	var models []*model.Model
	query := d.db.WithContext(ctx).Where("display = ? AND (default_chat_model = ? OR default_vision_model = ?)", 1, true, true)

	if language != "" {
		query = query.Where("language = ?", language)
	}

	err := query.Order("sort DESC").Find(&models).Error
	if err != nil {
		return nil, nil, err
	}

	var chatModel, visionModel *model.Model
	for _, m := range models {
		if m.DefaultChatModel {
			chatModel = m
		}
		if m.DefaultVisionModel {
			visionModel = m
		}
	}

	return chatModel, visionModel, nil
}
