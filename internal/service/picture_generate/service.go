package picture_generate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/conf"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	aigcclient "va_visionai_server/internal/service/ai_clients/aigc_core"
	"va_visionai_server/internal/service/ai_clients/gpt4o"
	"va_visionai_server/internal/service/ai_clients/klingai"
	"va_visionai_server/internal/service/ai_clients/minimax"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/service/event"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// ExecutorService 负责管理和选择合适的任务执行器
type ExecutorService struct {
	taskDao         *dao.PictureTaskDao
	uploadDao       *dao.UploadDao
	configDao       *dao.ConfigDao
	rdb             *redis.Client
	creditService   credit.Service
	evaluateService *EvaluateService
	aigcClient      *aigcclient.Client
	promptDao       *dao.PromptDao
	executors       map[string]TaskExecutor
	retryExecutors  map[string]TaskExecutorForRetry
	syncExecutors   map[string]TaskExecutorForSync
	mu              sync.RWMutex
}

// NewExecutorService 创建一个新的 ExecutorService
func NewExecutorService(
	taskDao *dao.PictureTaskDao,
	uploadDao *dao.UploadDao,
	configDao *dao.ConfigDao,
	rdb *redis.Client,
	creditService credit.Service,
	evaluateService *EvaluateService,
	aigcClient *aigcclient.Client,
	promptDao *dao.PromptDao,
) *ExecutorService {
	service := &ExecutorService{
		taskDao:         taskDao,
		uploadDao:       uploadDao,
		configDao:       configDao,
		rdb:             rdb,
		creditService:   creditService,
		evaluateService: evaluateService,
		aigcClient:      aigcClient,
		promptDao:       promptDao,
	}

	// 初始化并注册执行器
	service.registerExecutors()

	return service
}

// SetTaskFailureHandler 为所有已注册的执行器设置失败处理器
func (s *ExecutorService) SetTaskFailureHandler(handler TaskFailureHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for name, executor := range s.executors {
		executor.SetFailureHandler(handler)
		zlog.Logger.Debug("Set failure handler for executor", zap.String("executorName", name))
	}
}

// registerExecutors 注册所有可用的执行器
func (s *ExecutorService) registerExecutors() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.executors = make(map[string]TaskExecutor)
	s.retryExecutors = make(map[string]TaskExecutorForRetry)
	s.syncExecutors = make(map[string]TaskExecutorForSync)

	// ComfyUI Executor
	configDao := dao.NewConfigDao()
	comfyUIExecutor := NewComfyUIExecutor(
		s.taskDao,
		s.uploadDao,
		dao.NewPictureForgeDao(db.GetDB()),
		s.rdb,
		&http.Client{},
		s.evaluateService,
		configDao,
	)
	s.executors[comfyUIExecutor.GetName()] = comfyUIExecutor
	s.retryExecutors[comfyUIExecutor.GetName()] = comfyUIExecutor
	s.syncExecutors[comfyUIExecutor.GetName()] = comfyUIExecutor

	// Kling Executor
	klingClient := klingai.NewKlingClient(klingai.KlingConfig{
		Endpoint: conf.GlobalConfig.LlmConfig.KlingAPI.Endpoint,
		AppID:    conf.GlobalConfig.LlmConfig.KlingAPI.AppID,
		Secret:   conf.GlobalConfig.LlmConfig.KlingAPI.Secret,
	})
	klingImg2VideoExecutor := NewKlingImage2VideoExecutor(
		s.taskDao,
		s.uploadDao,
		klingClient,
		s.rdb,
		s.configDao,
		s.creditService,
	)
	s.executors[klingImg2VideoExecutor.GetName()] = klingImg2VideoExecutor
	s.retryExecutors[klingImg2VideoExecutor.GetName()] = klingImg2VideoExecutor
	s.syncExecutors[klingImg2VideoExecutor.GetName()] = klingImg2VideoExecutor

	// GPT-4o Executor
	gpt4oClient := gpt4o.NewGpt4oClient(gpt4o.Gpt4oConfig{
		Endpoint: conf.GlobalConfig.LlmConfig.GptImage.Endpoint,
		ApiKey:   conf.GlobalConfig.LlmConfig.GptImage.APIKey,
	})
	// gpt4oImg2ImgExecutor := NewGpt4oImage2ImageExecutor(
	// 	s.taskDao,
	// 	s.uploadDao,
	// 	gpt4oClient,
	// 	s.rdb,
	// )
	// s.executors[gpt4oImg2ImgExecutor.GetName()] = gpt4oImg2ImgExecutor
	// s.retryExecutors[gpt4oImg2ImgExecutor.GetName()] = gpt4oImg2ImgExecutor
	// s.syncExecutors[gpt4oImg2ImgExecutor.GetName()] = gpt4oImg2ImgExecutor

	// GPT-4o Executor V2
	gpt4oImg2ImgExecutorV2 := NewGpt4oImage2ImageExecutorV2(
		s.taskDao,
		s.uploadDao,
		gpt4oClient,
		s.rdb,
	)
	s.executors[gpt4oImg2ImgExecutorV2.GetName()] = gpt4oImg2ImgExecutorV2
	s.retryExecutors[gpt4oImg2ImgExecutorV2.GetName()] = gpt4oImg2ImgExecutorV2
	s.syncExecutors[gpt4oImg2ImgExecutorV2.GetName()] = gpt4oImg2ImgExecutorV2

	// Minimax Executor
	minimaxClient := minimax.NewMinimaxClient(minimax.MinimaxConfig{
		Endpoint: conf.GlobalConfig.LlmConfig.MinimaxAPI.Endpoint,
		AppID:    conf.GlobalConfig.LlmConfig.MinimaxAPI.AppID,
		Secret:   conf.GlobalConfig.LlmConfig.MinimaxAPI.Secret,
	})
	minimaxVideoExecutor := NewMinimaxVideoExecutor(
		s.taskDao,
		s.uploadDao,
		minimaxClient,
		s.rdb,
		s.configDao,
		s.creditService,
	)
	s.executors[minimaxVideoExecutor.GetName()] = minimaxVideoExecutor
	s.retryExecutors[minimaxVideoExecutor.GetName()] = minimaxVideoExecutor
	s.syncExecutors[minimaxVideoExecutor.GetName()] = minimaxVideoExecutor

	// Cloud Comfy Executor
	cloudComfyExecutor := NewCloudComfyExecutor(
		s.aigcClient,
		s.taskDao,
		dao.NewPictureForgeDao(db.GetDB()),
		s.rdb,
	)
	s.executors[cloudComfyExecutor.GetName()] = cloudComfyExecutor
	s.retryExecutors[cloudComfyExecutor.GetName()] = cloudComfyExecutor
	s.syncExecutors[cloudComfyExecutor.GetName()] = cloudComfyExecutor

	// Gemini Executor
	geminiExecutor := NewGeminiExecutor(
		s.aigcClient,
		s.taskDao,
		dao.NewPictureForgeDao(db.GetDB()),
		s.rdb,
	)
	s.executors[geminiExecutor.GetName()] = geminiExecutor
	s.retryExecutors[geminiExecutor.GetName()] = geminiExecutor
	s.syncExecutors[geminiExecutor.GetName()] = geminiExecutor

	// VectorEngine Gemini Executor
	vectorEngineGeminiExecutor := NewVectorEngineGeminiExecutor(
		s.aigcClient,
		s.taskDao,
		dao.NewPictureForgeDao(db.GetDB()),
		s.rdb,
	)
	s.executors[vectorEngineGeminiExecutor.GetName()] = vectorEngineGeminiExecutor
	s.retryExecutors[vectorEngineGeminiExecutor.GetName()] = vectorEngineGeminiExecutor
	s.syncExecutors[vectorEngineGeminiExecutor.GetName()] = vectorEngineGeminiExecutor

	// 注册Minimax模板视频执行器
	minimaxTemplateVideoExecutor := NewMinimaxTemplateVideoExecutor(
		s.taskDao,
		s.aigcClient,
		s.rdb,
		s.configDao,
	)
	s.executors[minimaxTemplateVideoExecutor.GetName()] = minimaxTemplateVideoExecutor
	s.retryExecutors[minimaxTemplateVideoExecutor.GetName()] = minimaxTemplateVideoExecutor
	s.syncExecutors[minimaxTemplateVideoExecutor.GetName()] = minimaxTemplateVideoExecutor
	// GPT-4o Text2Image Executor (Disabled - Soulmate service removed)
	// gpt4oText2ImageClient := gpt4o.NewGpt4oClient(gpt4o.Gpt4oConfig{
	// 	Endpoint: conf.GlobalConfig.LlmConfig.GptText2Image.Endpoint,
	// 	ApiKey:   conf.GlobalConfig.LlmConfig.GptText2Image.APIKey,
	// })
	// gpt4oText2ImgExecutor := NewGpt4oText2ImageExecutor(
	// 	s.taskDao,
	// 	s.uploadDao,
	// 	gpt4oText2ImageClient,
	// 	s.rdb,
	// 	s.promptDao,
	// )
	// s.executors[gpt4oText2ImgExecutor.GetName()] = gpt4oText2ImgExecutor
	// s.retryExecutors[gpt4oText2ImgExecutor.GetName()] = gpt4oText2ImgExecutor
	// s.syncExecutors[gpt4oText2ImgExecutor.GetName()] = gpt4oText2ImgExecutor

	// Sora2 Video Executor
	sora2Executor := NewSora2Executor(
		s.aigcClient,
		s.taskDao,
		s.rdb,
	)
	s.executors[sora2Executor.GetName()] = sora2Executor
	s.retryExecutors[sora2Executor.GetName()] = sora2Executor
	s.syncExecutors[sora2Executor.GetName()] = sora2Executor

	// Qwen Executor
	qwenExecutor := NewQwenExecutor(
		s.aigcClient,
		s.taskDao,
		dao.NewPictureForgeDao(db.GetDB()),
		s.rdb,
	)
	s.executors[qwenExecutor.GetName()] = qwenExecutor
	s.retryExecutors[qwenExecutor.GetName()] = qwenExecutor
	s.syncExecutors[qwenExecutor.GetName()] = qwenExecutor

	// GPT-Image-2 Executor
	gptImage2Executor := NewGPTImage2Executor(
		s.aigcClient,
		s.taskDao,
		dao.NewPictureForgeDao(db.GetDB()),
		s.rdb,
	)
	s.executors[gptImage2Executor.GetName()] = gptImage2Executor
	s.retryExecutors[gptImage2Executor.GetName()] = gptImage2Executor
	s.syncExecutors[gptImage2Executor.GetName()] = gptImage2Executor

	// GPT-Image-2 Image-to-Image Executor
	gptImage2I2IExecutor := NewGPTImage2I2IExecutor(
		s.aigcClient,
		s.taskDao,
		dao.NewPictureForgeDao(db.GetDB()),
		s.rdb,
	)
	s.executors[gptImage2I2IExecutor.GetName()] = gptImage2I2IExecutor
	s.retryExecutors[gptImage2I2IExecutor.GetName()] = gptImage2I2IExecutor
	s.syncExecutors[gptImage2I2IExecutor.GetName()] = gptImage2I2IExecutor

	// 这里可以注册更多执行器
}

// GetExecutorForTask 根据任务参数选择合适的执行器
func (s *ExecutorService) GetExecutorForTask(params map[string]string) TaskExecutor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, executor := range s.executors {
		if executor.Match(params) {
			return executor
		}
	}
	return nil
}

// GetExecutorForRetry 根据执行器名称获取支持重试的执行器
func (s *ExecutorService) GetExecutorForRetry(name string) TaskExecutorForRetry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.retryExecutors[name]
}

// GetExecutorForSync 根据执行器名称获取支持同步的执行器
func (s *ExecutorService) GetExecutorForSync(name string) TaskExecutorForSync {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.syncExecutors[name]
}

// updateTaskProgressAndSendEvent 通用的进度更新和事件发送函数
// 这个函数原子化地执行两个操作：
// 1. 更新数据库中 PictureTask 的 Progress 和 Status 字段
// 2. 向前端发送一个包含最新进度的 TaskProgressEvent 事件
func updateTaskProgressAndSendEvent(
	ctx context.Context,
	taskDao *dao.PictureTaskDao,
	rdb *redis.Client,
	task *model.PictureTask,
	progress int32,
	status pb.WorkflowTaskStatus,
) error {
	// 更新任务进度和状态
	if progress >= 0 {
		task.Progress = progress
	}

	if status != 0 {
		task.Status = int32(status)
	}

	// 任务完成时清理进度缓存
	if status == pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED {
		rdb.Del(constants.RedisKeyPictureTaskProgress + ":" + task.TaskID)
	}

	// 更新数据库
	if err := taskDao.UpdateTask(context.Background(), task); err != nil {
		zlog.LogWithContext(ctx).Error("更新任务状态失败",
			zap.String("taskID", task.TaskID),
			zap.Int32("progress", progress),
			zap.Int32("status", int32(status)),
			zap.Error(err))
		return err
	}

	// 发送进度事件
	SendProgressEvent(task)

	// 在任务终止时触发资源清理hook（同步，确保资源释放）
	// 适用于任务完成或失败的情况
	if status == pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED ||
		status == pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED {
		TriggerOnTaskTerminated(ctx, task)
	}

	// 在任务完成后触发链式编排Hook（异步，不影响主流程）
	if status == pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED {
		if onTaskCompletedHook != nil {
			go onTaskCompletedHook(task)
		}
	}
	return nil
}

// SendProgressEvent 发送进度事件（从原有执行器中提取的通用函数）
func SendProgressEvent(task *model.PictureTask) {
	eventData := &pb.TaskProgressEventData{
		PictureTaskId: task.TaskID,
		Progress:      task.Progress,
		CanRetry:      task.CanRetry,
	}

	// 如果任务完成且有结果，则附加结果信息
	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) && task.ResultJSON != "" {
		var result model.PictureTaskResult
		if err := json.Unmarshal([]byte(task.ResultJSON), &result); err == nil {
			switch task.WorkflowType {
			case pb.WorkflowKindType_WORKFLOW_KIND_TYPE_VIDEO.String():
				// 视频任务
				eventData.PictureInfo = &pb.PictureInfo{
					Url:          result.ResultURL,
					AspectRatio:  float32(result.VideoFrameAspectRatio),
					ThumbnailUrl: result.VideoFrameURL,
				}
			default:
				// 图片任务
				eventData.PictureInfo = &pb.PictureInfo{
					Url:          result.ResultURL,
					AspectRatio:  float32(result.AspectRatio),
					ThumbnailUrl: result.UserShowImageURL,
				}
			}
		}
	}

	event.GetEventRegistry().DispatchToUser(
		pb.WatchEventType_WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS,
		eventData,
		task.ProjectID,
		task.UserID,
	)
}

// onTaskCompletedHook 是一个可选的回调，用于任务完成后的扩展（例如任务链编排）
var onTaskCompletedHook func(task *model.PictureTask)

// onTaskTerminatedHook 是一个可选的回调，用于任务终止时的清理（包括完成和失败）
// 用于释放资源，如并发槽位
var onTaskTerminatedHook func(ctx context.Context, task *model.PictureTask)

// RegisterOnTaskCompletedHook 注册任务完成回调
func RegisterOnTaskCompletedHook(fn func(task *model.PictureTask)) {
	onTaskCompletedHook = fn
}

// RegisterOnTaskTerminatedHook 注册任务终止回调（用于清理资源）
func RegisterOnTaskTerminatedHook(fn func(ctx context.Context, task *model.PictureTask)) {
	onTaskTerminatedHook = fn
}

// TriggerOnTaskCompleted 触发任务完成回调
func TriggerOnTaskCompleted(task *model.PictureTask) {
	if onTaskCompletedHook != nil {
		go onTaskCompletedHook(task)
	}
}

// TriggerOnTaskTerminated 触发任务终止回调（同步调用，确保资源释放）
func TriggerOnTaskTerminated(ctx context.Context, task *model.PictureTask) {
	if onTaskTerminatedHook != nil {
		onTaskTerminatedHook(ctx, task)
	}
}

func GetGeneratorName(task *model.PictureTask, format string) string {
	return fmt.Sprintf("%s/generated/%s/cloud_comfy_%s.%s", task.ProjectID, task.UserID, task.TaskID, format)
}

func GetGeneratorThumbnailName(task *model.PictureTask, format string) string {
	return fmt.Sprintf("%s/generated/%s/cloud_comfy_%s_thumbnail.%s", task.ProjectID, task.UserID, task.TaskID, format)
}
