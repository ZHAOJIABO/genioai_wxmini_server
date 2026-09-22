package picture_generate

import (
	"context"
	"encoding/json"

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

// VectorEngineGeminiExecutor 通过 vectorengine 平台调用 Gemini 的执行器。
// 所有业务逻辑与 GeminiExecutor 完全一致，仅 ProviderHint 不同。
type VectorEngineGeminiExecutor struct {
	GeminiExecutor
}

// 编译时接口检查
var _ TaskExecutor = (*VectorEngineGeminiExecutor)(nil)

func NewVectorEngineGeminiExecutor(
	aigcClient *aigc_core.Client,
	taskDao *dao.PictureTaskDao,
	pictureForgeDao *dao.PictureForgeDao,
	rdb *redis.Client,
) *VectorEngineGeminiExecutor {
	return &VectorEngineGeminiExecutor{
		GeminiExecutor: GeminiExecutor{
			aigcClient:      aigcClient,
			taskDao:         taskDao,
			pictureForgeDao: pictureForgeDao,
			rdb:             rdb,
		},
	}
}

func (e *VectorEngineGeminiExecutor) GetName() string {
	return constants.VectorEngineGeminiExecutorName
}

func (e *VectorEngineGeminiExecutor) Match(params map[string]string) bool {
	if provider, exists := params["provider"]; exists {
		return provider == constants.VectorEngineGeminiExecutorName
	}
	return false
}

func (e *VectorEngineGeminiExecutor) Process(ctx context.Context, taskID string, params map[string]string) error {
	task, err := e.taskDao.GetTask(ctx, taskID)
	if err != nil {
		return errors.Wrap(err, "获取任务失败")
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		zlog.LogWithContext(ctx).Info("VectorEngine Gemini任务已完成或失败，跳过处理",
			zap.String("taskID", taskID),
			zap.Int32("status", task.Status))
		return nil
	}

	task.Executer = constants.VectorEngineGeminiExecutorName
	task.RetryCount = 0
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 5, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		return errors.Wrap(err, "更新任务初始状态失败")
	}

	requestParams, err := e.buildGeminiParameters(ctx, task, params)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, false)
	}

	clientID := uuid.New().String()
	parametersJson, err := json.Marshal(requestParams)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "序列化参数失败"), false)
	}

	taskType := e.determineTaskType(params)

	submitReq := &aigcv1.SubmitAsyncTaskRequest{
		TaskType:       taskType,
		Model:          e.getModelFromApiConfig(task),
		ProviderHint:   "vectorengine",
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

	zlog.LogWithContext(ctx).Info("VectorEngine Gemini任务提交成功",
		zap.String("taskID", taskID),
		zap.String("aigcTaskID", resp.GetTaskId()),
		zap.String("clientID", clientID),
		zap.String("taskType", taskType))

	return nil
}

func (e *VectorEngineGeminiExecutor) SyncProviderStatus(ctx context.Context, task *model.PictureTask) (pb.WorkflowTaskStatus, error) {
	if task.Executer != constants.VectorEngineGeminiExecutorName {
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED,
			errors.New("执行器不匹配，无法同步状态")
	}

	if task.ExecuterTaskID == "" {
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED,
			errors.New("AIGC任务ID为空，无法同步状态")
	}

	getTaskReq := &aigcv1.GetAsyncTaskRequest{
		TaskId: task.ExecuterTaskID,
	}

	resp, err := e.aigcClient.GetAsyncTask(context.Background(), getTaskReq)
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
				zlog.LogWithContext(ctx).Error("更新VectorEngine Gemini进度失败", zap.Error(err))
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

func (e *VectorEngineGeminiExecutor) RetrySubmission(ctx context.Context, task *model.PictureTask) error {
	if task.Executer != constants.VectorEngineGeminiExecutorName {
		return errors.New("任务执行器不匹配，无法重试")
	}

	var taskParams map[string]string
	if err := json.Unmarshal([]byte(task.ParamJSON), &taskParams); err != nil {
		return errors.Wrap(err, "解析任务参数失败")
	}

	taskParams["provider"] = constants.VectorEngineGeminiExecutorName

	return e.Process(ctx, task.TaskID, taskParams)
}
