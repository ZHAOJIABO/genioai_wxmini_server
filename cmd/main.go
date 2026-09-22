package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"golang.org/x/net/netutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"va_visionai_server/conf"
	"va_visionai_server/internal/admin"
	"va_visionai_server/internal/api"
	"va_visionai_server/internal/bootstrap"
	"va_visionai_server/internal/common"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/geoip"
	"va_visionai_server/internal/prometheus"
	"va_visionai_server/internal/rpc"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/task"
	picture_workflow_task "va_visionai_server/internal/task/picture_workflow"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"

	_ "github.com/go-sql-driver/mysql"
)

const (
	VisionServerVersion = "__VISION_SERVER_VERSION__"
)

// App 结构体用于管理应用程序的生命周期
type App struct {
	httpServer                  *http.Server
	grpcServer                  *grpc.Server
	grpcLis                     net.Listener
	gwmux                       *runtime.ServeMux
	serviceProvider             bootstrap.ServiceProvider
	submissionRetryProcessor    *picture_workflow_task.SubmissionRetryProcessor
	providerStatusSyncProcessor *picture_workflow_task.ProviderStatusSyncProcessor
	taskExecutionService        *picture_workflow_task.TaskExecutionService
	creditExpiryProcessor       *task.CreditExpiryProcessor
}

// NewApp 创建并初始化应用程序
func NewApp() (*App, error) {
	app := &App{}
	var err error
	provider, err := bootstrap.InitServiceProvider()
	if err != nil {
		return nil, fmt.Errorf("初始化服务提供者失败: %v", err)
	}
	app.serviceProvider = *provider

	// 初始化依赖
	deps := app.serviceProvider.GetServerDeps()
	app.creditExpiryProcessor = app.serviceProvider.GetCreditExpiryProcessor()

	// 初始化 gRPC 服务器
	grpcServer, lis, err := initGRPCServer(deps, app.serviceProvider.GetRepositories())
	if err != nil {
		return nil, fmt.Errorf("初始化 gRPC 服务器失败: %v", err)
	}
	app.grpcServer = grpcServer
	app.grpcLis = lis

	// 获取依赖
	pictureModule := provider.PictureModule
	if pictureModule == nil {
		return nil, errors.New("PictureModule 未初始化")
	}

	appLogger := zlog.Logger

	// 配置后台处理器
	retryConfig := &picture_workflow_task.SubmissionRetryProcessorConfig{
		PollInterval:     60 * time.Second,
		MaxRetryAttempts: 3,
		BatchSize:        5,
	}
	syncConfig := &picture_workflow_task.ProviderStatusSyncConfig{
		PollInterval: 5 * time.Second,
		BatchSize:    5,
		TaskTimeout:  15 * time.Minute,
	}

	app.submissionRetryProcessor = picture_workflow_task.NewSubmissionRetryProcessor(
		provider.Repositories.Task,
		provider.ExecutorService.GetExecutorForRetry,
		retryConfig,
		appLogger,
		provider.Redis,
	)
	app.providerStatusSyncProcessor = picture_workflow_task.NewProviderStatusSyncProcessor(
		provider.Repositories.Task,
		provider.ExecutorService.GetExecutorForSync,
		syncConfig,
		appLogger,
		provider.Redis,
		provider.PictureModule.PictureTaskService,
	)

	// 初始化 TaskExecutionService - 直接从 ServiceProvider 获取
	if provider.TaskExecutionService == nil {
		return nil, errors.New("TaskExecutionService not initialized in ServiceProvider")
	}
	app.taskExecutionService = provider.TaskExecutionService

	return app, nil
}

// Start 启动应用程序
func (a *App) Start() error {
	log.Println("当前区域: ", conf.GlobalConfig.VisionAiServerConfig.Region)
	// 启动gRPC服务器
	go func() {
		log.Println("正在启动gRPC服务器...")
		if err := a.grpcServer.Serve(a.grpcLis); err != nil {
			log.Fatalf("gRPC服务器启动失败: %v", err)
		}
	}()

	// 启动HTTP服务器
	ctx := context.Background()
	go func() {
		if err := a.startHTTPServer(ctx); err != nil {
			log.Fatalf("HTTP服务器启动失败: %v", err)
		}
	}()

	// 启动新的事件消费者
	if a.serviceProvider.EventService != nil && a.serviceProvider.EventService.ConsumerRegistry != nil {
		a.serviceProvider.EventService.ConsumerRegistry.StartAll()
		zlog.Logger.Info("Event consumers started.")
	}

	// 启动后台处理器
	if a.submissionRetryProcessor != nil {
		a.submissionRetryProcessor.Start()
		zlog.Logger.Info("SubmissionRetryProcessor started.")
	}
	if a.providerStatusSyncProcessor != nil {
		a.providerStatusSyncProcessor.Start()
		zlog.Logger.Info("ProviderStatusSyncProcessor started.")
	}
	if a.taskExecutionService != nil {
		a.taskExecutionService.Start()
		zlog.Logger.Info("TaskExecutionService started.")
	}
	if a.creditExpiryProcessor != nil {
		go a.creditExpiryProcessor.Start(context.Background())
		zlog.Logger.Info("CreditExpiryProcessor 已启动")
	}

	// 启动其他任务处理器
	deps := a.serviceProvider.GetServerDeps()
	startTaskProcessors(
		deps.TTSService,
		deps.UserAmountTaskProcessor,
		a.creditExpiryProcessor,
	)

	// 启动订阅优惠推送处理器
	subscriptionPromoPushProcessor := task.NewSubscriptionPromoPushProcessor(
		a.serviceProvider.Redis,
		a.serviceProvider.Repositories.User,
		a.serviceProvider.PushGatewayClient,
		a.serviceProvider.ConfigService,
	)
	go subscriptionPromoPushProcessor.Start(context.Background())
	zlog.Logger.Info("SubscriptionPromoPushProcessor 已启动")

	log.Println("Env Mode: ", conf.GlobalConfig.VisionAiServerConfig.Mode)
	return nil
}

// Shutdown 优雅关闭应用程序
func (a *App) Shutdown(ctx context.Context) error {
	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(ctx); err != nil {
			log.Printf("HTTP服务器关闭失败: %v", err)
		}
	}
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}
	// 停止事件消费者
	if a.serviceProvider.EventService != nil && a.serviceProvider.EventService.ConsumerRegistry != nil {
		a.serviceProvider.EventService.ConsumerRegistry.StopAll()
		log.Println("Event consumers stopped.")
	}
	if a.submissionRetryProcessor != nil {
		a.submissionRetryProcessor.Stop()
		log.Println("Submission retry processor stopped.")
	}
	if a.providerStatusSyncProcessor != nil {
		a.providerStatusSyncProcessor.Stop()
		log.Println("Provider status sync processor stopped.")
	}
	if a.taskExecutionService != nil {
		a.taskExecutionService.Stop()
		log.Println("TaskExecutionService stopped.")
	}
	if a.creditExpiryProcessor != nil {
		a.creditExpiryProcessor.Stop()
		log.Println("CreditExpiryProcessor stopped.")
	}
	// 关闭GeoIP服务
	geoip.Close()
	log.Println("GeoIP closed.")
	// 关闭 EventReporter，刷新队列中的事件
	if a.serviceProvider.EventReporter != nil {
		if err := a.serviceProvider.EventReporter.Shutdown(ctx); err != nil {
			log.Printf("EventReporter 关闭失败: %v", err)
		} else {
			log.Println("EventReporter stopped.")
		}
	}
	log.Println("Waiting for ongoing tasks to complete...")
	time.Sleep(2 * time.Second)
	return nil
}

// Run 运行应用程序并处理信号
func (a *App) Run() error {
	// 启动应用程序
	if err := a.Start(); err != nil {
		return err
	}

	// 设置信号处理
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在关闭服务器...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return a.Shutdown(ctx)
}

// startTaskProcessors 启动各种任务处理器
func startTaskProcessors(
	taskService *service.TTSService,
	userChatAmountProcessor *task.UserChatAmountProcessor,
	creditExpiryProcessor *task.CreditExpiryProcessor,
) {
	// 启动图片任务处理器
	// picture_workflow_task.NewPictureTaskProcessor().Start() // 已移除
	// pictureTaskService.TaskProcessor() // 已移除

	// 启动其他任务处理器，根据实际情况调整
	task.Voice.Listen()
	task.Chat.Listen()
	taskService.StartProcess()
	if userChatAmountProcessor != nil {
		userChatAmountProcessor.Start(context.Background())
	}
	if creditExpiryProcessor != nil {
		go creditExpiryProcessor.Start(context.Background())
		zlog.Logger.Info("CreditExpiryProcessor 已启动")
	}
}

func (a *App) startHTTPServer(ctx context.Context) error {
	// 创建HTTP服务器
	multiMarshaler := common.NewMultiMarshaler()
	isProd := conf.IsProd()
	if isProd {
		a.gwmux = runtime.NewServeMux(
			runtime.WithMarshalerOption(runtime.MIMEWildcard, multiMarshaler),
		)
	} else {
		a.gwmux = runtime.NewServeMux()
	}

	// 设置生成网关
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcEndpoint := fmt.Sprintf(":%d", conf.GlobalConfig.VisionAiServerConfig.Port)

	handlers := []func(context.Context, *runtime.ServeMux, string, []grpc.DialOption) error{
		vai.RegisterPromptServiceHandlerFromEndpoint,
		vai.RegisterTTSServiceHandlerFromEndpoint,
		vai.RegisterPictureForgeServiceHandlerFromEndpoint,
		vai.RegisterMediaServiceHandlerFromEndpoint,
		vai.RegisterChatServiceHandlerFromEndpoint,
		vai.RegisterSystemServiceHandlerFromEndpoint,
		vai.RegisterAuthServiceHandlerFromEndpoint,
		vai.RegisterUserServiceHandlerFromEndpoint,   // UserService HTTP 网关
		vai.RegisterSubscribeServiceHandlerFromEndpoint, // SubscribeService HTTP 网关
		vai.RegisterToolServiceHandlerFromEndpoint,
		vai.RegisterInviteServiceHandlerFromEndpoint,
		vai.RegisterReportServiceHandlerFromEndpoint, // ReportService HTTP 网关（包含 EventWatch）
	}

	for _, handler := range handlers {
		if err := handler(ctx, a.gwmux, grpcEndpoint, dialOpts); err != nil {
			return fmt.Errorf("注册网关服务失败: %v", err)
		}
	}

	// 创建带有请求路径中间件的处理器
	pathMiddleware := func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 设置请求路径信息到上下文中
			ctx := context.WithValue(r.Context(), "path", r.URL.Path)
			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	mux := http.NewServeMux()

	// 注册后台管理路由
	admin.RegisterAdminRoutes(mux, a.serviceProvider.DB, a.serviceProvider.Repositories, a.serviceProvider.CreditService)

	// 使用中间件包装 gwmux
	mux.Handle("/", pathMiddleware(a.gwmux))

	// 添加指标接口
	mux.Handle("/metrics", promhttp.Handler())

	// 创建并启动HTTP服务器
	addr := fmt.Sprintf(":%d", conf.GlobalConfig.VisionAiServerConfig.HTTPPort)
	a.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("正在启动HTTP服务器... %s", addr)
	if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP服务器启动失败: %v", err)
	}

	return nil
}

func initGRPCServer(deps *bootstrap.ServerDeps, repos *dao.Repositories) (*grpc.Server, net.Listener, error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", conf.GlobalConfig.VisionAiServerConfig.Port))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen: %v", err)
	}
	lis = netutil.LimitListener(lis, 1000)

	statusCodeMapper := rpc.NewStatusCodeMapper()
	opts := rpc.GetOpts(statusCodeMapper)
	grpcServer := grpc.NewServer(opts...)

	// 根据当前项目结构调整服务器初始化代码
	// 注意：这里的代码可能需要根据实际的 ServerDeps 结构调整参数
	promptServer := api.NewPromptServer(deps.PromptService, deps.PromptKindService, deps.PromptOptimizer, deps.PictureForgeService, deps.InspirationService)
	ttsServer := api.NewTTSServer(deps.TTSService, deps.MessageService, deps.ConfigService)
	reportServer := api.NewReportServer(deps.ReportService, deps.EventService, deps.WatchService, deps.InspirationService)
	subscribeServer := api.NewSubscribeServer(deps.SubscribeService, deps.ProductService, deps.CreditService, repos.Task)
	chainSvc := service.NewTaskChainService(db.GetDB(), repos, db.GetRedis(), deps.ConfigService, deps.CreditService)
	// 设置任务链服务到图片任务服务，使其能够处理链失败
	deps.PictureTaskService.SetTaskChainService(chainSvc)

	// 初始化模型配置管理器
	modelConfigManager, err := service.NewModelConfigManager("conf/model_config.json")
	if err != nil {
		zlog.Logger.Warn("Failed to initialize ModelConfigManager, model direct mode will not be available", zap.Error(err))
		// 如果配置文件不存在，使用默认配置继续运行
		modelConfigManager, _ = service.NewModelConfigManager("")
	}

	pictureForgeServer := api.NewPictureForgeServer(deps.PictureForgeService, deps.PictureTaskService, deps.UploadService, deps.SubscribeService, deps.DailyFreeCreditsService, deps.CreditService, deps.ConfigService, chainSvc, deps.EventReporter, modelConfigManager)
	userServer := api.NewUserServer(deps.UserService, deps.UserPersonalInfoService, deps.ConfigService, deps.SubscribeService, deps.AuthService, nil, deps.ChatService, deps.ProfileService, deps.PushGatewayClient)
	mediaServer := api.NewMediaServer(deps.UploadService, deps.UserService, deps.FaceDetectService)
	chatServer := api.NewChatServer(deps.ChatService, deps.UserService, deps.MessageService, deps.SubscribeService, deps.PromptService, repos.Chat, deps.ChatParticipantService, deps.ChatMessageService)
	systemServer := api.NewSystemServer(repos.PopupNotification)
	authServer := api.NewAuthServer(deps.AuthService, deps.UserService, deps.UserPersonalInfoService, deps.SmsVerifyService, deps.ConfigService, deps.SubscribeService)
	// profileServer := api.NewProfileServer(deps.UserPersonalInfoService, nil, deps.ProfileService) // Profile API已删除
	// 初始化工具聚合服务
	toolServer := api.NewToolServer(deps.ToolAggregateService)
	// 初始化邀请码服务
	inviteServer := api.NewInviteServer(deps.InviteService, deps.ConfigService)

	vai.RegisterPromptServiceServer(grpcServer, promptServer)
	vai.RegisterTTSServiceServer(grpcServer, ttsServer)
	vai.RegisterReportServiceServer(grpcServer, reportServer)
	vai.RegisterSubscribeServiceServer(grpcServer, subscribeServer)
	vai.RegisterPictureForgeServiceServer(grpcServer, pictureForgeServer)
	vai.RegisterUserServiceServer(grpcServer, userServer)
	vai.RegisterMediaServiceServer(grpcServer, mediaServer)
	vai.RegisterChatServiceServer(grpcServer, chatServer)
	vai.RegisterSystemServiceServer(grpcServer, systemServer)
	vai.RegisterAuthServiceServer(grpcServer, authServer)
	// vai.RegisterProfileServiceServer(grpcServer, profileServer) // Profile API已删除
	// 注册工具聚合服务
	vai.RegisterToolServiceServer(grpcServer, toolServer)
	// 注册邀请码服务
	vai.RegisterInviteServiceServer(grpcServer, inviteServer)

	if conf.IsLocal() {
		reflection.Register(grpcServer)
	}

	return grpcServer, lis, nil
}

func main() {
	// 解析命令行标志
	configFile := flag.String("c", "conf/server.yaml", "default conf/server.yaml")
	flag.Parse()

	// 初始化配置
	if err := conf.ConfigInit(*configFile); err != nil {
		log.Fatalf("配置初始化失败: %v", err)
	}

	// 初始化日志，使用项目中存在的日志初始化函数
	log.Println("正在初始化日志...")
	zlog.InitLogger()

	// 初始化数据库连接
	log.Println("正在初始化数据库...")
	if err := db.RegisterDB(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	// 初始化Prometheus度量
	addr := conf.GlobalConfig.Metrics.Addr
	namespace := conf.GlobalConfig.Metrics.Namespace
	if namespace == "" {
		namespace = prometheus.DefaultNamespace
		log.Printf("未配置 metrics.namespace,使用默认值: %s", namespace)
	}
	if _, err := prometheus.RegisterPrometheus(addr, namespace); err != nil {
		log.Fatalf("初始化Prometheus监控失败: %v", err)
	}
	log.Printf("Prometheus 已启动: %s (namespace: %s)", addr, namespace)

	// 初始化GeoIP
	if conf.GlobalConfig.GeoIP.Enabled {
		if err := geoip.InitGeoIP(conf.GlobalConfig.GeoIP.DatabasePath); err != nil {
			log.Printf("⚠️  警告: GeoIP初始化失败，用户地理信息（Country/Timezone）功能将不可用: %v (数据库路径: %s)",
				err, conf.GlobalConfig.GeoIP.DatabasePath)
			log.Printf("    影响: 新用户注册时 Country 和 Timezone 字段将为空")
		} else {
			log.Printf("✓ GeoIP 已启动: %s", conf.GlobalConfig.GeoIP.DatabasePath)
		}
	} else {
		log.Println("GeoIP 未启用，用户地理信息功能关闭")
	}

	hostName, _ := os.Hostname()
	log.Printf("va_visionai_server v%s : server %s - %s\n", VisionServerVersion, conf.GlobalConfig.ServerName, hostName)

	// 创建并运行应用程序
	app, err := NewApp()
	if err != nil {
		log.Fatalf("创建应用程序失败: %v", err)
	}

	if err := app.Run(); err != nil {
		log.Fatalf("运行应用程序失败: %v", err)
	}
}
