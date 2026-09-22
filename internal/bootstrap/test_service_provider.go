package bootstrap

import (
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/prompt"
	"va_visionai_server/internal/task"
)

// TestServiceProvider 提供测试服务的结构体
type TestServiceProvider struct {
	// 基础依赖
	Repositories *dao.Repositories

	// ServerDeps缓存
	deps *ServerDeps
}

// NewTestServiceProvider 创建测试服务提供者
func NewTestServiceProvider(repositories *dao.Repositories) *TestServiceProvider {
	return &TestServiceProvider{
		Repositories: repositories,
	}
}

// InitTestService 初始化测试服务
func (p *TestServiceProvider) InitTestService() (*ServerDeps, error) {
	if p.deps != nil {
		return p.deps, nil
	}

	// 创建LLM工厂
	llmFactory := InitLLMFactory(p.Repositories)

	// 创建必要的服务
	userService, err := service.NewUserService(p.Repositories.User, p.Repositories.UserAmount, p.deps.UserDeviceService)
	if err != nil {
		return nil, err
	}

	userPersonalInfoService := service.NewUserPersonalInfoService(
		p.Repositories.UserPersonalInfo,
		p.Repositories.User,
		nil,
	)

	// 创建简化版的服务依赖，仅包含测试需要的服务
	deps := &ServerDeps{
		UserService:             userService,
		UserPersonalInfoService: userPersonalInfoService,
		PromptService:           prompt.NewPromptService(nil, nil, nil),
		ConfigService:           service.NewConfigService(),
		LLMFactory:              llmFactory,
	}

	p.deps = deps
	return deps, nil
}

// GetServerDeps 实现ServiceInterface接口
func (p *TestServiceProvider) GetServerDeps() *ServerDeps {
	deps, _ := p.InitTestService()
	return deps
}

// GetCreditExpiryProcessor 实现ServiceInterface接口
func (p *TestServiceProvider) GetCreditExpiryProcessor() *task.CreditExpiryProcessor {
	// 在测试环境中，可能不需要这个处理器，返回nil
	return nil
}

// GetRepositories 实现ServiceInterface接口
func (p *TestServiceProvider) GetRepositories() *dao.Repositories {
	return p.Repositories
}

// 确保TestServiceProvider实现了必要的接口
var _ TestServiceInitializer = (*TestServiceProvider)(nil)
var _ ServiceInterface = (*TestServiceProvider)(nil)
