package service

import (
	"context"

	"gorm.io/gorm"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
)

type ModelService struct {
	modelDao *dao.ModelDao
}

func NewModelService(db *gorm.DB) *ModelService {
	return &ModelService{
		modelDao: dao.NewModelDao(db),
	}
}

// ListModelsWithLang returns all available models filtered by language
func (s *ModelService) ListModelsWithLang(ctx context.Context, projectID string, language string) ([]*model.Model, error) {
	return s.modelDao.ListModelsWithLang(ctx, projectID, language)
}

// GetModelByID returns a model by its ID
func (s *ModelService) GetModelByID(modelID string) (vai.Model, error) {
	if val, ok := vai.Model_value[modelID]; ok {
		return vai.Model(val), nil
	}
	return vai.Model_MODEL_GPT4O, nil
}
