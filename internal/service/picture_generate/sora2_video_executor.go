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

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/ai_clients/aigc_core"
	"va_visionai_server/internal/utils"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
	aigcv1 "va_visionai_server/internal/aibrain/v1"
)

// Sora2Executor Sora2视频生成执行器
type Sora2Executor struct {
	aigcClient     *aigc_core.Client
	taskDao        *dao.PictureTaskDao
	rdb            *redis.Client
	failureHandler TaskFailureHandler
}

// NewSora2Executor 创建Sora2执行器
func NewSora2Executor(
	aigcClient *aigc_core.Client,
	taskDao *dao.PictureTaskDao,
	rdb *redis.Client,
) *Sora2Executor {
	return &Sora2Executor{
		aigcClient: aigcClient,
		taskDao:    taskDao,
		rdb:        rdb,
	}
}

// SetFailureHandler 设置失败处理器
func (e *Sora2Executor) SetFailureHandler(handler TaskFailureHandler) {
	e.failureHandler = handler
}

// GetName 获取执行器名称
func (e *Sora2Executor) GetName() string {
	return constants.SoraExecutorName
}

// Match 匹配执行器
func (e *Sora2Executor) Match(params map[string]string) bool {
	if provider, exists := params["provider"]; exists {
		return provider == constants.SoraExecutorName
	}
	return false
}

// Process 处理任务
func (e *Sora2Executor) Process(ctx context.Context, taskID string, params map[string]string) error {
	// 1. 获取任务
	task, err := e.taskDao.GetTask(ctx, taskID)
	if err != nil {
		return errors.Wrap(err, "获取任务失败")
	}

	// 2. 检查任务状态
	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		zlog.LogWithContext(ctx).Info("Sora2任务已完成或失败，跳过处理",
			zap.String("taskID", taskID),
			zap.Int32("status", task.Status))
		return nil
	}

	// 3. 更新任务状态为处理中
	task.Executer = constants.SoraExecutorName
	task.RetryCount = 0
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, constants.ProgressSubmitted, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		return errors.Wrap(err, "更新任务初始状态失败")
	}

	// 4. 构建请求参数
	requestParams, err := e.buildSoraParameters(ctx, task, params)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, false)
	}

	// 5. 确定任务类型
	taskType := e.determineTaskType(params)

	// 6. 构建AIGC请求
	parametersJson, err := json.Marshal(requestParams)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "序列化参数失败"), false)
	}

	submitReq := &aigcv1.SubmitAsyncTaskRequest{
		TaskType:       taskType,
		Model:          e.getModel(params),
		ProviderHint:   "sora",
		ParametersJson: string(parametersJson),
	}

	// 7. 提交任务到AIGC Core
	resp, err := e.aigcClient.SubmitAsyncTask(ctx, submitReq)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, e.IsSubmissionErrorRetryable(err))
	}

	// 8. 保存AIGC任务ID
	task.ExecuterTaskID = resp.GetTaskId()
	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		return errors.Wrap(err, "保存AIGC任务ID失败")
	}

	// 9. 更新进度
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, constants.ProgressProviderProcessing, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION); err != nil {
		return errors.Wrap(err, "更新任务进度失败")
	}

	zlog.LogWithContext(ctx).Info("Sora2任务提交成功",
		zap.String("taskID", taskID),
		zap.String("aigcTaskID", task.ExecuterTaskID),
		zap.String("taskType", taskType))

	return nil
}

// SyncProviderStatus 同步提供商状态
func (e *Sora2Executor) SyncProviderStatus(ctx context.Context, task *model.PictureTask) (pb.WorkflowTaskStatus, error) {
	if task.ExecuterTaskID == "" {
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.New("AIGC任务ID为空")
	}

	// 获取AIGC任务状态
	resp, err := e.aigcClient.GetAsyncTask(ctx, &aigcv1.GetAsyncTaskRequest{
		TaskId: task.ExecuterTaskID,
	})
	if err != nil {
		// 对齐其他执行器策略：查询失败不直接判定失败，维持等待状态
		zlog.LogWithContext(ctx).Debug("获取AIGC任务状态失败，维持等待",
			zap.String("taskID", task.TaskID),
			zap.String("aigcTaskID", task.ExecuterTaskID),
			zap.Error(err))
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil
	}

	// 根据AIGC状态更新任务状态
	switch resp.GetStatus() {
	case aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_COMPLETED:
		zlog.LogWithContext(ctx).Info("AIGC任务已完成",
			zap.String("taskID", task.TaskID),
			zap.String("aigcTaskID", task.ExecuterTaskID))
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED, nil

	case aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_FAILED:
		errorMsg := resp.GetErrorMessage()
		if errorMsg == "" {
			errorMsg = "AIGC任务执行失败"
		}
		zlog.LogWithContext(ctx).Error("AIGC任务失败",
			zap.String("taskID", task.TaskID),
			zap.String("aigcTaskID", task.ExecuterTaskID),
			zap.String("error", errorMsg))

		reason, fullReason := constants.GetGenericErrorInfo(errorMsg)
		task.FailedReason = reason
		task.FailedReasonFull = fullReason
		if err := e.taskDao.UpdateTask(ctx, task); err != nil {
			zlog.LogWithContext(ctx).Error("更新失败原因失败", zap.Error(err))
		}

		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.New(errorMsg)

	case aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_PENDING,
		aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_PROCESSING,
		aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_AWAITING_PROVIDER,
		aigcv1.AsyncTaskStatus_ASYNC_TASK_STATUS_PENDING_RETRY:
		// 模拟进度增长
		if task.Progress < constants.ProgressProviderMaxSimulated {
			increment := int32(3 + (task.Progress % 3)) // 3-5%递增
			newProgress := task.Progress + increment
			if newProgress > constants.ProgressProviderMaxSimulated {
				newProgress = constants.ProgressProviderMaxSimulated
			}
			task.Progress = newProgress
			if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, newProgress, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION); err != nil {
				zlog.LogWithContext(ctx).Error("更新任务进度失败", zap.Error(err))
			}
		}
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil

	default:
		zlog.LogWithContext(ctx).Warn("未知的AIGC任务状态",
			zap.String("taskID", task.TaskID),
			zap.String("aigcTaskID", task.ExecuterTaskID),
			zap.String("status", resp.GetStatus().String()))
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil
	}
}

// ProcessSuccessfulResult 处理成功结果
func (e *Sora2Executor) ProcessSuccessfulResult(ctx context.Context, task *model.PictureTask) error {
	// 获取AIGC任务结果
	resp, err := e.aigcClient.GetAsyncTask(ctx, &aigcv1.GetAsyncTaskRequest{
		TaskId: task.ExecuterTaskID,
	})
	if err != nil {
		return errors.Wrap(err, "获取AIGC任务结果失败")
	}

	// 更新进度到下载阶段
	task.Progress = constants.ProgressDownloading
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, constants.ProgressDownloading, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		zlog.LogWithContext(ctx).Error("更新下载进度失败", zap.Error(err))
	}

	// 处理视频结果
	var resultData model.PictureTaskResult
	switch result := resp.GetResult().GetResult().(type) {
	case *aigcv1.TaskResult_VideoResult:
		resultData, err = e.processSoraVideoResult(ctx, task, result.VideoResult)
		if err != nil {
			return errors.Wrap(err, "处理视频结果失败")
		}
	default:
		return errors.New("不支持的结果类型")
	}

	// 更新进度到上传完成
	task.Progress = constants.ProgressUploading
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, constants.ProgressUploading, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		zlog.LogWithContext(ctx).Error("更新上传进度失败", zap.Error(err))
	}

	// 序列化结果
	resultJson, err := json.Marshal(resultData)
	if err != nil {
		return errors.Wrap(err, "序列化结果失败")
	}

	// 保存结果并发送完成事件（原子更新）
	task.ResultJSON = string(resultJson)
	task.UnreadTaskResult = true
	task.CompletedAt = sql.NullTime{Time: time.Now(), Valid: true}
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, constants.ProgressCompleted, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED); err != nil {
		zlog.LogWithContext(ctx).Error("发送完成事件失败", zap.Error(err))
	}

	zlog.LogWithContext(ctx).Info("Sora2任务处理成功",
		zap.String("taskID", task.TaskID),
		zap.String("aigcTaskID", task.ExecuterTaskID))

	return nil
}

// RetrySubmission 重试提交（幂等实现）
func (e *Sora2Executor) RetrySubmission(ctx context.Context, task *model.PictureTask) error {
	// 从task.ParamJSON恢复参数
	var params map[string]string
	if err := json.Unmarshal([]byte(task.ParamJSON), &params); err != nil {
		return errors.Wrap(err, "解析任务参数失败")
	}

	// 强制设置provider为sora2
	params["provider"] = constants.SoraExecutorName

	// 调用Process重新处理
	return e.Process(ctx, task.TaskID, params)
}

// IsSubmissionErrorRetryable 判断错误是否可重试
func (e *Sora2Executor) IsSubmissionErrorRetryable(err error) bool {
	return true
}

// buildSoraParameters 构建Sora请求参数
func (e *Sora2Executor) buildSoraParameters(ctx context.Context, task *model.PictureTask, params map[string]string) (map[string]interface{}, error) {
	requestParams := make(map[string]interface{})

	// 模型参数
	requestParams["model"] = e.getModel(params)

	// 文本提示词
	if prompt, exists := params["prompt"]; exists && prompt != "" {
		requestParams["prompt"] = prompt
	} else if prompt, exists := params["InputPrompt"]; exists && prompt != "" {
		requestParams["prompt"] = prompt
	}

	// 视频时长：支持别名 Seconds/seconds
	if duration, exists := params["duration"]; exists && duration != "" {
		requestParams["duration"] = duration
	} else if seconds, exists := params["Seconds"]; exists && seconds != "" {
		requestParams["duration"] = seconds
	} else if seconds, exists := params["seconds"]; exists && seconds != "" {
		requestParams["duration"] = seconds
	}

	// 视频分辨率：支持别名 Size/size，容忍×或x符号，并支持portrait/landscape映射
	normalizeResolution := func(res string) string {
		r := strings.TrimSpace(res)
		if r == "" {
			return r
		}
		lower := strings.ToLower(r)
		if lower == "portrait" {
			return "720x1280"
		}
		if lower == "landscape" {
			return "1280x720"
		}
		// 统一使用x
		r = strings.ReplaceAll(r, "×", "x")
		return r
	}
	if resolution, exists := params["resolution"]; exists && resolution != "" {
		requestParams["resolution"] = normalizeResolution(resolution)
	} else if size, exists := params["Size"]; exists && size != "" {
		requestParams["resolution"] = normalizeResolution(size)
	} else if size, exists := params["size"]; exists && size != "" {
		requestParams["resolution"] = normalizeResolution(size)
	}

	// 图生视频参数：优先通过AIGC Core上传托管，失败回退为原始URL
	if imageURL, exists := params["image"]; exists && imageURL != "" {
		if utils.IsImageURL(imageURL) {
			filename := uuid.New().String() + ".jpg"
			uploadReq := &aigcv1.UploadComfyUIInputFileRequest{
				FileName: filename,
				Url:      imageURL,
				FileType: aigcv1.InputFileType_INPUT_FILE_TYPE_IMAGE,
			}
			resp, err := e.aigcClient.UploadComfyUIInputFile(ctx, uploadReq)
			if err != nil {
				zlog.LogWithContext(ctx).Warn("图片上传到AIGC Core失败，使用原始URL",
					zap.String("image_url", imageURL),
					zap.Error(err))
				requestParams["input_image_url"] = imageURL
			} else if resp.GetFileName() == "" {
				zlog.LogWithContext(ctx).Warn("AIGC Core返回空文件名，使用原始URL",
					zap.String("image_url", imageURL))
				requestParams["input_image_url"] = imageURL
			} else {
				requestParams["input_image_url"] = resp.GetFileName()
			}
		} else {
			requestParams["input_image_url"] = imageURL
		}
	}

	// 支持多图输入（如首尾帧）
	var imageList []string
	for key, value := range params {
		if strings.HasPrefix(key, "LoadImage") && value != "" {
			imageList = append(imageList, value)
		}
	}
	if len(imageList) > 0 {
		requestParams["image_urls"] = imageList
	}

	// 默认值：如果未提供则应用默认
	if _, ok := requestParams["resolution"]; !ok {
		requestParams["resolution"] = "720x1280" // Portrait 默认
	}
	if _, ok := requestParams["duration"]; !ok {
		requestParams["duration"] = "4" // 默认 4 秒
	}

	return requestParams, nil
}

// determineTaskType 确定任务类型
func (e *Sora2Executor) determineTaskType(params map[string]string) string {
	// 检查是否存在图片参数
	hasImage := false
	for key, value := range params {
		if (key == "image" || strings.HasPrefix(key, "LoadImage")) && value != "" {
			hasImage = true
			break
		}
	}

	if hasImage {
		return "image_to_video"
	}
	return "text_to_video"
}

// processSoraVideoResult 处理Sora视频结果
func (e *Sora2Executor) processSoraVideoResult(ctx context.Context, task *model.PictureTask, videoResult *aigcv1.VideoResult) (model.PictureTaskResult, error) {
	videoURL := videoResult.GetVideoUrl()
	if videoURL == "" {
		return model.PictureTaskResult{}, errors.New("视频URL为空")
	}

	// 1. 下载视频（仅用于提取首帧）
	zlog.LogWithContext(ctx).Info("开始下载Sora视频",
		zap.String("taskID", task.TaskID),
		zap.String("videoURL", videoURL))

	videoData, err := utils.DownloadFile(videoURL)
	if err != nil {
		return model.PictureTaskResult{}, errors.Wrap(err, "下载视频失败")
	}

	// 2. 提取首帧
	zlog.LogWithContext(ctx).Info("提取视频首帧",
		zap.String("taskID", task.TaskID))

	frameBytes, err := utils.ExtractFrameFromVideoBytesWithFFmpeg(videoData)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("提取首帧失败",
			zap.String("taskID", task.TaskID),
			zap.Error(err))
		frameBytes = []byte{} // 使用空字节数组兜底
	}

	// 3. 上传首帧到OSS（只上传首帧，不上传视频）
	var frameOssURL string
	if len(frameBytes) > 0 {
		frameOssFileName := GetGeneratorThumbnailName(task, "jpg")
		frameOssURL, err = utils.UploadToOSS(frameOssFileName, frameBytes)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("上传首帧失败",
				zap.String("taskID", task.TaskID),
				zap.Error(err))
		}
	}

	// 4. 获取视频尺寸
	width := int(videoResult.GetWidth())
	height := int(videoResult.GetHeight())
	if width == 0 || height == 0 {
		// 尝试从首帧获取尺寸
		if len(frameBytes) > 0 {
			img, _, err := image.DecodeConfig(bytes.NewReader(frameBytes))
			if err == nil {
				width = img.Width
				height = img.Height
			}
		}
		// 兜底默认值
		if width == 0 || height == 0 {
			width = 1920
			height = 1080
		}
	}

	// 5. 计算宽高比
	aspectRatio := float64(height) / float64(width)
	aspectRatioText := utils.GetStandardAspectRatio(width, height)

	zlog.LogWithContext(ctx).Info("Sora视频处理完成",
		zap.String("taskID", task.TaskID),
		zap.String("videoURL", videoURL),
		zap.String("frameURL", frameOssURL),
		zap.Int("width", width),
		zap.Int("height", height))

	return model.PictureTaskResult{
		ResultURL:             videoURL,    // 直接使用AIGC Core返回的URL
		UserShowImageURL:      frameOssURL, // 首帧缩略图
		VideoFrameURL:         frameOssURL, // 视频首帧
		Width:                 width,
		Height:                height,
		AspectRatio:           aspectRatio,
		AspectRatioText:       aspectRatioText,
		VideoFrameAspectRatio: aspectRatio,
	}, nil
}

// handleSubmissionError 处理提交错误
func (e *Sora2Executor) handleSubmissionError(ctx context.Context, task *model.PictureTask, err error, retryable bool) error {
	zlog.LogWithContext(ctx).Error("Sora2任务提交失败",
		zap.String("taskID", task.TaskID),
		zap.Bool("retryable", retryable),
		zap.Error(err))

	// Sora2不支持重试，直接标记失败
	if e.failureHandler != nil {
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
func (e *Sora2Executor) getModel(params map[string]string) string {
	if model, exists := params["model"]; exists && model != "" {
		return model
	}
	return "sora2"
}
