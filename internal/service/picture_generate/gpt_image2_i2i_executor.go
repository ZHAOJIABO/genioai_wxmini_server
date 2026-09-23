package picture_generate

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
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

// GPTImage2I2IExecutor GPT-Image-2 图生图执行器，通过 AIGC Core 调用。
// 复用 GPTImage2Executor 的状态同步和结果处理逻辑，仅参数构建和任务类型不同。
type GPTImage2I2IExecutor struct {
	GPTImage2Executor
}

var _ TaskExecutor = (*GPTImage2I2IExecutor)(nil)

func NewGPTImage2I2IExecutor(
	aigcClient *aigc_core.Client,
	taskDao *dao.PictureTaskDao,
	pictureForgeDao *dao.PictureForgeDao,
	rdb *redis.Client,
) *GPTImage2I2IExecutor {
	return &GPTImage2I2IExecutor{
		GPTImage2Executor: GPTImage2Executor{
			GeminiExecutor: GeminiExecutor{
				aigcClient:      aigcClient,
				taskDao:         taskDao,
				pictureForgeDao: pictureForgeDao,
				rdb:             rdb,
			},
		},
	}
}

func (e *GPTImage2I2IExecutor) GetName() string {
	return constants.GPTImage2I2IExecutorName
}

func (e *GPTImage2I2IExecutor) Match(params map[string]string) bool {
	if provider, exists := params["provider"]; exists {
		return provider == constants.GPTImage2I2IExecutorName
	}
	return false
}

func (e *GPTImage2I2IExecutor) Process(ctx context.Context, taskID string, params map[string]string) error {
	task, err := e.taskDao.GetTask(ctx, taskID)
	if err != nil {
		return errors.Wrap(err, "获取任务失败")
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		zlog.LogWithContext(ctx).Info("GPT-Image-2图生图任务已完成或失败，跳过处理",
			zap.String("taskID", taskID),
			zap.Int32("status", task.Status))
		return nil
	}

	task.Executer = constants.GPTImage2I2IExecutorName
	task.RetryCount = 0
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 5, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		return errors.Wrap(err, "更新任务初始状态失败")
	}

	requestParams := e.buildGPTImage2I2IParameters(ctx, task, params)

	clientID := uuid.New().String()
	parametersJson, err := json.Marshal(requestParams)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "序列化参数失败"), false)
	}

	submitReq := &aigcv1.SubmitAsyncTaskRequest{
		TaskType:       "image_to_image",
		Model:          e.getGPTImage2Model(task),
		ProviderHint:   "gptimage",
		ParametersJson: string(parametersJson),
	}

	resp, err := e.aigcClient.SubmitAsyncTask(ctx, submitReq)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "提交图生图任务到AIGC Core失败"), true)
	}

	task.ExecuterTaskID = resp.GetTaskId()
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 15, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION); err != nil {
		return errors.Wrap(err, "保存AIGC任务ID失败")
	}

	zlog.LogWithContext(ctx).Info("GPT-Image-2图生图任务提交成功",
		zap.String("taskID", taskID),
		zap.String("aigcTaskID", resp.GetTaskId()),
		zap.String("clientID", clientID))

	return nil
}

func (e *GPTImage2I2IExecutor) buildGPTImage2I2IParameters(ctx context.Context, task *model.PictureTask, params map[string]string) map[string]interface{} {
	prompt := params["prompt"]
	if val, ok := params["InputPrompt"]; ok && val != "" {
		prompt = val
	}

	requestParams := map[string]interface{}{
		"prompt": prompt,
	}

	if imageURLs := collectOrderedImageURLs(params); len(imageURLs) > 0 {
		requestParams["image_urls"] = imageURLs
	}

	// size 参数
	if size, ok := params["size"]; ok && size != "" {
		requestParams["size"] = size
	} else if resolution, ok := params["resolution"]; ok && resolution != "" {
		requestParams["size"] = resolution
	} else if width, ok := params["width"]; ok && width != "" {
		if height, ok2 := params["height"]; ok2 && height != "" {
			requestParams["size"] = fmt.Sprintf("%sx%s", width, height)
		}
	}

	// quality 参数：强制使用 low
	requestParams["quality"] = "low"

	// n 参数：生成图片数量
	if n, ok := params["num_images"]; ok && n != "" && n != "1" {
		nInt, _ := strconv.Atoi(n)
		if nInt > 1 {
			requestParams["n"] = nInt
		}
	}

	// format 参数
	if format, ok := params["format"]; ok && format != "" {
		requestParams["format"] = format
	}

	// background 参数
	if background, ok := params["background"]; ok && background != "" {
		requestParams["background"] = background
	}

	// moderation 参数
	if moderation, ok := params["moderation"]; ok && moderation != "" {
		requestParams["moderation"] = moderation
	}

	// negative_prompt 参数
	if negativePrompt, ok := params["negative_prompt"]; ok && negativePrompt != "" {
		requestParams["negative_prompt"] = negativePrompt
	}

	return requestParams
}

// collectOrderedImageURLs 收集图片入参（input_image_N / LoadImageN），按编号升序返回。
// params 是 map，直接遍历顺序随机，会导致多图场景下传给模型的图片次序每次不同。
func collectOrderedImageURLs(params map[string]string) []string {
	type imgItem struct {
		order int
		key   string
		url   string
	}

	images := make([]imgItem, 0, 4)
	for key, value := range params {
		var suffix string
		switch {
		case strings.HasPrefix(key, "input_image_"):
			suffix = strings.TrimPrefix(key, "input_image_")
		case strings.HasPrefix(key, "LoadImage"):
			suffix = strings.TrimPrefix(key, "LoadImage")
		default:
			continue
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		// 无编号或编号非法的入参按 1 处理，再由 key 兜底保证稳定
		order := 1
		if n, err := strconv.Atoi(suffix); err == nil && n > 0 {
			order = n
		}
		images = append(images, imgItem{order: order, key: key, url: value})
	}

	sort.Slice(images, func(i, j int) bool {
		if images[i].order == images[j].order {
			return images[i].key < images[j].key
		}
		return images[i].order < images[j].order
	})

	urls := make([]string, 0, len(images))
	for _, img := range images {
		urls = append(urls, img.url)
	}
	return urls
}

// SyncProviderStatus 覆写父类方法，修正执行器名称校验
func (e *GPTImage2I2IExecutor) SyncProviderStatus(ctx context.Context, task *model.PictureTask) (pb.WorkflowTaskStatus, error) {
	if task.Executer != constants.GPTImage2I2IExecutorName {
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
					zlog.LogWithContext(ctx).Error("更新GPT-Image-2图生图进度失败", zap.Error(err))
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

// RetrySubmission 覆写父类方法，修正执行器名称校验
func (e *GPTImage2I2IExecutor) RetrySubmission(ctx context.Context, task *model.PictureTask) error {
	if task.Executer != constants.GPTImage2I2IExecutorName {
		return errors.New("任务执行器不匹配，无法重试")
	}

	var taskParams map[string]string
	if err := json.Unmarshal([]byte(task.ParamJSON), &taskParams); err != nil {
		return errors.Wrap(err, "解析任务参数失败")
	}

	taskParams["provider"] = constants.GPTImage2I2IExecutorName

	return e.Process(ctx, task.TaskID, taskParams)
}
