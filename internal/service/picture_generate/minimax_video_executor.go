package picture_generate

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/go-redis/redis"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/ai_clients/minimax"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/service/event"
	"va_visionai_server/internal/utils"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

const (
	MinimaxExecutorName = constants.MinimaxExecutorName
	// Maximum number of provider sync errors before marking task as failed
	maxMinimaxSyncErrors = 5
)

// MinimaxApiCallInfo 存储Minimax API调用信息
type MinimaxApiCallInfo struct {
	MinimaxApiType string                 `json:"minimax_api_type"`
	Payload        map[string]interface{} `json:"payload"`
}

// MinimaxVideoExecutor Minimax视频生成执行器
type MinimaxVideoExecutor struct {
	taskDao       *dao.PictureTaskDao
	uploadDao     *dao.UploadDao
	minimaxClient *minimax.MinimaxClient
	rdb           *redis.Client
	configDao     *dao.ConfigDao
	// 参数准备器映射表
	paramPreparers map[string]ApiParameterPreparerFunc
	creditService  credit.Service
	failureHandler TaskFailureHandler
}

func (e *MinimaxVideoExecutor) SetFailureHandler(handler TaskFailureHandler) {
	e.failureHandler = handler
}

func NewMinimaxVideoExecutor(
	taskDao *dao.PictureTaskDao,
	uploadDao *dao.UploadDao,
	minimaxClient *minimax.MinimaxClient,
	rdb *redis.Client,
	configDao *dao.ConfigDao,
	creditService credit.Service,
) *MinimaxVideoExecutor {
	e := &MinimaxVideoExecutor{
		taskDao:       taskDao,
		uploadDao:     uploadDao,
		minimaxClient: minimaxClient,
		rdb:           rdb,
		configDao:     configDao,
		creditService: creditService,
	}

	// 初始化参数准备器
	e.paramPreparers = map[string]ApiParameterPreparerFunc{
		constants.MinimaxApiTypeText2Video:  e.prepareText2VideoPayload,
		constants.MinimaxApiTypeImage2Video: e.prepareImage2VideoPayload,
	}

	return e
}

func (e *MinimaxVideoExecutor) Match(params map[string]string) bool {
	provider, ok := params["provider"]
	if !ok {
		return false
	}
	apiIden, ok := params["api_iden"]
	if !ok {
		return false
	}
	return provider == constants.MinimaxExecutorName && (apiIden == "image2video" || apiIden == "text2video")
}

// IsMinimaxErrorRetryable 判断Minimax错误是否可重试
func isMinimaxErrorRetryable(err error) bool {
	// var minimaxErr *minimax.MinimaxAPIError
	// if errors.As(err, &minimaxErr) {
	// 	// HTTP 5xx错误或特定的业务错误码可重试
	// 	if minimaxErr.HTTPStatus >= 500 && minimaxErr.HTTPStatus < 600 {
	// 		return true
	// 	}
	// 	// 特定的业务错误码可重试（如资源不足、限流等）
	// 	switch minimaxErr.BizCode {
	// 	case 1003, 1004, 1005: // 假设这些是可重试的错误码
	// 		return true
	// 	}
	// }

	// // 网络错误通常可重试
	// if strings.Contains(err.Error(), "timeout") ||
	// 	strings.Contains(err.Error(), "connection") ||
	// 	strings.Contains(err.Error(), "network") {
	// 	return true
	// }
	if errors.Is(err, utils.ErrMaxRetriesReached) {
		return true
	}
	return false
}

func (e *MinimaxVideoExecutor) Process(ctx context.Context, taskID string, params map[string]string) error {
	zlog.LogWithContext(ctx).Info("开始处理Minimax视频生成任务", zap.String("taskId", taskID))

	task, err := e.taskDao.GetTask(ctx, taskID)
	if err != nil {
		return errors.Wrapf(err, "查找任务失败 taskId=%s", taskID)
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		zlog.LogWithContext(ctx).Info("任务已完成或失败，跳过处理", zap.String("taskID", taskID), zap.Int32("status", task.Status))
		return nil
	}

	// 设置任务为处理中状态
	task.Executer = MinimaxExecutorName
	task.RetryCount = 0
	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING)
	task.Progress = constants.ProgressSubmitted
	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		return errors.Wrap(err, "更新任务初始状态失败")
	}
	e.sendProgressEvent(task)

	// 获取API配置
	// 准备API调用参数
	payloadBytes, apiCallInfoForStorage, err := e.prepareMinimaxApiCall(ctx, params, task.ApiConfig)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, false)
	}

	// 保存请求参数
	task.RequestApiParams = string(payloadBytes)
	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		zlog.LogWithContext(ctx).Error("保存请求API参数失败", zap.Error(err), zap.String("taskID", task.TaskID))
	}

	// 提交任务到Minimax
	jobResult, err := e.submitMinimaxJobToProvider(ctx, task.ApiConfig.ApiIden, payloadBytes)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, isMinimaxErrorRetryable(err))
	}

	if jobResult == nil || jobResult.JobID == "" {
		err := errors.New("Minimax API返回了空的jobResult或JobID")
		return e.handleSubmissionError(ctx, task, err, false)
	}

	// 存储API调用信息和更新任务状态
	apiCallInfoBytes, err := json.Marshal(apiCallInfoForStorage)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "序列化ExecuterTaskInfo失败"), false)
	}

	task.ExecuterTaskInfo = string(apiCallInfoBytes)
	task.ExecuterTaskID = jobResult.JobID
	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION)
	task.Progress = constants.ProgressProviderProcessing
	task.Error = ""

	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		zlog.LogWithContext(ctx).Error("更新任务状态为AWAITING_PROVIDER_COMPLETION失败", zap.Error(err), zap.String("taskID", task.TaskID))
		return errors.Wrap(err, "更新任务状态失败")
	}

	e.sendProgressEvent(task)

	zlog.LogWithContext(ctx).Info("Minimax任务提交成功",
		zap.String("taskId", taskID),
		zap.String("minimaxJobId", jobResult.JobID))

	return nil
}

func (e *MinimaxVideoExecutor) handleSubmissionError(ctx context.Context, task *model.PictureTask, err error, isRetryable bool) error {
	log := zlog.LogWithContext(ctx).With(zap.String("taskID", task.TaskID))
	task.Error = err.Error()

	if isRetryable {
		task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING_SUBMISSION_RETRY)
		log.Warn("任务提交失败，将等待重试", zap.Error(err))
	} else {
		log.Error("任务提交失败且不可重试", zap.Error(err))
		if e.failureHandler != nil {
			if failErr := e.failureHandler.FailTask(ctx, task, err.Error()); failErr != nil {
				log.Error("调用 failureHandler.FailTask 失败", zap.Error(failErr))
			}
		} else {
			log.Error("failureHandler 未设置，无法处理任务失败")
		}
		return err
	}

	if updateErr := e.taskDao.UpdateTask(ctx, task); updateErr != nil {
		log.Error("更新任务状态失败(handleSubmissionError)", zap.Error(updateErr), zap.Int32("targetStatus", task.Status))
		return errors.Wrapf(err, "提交失败后更新任务状态也失败: %v", updateErr)
	}

	e.sendProgressEvent(task)
	return err
}

func (e *MinimaxVideoExecutor) RetrySubmission(ctx context.Context, task *model.PictureTask) error {
	zlog.LogWithContext(ctx).Info("重试提交Minimax任务", zap.String("taskId", task.TaskID))

	if task.Executer != MinimaxExecutorName {
		return errors.New("任务执行器不匹配，无法重试")
	}
	if task.ExecuterTaskInfo == "" {
		return errors.New("无法重试：ExecuterTaskInfo 为空")
	}

	var storedApiCallInfo MinimaxApiCallInfo
	if err := json.Unmarshal([]byte(task.ExecuterTaskInfo), &storedApiCallInfo); err != nil {
		return errors.Wrap(err, "解析 ExecuterTaskInfo 失败，无法重试")
	}

	// 重新序列化payload
	payloadBytes, err := json.Marshal(storedApiCallInfo.Payload)
	if err != nil {
		return errors.Wrap(err, "序列化 API payload 失败，无法重试")
	}

	// 重新提交
	jobResult, err := e.submitMinimaxJobToProvider(ctx, storedApiCallInfo.MinimaxApiType, payloadBytes)
	if err != nil {
		return errors.Wrapf(err, "重试提交Minimax %s 任务失败", storedApiCallInfo.MinimaxApiType)
	}

	if jobResult == nil || jobResult.JobID == "" {
		return errors.Errorf("重试调用Minimax %s API 返回了空的 jobResult 或 JobID", storedApiCallInfo.MinimaxApiType)
	}

	task.ExecuterTaskID = jobResult.JobID
	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION)
	task.Progress = constants.ProgressProviderProcessing
	task.Error = ""

	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		return errors.Wrapf(err, "重试 %s 任务成功后更新任务状态失败", storedApiCallInfo.MinimaxApiType)
	}

	e.sendProgressEvent(task)

	zlog.LogWithContext(ctx).Info("Minimax任务重试提交成功",
		zap.String("taskId", task.TaskID),
		zap.String("minimaxJobId", jobResult.JobID))

	return nil
}

func (e *MinimaxVideoExecutor) SyncProviderStatus(ctx context.Context, task *model.PictureTask) (updatedTaskStatus pb.WorkflowTaskStatus, err error) {
	if task.Executer != MinimaxExecutorName || task.ExecuterTaskID == "" {
		task.Error = "无法同步外部任务状态：执行器不匹配或外部任务ID丢失"
		task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
		task.Progress = constants.ProgressFailed
		e.rdb.Del(constants.RedisKeyPictureTaskProgress + ":" + task.TaskID)
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.New(task.Error)
	}

	log := zlog.LogWithContext(ctx).With(zap.String("taskID", task.TaskID), zap.String("executerTaskID", task.ExecuterTaskID))

	// 获取Minimax任务状态
	status, err := e.minimaxClient.GetJobStatus(ctx, task.ExecuterTaskID)
	if err != nil {
		retryError := isMinimaxErrorRetryable(err)
		if !retryError {
			task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			task.Error = fmt.Sprintf("查询Minimax任务状态失败且不可重试: %v", err)
			log.Error("查询Minimax任务状态失败且不可重试", zap.Error(err))
			e.rdb.Del(constants.RedisKeyPictureTaskProgress + ":" + task.TaskID)
			task.Progress = constants.ProgressFailed
		} else {
			task.ProviderSyncErrorCount++
			task.Error = fmt.Sprintf("查询Minimax状态失败(可重试, 第 %d 次): %v", task.ProviderSyncErrorCount, err)
			log.Warn("查询Minimax状态失败，将等待下次同步", zap.Error(err), zap.Int32("syncErrorCount", task.ProviderSyncErrorCount))

			if task.ProviderSyncErrorCount >= maxMinimaxSyncErrors {
				task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
				task.Error = fmt.Sprintf("查询Minimax状态失败次数过多(%d次)，最终失败: %v", task.ProviderSyncErrorCount, err)
				log.Error("查询Minimax状态失败次数达到上限，任务标记失败", zap.Error(err), zap.Int32("syncErrorCount", task.ProviderSyncErrorCount))
				task.Progress = constants.ProgressFailed
				e.rdb.Del(constants.RedisKeyPictureTaskProgress + ":" + task.TaskID)
				retryError = false
			}
		}

		if retryError {
			return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, errors.Wrap(err, "查询Minimax任务状态失败,等待重试")
		}

		if e.failureHandler != nil {
			if failErr := e.failureHandler.FailTask(ctx, task, task.Error); failErr != nil {
				log.Error("调用 failureHandler.FailTask 失败", zap.Error(failErr))
			}
		}

		// 统一基于错误码映射失败原因（标题 + 全量描述），仅当返回了 provider 源错误码时
		if status != nil && status.ServerOriginCode != 0 {
			errInfo := constants.GetMinimaxErrorInfo(status.ServerOriginCode)
			task.FailedReason = errInfo.Title
			task.FailedReasonFull = errInfo.Description
			log.Error("Minimax任务失败映射", zap.Uint16("code", status.ServerOriginCode), zap.String("title", errInfo.Title), zap.String("desc", task.FailedReasonFull))
		}

		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.Wrap(err, "查询Minimax任务状态失败")
	}

	// Reset error count on successful sync
	if task.ProviderSyncErrorCount > 0 {
		task.ProviderSyncErrorCount = 0
	}

	// 更新进度
	if !status.Done && task.Progress < constants.ProgressProviderMaxSimulated {
		currentProgress := task.Progress
		delta := rand.IntN(5) + 3
		task.Progress = int32(math.Min(float64(currentProgress)+float64(delta), constants.ProgressProviderMaxSimulated))
	}

	if status.Done {
		if status.Error != "" {
			// 任务失败
			log.Error("Minimax任务执行失败", zap.String("error", status.Error))
			if e.failureHandler != nil {
				if failErr := e.failureHandler.FailTask(ctx, task, status.Error); failErr != nil {
					log.Error("调用 failureHandler.FailTask 失败", zap.Error(failErr))
				}
			}
			// 当任务失败且 provider 提供了错误码时，填充失败原因
			if status.ServerOriginCode != 0 {
				info := constants.GetMinimaxErrorInfo(status.ServerOriginCode)
				task.FailedReason = info.Title
				task.FailedReasonFull = info.Description
				log.Error("Minimax任务失败(完成态)映射", zap.Uint16("code", status.ServerOriginCode), zap.String("title", info.Title), zap.String("desc", task.FailedReasonFull))
			}
			return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, nil
		} else if status.VideoURL != "" {
			// 任务成功完成
			log.Info("Minimax任务执行成功", zap.String("videoURL", status.VideoURL))

			if err := e.processSuccessfulMinimaxResult(ctx, task, status.VideoURL); err != nil {
				log.Error("处理Minimax成功结果时出错", zap.Error(err))

				if e.failureHandler != nil {
					if failErr := e.failureHandler.FailTask(ctx, task, err.Error()); failErr != nil {
						log.Error("调用 failureHandler.FailTask 失败", zap.Error(failErr))
					}
				}
				return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, nil
			}
			return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED, nil
		}
	}

	// 任务仍在处理中
	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		log.Error("更新任务进度失败", zap.Int32("progress", task.Progress), zap.Error(err))
	}

	e.sendProgressEvent(task)
	return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil
}

func (e *MinimaxVideoExecutor) processSuccessfulMinimaxResult(ctx context.Context, task *model.PictureTask, videoURL string) error {
	// 下载视频
	videoData, err := e.minimaxClient.DownloadVideo(ctx, videoURL)
	if err != nil {
		return errors.Wrap(err, "下载Minimax视频失败")
	}

	// 上传到OSS
	projectID := task.ProjectID
	ossFileName := fmt.Sprintf("%s/generated/%s/%s.mp4", projectID, task.UserID, task.TaskID)
	ossURL, err := utils.UploadToOSS(ossFileName, videoData)
	if err != nil {
		return errors.Wrap(err, "上传视频到OSS失败")
	}

	// 生成视频第一帧作为缩略图
	frameBytes, err := utils.ExtractFrameFromVideoBytesWithFFmpeg(videoData)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("提取视频第一帧失败，使用默认缩略图", zap.Error(err))
		frameBytes = []byte{} // 使用空字节数组作为默认值
	}

	var frameOssURL string
	if len(frameBytes) > 0 {
		frameOssFileName := fmt.Sprintf("%s/generated/%s/%s_frame.jpg", projectID, task.UserID, task.TaskID)
		frameOssURL, err = utils.UploadToOSS(frameOssFileName, frameBytes)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("上传视频缩略图失败", zap.Error(err))
			frameOssURL = "" // 如果缩略图上传失败，使用空字符串
		}
	}

	// 完成内部任务
	return e.completeInternalTask(ctx, task, ossURL, frameOssURL, frameBytes)
}

func (e *MinimaxVideoExecutor) completeInternalTask(ctx context.Context, task *model.PictureTask, ossURL, frameOssURL string, frameBytes []byte) error {
	// 计算视频宽高比（默认16:9）
	aspectRatio := 16.0 / 9.0

	// 构建结果
	result := model.PictureTaskResult{
		ResultURL:             ossURL,
		AspectRatio:           aspectRatio,
		VideoFrameURL:         frameOssURL,
		VideoFrameAspectRatio: aspectRatio,
	}
	frameImage, _, _ := image.Decode(bytes.NewReader(frameBytes))
	if frameImage != nil {
		result.Height, result.Width, result.VideoFrameAspectRatio = utils.ImageAspectRatio(frameImage)
	} else {
		result.Height, result.Width, result.VideoFrameAspectRatio = 1920, 1080, 1920.0/1080.0
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return errors.Wrap(err, "序列化任务结果失败")
	}

	// 更新任务状态
	task.ResultJSON = string(resultJSON)
	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED)
	task.Progress = constants.ProgressCompleted
	task.UnreadTaskResult = true
	task.CompletedAt = sql.NullTime{Time: time.Now(), Valid: true}

	// 使用统一的更新与事件函数以触发终态钩子（释放并发/队列槽位）
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, constants.ProgressCompleted, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED); err != nil {
		return errors.Wrap(err, "更新任务完成状态失败")
	}

	zlog.LogWithContext(ctx).Info("Minimax任务处理完成",
		zap.String("taskId", task.TaskID),
		zap.String("resultURL", ossURL))

	return nil
}

func (e *MinimaxVideoExecutor) sendProgressEvent(task *model.PictureTask) {
	eventData := &pb.TaskProgressEventData{
		PictureTaskId: task.TaskID,
		Progress:      task.Progress,
		CanRetry:      task.CanRetry,
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) && task.ResultJSON != "" {
		var result model.PictureTaskResult
		if err := json.Unmarshal([]byte(task.ResultJSON), &result); err == nil {
			switch task.WorkflowType {
			case pb.WorkflowKindType_WORKFLOW_KIND_TYPE_VIDEO.String():
				eventData.PictureInfo = &pb.PictureInfo{
					Url:          result.ResultURL,
					AspectRatio:  float32(result.VideoFrameAspectRatio),
					ThumbnailUrl: result.VideoFrameURL,
				}
			default:
				eventData.PictureInfo = &pb.PictureInfo{
					Url:          result.ResultURL,
					AspectRatio:  float32(result.AspectRatio),
					ThumbnailUrl: "",
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

func (e *MinimaxVideoExecutor) GetName() string {
	return MinimaxExecutorName
}

// IsSubmissionErrorRetryable 实现TaskExecutorForRetry接口，判断提交错误是否可重试
func (e *MinimaxVideoExecutor) IsSubmissionErrorRetryable(err error) bool {
	return isMinimaxErrorRetryable(err)
}

// prepareText2VideoPayload 为 "text2video" API 准备请求体
func (e *MinimaxVideoExecutor) prepareText2VideoPayload(ctx context.Context, inputParams map[string]string, apiconfig model.WorkflowApiConfig) (map[string]interface{}, error) {
	var baseConfig struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}

	minimaxConfigStr, configErr := e.configDao.GetConfigValue(constants.ConfigKeyMinimaxText2VideoConfig)
	if configErr != nil {
		return nil, errors.Wrap(configErr, "获取Minimax文生视频配置失败")
	}

	if unmarshalErr := json.Unmarshal([]byte(minimaxConfigStr), &baseConfig); unmarshalErr != nil {
		return nil, errors.Wrap(unmarshalErr, "解析Minimax文生视频配置失败")
	}

	if val, ok := inputParams["prompt"]; ok {
		baseConfig.Prompt = val
	}

	if val, ok := inputParams["custom_prompt"]; ok {
		if val != "" {
			baseConfig.Prompt = val
		}
	}

	if val, ok := inputParams["InputPrompt"]; ok {
		if val != "" {
			baseConfig.Prompt = val
		}
	}

	payloadMap := make(map[string]interface{})
	tempBytes, err := json.Marshal(baseConfig)
	if err != nil {
		return nil, errors.Wrap(err, "序列化文生视频最终参数到临时字节失败")
	}

	if err := json.Unmarshal(tempBytes, &payloadMap); err != nil {
		return nil, errors.Wrap(err, "反序列化文生视频临时字节到map失败")
	}

	if val, ok := inputParams["duration"]; ok {
		duration, err := strconv.Atoi(val)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("duration 参数格式错误", zap.String("duration", val))
		}
		if duration > 0 {
			payloadMap["duration"] = duration
		}
	}

	if val, ok := inputParams["resolution"]; ok {
		if val != "" {
			payloadMap["resolution"] = val
		}
	}

	return payloadMap, nil
}

// prepareImage2VideoPayload 为 "image2video" API 准备请求体
func (e *MinimaxVideoExecutor) prepareImage2VideoPayload(ctx context.Context, inputParams map[string]string, apiconfig model.WorkflowApiConfig) (map[string]interface{}, error) {
	var baseConfig struct {
		Model           string `json:"model"`
		Prompt          string `json:"prompt"`
		PromptOptimizer bool   `json:"prompt_optimizer"`
		FirstFrameImage string `json:"first_frame_image"`
	}

	minimaxConfigStr, configErr := e.configDao.GetConfigValue(constants.ConfigKeyMinimaxImage2VideoConfig)
	if configErr != nil {
		return nil, errors.Wrap(configErr, "获取Minimax图生视频配置失败")
	}

	if unmarshalErr := json.Unmarshal([]byte(minimaxConfigStr), &baseConfig); unmarshalErr != nil {
		return nil, errors.Wrap(unmarshalErr, "解析Minimax图生视频配置失败")
	}

	if val, ok := inputParams["LoadImage1"]; ok {
		baseConfig.FirstFrameImage = val
	} else if baseConfig.FirstFrameImage == "" {
		return nil, errors.New("图生视频缺少必要的 'LoadImage1' (first_frame_image) 参数")
	}

	if val, ok := inputParams["prompt"]; ok {
		baseConfig.Prompt = val
	}

	if val, ok := inputParams["custom_prompt"]; ok {
		baseConfig.Prompt = val
	}

	payloadMap := make(map[string]interface{})
	tempBytes, err := json.Marshal(baseConfig)
	if err != nil {
		return nil, errors.Wrap(err, "序列化图生视频最终参数到临时字节失败")
	}

	if err := json.Unmarshal(tempBytes, &payloadMap); err != nil {
		return nil, errors.Wrap(err, "反序列化图生视频临时字节到map失败")
	}

	if val, ok := inputParams["duration"]; ok {
		duration, err := strconv.Atoi(val)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("duration 参数格式错误", zap.String("duration", val))
		}
		if duration > 0 {
			payloadMap["duration"] = duration
		}
	}

	if val, ok := inputParams["resolution"]; ok {
		if val != "" {
			payloadMap["resolution"] = val
		}
	}

	return payloadMap, nil
}

// prepareMinimaxApiCall 准备API调用所需的参数和存储信息
func (e *MinimaxVideoExecutor) prepareMinimaxApiCall(
	ctx context.Context,
	inputParams map[string]string,
	apiconfig model.WorkflowApiConfig,
) (payloadBytes []byte, apiCallInfoForStorage *MinimaxApiCallInfo, err error) {
	preparer, ok := e.paramPreparers[apiconfig.ApiIden]
	if !ok {
		return nil, nil, errors.Errorf("不支持的 minimaxapi-iden 类型: %s，没有找到对应的参数准备器", apiconfig.ApiIden)
	}

	actualApiPayload, err := preparer(ctx, inputParams, apiconfig)
	if err != nil {
		return nil, nil, err
	}

	payloadBytes, err = json.Marshal(actualApiPayload)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "序列化 %s API参数失败", apiconfig.ApiIden)
	}

	apiCallInfoForStorage = &MinimaxApiCallInfo{
		MinimaxApiType: apiconfig.ApiIden,
		Payload:        actualApiPayload,
	}

	return payloadBytes, apiCallInfoForStorage, nil
}

// submitMinimaxJobToProvider 调用MinimaxClient提交任务
func (e *MinimaxVideoExecutor) submitMinimaxJobToProvider(
	ctx context.Context,
	minimaxApiType string,
	payloadBytes []byte,
) (*minimax.JobResult, error) {
	switch minimaxApiType {
	case constants.MinimaxApiTypeText2Video, constants.MinimaxApiTypeImage2Video:
		return e.minimaxClient.StartVideoGeneration(ctx, payloadBytes)
	default:
		return nil, errors.Errorf("内部错误：尝试提交未知的Minimax API类型: %s", minimaxApiType)
	}
}

// ProcessSuccessfulResult 处理成功结果
func (e *MinimaxVideoExecutor) ProcessSuccessfulResult(ctx context.Context, task *model.PictureTask) error {
	var apiCallInfo MinimaxApiCallInfo
	if err := json.Unmarshal([]byte(task.ExecuterTaskInfo), &apiCallInfo); err != nil {
		return errors.Wrap(err, "failed to unmarshal ExecuterTaskInfo for result processing")
	}

	// 重新从Minimax获取最新的任务状态，确保拿到最新的 videoURL
	status, err := e.minimaxClient.GetJobStatus(ctx, task.ExecuterTaskID)
	if err != nil {
		return errors.Wrapf(err, "failed to get final job status for task %s before processing result", task.TaskID)
	}

	if !status.Done || status.VideoURL == "" {
		return errors.Errorf("task %s is not truly complete or is missing video URL upon result processing", task.TaskID)
	}

	return e.processSuccessfulMinimaxResult(ctx, task, status.VideoURL)
}

func (e *MinimaxVideoExecutor) MappingMinimaxErrorCodeToFailedReason(code uint16) string {
	// 兼容旧接口：返回标题（简要失败原因）
	info := constants.GetMinimaxErrorInfo(code)
	return info.Title
}
