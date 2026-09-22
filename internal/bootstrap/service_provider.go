package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/conf"
	api "va_visionai_server/internal/api"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service"
	aigcclient "va_visionai_server/internal/service/ai_clients/aigc_core"
	"va_visionai_server/internal/service/auth"
	"va_visionai_server/internal/service/cache"
	"va_visionai_server/internal/service/chat"
	chatbiz "va_visionai_server/internal/service/chat/biz"
	chat_message "va_visionai_server/internal/service/chat_message"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/service/event"
	"va_visionai_server/internal/service/event/consumer"
	event_handlers "va_visionai_server/internal/service/event/handlers"
	"va_visionai_server/internal/service/event_reporter"
	"va_visionai_server/internal/service/invite"
	"va_visionai_server/internal/service/llm"
	"va_visionai_server/internal/service/picture_forge"
	"va_visionai_server/internal/service/picture_generate"
	"va_visionai_server/internal/service/profile"
	"va_visionai_server/internal/service/prompt"
	"va_visionai_server/internal/service/push_gateway"
	"va_visionai_server/internal/task"
	picture_workflow_task "va_visionai_server/internal/task/picture_workflow"
	slotrecon "va_visionai_server/internal/task/slot"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type EventServiceV2 struct {
	*event.EventService
	ConsumerRegistry       *consumer.Registry
	ProfileService         *profile.ProfileService
	ChatParticipantService *chat.ChatParticipantService
	ChatMessageService     *chat_message.ChatMessageService
	CreditService          credit.Service
	MembershipService      credit.MembershipService
	CreditExpiryProcessor  *task.CreditExpiryProcessor
	FaceDetectService      *service.FaceDetectService
	ExecutorService        *picture_generate.ExecutorService
	PictureTaskService     *service.PictureTaskService
	TaskExecutionService   *picture_workflow_task.TaskExecutionService
}

// ServiceProvider 提供应用程序所需的所有服务
type ServiceProvider struct {
	// 数据库连接
	DB    *gorm.DB
	Redis *redis.Client
	Mongo *mongo.Client

	// 公共依赖
	Repositories *dao.Repositories
	LLMFactory   *llm.LLMFactoryImpl
	HeatTracker  *prompt.HeatTracker

	// 模块依赖
	UserModule    *UserModuleDeps
	PictureModule *PictureModuleDeps

	// 单独服务
	ChatService             *chat.ChatService
	MessageService          *service.MessageService
	ConfigService           *service.ConfigService
	UploadService           *service.UploadService
	SubscribeService        *service.SubscribeService
	PromptService           *prompt.PromptService
	PromptKindService       *prompt.PromptKindService
	OptimizerService        *prompt.OptimizerService
	InspirationService      *prompt.InspirationService
	TTSService              *service.TTSService
	ReportService           *service.ReportService
	EventService            *EventServiceV2
	WatchService            *event.WatchService
	ProductService          *service.ProductService
	AuthService             *auth.AuthService
	SmsVerifyService        *service.SmsVerifyService
	AmountTaskProcessor     *task.UserChatAmountProcessor
	DeviceService           *service.UserDeviceService
	ProfileService          *profile.ProfileService
	ChatParticipantService  *chat.ChatParticipantService
	ChatMessageService      *chat_message.ChatMessageService
	CreditService           credit.Service
	MembershipService       credit.MembershipService
	CreditExpiryProcessor   *task.CreditExpiryProcessor
	FaceDetectService       *service.FaceDetectService
	ExecutorService         *picture_generate.ExecutorService
	PictureTaskService      *service.PictureTaskService
	TaskExecutionService    *picture_workflow_task.TaskExecutionService
	DailyFreeCreditsService *service.DailyFreeCreditsService
	// AIGC Core客户端
	AIGCClient *aigcclient.Client
	// CacheService *service.CacheService
	CacheService *cache.CacheService
	// EventReporter 事件上报客户端
	EventReporter event_reporter.EventReporter
	// PushGatewayClient 推送网关客户端
	PushGatewayClient *push_gateway.Client
	// 邀请模块
	InviteModule  *InviteModuleDeps
	InviteService *invite.InviteService
}

// InitServiceProvider 完整初始化所有服务
func InitServiceProvider() (*ServiceProvider, error) {
	provider := &ServiceProvider{}

	// 初始化数据库连接
	var err error
	provider.DB, err = InitDatabase()
	if err != nil {
		return nil, fmt.Errorf("init database failed: %v", err)
	}
	provider.Redis = InitRedis()
	provider.Mongo = InitMongo()

	// 初始化公共依赖
	provider.Repositories = dao.NewRepositoriesWithMongo(provider.DB, provider.Mongo)
	provider.LLMFactory = InitLLMFactory(provider.Repositories)
	provider.HeatTracker = prompt.NewHeatTracker()

	// 初始化AIGC Core客户端
	aigcConfig := aigcclient.AIGCConfig{
		Addr: conf.GlobalConfig.AIBrainServer.Addr,
	}
	provider.AIGCClient, err = aigcclient.NewClient(context.Background(), aigcConfig)
	if err != nil {
		return nil, fmt.Errorf("初始化AIGC Core客户端失败: %w", err)
	}

	// 初始化缓存服务
	provider.CacheService = cache.NewCacheService()

	// 初始化 EventReporter 事件上报客户端
	eventReporterCfg := event_reporter.Config{
		Enabled:         conf.GlobalConfig.EventSink.Enabled,
		Addr:            conf.GlobalConfig.EventSink.Addr,
		QueueSize:       conf.GlobalConfig.EventSink.QueueSize,
		BatchSize:       conf.GlobalConfig.EventSink.BatchSize,
		FlushInterval:   conf.GlobalConfig.EventSink.FlushInterval,
		ShutdownTimeout: conf.GlobalConfig.EventSink.ShutdownTimeout,
		Source:          conf.GlobalConfig.EventSink.Source,
		ServerNode:      conf.GlobalConfig.EventSink.ServerNode,
	}
	provider.EventReporter, err = event_reporter.New(
		context.Background(),
		eventReporterCfg,
		zlog.LogWithContext(context.Background()).Named("event_reporter"),
	)
	if err != nil {
		// EventReporter 初始化失败不影响服务启动，只记录警告
		zlog.LogWithContext(context.Background()).Warn("初始化 EventReporter 失败，将使用空操作模式", zap.Error(err))
		provider.EventReporter, _ = event_reporter.New(
			context.Background(),
			event_reporter.Config{Enabled: false},
			zlog.LogWithContext(context.Background()).Named("event_reporter"),
		)
	}

	// 初始化 PushGateway 推送网关客户端
	provider.PushGatewayClient = push_gateway.NewClient(
		conf.GlobalConfig.PushGateway.Enabled,
		conf.GlobalConfig.PushGateway.Addr,
		conf.GlobalConfig.PushGateway.Timeout,
		conf.GlobalConfig.PushGateway.Env,
		zlog.LogWithContext(context.Background()).Named("push_gateway"),
	)

	// 初始化各模块
	provider.UserModule, err = InitUserModule(provider.Repositories)
	if err != nil {
		return nil, fmt.Errorf("init user module failed: %v", err)
	}
	provider.PictureModule = InitPictureModule(provider.Repositories, provider.LLMFactory, provider.Redis, provider.AIGCClient, provider.CacheService, provider.EventReporter)
	provider.CreditService = provider.PictureModule.CreditService

	// 初始化配置服务（需要在邀请模块之前初始化）
	provider.ConfigService = service.NewConfigService()

	// 初始化邀请模块
	provider.InviteModule = InitInviteModule(provider.Repositories, provider.CreditService, provider.ConfigService, provider.DB)
	provider.InviteService = provider.InviteModule.InviteService

	// 初始化其他服务
	provider.MessageService = service.NewMessageService(provider.Mongo, service.NewVoiceService(), provider.Repositories)

	// 创建 MembershipService
	provider.MembershipService = credit.NewMembershipService(provider.CreditService)
	provider.SubscribeService = service.NewSubscribeService(provider.Repositories.UserAmount, provider.Repositories.User, provider.MembershipService, provider.CreditService, provider.CacheService, provider.Repositories.Task)
	// 初始化SMS验证服务
	provider.SmsVerifyService, err = service.NewSmsVerifyService()
	if err != nil {
		return nil, fmt.Errorf("init sms verify service failed: %v", err)
	}

	// 订阅与每日积分服务在此处装配，避免 picture_module 依赖 Service 层
	dailySvc := service.NewDailyFreeCreditsService(provider.CreditService, provider.ConfigService, provider.SubscribeService, provider.CacheService)
	provider.DailyFreeCreditsService = dailySvc

	// 初始化认证服务
	provider.AuthService, err = auth.InitAuthServiceWithConfig(
		context.TODO(),
		provider.Repositories.User,
		*provider.SmsVerifyService,
		provider.Redis,
		provider.ConfigService,
		provider.DeviceService,
		provider.DailyFreeCreditsService,
	)
	if err != nil {
		return nil, fmt.Errorf("init auth service failed: %v", err)
	}

	// 初始化事件服务
	eventService := event.NewEventService(
		provider.Repositories.Event,
		provider.Repositories.Task,
		provider.Repositories.Tracking,
		provider.Repositories.Attribution,
	)

	// --- 开始装配新的事件消费者和处理器 ---
	// 1. 实例化新的 PaymentEventHandler
	paymentHandler := event_handlers.NewPaymentEventHandler(provider.CreditService, provider.CacheService)

	// 2. 将其注册到 EventService
	eventService.Register(paymentHandler)

	// 3. 实例化消费者注册表
	consumerRegistry := consumer.NewRegistry()

	// 4. 实例化 RedisStreamConsumer 并注册
	redisConsumer := consumer.NewRedisStreamConsumer(provider.Redis, eventService)
	consumerRegistry.Register(redisConsumer)
	// --- 装配结束 ---

	provider.ChatMessageService = chat_message.NewChatMessageService(
		provider.Mongo,
		conf.GlobalConfig.MongoDB.Db,
		provider.Repositories.Msg,
		provider.Repositories.Chat,
	)
	// 创建 ChatLLMService
	chatLLMService := chat.NewChatLLMService(
		provider.LLMFactory,
		provider.Repositories.Model,
		provider.Repositories.Upload,
		provider.Repositories.Prompt,
		chat.NewDoubaoTransformer(),
		chat.NewVideoTransformer(),
	)
	provider.ChatParticipantService = chat.NewChatParticipantService(
		provider.Repositories.ChatParticipant,
		provider.Repositories.Chat,
		provider.MessageService,
		provider.LLMFactory,
		provider.Repositories.Profile,
		provider.Repositories.Config,
		provider.ChatMessageService,
		provider.Repositories.Prompt,
		chatLLMService,
	)

	provider.ProfileService = profile.NewProfileService(provider.Repositories.Profile, provider.ConfigService)

	provider.EventService = &EventServiceV2{
		EventService:           eventService,
		ConsumerRegistry:       consumerRegistry,
		ProfileService:         provider.ProfileService,
		ChatParticipantService: provider.ChatParticipantService,
		ChatMessageService:     provider.ChatMessageService,
		CreditService:          provider.CreditService,
		MembershipService:      provider.MembershipService,
		CreditExpiryProcessor:  task.NewCreditExpiryProcessor(provider.MembershipService, provider.Repositories.User),
		FaceDetectService:      service.NewFaceDetectService(),
		ExecutorService:        nil,
		PictureTaskService:     nil,
		TaskExecutionService:   nil,
	}

	provider.WatchService = event.NewWatchService()

	// 初始化其他服务
	provider.PromptService = prompt.NewPromptService(provider.Redis, provider.HeatTracker, provider.DB)
	provider.PromptKindService = prompt.NewPromptKindService()
	provider.OptimizerService = prompt.NewOptimizerService(provider.AIGCClient, provider.ConfigService, provider.EventReporter)
	provider.InspirationService = prompt.NewInspirationServiceWithDB(provider.DB)
	provider.TTSService = service.NewTTSService()
	provider.ReportService = service.NewReportService(provider.Repositories)
	provider.ProductService = service.NewProductServiceWithConfig(provider.ConfigService)
	provider.UploadService = service.NewUploadService(provider.LLMFactory, provider.Repositories.Prompt)

	// 创建新版本的ChatService
	newChatService := chat.NewChatService(
		chatLLMService,
		provider.MessageService,
		service.NewVoiceService(),
		provider.HeatTracker,
		provider.UploadService,
		provider.DB,
		provider.Repositories.Upload,
		provider.Repositories.Model,
		provider.Repositories.Prompt,
		provider.Repositories,
	)
	provider.ChatService = newChatService
	// 注册所有处理器
	chatbiz.RegisterHandlers(
		chatbiz.NewStandardLLMHandler(chatLLMService, provider.MessageService, provider.Repositories.Chat),
		chatbiz.NewCustomUserPersonalInfoLLMHandler(
			chatLLMService,
			provider.MessageService,
			provider.Repositories.UserPersonalInfo,
			provider.Repositories.UserPersonalChat,
			provider.Repositories.Prompt,
			provider.Repositories.Chat,
			provider.Repositories.ChatParticipant,
		),
	)
	provider.AmountTaskProcessor = task.NewUserChatAmountProcessor(
		provider.Repositories.User,
		provider.Repositories.UserAmount,
	)

	provider.ExecutorService = provider.PictureModule.ExecutorService
	picTaskSvc := provider.PictureModule.PictureTaskService
	provider.PictureTaskService = picTaskSvc
	picTaskSvc.SetExecutorService(provider.ExecutorService)
	// 向任务服务注入依赖
	provider.PictureTaskService.SetSubscribeService(provider.SubscribeService)
	provider.PictureTaskService.SetDailyFreeCreditsService(provider.DailyFreeCreditsService)
	provider.PictureTaskService.SetPushGatewayClient(provider.PushGatewayClient)

	// 注册任务完成Hook → 任务链服务
	chainSvc := service.NewTaskChainService(provider.DB, provider.Repositories, provider.Redis, provider.ConfigService, provider.CreditService)

	// 初始化模型配置管理器
	modelConfigManager, err := service.NewModelConfigManager("conf/model_config.json")
	if err != nil {
		zlog.LogWithContext(context.Background()).Warn("Failed to initialize ModelConfigManager in bootstrap", zap.Error(err))
		// 如果配置文件不存在，使用默认配置继续运行
		modelConfigManager, _ = service.NewModelConfigManager("")
	}

	// 注入提交函数，复用当前API层的提交路径（扣费/事务/退款统一）
	chainSvc.SetSubmitFunc(func(ctx context.Context, req *vai.SubmitPictureForgeTaskRequest) (*vai.SubmitPictureForgeTaskResponse, error) {
		// 构造一个临时的API服务器以复用逻辑
		apiServer := api.NewPictureForgeServer(provider.PictureModule.PictureForgeService, provider.PictureTaskService, provider.UploadService, provider.SubscribeService, provider.DailyFreeCreditsService, provider.CreditService, provider.ConfigService, chainSvc, provider.EventReporter, modelConfigManager)
		return apiServer.SubmitPictureForgeTask(ctx, req)
	})

	// 设置任务链服务到图片任务服务，使其能够处理链失败
	picTaskSvc.SetTaskChainService(chainSvc)
	picture_generate.RegisterOnTaskCompletedHook(func(task *model.PictureTask) {
		// 为了避免在bootstrap引入循环依赖，这里只转调服务层；
		// 该回调内部自行做幂等控制
		chainSvc.OnTaskCompleted(context.Background(), task)
	})

	// 注册任务终止回调，用于释放并发槽位和队列槽位
	picture_generate.RegisterOnTaskTerminatedHook(func(ctx context.Context, task *model.PictureTask) {
		// 释放并发槽位
		if err := picTaskSvc.GetTaskQuotaChecker().ReleaseConcurrentSlot(ctx, task.UserID, task.TaskID); err != nil {
			// 仅记录错误，不影响任务流程
			log := zlog.LogWithContext(ctx).With(
				zap.String("task_id", task.TaskID),
				zap.String("user_id", task.UserID),
			)
			log.Error("failed to release concurrent slot on task terminated", zap.Error(err))
		}

		// 释放队列槽位（新增）
		if err := picTaskSvc.GetTaskQuotaChecker().ReleaseQueueSlot(ctx, task.UserID, task.TaskID); err != nil {
			// 仅记录错误，不影响任务流程
			log := zlog.LogWithContext(ctx).With(
				zap.String("task_id", task.TaskID),
				zap.String("user_id", task.UserID),
			)
			log.Error("failed to release queue slot on task terminated", zap.Error(err))
		}
	})

	taskExecutionSvc := picture_workflow_task.NewTaskExecutionService(
		picture_workflow_task.TaskExecutionConfig{},
		provider.Repositories.Task,
		provider.Repositories.PicForge,
		provider.Redis,
		provider.ExecutorService,
		provider.PictureModule.ImageGenerator,
		picTaskSvc,
		provider.PictureTaskService.GetTaskConfigManager(),
		provider.PictureTaskService.GetTaskStateManager(),
		provider.SubscribeService, // subscribeService
	)
	taskExecutionSvc.Start()
	provider.TaskExecutionService = taskExecutionSvc

	// 启动并发槽位对账器
	zlog.LogWithContext(context.Background()).Info("启动并发槽位对账器", zap.Bool("enabled", conf.GlobalConfig.Reconciler.Enabled))
	if conf.GlobalConfig.Reconciler.Enabled {
		// 兼容：优先使用 concurrent 子段覆盖顶层参数
		cc := conf.GlobalConfig.Reconciler.Concurrent
		reconCfg := slotrecon.SlotReconcilerConfig{
			Enabled:          ternaryBool(cc.Enabled, cc.Enabled, conf.GlobalConfig.Reconciler.Enabled),
			Interval:         ternaryDuration(cc.Interval != 0, cc.Interval, conf.GlobalConfig.Reconciler.Interval),
			RedisScanCount:   ternaryInt64(cc.RedisScanCount > 0, cc.RedisScanCount, conf.GlobalConfig.Reconciler.RedisScanCount),
			MaxTasksPerRound: ternaryInt(cc.MaxTasksPerRound > 0, cc.MaxTasksPerRound, conf.GlobalConfig.Reconciler.MaxTasksPerRound),
			WorkerPoolSize:   ternaryInt(cc.WorkerPoolSize > 0, cc.WorkerPoolSize, conf.GlobalConfig.Reconciler.WorkerPoolSize),
		}
		// 统一释放接口适配（兼容并发对账器）
		unified := NewUnifiedReleaserAdapter(provider.PictureTaskService.GetTaskQuotaChecker())
		concurrentCompat := NewConcurrentReleaserCompat(unified)
		reconciler := slotrecon.NewConcurrentSlotReconciler(
			reconCfg,
			provider.Redis,
			provider.Repositories.Task,
			concurrentCompat,
			zlog.LogWithContext(context.Background()).Named("SlotReconciler"),
		)
		reconciler.Start()

		// 启动队列槽位对账器
		if conf.GlobalConfig.Reconciler.Queue.Enabled {
			// 如果队列对账器有独立配置，则使用独立配置，否则继承总配置
			queueCfg := slotrecon.SlotReconcilerConfig{
				Enabled:          conf.GlobalConfig.Reconciler.Queue.Enabled,
				Interval:         conf.GlobalConfig.Reconciler.Queue.Interval,
				RedisScanCount:   conf.GlobalConfig.Reconciler.Queue.RedisScanCount,
				MaxTasksPerRound: conf.GlobalConfig.Reconciler.Queue.MaxTasksPerRound,
				WorkerPoolSize:   conf.GlobalConfig.Reconciler.Queue.WorkerPoolSize,
			}
			// 如果队列配置为0，则使用总配置的值
			if queueCfg.Interval == 0 {
				queueCfg.Interval = conf.GlobalConfig.Reconciler.Interval
			}
			if queueCfg.RedisScanCount == 0 {
				queueCfg.RedisScanCount = conf.GlobalConfig.Reconciler.RedisScanCount
			}
			if queueCfg.MaxTasksPerRound == 0 {
				queueCfg.MaxTasksPerRound = conf.GlobalConfig.Reconciler.MaxTasksPerRound
			}
			if queueCfg.WorkerPoolSize == 0 {
				queueCfg.WorkerPoolSize = conf.GlobalConfig.Reconciler.WorkerPoolSize
			}

			// 统一释放接口适配（兼容队列对账器）
			queueCompat := NewQueueReleaserCompat(unified)
			queueReconciler := slotrecon.NewQueueSlotReconciler(
				queueCfg,
				provider.Redis,
				provider.Repositories.Task,
				queueCompat,
				zlog.LogWithContext(context.Background()).Named("QueueSlotReconciler"),
			)
			zlog.LogWithContext(context.Background()).Info("启动队列槽位对账器",
				zap.Bool("enabled", queueCfg.Enabled),
				zap.Duration("interval", queueCfg.Interval),
				zap.Int64("redis_scan_count", queueCfg.RedisScanCount),
				zap.Int("max_tasks_per_round", queueCfg.MaxTasksPerRound),
				zap.Int("worker_pool_size", queueCfg.WorkerPoolSize),
			)
			queueReconciler.Start()
		}
	}

	return provider, nil
}

// GetServerDeps 获取服务器依赖，用于兼容原有结构
func (p *ServiceProvider) GetServerDeps() *ServerDeps {
	return &ServerDeps{
		UserService:             p.UserModule.UserService,
		UserPersonalInfoService: p.UserModule.UserPersonalInfoService,
		ChatService:             p.ChatService,
		MessageService:          p.MessageService,
		ConfigService:           p.ConfigService,
		UploadService:           p.UploadService,
		SubscribeService:        p.SubscribeService,
		PromptService:           p.PromptService,
		PromptKindService:       p.PromptKindService,
		PromptOptimizer:         p.OptimizerService,
		InspirationService:      p.InspirationService,
		TTSService:              p.TTSService,
		ReportService:           p.ReportService,
		EventService:            p.EventService.EventService,
		WatchService:            p.WatchService,
		PictureForgeService:     p.PictureModule.PictureForgeService,
		PictureTaskService:      p.PictureTaskService,
		ProductService:          p.ProductService,
		FaceDetectService:       p.FaceDetectService,
		UserAmountTaskProcessor: p.AmountTaskProcessor,
		AuthService:             p.AuthService,
		SmsVerifyService:        p.SmsVerifyService,
		LLMFactory:              p.LLMFactory,
		ProfileService:          p.ProfileService,
		ChatParticipantService:  p.ChatParticipantService,
		ChatMessageService:      p.ChatMessageService,
		UserDeviceService:       p.DeviceService,
		CreditService:           p.CreditService,
		MembershipService:       p.MembershipService,
		CreditExpiryProcessor:   p.CreditExpiryProcessor,
		AIGCClient:              p.AIGCClient,
		DailyFreeCreditsService: p.DailyFreeCreditsService,
		EventReporter:           p.EventReporter,
		ToolAggregateService:    p.PictureModule.ToolAggregateService,
		PushGatewayClient:       p.PushGatewayClient,
		InviteService:           p.InviteService,
	}
}

// ServerDeps 原有的服务器依赖结构（兼容原有代码）
type ServerDeps struct {
	UserService             *service.UserService
	UserPersonalInfoService *service.UserPersonalInfoService
	ChatService             *chat.ChatService
	MessageService          *service.MessageService
	ConfigService           *service.ConfigService
	UploadService           *service.UploadService
	SubscribeService        *service.SubscribeService
	PromptService           *prompt.PromptService
	PromptKindService       *prompt.PromptKindService
	PromptOptimizer         *prompt.OptimizerService
	InspirationService      *prompt.InspirationService
	TTSService              *service.TTSService
	ReportService           *service.ReportService
	EventService            *event.EventService
	WatchService            *event.WatchService
	PictureForgeService     *picture_forge.PictureForgeService
	PictureTaskService      *service.PictureTaskService
	ProductService          *service.ProductService
	FaceDetectService       *service.FaceDetectService
	UserAmountTaskProcessor *task.UserChatAmountProcessor
	AuthService             *auth.AuthService
	SmsVerifyService        *service.SmsVerifyService
	LLMFactory              *llm.LLMFactoryImpl
	ProfileService          *profile.ProfileService
	ChatParticipantService  *chat.ChatParticipantService
	ChatMessageService      *chat_message.ChatMessageService
	UserDeviceService       *service.UserDeviceService
	CreditService           credit.Service
	MembershipService       credit.MembershipService
	CreditExpiryProcessor   *task.CreditExpiryProcessor
	AIGCClient              *aigcclient.Client
	DailyFreeCreditsService *service.DailyFreeCreditsService
	EventReporter           event_reporter.EventReporter
	ToolAggregateService    *picture_forge.ToolAggregateService
	PushGatewayClient       *push_gateway.Client
	InviteService           *invite.InviteService
}

// GetCreditExpiryProcessor 获取额度过期处理器
func (p *ServiceProvider) GetCreditExpiryProcessor() *task.CreditExpiryProcessor {
	return p.CreditExpiryProcessor
}

// GetRepositories 获取数据仓库
func (p *ServiceProvider) GetRepositories() *dao.Repositories {
	return p.Repositories
}

// ServiceInterface 定义了服务接口，用于解决循环引用问题
type ServiceInterface interface {
	// GetServerDeps 获取服务依赖
	GetServerDeps() *ServerDeps
	GetCreditExpiryProcessor() *task.CreditExpiryProcessor
	GetRepositories() *dao.Repositories
}

// TestServiceInitializer 定义了测试初始化接口
type TestServiceInitializer interface {
	// InitTestService 初始化测试服务
	InitTestService() (*ServerDeps, error)
}

// 确保ServiceProvider实现了ServiceInterface
var _ ServiceInterface = (*ServiceProvider)(nil)

// 轻量三元工具，避免到处写 if-else
func ternaryBool(cond bool, a, b bool) bool {
	if cond {
		return a
	}
	return b
}
func ternaryInt(cond bool, a, b int) int {
	if cond {
		return a
	}
	return b
}
func ternaryInt64(cond bool, a, b int64) int64 {
	if cond {
		return a
	}
	return b
}
func ternaryDuration(cond bool, a, b time.Duration) time.Duration {
	if cond {
		return a
	}
	return b
}
