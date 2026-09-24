package picture_generate

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	aigcv1 "va_visionai_server/internal/aibrain/v1"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/ai_clients/aigc_core"
	"va_visionai_server/internal/utils"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type GeminiExecutor struct {
	aigcClient      *aigc_core.Client
	taskDao         *dao.PictureTaskDao
	pictureForgeDao *dao.PictureForgeDao
	rdb             *redis.Client
	failureHandler  TaskFailureHandler
}

func NewGeminiExecutor(
	aigcClient *aigc_core.Client,
	taskDao *dao.PictureTaskDao,
	pictureForgeDao *dao.PictureForgeDao,
	rdb *redis.Client,
) *GeminiExecutor {
	return &GeminiExecutor{
		aigcClient:      aigcClient,
		taskDao:         taskDao,
		pictureForgeDao: pictureForgeDao,
		rdb:             rdb,
	}
}

func (e *GeminiExecutor) SetFailureHandler(handler TaskFailureHandler) {
	e.failureHandler = handler
}

func (e *GeminiExecutor) GetName() string {
	return constants.GeminiExecutorName
}

func (e *GeminiExecutor) Match(params map[string]string) bool {
	if provider, exists := params["provider"]; exists {
		return provider == constants.GeminiExecutorName
	}
	return false
}

func (e *GeminiExecutor) Process(ctx context.Context, taskID string, params map[string]string) error {
	task, err := e.taskDao.GetTask(ctx, taskID)
	if err != nil {
		return errors.Wrap(err, "获取任务失败")
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		zlog.LogWithContext(ctx).Info("Gemini任务已完成或失败，跳过处理",
			zap.String("taskID", taskID),
			zap.Int32("status", task.Status))
		return nil
	}

	task.Executer = constants.GeminiExecutorName
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
		ProviderHint:   "gemini",
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

	zlog.LogWithContext(ctx).Info("Gemini任务提交成功",
		zap.String("taskID", taskID),
		zap.String("aigcTaskID", resp.GetTaskId()),
		zap.String("clientID", clientID),
		zap.String("taskType", taskType))

	return nil
}

func (e *GeminiExecutor) SyncProviderStatus(ctx context.Context, task *model.PictureTask) (pb.WorkflowTaskStatus, error) {
	if task.Executer != constants.GeminiExecutorName {
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

		// AIGC Core 的进度已足够平滑，直接采用；它不上报（0）时保持当前值不动。
		if aigc := resp.GetProgress(); aigc > 0 {
			newProgress := mapAigcProgress(aigc)
			if newProgress > task.Progress {
				if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, newProgress, 0); err != nil {
					zlog.LogWithContext(ctx).Error("更新Gemini进度失败", zap.Error(err))
				}
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

func (e *GeminiExecutor) ProcessSuccessfulResult(ctx context.Context, task *model.PictureTask) error {
	if task.ExecuterTaskID == "" {
		return errors.New("AIGC任务ID为空，无法处理结果")
	}

	getTaskReq := &aigcv1.GetAsyncTaskRequest{
		TaskId: task.ExecuterTaskID,
	}

	resp, err := e.aigcClient.GetAsyncTask(context.Background(), getTaskReq)
	if err != nil {
		return errors.Wrap(err, "获取AIGC任务结果失败")
	}

	if resp.GetStatus() != aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_COMPLETED {
		return errors.New("任务未完成，无法处理结果")
	}

	if resp.GetResult() == nil {
		return errors.New("任务结果为空")
	}

	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, constants.ProgressDownloading, 0); err != nil {
		zlog.LogWithContext(ctx).Error("更新下载开始进度失败", zap.Error(err))
	}
	ramp := startTailRamp(e.taskDao, task)
	defer ramp.Stop()

	var resultData model.PictureTaskResult

	switch result := resp.GetResult().GetResult().(type) {
	case *aigcv1.TaskResult_ImageResult:
		resultData, err = e.processGeminiImageResult(ctx, task, result.ImageResult)
	default:
		return errors.New("不支持的结果类型")
	}

	if err != nil {
		return errors.Wrap(err, "处理任务结果失败")
	}

	ramp.Stop()
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, constants.ProgressFinalizing, 0); err != nil {
		zlog.LogWithContext(ctx).Error("更新上传完成进度失败", zap.Error(err))
	}

	resultJSON, err := json.Marshal(resultData)
	if err != nil {
		return errors.Wrap(err, "序列化结果失败")
	}

	task.ResultJSON = string(resultJSON)
	task.UnreadTaskResult = true
	task.CompletedAt = sql.NullTime{Time: time.Now(), Valid: true}
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 100, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED); err != nil {
		return errors.Wrap(err, "更新最终任务结果失败")
	}

	zlog.LogWithContext(ctx).Info("Gemini任务处理成功",
		zap.String("taskID", task.TaskID),
		zap.String("aigcTaskID", task.ExecuterTaskID))

	return nil
}

func (e *GeminiExecutor) RetrySubmission(ctx context.Context, task *model.PictureTask) error {
	if task.Executer != constants.GeminiExecutorName {
		return errors.New("任务执行器不匹配，无法重试")
	}

	var taskParams map[string]string
	if err := json.Unmarshal([]byte(task.ParamJSON), &taskParams); err != nil {
		return errors.Wrap(err, "解析任务参数失败")
	}

	taskParams["provider"] = constants.GeminiExecutorName

	return e.Process(ctx, task.TaskID, taskParams)
}

func (e *GeminiExecutor) IsSubmissionErrorRetryable(err error) bool {
	return false
}

func (e *GeminiExecutor) processImageParameters(ctx context.Context, params map[string]string) (map[string]string, error) {
	processedParams := make(map[string]string)

	for key, value := range params {
		if strings.HasPrefix(value, "http") {
			zlog.LogWithContext(ctx).Debug("检测到图片URL，准备转换为base64",
				zap.String("param_key", key),
				zap.String("image_url", value))

			base64Data, err := utils.URLToBase64(value)
			if err != nil {
				zlog.LogWithContext(ctx).Warn("图片URL转base64失败，使用原始URL",
					zap.String("param_key", key),
					zap.String("image_url", value),
					zap.Error(err))
				processedParams[key] = value
			} else {
				processedParams[key] = base64Data
				zlog.LogWithContext(ctx).Info("图片URL转base64成功",
					zap.String("param_key", key),
					zap.String("original_url", value))
			}
		} else {
			processedParams[key] = value
		}
	}

	return processedParams, nil
}

func (e *GeminiExecutor) buildGeminiParameters(ctx context.Context, task *model.PictureTask, params map[string]string) (map[string]interface{}, error) {
	model := e.getModelFromApiConfig(task)
	prompt := params["prompt"]

	if val, ok := params["InputPrompt"]; ok {
		if val != "" {
			prompt = val
		}
	}

	requestParams := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
	}

	if imageURLs := collectOrderedImageURLs(params); len(imageURLs) > 0 {
		requestParams["image_urls"] = imageURLs
	}

	// 处理可选的 aspect_ratio 参数
	// 优先使用显式传入的 aspect_ratio，其次使用 resolution
	if aspectRatio, ok := params["aspect_ratio"]; ok && aspectRatio != "" {
		requestParams["aspect_ratio"] = aspectRatio
	} else if resolution, ok := params["resolution"]; ok && resolution != "" {
		requestParams["aspect_ratio"] = resolution
	}

	// 处理可选的图片质量参数（对应 Gemini API 的 ImageSize）
	if quality, ok := params["quality"]; ok && quality != "" {
		requestParams["image_size"] = quality
	}

	return requestParams, nil
}

func (e *GeminiExecutor) determineTaskType(params map[string]string) string {
	hasImage := false
	for _, value := range params {
		if strings.HasPrefix(value, "http") || strings.HasPrefix(value, "data:image") {
			hasImage = true
			break
		}
	}

	if hasImage {
		return "image_to_image"
	}
	return "text_to_image"
}

func (e *GeminiExecutor) processGeminiImageResult(ctx context.Context, task *model.PictureTask, imageResult *aigcv1.ImageResult) (model.PictureTaskResult, error) {
	if len(imageResult.GetImageUrls()) == 0 {
		return model.PictureTaskResult{}, errors.New("图片结果为空")
	}

	imageURL := imageResult.GetImageUrls()[0]

	width := int(imageResult.GetWidth())
	height := int(imageResult.GetHeight())

	resultURL := imageURL
	userShowImageURL := imageURL

	data, err := utils.DownloadFile(imageURL)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("下载图片失败，使用原始URL",
			zap.String("image_url", imageURL),
			zap.Error(err))
	} else {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			zlog.LogWithContext(ctx).Warn("解码图片失败，使用原始URL",
				zap.String("image_url", imageURL),
				zap.Error(err))
		} else {
			if width == 0 || height == 0 {
				bounds := img.Bounds()
				width = bounds.Dx()
				height = bounds.Dy()
			}

			webpData, err := utils.CompressImageWithMaxSize(img, 512, 0)
			if err != nil {
				zlog.LogWithContext(ctx).Warn("生成WebP缩略图失败",
					zap.String("image_url", imageURL),
					zap.Error(err))
			} else {
				webpFileName := GetGeneratorName(task, "webp")
				webpURL, err := utils.UploadToOSS(webpFileName, webpData)
				if err != nil {
					zlog.LogWithContext(ctx).Warn("上传WebP缩略图失败",
						zap.String("image_url", imageURL),
						zap.Error(err))
				} else {
					userShowImageURL = webpURL
				}
			}
		}
	}

	if width == 0 || height == 0 {
		width = 1024
		height = 1024
	}

	aspectRatio := float64(height) / float64(width)
	aspectRatioText := utils.GetStandardAspectRatio(width, height)

	return model.PictureTaskResult{
		ResultURL:        resultURL,
		UserShowImageURL: userShowImageURL,
		Width:            width,
		Height:           height,
		AspectRatio:      aspectRatio,
		AspectRatioText:  aspectRatioText,
	}, nil
}

func (e *GeminiExecutor) handleSubmissionError(ctx context.Context, task *model.PictureTask, err error, retryable bool) error {
	zlog.LogWithContext(ctx).Error("Gemini任务提交失败",
		zap.String("taskID", task.TaskID),
		zap.Bool("retryable", retryable),
		zap.Error(err))

	if e.failureHandler != nil && !retryable {
		reason, fullReason := constants.GetGenericErrorInfo(err.Error())
		task.FailedReasonFull = fullReason
		if failErr := e.failureHandler.FailTaskWithReason(ctx, task, err.Error(), reason); failErr != nil {
			zlog.LogWithContext(ctx).Error("标记任务失败出错",
				zap.String("taskID", task.TaskID),
				zap.Error(failErr))
		}
	}

	return err
}

// getModel 获取模型名称
func (e *GeminiExecutor) getModelFromApiConfig(task *model.PictureTask) string {
	if task.ApiConfig.ModelName != "" {
		return task.ApiConfig.ModelName
	}
	return "gemini-2.5-flash-image-preview"
}
