package picture_generate

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"strconv"
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

// GPTImage2Executor GPT-Image-2 文生图执行器，通过 AIGC Core 调用。
// 复用 GeminiExecutor 的状态同步和结果处理逻辑，仅参数构建不同。
type GPTImage2Executor struct {
	GeminiExecutor
}

var _ TaskExecutor = (*GPTImage2Executor)(nil)

func NewGPTImage2Executor(
	aigcClient *aigc_core.Client,
	taskDao *dao.PictureTaskDao,
	pictureForgeDao *dao.PictureForgeDao,
	rdb *redis.Client,
) *GPTImage2Executor {
	return &GPTImage2Executor{
		GeminiExecutor: GeminiExecutor{
			aigcClient:      aigcClient,
			taskDao:         taskDao,
			pictureForgeDao: pictureForgeDao,
			rdb:             rdb,
		},
	}
}

func (e *GPTImage2Executor) GetName() string {
	return constants.GPTImage2ExecutorName
}

func (e *GPTImage2Executor) Match(params map[string]string) bool {
	if provider, exists := params["provider"]; exists {
		return provider == constants.GPTImage2ExecutorName
	}
	return false
}

func (e *GPTImage2Executor) Process(ctx context.Context, taskID string, params map[string]string) error {
	task, err := e.taskDao.GetTask(ctx, taskID)
	if err != nil {
		return errors.Wrap(err, "获取任务失败")
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		zlog.LogWithContext(ctx).Info("GPT-Image-2任务已完成或失败，跳过处理",
			zap.String("taskID", taskID),
			zap.Int32("status", task.Status))
		return nil
	}

	task.Executer = constants.GPTImage2ExecutorName
	task.RetryCount = 0
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 5, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		return errors.Wrap(err, "更新任务初始状态失败")
	}

	requestParams := e.buildGPTImage2Parameters(ctx, task, params)

	clientID := uuid.New().String()
	parametersJson, err := json.Marshal(requestParams)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "序列化参数失败"), false)
	}

	submitReq := &aigcv1.SubmitAsyncTaskRequest{
		TaskType:       "text_to_image",
		Model:          e.getGPTImage2Model(task),
		ProviderHint:   "gptimage",
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

	zlog.LogWithContext(ctx).Info("GPT-Image-2任务提交成功",
		zap.String("taskID", taskID),
		zap.String("aigcTaskID", resp.GetTaskId()),
		zap.String("clientID", clientID))

	return nil
}

// buildGPTImage2Parameters 构建 GPT-Image-2 文生图请求参数
// 支持字段：prompt, size, quality(low/medium/high/auto)
func (e *GPTImage2Executor) buildGPTImage2Parameters(ctx context.Context, task *model.PictureTask, params map[string]string) map[string]interface{} {
	modelName := e.getGPTImage2Model(task)
	prompt := params["prompt"]
	if val, ok := params["InputPrompt"]; ok && val != "" {
		prompt = val
	}

	requestParams := map[string]interface{}{
		"model":  modelName,
		"prompt": prompt,
	}

	// size 参数，如 1024x1024, 1536x1024 等
	// 优先使用显式的 size，其次使用 resolution，最后用 width+height 拼接
	if size, ok := params["size"]; ok && size != "" {
		requestParams["size"] = size
	} else if resolution, ok := params["resolution"]; ok && resolution != "" {
		requestParams["size"] = resolution
	} else if width, ok := params["width"]; ok && width != "" {
		if height, ok2 := params["height"]; ok2 && height != "" {
			requestParams["size"] = fmt.Sprintf("%sx%s", width, height)
		}
	}

	// quality 参数：强制使用 low，忽略前端传入的值
	requestParams["quality"] = "low"
	// if quality, ok := params["quality"]; ok && quality != "" {
	// 	requestParams["quality"] = quality
	// } else {
	// 	requestParams["quality"] = "auto"
	// }

	// n 参数：生成图片数量
	if n, ok := params["num_images"]; ok && n != "" && n != "1" {
		nInt, _ := strconv.Atoi(n)
		if nInt > 1 {
			requestParams["n"] = nInt
		}
	}

	return requestParams
}

// getGPTImage2Model 获取模型名称
func (e *GPTImage2Executor) getGPTImage2Model(task *model.PictureTask) string {
	if task.ApiConfig.ModelName != "" {
		return task.ApiConfig.ModelName
	}
	return constants.DefaultGPTImageModel
}

// ProcessSuccessfulResult 覆写父类方法，支持多图结果处理
func (e *GPTImage2Executor) ProcessSuccessfulResult(ctx context.Context, task *model.PictureTask) error {
	if task.ExecuterTaskID == "" {
		return errors.New("AIGC任务ID为空，无法处理结果")
	}

	getTaskReq := &aigcv1.GetAsyncTaskRequest{
		TaskId: task.ExecuterTaskID,
	}

	resp, err := e.aigcClient.GetAsyncTask(ctx, getTaskReq)
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
		resultData, err = e.processGPTImage2MultiImageResult(ctx, task, result.ImageResult)
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

	zlog.LogWithContext(ctx).Info("GPT-Image-2任务处理成功",
		zap.String("taskID", task.TaskID),
		zap.String("aigcTaskID", task.ExecuterTaskID),
		zap.Int("imageCount", len(resultData.ImageResults)))

	return nil
}

// processGPTImage2MultiImageResult 处理 GPT-Image-2 多图结果
func (e *GPTImage2Executor) processGPTImage2MultiImageResult(ctx context.Context, task *model.PictureTask, imageResult *aigcv1.ImageResult) (model.PictureTaskResult, error) {
	imageURLs := imageResult.GetImageUrls()
	if len(imageURLs) == 0 {
		return model.PictureTaskResult{}, errors.New("图片结果为空")
	}

	var result model.PictureTaskResult
	var imageResults []model.ImageResultItem

	for i, imageURL := range imageURLs {
		item := e.processOneImage(ctx, task, imageURL, i)
		imageResults = append(imageResults, item)

		// 第一张图同时写入主字段，保持向后兼容
		if i == 0 {
			result.ResultURL = item.ResultURL
			result.UserShowImageURL = item.UserShowImageURL
			result.Width = item.Width
			result.Height = item.Height
			result.AspectRatio = item.AspectRatio
			result.AspectRatioText = item.AspectRatioText
		}
	}

	result.ImageResults = imageResults
	return result, nil
}

// processOneImage 处理单张图片：下载、获取宽高、生成缩略图
func (e *GPTImage2Executor) processOneImage(ctx context.Context, task *model.PictureTask, imageURL string, index int) model.ImageResultItem {
	width := 0
	height := 0
	resultURL := imageURL
	userShowImageURL := imageURL

	data, err := utils.DownloadFile(imageURL)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("下载图片失败，使用原始URL",
			zap.String("image_url", imageURL),
			zap.Int("index", index),
			zap.Error(err))
	} else {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			zlog.LogWithContext(ctx).Warn("解码图片失败，使用原始URL",
				zap.String("image_url", imageURL),
				zap.Int("index", index),
				zap.Error(err))
		} else {
			bounds := img.Bounds()
			width = bounds.Dx()
			height = bounds.Dy()

			webpData, err := utils.CompressImageWithMaxSize(img, 512, 0)
			if err != nil {
				zlog.LogWithContext(ctx).Warn("生成WebP缩略图失败",
					zap.String("image_url", imageURL),
					zap.Int("index", index),
					zap.Error(err))
			} else {
				webpFileName := fmt.Sprintf("%s/generated/%s/gpt_image2_%s_%d.webp", task.ProjectID, task.UserID, task.TaskID, index)
				webpURL, err := utils.UploadToOSS(webpFileName, webpData)
				if err != nil {
					zlog.LogWithContext(ctx).Warn("上传WebP缩略图失败",
						zap.String("image_url", imageURL),
						zap.Int("index", index),
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

	return model.ImageResultItem{
		ResultURL:        resultURL,
		UserShowImageURL: userShowImageURL,
		Width:            width,
		Height:           height,
		AspectRatio:      aspectRatio,
		AspectRatioText:  aspectRatioText,
	}
}

// SyncProviderStatus 覆写父类方法，修正执行器名称校验
func (e *GPTImage2Executor) SyncProviderStatus(ctx context.Context, task *model.PictureTask) (pb.WorkflowTaskStatus, error) {
	if task.Executer != constants.GPTImage2ExecutorName {
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

		// AIGC Core 的进度已足够平滑，直接采用；它不上报（0）时保持当前值不动。
		if aigc := resp.GetProgress(); aigc > 0 {
			newProgress := mapAigcProgress(aigc)
			if newProgress > task.Progress {
				if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, newProgress, 0); err != nil {
					zlog.LogWithContext(ctx).Error("更新GPT-Image-2进度失败", zap.Error(err))
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

// RetrySubmission 覆写父类方法，修正执行器名称校验和 provider 设置
func (e *GPTImage2Executor) RetrySubmission(ctx context.Context, task *model.PictureTask) error {
	if task.Executer != constants.GPTImage2ExecutorName {
		return errors.New("任务执行器不匹配，无法重试")
	}

	var taskParams map[string]string
	if err := json.Unmarshal([]byte(task.ParamJSON), &taskParams); err != nil {
		return errors.Wrap(err, "解析任务参数失败")
	}

	taskParams["provider"] = constants.GPTImage2ExecutorName

	return e.Process(ctx, task.TaskID, taskParams)
}
