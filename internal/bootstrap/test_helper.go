package bootstrap

import (
	"context"
	"net/http"

	"github.com/go-redis/redis"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/llm"
	"va_visionai_server/internal/service/picture_generate"
)

// TestDependencies 提供单元测试所需的依赖
type TestDependencies struct {
	// 基础设施
	DB    *gorm.DB
	Redis *redis.Client
	Mongo *mongo.Client

	// 仓库
	Repositories *dao.Repositories

	// 服务
	LLMFactory        *llm.LLMFactoryImpl
	EvaluateService   *picture_generate.EvaluateService
	ImageGenerator    *picture_generate.ImageGenerator
	UserService       *service.UserService
	UserPersonalInfo  *service.UserPersonalInfoService
	UserDeviceService *service.UserDeviceService
}

// InitTestDependencies 初始化单元测试依赖
// 此函数接受可选的参数以支持依赖注入或使用模拟对象
func InitTestDependencies(opts ...TestOption) (*TestDependencies, error) {
	deps := &TestDependencies{}

	// 应用选项
	for _, opt := range opts {
		opt(deps)
	}

	// 如果没有提供数据库连接，则初始化默认的
	if deps.DB == nil {
		deps.DB = db.GetDB()
	}

	// 如果没有提供Redis连接，则初始化默认的
	if deps.Redis == nil {
		deps.Redis = InitRedis()
	}

	// 如果没有提供MongoDB连接，则初始化默认的
	if deps.Mongo == nil {
		deps.Mongo = InitMongo()
	}

	// 如果没有提供仓库，则初始化默认的
	if deps.Repositories == nil {
		deps.Repositories = InitRepositories(deps.DB)
	}

	// 如果没有提供LLM工厂，则初始化默认的
	if deps.LLMFactory == nil {
		deps.LLMFactory = InitLLMFactory(deps.Repositories)
	}

	// 初始化评估服务（如果未提供）
	if deps.EvaluateService == nil {
		deps.EvaluateService = picture_generate.NewEvaluateService(
			deps.Repositories.Task,
			deps.LLMFactory,
			deps.Repositories.Prompt,
		)
	}

	// 初始化图像生成器（如果未提供）
	if deps.ImageGenerator == nil {
		deps.ImageGenerator = picture_generate.NewImageGenerator(
			deps.Repositories.Task,
			deps.EvaluateService,
			deps.Redis,
			&http.Client{},
			deps.Repositories.Upload,
		)
	}

	// 初始化用户服务（如果未提供）
	if deps.UserService == nil {
		userService, err := service.NewUserService(deps.Repositories.User, deps.Repositories.UserAmount, deps.UserDeviceService)
		if err != nil {
			return nil, err
		}
		deps.UserService = userService
	}

	// 初始化用户个人信息服务（如果未提供）
	if deps.UserPersonalInfo == nil {
		deps.UserPersonalInfo = service.NewUserPersonalInfoService(
			deps.Repositories.UserPersonalInfo,
			deps.Repositories.User,
			nil,
		)
	}

	return deps, nil
}

// TestOption 是用于配置TestDependencies的函数选项
type TestOption func(*TestDependencies)

// WithTestDB 设置测试数据库连接
func WithTestDB(db *gorm.DB) TestOption {
	return func(d *TestDependencies) {
		d.DB = db
	}
}

// WithTestRedis 设置测试Redis连接
func WithTestRedis(redis *redis.Client) TestOption {
	return func(d *TestDependencies) {
		d.Redis = redis
	}
}

// WithTestMongo 设置测试MongoDB连接
func WithTestMongo(mongo *mongo.Client) TestOption {
	return func(d *TestDependencies) {
		d.Mongo = mongo
	}
}

// WithTestRepositories 设置测试仓库
func WithTestRepositories(repos *dao.Repositories) TestOption {
	return func(d *TestDependencies) {
		d.Repositories = repos
	}
}

// WithMockLLMFactory 设置模拟LLM工厂
func WithMockLLMFactory(factory *llm.LLMFactoryImpl) TestOption {
	return func(d *TestDependencies) {
		d.LLMFactory = factory
	}
}

// WithMockEvaluateService 设置模拟评估服务
func WithMockEvaluateService(service *picture_generate.EvaluateService) TestOption {
	return func(d *TestDependencies) {
		d.EvaluateService = service
	}
}

// InitUserPersonalInfoTest 为用户个人信息API测试初始化依赖
// 特别针对GetUserPersonalInfo接口的测试
func InitUserPersonalInfoTest(ctx context.Context) (*service.UserPersonalInfoService, *dao.ProfileDao, error) {
	deps, err := InitTestDependencies()
	if err != nil {
		return nil, nil, err
	}

	return deps.UserPersonalInfo, deps.Repositories.UserPersonalInfo, nil
}

// GetTestServiceProvider 获取测试服务提供者
func GetTestServiceProvider() (ServiceInterface, error) {
	db := db.GetDB()
	repos := InitRepositories(db)

	return NewTestServiceProvider(repos), nil
}
