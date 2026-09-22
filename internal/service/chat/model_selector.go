package chat

import (
	"context"

	"go.uber.org/zap"

	"va_visionai_server/internal/dao"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// ModelSelector 集中处理模型选择逻辑
type ModelSelector struct {
	modelDao *dao.ModelDao
}

// NewModelSelector 创建ModelSelector实例
func NewModelSelector(modelDao *dao.ModelDao) *ModelSelector {
	return &ModelSelector{
		modelDao: modelDao,
	}
}

// SelectModel 根据语言和消息类型选择合适的模型
func (s *ModelSelector) SelectModel(ctx context.Context, language string, isVisual bool) (vai.Model, error) {
	chatModel, visionModel, err := s.modelDao.GetDefaultModelsWithLang(ctx, language)
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取默认模型失败", zap.Error(err))
		return vai.Model_MODEL_GEMINI_2_0_FLASH, err
	}

	if isVisual && visionModel != nil {
		// 获取模型枚举值
		modelEnum, err := getModelEnum(visionModel.ModelID)
		if err != nil {
			zlog.LogWithContext(ctx).Error("获取视觉模型枚举失败", zap.Error(err))
			return vai.Model_MODEL_GEMINI_2_0_FLASH, err
		}
		return modelEnum, nil
	}

	// 获取模型枚举值
	if chatModel == nil {
		return vai.Model_MODEL_GEMINI_2_0_FLASH, nil
	}

	modelEnum, err := getModelEnum(chatModel.ModelID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取聊天模型枚举失败", zap.Error(err))
		return vai.Model_MODEL_GEMINI_2_0_FLASH, err
	}

	return modelEnum, nil
}

// getModelEnum 将模型ID转换为枚举值
func getModelEnum(modelID string) (vai.Model, error) {
	if val, ok := vai.Model_value[modelID]; ok {
		return vai.Model(val), nil
	}
	return vai.Model_MODEL_GEMINI_2_0_FLASH, nil
}
