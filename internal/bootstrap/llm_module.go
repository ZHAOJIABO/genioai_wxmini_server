package bootstrap

import (
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service/llm"
)

// InitLLMFactory 初始化LLM工厂
func InitLLMFactory(repos *dao.Repositories) *llm.LLMFactoryImpl {
	return llm.NewLLMFactoryImpl(repos.Model, repos.Voice, repos.Upload)
}
