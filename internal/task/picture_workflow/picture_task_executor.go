package picture_workflow_task

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/picture_generate"
	qcfg "va_visionai_server/internal/service/quota"
	pb "va_visionai_server/internal/va_interface"
)

// Default configuration values, can be overridden by actual config.
const (
	defaultUserProcessingLockTTL = 15 * time.Minute
	defaultSchedulerInterval     = 5 * time.Second
	defaultPendingTaskQueryLimit = 100
	defaultTaskTimeout           = 30 * time.Minute // Default timeout for a single task execution
)

// TaskExecutionConfig holds configuration for the TaskExecutionService.
type TaskExecutionConfig struct {
	UserProcessingLockTTL time.Duration
	SchedulerInterval     time.Duration
	PendingTaskQueryLimit int
	TaskTimeout           time.Duration
}

type TaskExecutionService struct {
	config          TaskExecutionConfig // Store the config
	taskDao         *dao.PictureTaskDao
	picForgeDao     *dao.PictureForgeDao
	rdb             *redis.Client
	executorService *picture_generate.ExecutorService
	imageGenerator  *picture_generate.ImageGenerator
	taskService     *service.PictureTaskService
	stopChan        chan struct{}
	logger          *zap.Logger

	// 并发控制组件
	taskConfigManager *qcfg.TaskConfigManager
	taskStateManager  *qcfg.TaskStateManager
	subscribeService  *service.SubscribeService
}

func NewTaskExecutionService(
	cfg TaskExecutionConfig, // Accept config struct
	taskDao *dao.PictureTaskDao,
	picForgeDao *dao.PictureForgeDao,
	rdb *redis.Client,
	executorService *picture_generate.ExecutorService,
	imageGenerator *picture_generate.ImageGenerator,
	taskService *service.PictureTaskService,
	taskConfigManager *qcfg.TaskConfigManager,
	taskStateManager *qcfg.TaskStateManager,
	subscribeService *service.SubscribeService,
) *TaskExecutionService {
	// Apply defaults if values are zero
	if cfg.UserProcessingLockTTL == 0 {
		cfg.UserProcessingLockTTL = defaultUserProcessingLockTTL
	}
	if cfg.SchedulerInterval == 0 {
		cfg.SchedulerInterval = defaultSchedulerInterval
	}
	if cfg.PendingTaskQueryLimit == 0 {
		cfg.PendingTaskQueryLimit = defaultPendingTaskQueryLimit
	}
	if cfg.TaskTimeout == 0 {
		cfg.TaskTimeout = defaultTaskTimeout
	}

	return &TaskExecutionService{
		config:            cfg, // Store the resolved config
		taskDao:           taskDao,
		picForgeDao:       picForgeDao,
		rdb:               rdb,
		executorService:   executorService,
		imageGenerator:    imageGenerator,
		taskService:       taskService,
		stopChan:          make(chan struct{}),
		logger:            zap.L().Named("TaskExecutionService"),
		taskConfigManager: taskConfigManager,
		taskStateManager:  taskStateManager,
		subscribeService:  subscribeService,
	}
}

func (s *TaskExecutionService) Start() {
	s.logger.Info("TaskExecutionService starting...",
		zap.Duration("scheduler_interval", s.config.SchedulerInterval),
		zap.Int("pending_task_query_limit", s.config.PendingTaskQueryLimit),
	)
	go s.schedulerLoop()
}

func (s *TaskExecutionService) Stop() {
	s.logger.Info("TaskExecutionService stopping...")
	close(s.stopChan)
	s.logger.Info("TaskExecutionService stopped.")
}

func (s *TaskExecutionService) schedulerLoop() {
	ticker := time.NewTicker(s.config.SchedulerInterval) // Use config
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			s.logger.Info("Scheduler loop stopping.")
			return
		case <-ticker.C:
			s.logger.Debug("Scheduler loop triggered.")
			s.processPendingTasks()
		}
	}
}

func (s *TaskExecutionService) processPendingTasks() {
	ctx := context.Background()
	tasks, err := s.taskDao.GetPendingTasks(ctx, s.config.PendingTaskQueryLimit) // Use config
	if err != nil {
		s.logger.Error("Failed to get pending tasks", zap.Error(err))
		return
	}
	if len(tasks) == 0 {
		s.logger.Debug("No pending tasks found.")
		return
	}
	s.logger.Info("Processing pending tasks", zap.Int("count", len(tasks)))
	for _, task := range tasks {
		s.tryDispatchTask(ctx, task)
	}
}

func (s *TaskExecutionService) tryDispatchTask(ctx context.Context, task *model.PictureTask) {
	log := s.logger.With(
		zap.String("task_id", task.TaskID),
		zap.String("user_id", task.UserID),
	)
	ctx = common.CtxSetValue(ctx, constants.CtxProjectID, task.ProjectID)

	// 1. 判断用户是否为会员
	isMember := false
	if s.subscribeService != nil {
		if info, _ := s.subscribeService.GetSubscribeInfo(ctx, task.UserID, "", "", ""); info != nil {
			isMember = info.GetSubscribeLevel() > 0
		}
	}

	// 2. 加载配置
	config := s.taskConfigManager.LoadConfig(ctx, isMember)
	log.Debug("loaded task config",
		zap.Int("max_concurrent", config.MaxConcurrent),
		zap.Bool("is_member", isMember))

	// 3. 使用 TaskStateManager 原子转换状态（包含并发检查和槽位预留）
	updatedTask, err := s.taskStateManager.TransitionToProcessing(
		ctx,
		task.TaskID,
		task.UserID,
		config.MaxConcurrent,
	)
	if err != nil {
		// 并发槽位已满或其他错误，任务继续等待下次调度
		log.Debug("failed to transition task to PROCESSING, will retry later",
			zap.Error(err))
		return
	}

	log.Info("task transitioned to PROCESSING, starting execution",
		zap.String("task_id", updatedTask.TaskID))

	// 4. 启动任务执行
	go s.executeTaskWorker(context.Background(), updatedTask)
}

// resolveImageProviderFromModel 查 va_image_generation_model 得到图像类模版应使用的执行器。
// 模型名取 workflow 的 api_config.model_name，未配置时回落到 constants.DefaultGPTImageModel。
func resolveImageProviderFromModel(ctx context.Context, projectID, modelName string) (string, error) {
	if modelName == "" {
		modelName = constants.DefaultGPTImageModel
	}
	if projectID == "" || projectID == constants.ProjectIdVisualAI {
		projectID = constants.ProjectIdVisionAI
	}

	row, err := dao.NewImageGenerationModelDao(db.GetDB()).GetModelByName(ctx, modelName, projectID)
	if err != nil {
		return "", fmt.Errorf("get image generation model %q of project %q: %w", modelName, projectID, err)
	}
	if row.Provider == "" {
		return "", fmt.Errorf("image generation model %q has no provider configured", modelName)
	}
	return row.Provider, nil
}

func (s *TaskExecutionService) executeTaskWorker(baseCtx context.Context, task *model.PictureTask) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Panic recovered in executeTaskWorker",
				zap.String("task_id", task.TaskID),
				zap.Any("panic", r),
				zap.Stack("stack"),
			)
			if err := s.taskService.FailTask(context.Background(), task, "panic in task execution"); err != nil {
				s.logger.Error("Failed to handle task failure after panic", zap.String("task_id", task.TaskID), zap.Error(err))
			}
		}
	}()

	ctx, cancel := context.WithTimeout(baseCtx, s.config.TaskTimeout)
	defer cancel()
	s.logger.Info("Starting task execution", zap.String("task_id", task.TaskID), zap.String("user_id", task.UserID), zap.Duration("timeout", s.config.TaskTimeout))

	// 检查是否为模型直连模式（虚拟 workflow）
	isModelDirectMode := strings.HasPrefix(task.WorkflowID, "model_direct_")

	// 解析任务参数
	var taskParams map[string]string
	if err := json.Unmarshal([]byte(task.ParamJSON), &taskParams); err != nil {
		s.logger.Error("Failed to unmarshal task params for task",
			zap.String("task_id", task.TaskID),
			zap.String("param_json", task.ParamJSON),
			zap.Error(err),
		)
		if failErr := s.taskService.FailTask(ctx, task, fmt.Sprintf("unmarshal task params error: %v", err)); failErr != nil {
			s.logger.Error("Failed to handle task failure after unmarshalling params failed", zap.String("task_id", task.TaskID), zap.Error(failErr))
		}
		return
	}

	// 获取 provider 信息
	var provider string
	var useApi bool
	if isModelDirectMode {
		// 模型直连模式：从 task.Executer 获取 provider
		provider = task.Executer
		useApi = true
		s.logger.Info("Model direct mode detected, skipping workflow query",
			zap.String("task_id", task.TaskID),
			zap.String("workflow_id", task.WorkflowID),
			zap.String("provider", provider))
	} else {
		// Workflow 模式：查询数据库获取 workflow 信息
		workflowInfo, err := s.picForgeDao.GetWorkflow(ctx, db.GetDB(), task.WorkflowID)
		if err != nil {
			s.logger.Error("Failed to get workflow info for task",
				zap.String("task_id", task.TaskID),
				zap.String("workflow_id", task.WorkflowID),
				zap.Error(err),
			)
			if failErr := s.taskService.FailTask(ctx, task, fmt.Sprintf("get workflow info error: %v", err)); failErr != nil {
				s.logger.Error("Failed to handle task failure after getting workflow info failed", zap.String("task_id", task.TaskID), zap.Error(failErr))
			}
			return
		}
		provider = workflowInfo.Provider
		useApi = workflowInfo.UseApi

		// 图像类模版的执行器与模型直连模式统一由 va_image_generation_model.provider 决定，
		// workflow.provider 列对图像类模版不再生效
		if task.WorkflowType == pb.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE.String() {
			routed, resolveErr := resolveImageProviderFromModel(ctx, task.ProjectID, workflowInfo.ApiConfig.ModelName)
			if resolveErr != nil {
				s.logger.Error("Failed to resolve provider from model table for image workflow",
					zap.String("task_id", task.TaskID),
					zap.String("workflow_id", task.WorkflowID),
					zap.String("model_name", workflowInfo.ApiConfig.ModelName),
					zap.Error(resolveErr),
				)
				if failErr := s.taskService.FailTask(ctx, task, fmt.Sprintf("resolve image provider error: %v", resolveErr)); failErr != nil {
					s.logger.Error("Failed to handle task failure after resolving provider failed", zap.String("task_id", task.TaskID), zap.Error(failErr))
				}
				return
			}

			s.logger.Info("image workflow provider resolved from model table",
				zap.String("task_id", task.TaskID),
				zap.String("workflow_provider", provider),
				zap.String("model_name", workflowInfo.ApiConfig.ModelName),
				zap.String("routed_provider", routed))
			provider = routed
			useApi = true
		}
	}

	// 在taskParams中添加provider信息，用于执行器匹配
	if provider != "" {
		taskParams["provider"] = provider
	}

	s.logger.Info("Calling underlying generation service for task",
		zap.String("task_id", task.TaskID),
		zap.Bool("use_api", useApi),
		zap.Bool("is_model_direct_mode", isModelDirectMode),
	)

	var generationErr error
	executor := s.executorService.GetExecutorForTask(taskParams)
	workflowType := task.WorkflowType
	if executor != nil {
		s.logger.Info("Using executor for task",
			zap.String("task_id", task.TaskID),
			zap.String("executor", executor.GetName()),
		)
		generationErr = executor.Process(ctx, task.TaskID, taskParams)
	} else {
		s.logger.Error("Video task cannot use ImageGenerator fallback",
			zap.String("task_id", task.TaskID),
			zap.String("provider", provider),
			zap.String("workflow_type", workflowType),
		)
		generationErr = fmt.Errorf("no executor matched for provider=%s, video tasks cannot fallback to ImageGenerator", provider)
	}
	// else {
	// 	// 增强验证：检查是否是视频任务但没有匹配的执行器
	// 	provider := taskParams["provider"]
	// 	workflowType := task.WorkflowType

	// 	s.logger.Warn("No executor matched for task",
	// 		zap.String("task_id", task.TaskID),
	// 		zap.String("provider", provider),
	// 		zap.String("workflow_type", workflowType),
	// 	)

	// 	// 如果是视频任务且没有匹配的执行器，直接失败
	// 	if workflowType == pb.WorkflowKindType_WORKFLOW_KIND_TYPE_VIDEO.String() {
	// 		s.logger.Error("Video task cannot use ImageGenerator fallback",
	// 			zap.String("task_id", task.TaskID),
	// 			zap.String("provider", provider),
	// 			zap.String("workflow_type", workflowType),
	// 		)
	// 		generationErr = fmt.Errorf("no executor matched for provider=%s, video tasks cannot fallback to ImageGenerator", provider)
	// 	} else {
	// 		// 图片任务保持原有的回退行为
	// 		s.logger.Info("Using imageGenerator fallback for image task",
	// 			zap.String("task_id", task.TaskID),
	// 			zap.String("provider", provider),
	// 		)
	// 		if workflowInfo.UseApi {
	// 			generationErr = s.imageGenerator.ApiGenerate(ctx, workflowInfo.WorkflowJson, task.TaskID, taskParams)
	// 		} else {
	// 			generationErr = s.imageGenerator.Generate(ctx, workflowInfo.WorkflowJson, task.TaskID, taskParams)
	// 		}
	// 	}
	// }

	if generationErr != nil {
		s.logger.Error("Underlying generation service returned error for task",
			zap.String("task_id", task.TaskID),
			zap.Error(generationErr),
		)
		if failErr := s.taskService.FailTask(ctx, task, fmt.Sprintf("generation error: %v", generationErr)); failErr != nil {
			s.logger.Error("Failed to handle task failure after generation error", zap.String("task_id", task.TaskID), zap.Error(failErr))
		}
	} else {
		s.logger.Info("Underlying generation service completed successfully for task", zap.String("task_id", task.TaskID))
	}

	s.logger.Info("Task execution finished", zap.String("task_id", task.TaskID), zap.String("user_id", task.UserID))
}
