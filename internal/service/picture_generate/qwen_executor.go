package picture_generate

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/go-redis/redis"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	aigcv1 "va_visionai_server/internal/aibrain/v1"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/ai_clients/aigc_core"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// QwenExecutor 通义千问图生图执行器，通过 AIGC Core 调用。
// 复用 GeminiExecutor 的状态同步和结果处理逻辑，仅参数构建不同。
type QwenExecutor struct {
	GeminiExecutor // 嵌入以复用 SyncProviderStatus / ProcessSuccessfulResult / handleSubmissionError 等
}

// 编译时接口检查
var _ TaskExecutor = (*QwenExecutor)(nil)

func NewQwenExecutor(
	aigcClient *aigc_core.Client,
	taskDao *dao.PictureTaskDao,
	pictureForgeDao *dao.PictureForgeDao,
	rdb *redis.Client,
) *QwenExecutor {
	return &QwenExecutor{
		GeminiExecutor: GeminiExecutor{
			aigcClient:      aigcClient,
			taskDao:         taskDao,
			pictureForgeDao: pictureForgeDao,
			rdb:             rdb,
		},
	}
}

func (e *QwenExecutor) GetName() string {
	return constants.QwenExecutorName
}

func (e *QwenExecutor) Match(params map[string]string) bool {
	if provider, exists := params["provider"]; exists {
		return provider == constants.QwenExecutorName
	}
	return false
}

func (e *QwenExecutor) Process(ctx context.Context, taskID string, params map[string]string) error {
	task, err := e.taskDao.GetTask(ctx, taskID)
	if err != nil {
		return errors.Wrap(err, "获取任务失败")
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		zlog.LogWithContext(ctx).Info("Qwen任务已完成或失败，跳过处理",
			zap.String("taskID", taskID),
			zap.Int32("status", task.Status))
		return nil
	}

	task.Executer = constants.QwenExecutorName
	task.RetryCount = 0
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 5, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		return errors.Wrap(err, "更新任务初始状态失败")
	}

	requestParams := e.buildQwenParameters(ctx, task, params)

	clientID := uuid.New().String()
	parametersJson, err := json.Marshal(requestParams)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "序列化参数失败"), false)
	}

	taskType := e.determineQwenTaskType(params)

	submitReq := &aigcv1.SubmitAsyncTaskRequest{
		TaskType:       taskType,
		Model:          e.getQwenModel(task),
		ProviderHint:   "qwen",
		ParametersJson: string(parametersJson),
	}

	resp, err := e.aigcClient.SubmitAsyncTask(ctx, submitReq)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "提交任务到AIGC Core失败"), true)
	}

	task.ExecuterTaskID = resp.GetTaskId()
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 15, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION); err != nil {
		return errors.Wrap(err, "保存AIGC任务ID失败")
	}

	zlog.LogWithContext(ctx).Info("Qwen任务提交成功",
		zap.String("taskID", taskID),
		zap.String("aigcTaskID", resp.GetTaskId()),
		zap.String("clientID", clientID),
		zap.String("taskType", taskType))

	return nil
}

// buildQwenParameters 构建 Qwen 图生图请求参数
// 主要参数：prompt, image_urls(单图), width, height, seed
func (e *QwenExecutor) buildQwenParameters(ctx context.Context, task *model.PictureTask, params map[string]string) map[string]interface{} {
	modelName := e.getQwenModel(task)
	prompt := params["prompt"]
	if val, ok := params["InputPrompt"]; ok && val != "" {
		prompt = val
	}

	requestParams := map[string]interface{}{
		"model":  modelName,
		"prompt": prompt,
	}

	// 收集输入图片（单图：取第一张）
	for key, value := range params {
		if strings.HasPrefix(key, "LoadImage") || strings.HasPrefix(key, "input_image_") {
			if strings.TrimSpace(value) != "" {
				requestParams["image_urls"] = []string{value}
				break // 只取第一张
			}
		}
	}

	// 结果图宽度
	if width, ok := params["width"]; ok && width != "" {
		requestParams["width"] = width
	}

	// 结果图高度
	if height, ok := params["height"]; ok && height != "" {
		requestParams["height"] = height
	}

	// 随机种子
	if seed, ok := params["seed"]; ok && seed != "" {
		requestParams["seed"] = seed
	}

	return requestParams
}

// determineQwenTaskType 判断任务类型
func (e *QwenExecutor) determineQwenTaskType(params map[string]string) string {
	for key, value := range params {
		if strings.HasPrefix(key, "LoadImage") || strings.HasPrefix(key, "input_image_") {
			if strings.TrimSpace(value) != "" {
				return "image_to_image"
			}
		}
	}
	return "text_to_image"
}

// getQwenModel 获取模型名称
func (e *QwenExecutor) getQwenModel(task *model.PictureTask) string {
	if task.ApiConfig.ModelName != "" {
		return task.ApiConfig.ModelName
	}
	return "qwen-max-vl"
}

// SyncProviderStatus 覆写父类方法，修正执行器名称校验
func (e *QwenExecutor) SyncProviderStatus(ctx context.Context, task *model.PictureTask) (pb.WorkflowTaskStatus, error) {
	if task.Executer != constants.QwenExecutorName {
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED,
			errors.New("执行器不匹配，无法同步状态")
	}

	if task.ExecuterTaskID == "" {
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED,
			errors.New("AIGC任务ID为空，无法同步状态")
	}

	resp, err := e.aigcClient.GetAsyncTask(context.Background(), &aigcv1.GetAsyncTaskRequest{
		TaskId: task.ExecuterTaskID,
	})
	if err != nil {
		zlog.LogWithContext(ctx).Debug("获取AIGC任务状态失败",
			zap.String("taskID", task.TaskID),
			zap.String("aigcTaskID", task.ExecuterTaskID),
			zap.Error(err))
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil
	}

	switch resp.GetStatus() {
	case aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_PENDING,
		aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_PROCESSING,
		aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_AWAITING_PROVIDER,
		aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_PENDING_RETRY:

		var newProgress int32
		if resp.GetProgress() > 0 && resp.GetProgress() <= 100 {
			newProgress = resp.GetProgress()
			if newProgress > 85 {
				newProgress = 85
			}
		} else {
			if task.Progress < 85 {
				increment := int32(3 + (task.Progress % 3))
				newProgress = task.Progress + increment
				if newProgress > 85 {
					newProgress = 85
				}
			} else {
				newProgress = task.Progress
			}
		}

		if newProgress > task.Progress {
			if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, newProgress, 0); err != nil {
				zlog.LogWithContext(ctx).Error("更新Qwen进度失败", zap.Error(err))
			}
		}

		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil

	case aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_COMPLETED:
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED, nil

	case aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_FAILED:
		reason, fullReason := constants.GetGenericErrorInfo(resp.GetErrorMessage())
		task.FailedReason = reason
		task.FailedReasonFull = fullReason
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.New(resp.GetErrorMessage())

	default:
		zlog.LogWithContext(ctx).Warn("未知的AIGC任务状态",
			zap.String("taskID", task.TaskID),
			zap.String("status", resp.GetStatus().String()))
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil
	}
}

// RetrySubmission 覆写父类方法，修正执行器名称校验和 provider 设置
func (e *QwenExecutor) RetrySubmission(ctx context.Context, task *model.PictureTask) error {
	if task.Executer != constants.QwenExecutorName {
		return errors.New("任务执行器不匹配，无法重试")
	}

	var taskParams map[string]string
	if err := json.Unmarshal([]byte(task.ParamJSON), &taskParams); err != nil {
		return errors.Wrap(err, "解析任务参数失败")
	}

	taskParams["provider"] = constants.QwenExecutorName

	return e.Process(ctx, task.TaskID, taskParams)
}
