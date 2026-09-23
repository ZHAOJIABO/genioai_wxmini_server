package picture_generate

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/ai_clients/aigc_core"
	"va_visionai_server/internal/utils"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
	aigcv1 "va_visionai_server/internal/aibrain/v1"
)

type CloudComfyExecutor struct {
	aigcClient      *aigc_core.Client
	taskDao         *dao.PictureTaskDao
	pictureForgeDao *dao.PictureForgeDao
	rdb             *redis.Client
	failureHandler  TaskFailureHandler
}

func NewCloudComfyExecutor(
	aigcClient *aigc_core.Client,
	taskDao *dao.PictureTaskDao,
	pictureForgeDao *dao.PictureForgeDao,
	rdb *redis.Client,
) *CloudComfyExecutor {
	return &CloudComfyExecutor{
		aigcClient:      aigcClient,
		taskDao:         taskDao,
		pictureForgeDao: pictureForgeDao,
		rdb:             rdb,
	}
}

func (e *CloudComfyExecutor) SetFailureHandler(handler TaskFailureHandler) {
	e.failureHandler = handler
}

func (e *CloudComfyExecutor) GetName() string {
	return constants.CloudComfyExecutorName
}

func (e *CloudComfyExecutor) Match(params map[string]string) bool {
	if provider, exists := params["provider"]; exists {
		return provider == constants.CloudComfyExecutorName
	}
	return false
}

func (e *CloudComfyExecutor) Process(ctx context.Context, taskID string, params map[string]string) error {
	task, err := e.taskDao.GetTask(ctx, taskID)
	if err != nil {
		return errors.Wrap(err, "获取任务失败")
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		zlog.LogWithContext(ctx).Info("Cloud Comfy任务已完成或失败，跳过处理",
			zap.String("taskID", taskID),
			zap.Int32("status", task.Status))
		return nil
	}

	task.Executer = constants.CloudComfyExecutorName
	task.RetryCount = 0
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, constants.ProgressSubmitted, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		return errors.Wrap(err, "更新任务初始状态失败")
	}

	workflowJson, err := e.getWorkflowJson(ctx, task)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, false)
	}

	processedParams, err := e.processImageUploads(ctx, params)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "处理图片上传失败"), true)
	}

	clientID := uuid.New().String()
	parametersJson, err := e.buildParametersJson(workflowJson, clientID, processedParams)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, false)
	}

	taskType := params["task_type"]
	if taskType == "" {
		taskType = "image_to_image"
	}

	submitReq := &aigcv1.SubmitAsyncTaskRequest{
		TaskType:       taskType,
		Model:          "default",
		ProviderHint:   "cloud_comfy",
		ParametersJson: parametersJson,
	}

	resp, err := e.aigcClient.SubmitAsyncTask(ctx, submitReq)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "提交任务到AIGC Core失败"), true)
	}

	task.ExecuterTaskID = resp.GetTaskId()
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, constants.ProgressProviderProcessing, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION); err != nil {
		return errors.Wrap(err, "保存AIGC任务ID失败")
	}

	zlog.LogWithContext(ctx).Info("Cloud Comfy任务提交成功",
		zap.String("taskID", taskID),
		zap.String("aigcTaskID", resp.GetTaskId()),
		zap.String("clientID", clientID))

	return nil
}

func (e *CloudComfyExecutor) SyncProviderStatus(ctx context.Context, task *model.PictureTask) (pb.WorkflowTaskStatus, error) {
	if task.Executer != constants.CloudComfyExecutorName {
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
					zlog.LogWithContext(ctx).Error("更新Cloud Comfy进度失败", zap.Error(err))
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
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.New("Cloud Comfy Task Failed")

	default:
		zlog.LogWithContext(ctx).Warn("未知的AIGC任务状态",
			zap.String("taskID", task.TaskID),
			zap.String("status", resp.GetStatus().String()))
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil
	}
}

func (e *CloudComfyExecutor) ProcessSuccessfulResult(ctx context.Context, task *model.PictureTask) error {
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
	var err2 error

	switch result := resp.GetResult().GetResult().(type) {
	case *aigcv1.TaskResult_ImageResult:
		resultData, err2 = e.processImageResult(ctx, task, result.ImageResult)
	case *aigcv1.TaskResult_VideoResult:
		resultData, err2 = e.processVideoResult(ctx, task, result.VideoResult)
	default:
		return errors.New("不支持的结果类型")
	}

	if err2 != nil {
		return errors.Wrap(err2, "处理任务结果失败")
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
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, constants.ProgressCompleted, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED); err != nil {
		return errors.Wrap(err, "更新最终任务结果失败")
	}

	zlog.LogWithContext(ctx).Info("Cloud Comfy任务处理成功",
		zap.String("taskID", task.TaskID),
		zap.String("aigcTaskID", task.ExecuterTaskID))

	return nil
}

func (e *CloudComfyExecutor) RetrySubmission(ctx context.Context, task *model.PictureTask) error {
	if task.Executer != constants.CloudComfyExecutorName {
		return errors.New("任务执行器不匹配，无法重试")
	}

	var taskParams map[string]string
	if err := json.Unmarshal([]byte(task.ParamJSON), &taskParams); err != nil {
		return errors.Wrap(err, "解析任务参数失败")
	}

	taskParams["provider"] = constants.CloudComfyExecutorName

	return e.Process(ctx, task.TaskID, taskParams)
}

func (e *CloudComfyExecutor) IsSubmissionErrorRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

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
		if strings.Contains(errStr, retryable) {
			return true
		}
	}

	nonRetryableErrors := []string{
		"工作流json为空",
		"获取工作流信息失败",
		"构建参数json失败",
		"invalid workflow",
		"authentication",
		"unauthorized",
		"forbidden",
	}

	for _, nonRetryable := range nonRetryableErrors {
		if strings.Contains(errStr, nonRetryable) {
			return false
		}
	}

	return true
}

func (e *CloudComfyExecutor) getWorkflowJson(ctx context.Context, task *model.PictureTask) (string, error) {
	workflow, err := e.pictureForgeDao.GetWorkflow(ctx, db.GetDB(), task.WorkflowID)
	if err != nil {
		return "", errors.Wrap(err, "获取工作流信息失败")
	}

	if workflow.WorkflowJson == "" {
		return "", errors.New("工作流JSON为空")
	}

	return workflow.WorkflowJson, nil
}

func (e *CloudComfyExecutor) buildParametersJson(workflowJson, clientID string, processedParams map[string]string) (string, error) {
	randomSeed := utils.GenerateRandomNumber(13)
	workflowJson = strings.ReplaceAll(workflowJson, "{random_seed}", strconv.FormatInt(randomSeed, 10))

	workflowJson, err := e.safeReplacePlaceholders(workflowJson, processedParams)
	if err != nil {
		return "", errors.Wrap(err, "替换参数占位符失败")
	}
	var tempMap map[string]interface{}
	if err := json.Unmarshal([]byte(workflowJson), &tempMap); err != nil {
		return "", errors.Wrap(err, "替换参数后workflowJson不是有效的JSON")
	}

	params := map[string]interface{}{
		"prompt":    json.RawMessage(workflowJson),
		"client_id": clientID,
	}

	parametersBytes, err := json.Marshal(params)
	if err != nil {
		return "", errors.Wrap(err, "构建参数JSON失败")
	}

	return string(parametersBytes), nil
}

func (e *CloudComfyExecutor) safeReplacePlaceholders(workflowJson string, processedParams map[string]string) (string, error) {
	result := workflowJson

	for key, value := range processedParams {
		placeholder := "{" + key + "}"

		escapedValue := strings.ReplaceAll(value, "\\", "\\\\")
		escapedValue = strings.ReplaceAll(escapedValue, "\"", "\\\"")
		escapedValue = strings.ReplaceAll(escapedValue, "\n", "\\n")
		escapedValue = strings.ReplaceAll(escapedValue, "\r", "\\r")
		escapedValue = strings.ReplaceAll(escapedValue, "\t", "\\t")

		result = strings.ReplaceAll(result, placeholder, escapedValue)
	}

	return result, nil
}

func (e *CloudComfyExecutor) handleSubmissionError(ctx context.Context, task *model.PictureTask, err error, retryable bool) error {
	zlog.LogWithContext(ctx).Error("Cloud Comfy任务提交失败",
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

func (e *CloudComfyExecutor) processImageResult(ctx context.Context, task *model.PictureTask, imageResult *aigcv1.ImageResult) (model.PictureTaskResult, error) {
	if len(imageResult.GetImageUrls()) == 0 {
		return model.PictureTaskResult{}, errors.New("图片结果为空")
	}

	imageURL := imageResult.GetImageUrls()[0]

	imageData, err := e.downloadMediaFromURL(ctx, imageURL)
	if err != nil {
		return model.PictureTaskResult{}, errors.Wrap(err, "下载图片失败")
	}

	img, format, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return model.PictureTaskResult{}, errors.Wrap(err, "解码图片失败")
	}

	width := int(imageResult.GetWidth())
	height := int(imageResult.GetHeight())

	if width == 0 || height == 0 {
		bounds := img.Bounds()
		width = bounds.Dx()
		height = bounds.Dy()
	}

	aspectRatio := float64(height) / float64(width)
	fileName := GetGeneratorName(task, format)
	resultURL, err := utils.UploadToOSS(fileName, imageData)
	if err != nil {
		return model.PictureTaskResult{}, errors.Wrap(err, "上传图片到OSS失败")
	}

	var webpURL string
	webpData, err := utils.CompressImageWithMaxSize(img, 512, 0)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("生成WebP缩略图失败", zap.Error(err))
	} else {
		webpFileName := GetGeneratorThumbnailName(task, "webp")
		webpURL, err = utils.UploadToOSS(webpFileName, webpData)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("上传WebP缩略图失败", zap.Error(err))
		}
	}

	aspectRatioText := utils.GetStandardAspectRatio(width, height)

	return model.PictureTaskResult{
		ResultURL:        resultURL,
		UserShowImageURL: webpURL,
		Width:            width,
		Height:           height,
		AspectRatio:      aspectRatio,
		AspectRatioText:  aspectRatioText,
	}, nil
}

func (e *CloudComfyExecutor) processVideoResult(ctx context.Context, task *model.PictureTask, videoResult *aigcv1.VideoResult) (model.PictureTaskResult, error) {
	if videoResult.GetVideoUrl() == "" {
		return model.PictureTaskResult{}, errors.New("视频结果为空")
	}

	videoData, err := e.downloadMediaFromURL(ctx, videoResult.GetVideoUrl())
	if err != nil {
		return model.PictureTaskResult{}, errors.Wrap(err, "下载视频失败")
	}

	// fileName := fmt.Sprintf("cloud_comfy_%s.mp4", task.TaskID)
	// resultURL, err := utils.UploadToOSS(fileName, videoData)
	// if err != nil {
	// 	return model.PictureTaskResult{}, errors.Wrap(err, "上传视频到OSS失败")
	// }

	var thumbnailURL string
	thumbnailData, err := utils.ExtractFrameFromVideoBytesWithFFmpeg(videoData)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("提取视频缩略图失败", zap.Error(err))
	} else {
		thumbnailFileName := fmt.Sprintf("aigc/cloud_comfy_%s_thumbnail.jpg", task.TaskID)
		thumbnailURL, err = utils.UploadToOSS(thumbnailFileName, thumbnailData)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("上传视频缩略图失败", zap.Error(err))
		}
	}
	img, _, err := image.Decode(bytes.NewReader(thumbnailData))
	if err != nil {
		return model.PictureTaskResult{}, errors.Wrap(err, "解码视频缩略图失败")
	}
	width, height, aspectRatio := utils.ImageAspectRatio(img)
	aspectRatioText := utils.GetStandardAspectRatio(width, height)

	return model.PictureTaskResult{
		ResultURL:             videoResult.GetVideoUrl(),
		UserShowImageURL:      thumbnailURL,
		Width:                 width,
		Height:                height,
		AspectRatio:           aspectRatio,
		AspectRatioText:       aspectRatioText,
		VideoFrameURL:         thumbnailURL,
		VideoFrameAspectRatio: aspectRatio,
	}, nil
}

func (e *CloudComfyExecutor) downloadMediaFromURL(ctx context.Context, url string) ([]byte, error) {
	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		data, err := utils.DownloadFile(url)
		if err == nil {
			return data, nil
		}

		lastErr = err
		if i < maxRetries-1 {
			zlog.LogWithContext(ctx).Warn("下载媒体文件失败，重试中",
				zap.String("url", url),
				zap.Int("attempt", i+1),
				zap.Error(err))
		}
	}

	return nil, errors.Wrapf(lastErr, "下载媒体文件失败，已重试%d次", maxRetries)
}

func (e *CloudComfyExecutor) processImageUploads(ctx context.Context, params map[string]string) (map[string]string, error) {
	processedParams := make(map[string]string)

	for key, value := range params {
		if utils.IsImageURL(value) {
			zlog.LogWithContext(ctx).Debug("检测到图片URL，准备上传",
				zap.String("param_key", key),
				zap.String("image_url", value))

			uploadedFilename, err := e.uploadImageToAIGC(ctx, value)
			if err != nil {
				zlog.LogWithContext(ctx).Warn("图片上传失败，使用原始URL",
					zap.String("param_key", key),
					zap.String("image_url", value),
					zap.Error(err))
				processedParams[key] = value
			} else {
				processedParams[key] = uploadedFilename
				zlog.LogWithContext(ctx).Info("图片上传成功",
					zap.String("param_key", key),
					zap.String("original_url", value),
					zap.String("uploaded_filename", uploadedFilename))
			}
		} else {
			processedParams[key] = value
		}
	}

	return processedParams, nil
}

func (e *CloudComfyExecutor) hasImageUploads(original, processed map[string]string) bool {
	for key, originalValue := range original {
		if processedValue, exists := processed[key]; exists {
			if utils.IsImageURL(originalValue) && originalValue != processedValue {
				return true
			}
		}
	}
	return false
}

func (e *CloudComfyExecutor) uploadImageToAIGC(ctx context.Context, photoURL string) (string, error) {
	filename := uuid.New().String() + ".jpg"

	uploadReq := &aigcv1.UploadComfyUIInputFileRequest{
		FileName: filename,
		Url:      photoURL,
		FileType: aigcv1.InputFileType_INPUT_FILE_TYPE_IMAGE,
	}

	resp, err := e.aigcClient.UploadComfyUIInputFile(ctx, uploadReq)
	if err != nil {
		return "", errors.Wrap(err, "上传图片到AIGC Core失败")
	}

	uploadedFilename := resp.GetFileName()
	if uploadedFilename == "" {
		return "", errors.New("AIGC Core返回的文件名为空")
	}

	return uploadedFilename, nil
}
