package picture_generate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-redis/redis"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/ai_clients/aigc_core"
	"va_visionai_server/internal/utils"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
	aigcv1 "va_visionai_server/internal/aibrain/v1"
)

const (
	MinimaxTemplateVideoExecutorName = "minimax_template_video_executor"
)

// MinimaxTemplateVideoExecutor Minimax模板视频生成执行器
type MinimaxTemplateVideoExecutor struct {
	taskDao        *dao.PictureTaskDao
	aigcCoreClient *aigc_core.Client
	rdb            *redis.Client
	configDao      *dao.ConfigDao
	// paramPreparers map[string]ApiParameterPreparerFunc // 暂时不使用
	failureHandler TaskFailureHandler
}

// JobResult AIGC Core任务结果
type JobResult struct {
	TaskID string
	Status string
}

func NewMinimaxTemplateVideoExecutor(
	taskDao *dao.PictureTaskDao,
	aigcCoreClient *aigc_core.Client,
	rdb *redis.Client,
	configDao *dao.ConfigDao,
) *MinimaxTemplateVideoExecutor {
	e := &MinimaxTemplateVideoExecutor{
		taskDao:        taskDao,
		aigcCoreClient: aigcCoreClient,
		rdb:            rdb,
		configDao:      configDao,
	}

	// 初始化参数准备器 - 移除暂时不使用的paramPreparers

	return e
}

func (e *MinimaxTemplateVideoExecutor) SetFailureHandler(handler TaskFailureHandler) {
	e.failureHandler = handler
}

func (e *MinimaxTemplateVideoExecutor) GetName() string {
	return MinimaxTemplateVideoExecutorName
}

func (e *MinimaxTemplateVideoExecutor) Match(params map[string]string) bool {
	provider, ok := params["provider"]
	if !ok {
		return false
	}
	apiIden, ok := params["api_iden"]
	if !ok {
		return false
	}
	return provider == constants.MinimaxExecutorName && apiIden == "effects"
}

func (e *MinimaxTemplateVideoExecutor) Process(ctx context.Context, taskID string, params map[string]string) error {
	task, err := e.taskDao.GetTask(ctx, taskID)
	if err != nil {
		return errors.Wrap(err, "获取任务失败")
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		zlog.LogWithContext(ctx).Info("Minimax模板视频任务已完成或失败，跳过处理",
			zap.String("taskID", taskID),
			zap.Int32("status", task.Status))
		return nil
	}

	task.Executer = MinimaxTemplateVideoExecutorName
	task.RetryCount = 0
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 5, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		return errors.Wrap(err, "更新任务初始状态失败")
	}

	// 准备模板视频负载
	payload, err := e.prepareTemplateVideoPayload(ctx, task, params)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, false)
	}

	// 提交任务到AIGC Core
	jobResult, err := e.submitAigcJobToProvider(ctx, task, payload)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, e.IsSubmissionErrorRetryable(err))
	}

	// 更新任务状态
	task.ExecuterTaskID = jobResult.TaskID
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 15, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION); err != nil {
		return errors.Wrap(err, "更新任务提交状态失败")
	}

	zlog.LogWithContext(ctx).Info("Minimax模板视频任务提交成功",
		zap.String("taskID", taskID),
		zap.String("aigcTaskID", jobResult.TaskID))

	return nil
}

func (e *MinimaxTemplateVideoExecutor) prepareTemplateVideoPayload(ctx context.Context, task *model.PictureTask, inputParams map[string]string) (map[string]interface{}, error) {
	// 获取配置
	templateVideoConfig, err := e.configDao.GetConfigValue(constants.ConfigKeyMinimaxTemplateVideoConfig)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("获取Minimax模板视频配置失败，使用默认配置", zap.Error(err))
		templateVideoConfig = "{}"
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(templateVideoConfig), &config); err != nil {
		return nil, errors.Wrap(err, "解析Minimax模板视频配置失败")
	}

	// 获取模板ID
	var templateID string
	if templateIDParam, exists := inputParams["effect_scene"]; exists {
		templateID = templateIDParam
	} else {
		return nil, errors.New("未提供template_id参数")
	}

	// 构建参数结构
	parametersMap := make(map[string]interface{})
	parametersMap["template_id"] = templateID

	// 处理媒体输入
	if imageURL, exists := inputParams["LoadImage1"]; exists && imageURL != "" {
		mediaInputs := []map[string]interface{}{
			{"value": imageURL},
		}
		parametersMap["media_inputs"] = mediaInputs
	}

	// 处理文本输入
	if prompt, exists := inputParams["prompt"]; exists && prompt != "" {
		textInputs := []map[string]interface{}{
			{"value": prompt},
		}
		parametersMap["text_inputs"] = textInputs
	}

	// 将默认配置合并到参数中
	for key, value := range config {
		if _, exists := parametersMap[key]; !exists {
			parametersMap[key] = value
		}
	}

	// 序列化参数
	parametersJson, err := json.Marshal(parametersMap)
	if err != nil {
		return nil, errors.Wrap(err, "序列化Minimax模板视频参数失败")
	}

	payload := map[string]interface{}{
		"task_type":       constants.MinimaxApiTypeTemplateVideo,
		"model":           "default",
		"provider_hint":   "minimax",
		"parameters_json": string(parametersJson),
	}

	zlog.LogWithContext(ctx).Info("Minimax模板视频负载准备完成",
		zap.String("taskID", task.TaskID),
		zap.String("templateID", templateID),
		zap.String("parametersJson", string(parametersJson)))

	return payload, nil
}

func (e *MinimaxTemplateVideoExecutor) submitAigcJobToProvider(ctx context.Context, task *model.PictureTask, payload map[string]interface{}) (*JobResult, error) {
	req := &aigcv1.SubmitAsyncTaskRequest{
		TaskType:       payload["task_type"].(string),
		Model:          payload["model"].(string),
		ProviderHint:   payload["provider_hint"].(string),
		ParametersJson: payload["parameters_json"].(string),
	}

	resp, err := e.aigcCoreClient.SubmitAsyncTask(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "提交任务到AIGC Core失败")
	}

	return &JobResult{
		TaskID: resp.GetTaskId(),
		Status: "submitted",
	}, nil
}

func (e *MinimaxTemplateVideoExecutor) SyncProviderStatus(ctx context.Context, task *model.PictureTask) (pb.WorkflowTaskStatus, error) {
	if task.Executer != MinimaxTemplateVideoExecutorName {
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

	resp, err := e.aigcCoreClient.GetAsyncTask(context.Background(), getTaskReq)
	if err != nil {
		zlog.LogWithContext(ctx).Debug("获取AIGC任务状态失败",
			zap.String("taskID", task.TaskID),
			zap.String("aigcTaskID", task.ExecuterTaskID),
			zap.Error(err))
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil
	}

	// 处理不同的任务状态
	switch resp.GetStatus() {
	case aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_PENDING,
		aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_PROCESSING,
		aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_AWAITING_PROVIDER,
		aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_PENDING_RETRY:

		// AIGC Core 的进度已足够平滑，直接采用；它不上报（0）时保持当前值不动。
		if aigc := resp.GetProgress(); aigc > 0 {
			newProgress := mapAigcProgress(aigc)
			if newProgress > task.Progress {
				if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, newProgress, 0); err != nil {
					zlog.LogWithContext(ctx).Error("更新Minimax模板视频进度失败", zap.Error(err))
				}
			}
		}

		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil

	case aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_COMPLETED:
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED, nil

	case aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_FAILED:
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.New(resp.GetErrorMessage())

	default:
		zlog.LogWithContext(ctx).Warn("未知的AIGC任务状态",
			zap.String("taskID", task.TaskID),
			zap.String("status", resp.GetStatus().String()))
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil
	}
}

func (e *MinimaxTemplateVideoExecutor) ProcessSuccessfulResult(ctx context.Context, task *model.PictureTask) error {
	if task.ExecuterTaskID == "" {
		return errors.New("AIGC任务ID为空，无法处理结果")
	}

	getTaskReq := &aigcv1.GetAsyncTaskRequest{
		TaskId: task.ExecuterTaskID,
	}

	resp, err := e.aigcCoreClient.GetAsyncTask(ctx, getTaskReq)
	if err != nil {
		return errors.Wrap(err, "获取AIGC任务结果失败")
	}

	if resp.GetStatus() != aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_COMPLETED {
		return errors.New("任务未完成，无法处理结果")
	}

	if resp.GetResult() == nil {
		return errors.New("任务结果为空")
	}

	// 开始下载结果：更新进度并启动尾段平滑推进
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, constants.ProgressDownloading, 0); err != nil {
		zlog.LogWithContext(ctx).Error("更新下载开始进度失败", zap.Error(err))
	}
	ramp := startTailRamp(e.taskDao, task)
	defer ramp.Stop()

	// 处理视频结果
	videoResult := resp.GetResult().GetVideoResult()
	if videoResult == nil {
		return errors.New("视频结果为空")
	}

	taskResult, err := e.processVideoResult(ctx, task, videoResult)
	if err != nil {
		return errors.Wrap(err, "处理视频结果失败")
	}
	ramp.Stop()

	// 更新任务结果
	resultJson, err := json.Marshal(taskResult)
	if err != nil {
		return errors.Wrap(err, "序列化任务结果失败")
	}

	task.ResultJSON = string(resultJson)
	task.UserPictureInfoJson = string(resultJson) // 使用现有字段存储结果
	// 注意：PictureTask模型中没有ResultURL等字段，这些信息存储在ResultJSON中

	// 更新进度至100%并完成任务
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 100, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED); err != nil {
		return errors.Wrap(err, "更新任务完成状态失败")
	}

	zlog.LogWithContext(ctx).Info("Minimax模板视频任务处理完成",
		zap.String("taskID", task.TaskID),
		zap.String("resultURL", taskResult.ResultURL))

	return nil
}

func (e *MinimaxTemplateVideoExecutor) processVideoResult(ctx context.Context, task *model.PictureTask, videoResult *aigcv1.VideoResult) (model.PictureTaskResult, error) {
	if videoResult.GetVideoUrl() == "" {
		return model.PictureTaskResult{}, errors.New("视频结果为空")
	}

	// 下载视频
	videoData, err := e.aigcCoreClient.DownloadVideo(ctx, videoResult.GetVideoUrl())
	if err != nil {
		return model.PictureTaskResult{}, errors.Wrap(err, "下载视频失败")
	}

	// 上传视频到OSS
	fileName := fmt.Sprintf("minimax_template_video_%s.mp4", task.TaskID)
	resultURL, err := utils.UploadToOSS(fileName, videoData)
	if err != nil {
		return model.PictureTaskResult{}, errors.Wrap(err, "上传视频到OSS失败")
	}

	width := int(videoResult.GetWidth())
	height := int(videoResult.GetHeight())
	aspectRatio := float64(height) / float64(width)

	// 尝试生成视频缩略图（提取第一帧）
	var thumbnailURL string
	thumbnailData, err := utils.ExtractFrameFromVideoBytesWithFFmpeg(videoData)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("提取视频缩略图失败", zap.Error(err))
	} else {
		thumbnailFileName := fmt.Sprintf("minimax_template_video_%s_thumbnail.jpg", task.TaskID)
		thumbnailURL, err = utils.UploadToOSS(thumbnailFileName, thumbnailData)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("上传视频缩略图失败", zap.Error(err))
		}
	}

	return model.PictureTaskResult{
		ResultURL:             resultURL,
		UserShowImageURL:      thumbnailURL,
		Width:                 width,
		Height:                height,
		AspectRatio:           aspectRatio,
		VideoFrameURL:         thumbnailURL,
		VideoFrameAspectRatio: aspectRatio,
	}, nil
}

func (e *MinimaxTemplateVideoExecutor) IsSubmissionErrorRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// gRPC和网络相关错误可重试
	retryableErrors := []string{
		"timeout",
		"connection",
		"network",
		"dial",
		"context deadline",
		"temporarily unavailable",
		"service unavailable",
		"unavailable",
		"rpc error",
		"提交任务到AIGC Core失败",
	}

	for _, retryable := range retryableErrors {
		if contains(errStr, retryable) {
			return true
		}
	}

	// 非重试错误
	nonRetryableErrors := []string{
		"未提供template_id或template_name参数",
		"获取模板ID失败",
		"获取Minimax模板ID失败",
		"未知的模板名",
		"解析Minimax模板视频配置失败",
		"序列化Minimax模板视频参数失败",
		"authentication",
		"unauthorized",
		"forbidden",
	}

	for _, nonRetryable := range nonRetryableErrors {
		if contains(errStr, nonRetryable) {
			return false
		}
	}

	// 默认可重试
	return true
}

func (e *MinimaxTemplateVideoExecutor) RetrySubmission(ctx context.Context, task *model.PictureTask) error {
	taskParams := map[string]string{
		"task_type":     constants.MinimaxApiTypeTemplateVideo,
		"provider_hint": "minimax",
	}

	// 从任务的ParamJSON中恢复参数
	if task.ParamJSON != "" {
		var inputParams map[string]string
		if err := json.Unmarshal([]byte(task.ParamJSON), &inputParams); err == nil {
			for key, value := range inputParams {
				taskParams[key] = value
			}
		}
	}

	return e.Process(ctx, task.TaskID, taskParams)
}

func (e *MinimaxTemplateVideoExecutor) handleSubmissionError(ctx context.Context, task *model.PictureTask, err error, retryable bool) error {
	zlog.LogWithContext(ctx).Error("Minimax模板视频任务提交失败",
		zap.String("taskID", task.TaskID),
		zap.Bool("retryable", retryable),
		zap.Error(err))

	if e.failureHandler != nil && !retryable {
		if failErr := e.failureHandler.FailTask(ctx, task, err.Error()); failErr != nil {
			zlog.LogWithContext(ctx).Error("标记任务失败出错",
				zap.String("taskID", task.TaskID),
				zap.Error(failErr))
		}
	}

	return err
}

// contains 辅助函数，检查字符串是否包含子字符串（不区分大小写）
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
