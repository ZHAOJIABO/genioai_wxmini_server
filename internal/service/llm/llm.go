package llm

import (
	"context"

	"go.uber.org/zap"

	"va_visionai_server/conf"
	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// updateContextWithModel updates context with model information
func updateContextWithModel(ctx context.Context, modelID vai.Model, modelName string, serverName string) context.Context {
	ctx = context.WithValue(ctx, constants.CtxModelID, modelID.Number())
	ctx = context.WithValue(ctx, constants.CtxModelName, modelID.String())
	ctx = context.WithValue(ctx, constants.CtxModelDeploymentName, modelName)
	ctx = context.WithValue(ctx, constants.CtxServerName, serverName)
	return ctx
}

type LLMFactoryImpl struct {
	modelDao  *dao.ModelDao
	voiceDao  *dao.VoiceDao
	uploadDao *dao.UploadDao
}

func NewLLMFactoryImpl(modelDao *dao.ModelDao, voiceDao *dao.VoiceDao, uploadDao *dao.UploadDao) *LLMFactoryImpl {
	return &LLMFactoryImpl{
		modelDao:  modelDao,
		voiceDao:  voiceDao,
		uploadDao: uploadDao,
	}
}

func (f *LLMFactoryImpl) CreateHandler(ctx context.Context, modelID vai.Model) (context.Context, common.LlmHandler, error) {
	model, err := f.modelDao.GetModel(ctx, modelID.String())
	if err != nil {
		zlog.LogWithContext(ctx).Error("get model failed", zap.Error(err))
		return ctx, nil, err
	}
	deploymentName := model.DeploymentName
	apiKey := ""
	endpoint := ""
	serverName := ""
	switch modelID {
	case vai.Model_MODEL_DOUBAO_VISION_PRO,
		vai.Model_MODEL_DOUBAO,
		vai.Model_MODEL_DOUBAO_PRO,
		vai.Model_MODEL_DOUBAO_LITE,
		vai.Model_MODEL_HUOSHAN_DEEPSEEK_R1:
		config := conf.GlobalConfig.LlmConfig.DoubaoVision
		apiKey = config.APIKey
		endpoint = config.Endpoint
		serverName = "doubao"
	case vai.Model_MODEL_GPT4O,
		vai.Model_MODEL_GPT4,
		vai.Model_MODEL_GPT4O_MINI,
		vai.Model_MODEL_GPTO1_PREVIEW,
		vai.Model_MODEL_GPTO1_MINI,
		vai.Model_MODEL_GPT4_1:
		handler, err := NewGPTModel(f.voiceDao, f.uploadDao)
		if err != nil {
			return ctx, nil, err
		}
		return updateContextWithModel(ctx, modelID, deploymentName, "azure"), handler, nil
	case vai.Model_MODEL_KIMI_VISION_PREVIEW:
		config := conf.GlobalConfig.LlmConfig.Kimi
		apiKey = config.APIKey
		endpoint = config.Endpoint
		serverName = "kimi"
	case vai.Model_MODEL_DEEPSEEK_R1, vai.Model_MODEL_DEEPSEEK_V3:
		config := conf.GlobalConfig.LlmConfig.DeepSeek
		apiKey = config.APIKey
		endpoint = config.Endpoint
		serverName = "deepseek"
	case vai.Model_MODEL_TENCENT_HUNYUAN_VISION, vai.Model_MODEL_TENCENT_HUNYUAN_TURBOS:
		config := conf.GlobalConfig.LlmConfig.Tencent
		apiKey = config.APIKey
		endpoint = config.Endpoint
		serverName = "tencent"
	case vai.Model_MODEL_GEMINI_2_0_FLASH, vai.Model_MODEL_GEMINI_2_5_FLASH, vai.Model_MODEL_GEMINI_2_5_PRO_THINKING, vai.Model_MODEL_GEMINI_2_5_FLASH_THINKING:
		handler, err := NewGenaiClientWithCredentialsFile(conf.GlobalConfig.LlmConfig.Gemini.AuthFile, f.voiceDao, f.uploadDao)
		if err != nil {
			return ctx, nil, err
		}
		return updateContextWithModel(ctx, modelID, deploymentName, "gemini"), handler, nil
	default:
		return ctx, nil, constants.ERR_INVALID_MODELID
	}

	handler, err := NewOpenAIModel(OpenAIModelConfig{
		APIKey:     apiKey,
		Endpoint:   endpoint,
		ModelName:  deploymentName,
		ServerName: serverName,
	}, f.voiceDao, f.uploadDao)
	if err != nil {
		return ctx, nil, err
	}
	return updateContextWithModel(ctx, modelID, deploymentName, serverName), handler, nil
}
