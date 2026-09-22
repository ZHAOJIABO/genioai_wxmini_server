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
	"sort"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/ai_clients/klingai"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/service/event"
	"va_visionai_server/internal/utils"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

const KlingExecutorName = "kling_image2video"

const (
	// 花花世界特效
	KlingEffectScene_Single_BloomBloom = "bloombloom"
	// 魔力转圈圈
	KlingEffectScene_Single_DizzyDizzy = "dizzydizzy"
	// 快来惹毛我
	KlingEffectScene_Single_FuzzyFuzzy = "fuzzyfuzzy"
	// 捏捏乐
	KlingEffectScene_Single_Squish = "squish"
	// 万物膨胀
	KlingEffectScene_Single_Expansion = "expansion"

	// 爱的拥抱
	KlingEffectScene_Double_Hug = "hug"
	// 爱的亲吻
	KlingEffectScene_Double_Kiss = "kiss"
	// 比心
	KlingEffectScene_Double_HeartGesture = "heart_gesture"
	// 2026新年双人特效
	KlingEffectScene_Double_Cheers_2026 = "cheers_2026"
	// fight
	KlingEffectScene_Double_Fight = "fight"
	// fight_pro
	KlingEffectScene_Double_FightPro = "fight_pro"
)

// KlingApiCallInfo 定义了存储在 ExecuterTaskInfo 中的结构 (保持不变)
type KlingApiCallInfo struct {
	KlingApiType string                 `json:"kling_api_type"`
	Payload      map[string]interface{} `json:"payload"`
}

// ApiParameterPreparerFunc 定义了参数准备函数的类型签名
type ApiParameterPreparerFunc func(ctx context.Context, inputParams map[string]string, apiconfig model.WorkflowApiConfig) (payloadForApi map[string]interface{}, err error)

// KlingImage2VideoExecutor 实现可灵AI图生视频执行器
type KlingImage2VideoExecutor struct {
	taskDao     *dao.PictureTaskDao
	uploadDao   *dao.UploadDao
	klingClient *klingai.KlingClient
	rdb         *redis.Client
	configDao   *dao.ConfigDao
	// 新增参数准备器映射表
	paramPreparers map[string]ApiParameterPreparerFunc
	creditService  credit.Service
	failureHandler TaskFailureHandler
}

// SetFailureHandler 设置失败处理器
func (e *KlingImage2VideoExecutor) SetFailureHandler(handler TaskFailureHandler) {
	e.failureHandler = handler
}

// NewKlingImage2VideoExecutor 创建执行器实例
func NewKlingImage2VideoExecutor(
	taskDao *dao.PictureTaskDao,
	uploadDao *dao.UploadDao,
	klingClient *klingai.KlingClient,
	rdb *redis.Client,
	configDao *dao.ConfigDao,
	creditService credit.Service,
) *KlingImage2VideoExecutor {
	executor := &KlingImage2VideoExecutor{
		taskDao:       taskDao,
		uploadDao:     uploadDao,
		klingClient:   klingClient,
		rdb:           rdb,
		configDao:     configDao,
		creditService: creditService,
	}
	// 初始化参数准备器映射表
	executor.paramPreparers = map[string]ApiParameterPreparerFunc{
		"image2video":       executor.prepareImage2VideoPayload,
		"effects":           executor.prepareEffectsPayload,
		"multi_image2video": executor.prepareMultiImage2VideoPayload,
		// 未来可以添加更多API类型的准备器
	}
	return executor
}

// Match 判断是否匹配Kling图生视频任务
func (e *KlingImage2VideoExecutor) Match(params map[string]string) bool {
	// 优先检查provider字段
	if provider, exists := params["provider"]; exists {
		return provider == constants.KlingExecutorName
	}

	// 如果没有provider字段，回退到原来的逻辑（向后兼容）
	return params["workflow_type"] == pb.WorkflowKindType_WORKFLOW_KIND_TYPE_VIDEO.String()
}

// isKlingErrorRetryable 判断可灵API错误是否可重试
func isKlingErrorRetryable(err error) bool {
	// if err == nil {
	// 	return false
	// }
	// var klingErr *klingai.KlingAPIError
	// if errors.As(err, &klingErr) {
	// 	switch klingErr.HTTPStatus {
	// 	case 429:
	// 		return true
	// 	case 0:
	// 		return true
	// 	case 500:
	// 		return true
	// 	case 503:
	// 		return true
	// 	case 504:
	// 		return true
	// 	case 401, 403:
	// 		return false
	// 	case 400:
	// 		return false
	// 	}

	// 	switch klingErr.BizCode {
	// 	case 1302, 1303:
	// 		return true
	// 	case 0:
	// 		return true
	// 	case 5000:
	// 		return true
	// 	case 5001:
	// 		return true
	// 	case 5002:
	// 		return true
	// 	default:
	// 		return false
	// 	}
	// }

	return true
}

// Process 负责首次提交任务 (逻辑基本不变，调用重构后的 prepareKlingApiCall)
func (e *KlingImage2VideoExecutor) Process(ctx context.Context, taskID string, params map[string]string) error {
	task, err := e.taskDao.GetTask(ctx, taskID)
	if err != nil {
		return errors.Wrap(err, "[(e *KlingImage2VideoExecutor) Process] get task failed")
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		zlog.LogWithContext(ctx).Info("[(e *KlingImage2VideoExecutor) Process] task already completed or failed, skip", zap.String("taskID", taskID), zap.Int32("status", task.Status))
		return nil
	}

	task.Executer = KlingExecutorName
	task.RetryCount = 0
	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING)
	task.Progress = constants.ProgressSubmitted
	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		return errors.Wrap(err, "[(e *KlingImage2VideoExecutor) Process] update task initial status failed")
	}
	e.sendProgressEvent(task)

	payloadBytes, apiCallInfoForStorage, err := e.prepareKlingApiCall(ctx, params, task.ApiConfig)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, false) // 参数准备错误通常不可重试
	}

	// 提前序列化并赋值 ExecuterTaskInfo，确保即使提交失败也能保存以供重试
	storedExecInfoBytes, marshalErr := json.Marshal(apiCallInfoForStorage)
	if marshalErr != nil {
		// 序列化失败是严重内部错误，不可重试
		return e.handleSubmissionError(ctx, task, errors.Wrap(marshalErr, "序列化ExecuterTaskInfo失败"), false)
	}
	task.ExecuterTaskInfo = string(storedExecInfoBytes)

	// 将即将发送给远程API的参数保存到 RequestApiParams 字段
	task.RequestApiParams = string(payloadBytes)

	jobResult, err := e.submitKlingJobToProvider(ctx, apiCallInfoForStorage.KlingApiType, payloadBytes)
	if err != nil {
		// 此刻的 task 对象已包含 ExecuterTaskInfo，handleSubmissionError 会将其一并保存
		return e.handleSubmissionError(ctx, task, err, isKlingErrorRetryable(err))
	}
	if jobResult == nil || jobResult.JobID == "" {
		err := errors.New("可灵API调用成功但返回了空的 jobResult 或 JobID")
		return e.handleSubmissionError(ctx, task, err, false)
	}

	// ExecuterTaskInfo 已提前赋值，这里只需赋值 ExecuterTaskID
	task.ExecuterTaskID = jobResult.JobID
	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION)
	task.Progress = constants.ProgressProviderProcessing
	task.Error = ""
	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		zlog.LogWithContext(ctx).Error("更新任务状态为AWAITING_PROVIDER_COMPLETION失败，但API调用已发送", zap.Error(err), zap.String("taskID", task.TaskID))
		return errors.Wrap(err, "更新任务状态为AWAITING_PROVIDER_COMPLETION失败")
	}

	e.sendProgressEvent(task)
	return nil
}

// handleSubmissionError 处理提交任务时的错误，更新任务状态
func (e *KlingImage2VideoExecutor) handleSubmissionError(ctx context.Context, task *model.PictureTask, err error, isRetryable bool) error {
	log := zlog.LogWithContext(ctx).With(zap.String("taskID", task.TaskID))
	task.Error = err.Error()

	if isRetryable {
		task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING_SUBMISSION_RETRY)
		log.Warn("任务提交失败，将等待重试", zap.Error(err))
	} else {
		log.Error("任务提交失败且不可重试", zap.Error(err))
		if e.failureHandler != nil {
			// 调用统一的失败处理器，它会负责更新状态、退款、释放锁等
			if failErr := e.failureHandler.FailTask(ctx, task, err.Error()); failErr != nil {
				log.Error("调用 failureHandler.FailTask 失败", zap.Error(failErr))
			}
		} else {
			log.Error("failureHandler 未设置，无法处理任务失败")
		}
		return err // 返回原始错误
	}

	if updateErr := e.taskDao.UpdateTask(ctx, task); updateErr != nil {
		log.Error("更新任务状态失败(handleSubmissionError)", zap.Error(updateErr), zap.Int32("targetStatus", task.Status))
		return errors.Wrapf(err, "提交失败后更新任务状态也失败: %v", updateErr)
	}

	e.sendProgressEvent(task)
	return err
}

// RetrySubmission 由重试处理器调用
func (e *KlingImage2VideoExecutor) RetrySubmission(ctx context.Context, task *model.PictureTask) error {
	if task.Executer != KlingExecutorName {
		return errors.New("任务执行器不匹配，无法重试")
	}
	if task.ExecuterTaskInfo == "" {
		return errors.New("无法重试：ExecuterTaskInfo 为空")
	}

	var storedApiCallInfo KlingApiCallInfo
	if err := json.Unmarshal([]byte(task.ExecuterTaskInfo), &storedApiCallInfo); err != nil {
		return errors.Wrap(err, "解析 ExecuterTaskInfo 失败，无法重试")
	}

	payloadBytes, err := json.Marshal(storedApiCallInfo.Payload)
	if err != nil {
		return errors.Wrap(err, "序列化 API payload 失败，无法重试")
	}

	jobResult, err := e.submitKlingJobToProvider(ctx, storedApiCallInfo.KlingApiType, payloadBytes)
	if err != nil {
		return errors.Wrapf(err, "重试提交可灵 %s 任务失败", storedApiCallInfo.KlingApiType)
	}
	if jobResult == nil || jobResult.JobID == "" {
		return errors.Errorf("重试调用可灵 %s API 返回了空的 jobResult 或 JobID", storedApiCallInfo.KlingApiType)
	}

	task.ExecuterTaskID = jobResult.JobID
	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION)
	task.Progress = constants.ProgressProviderProcessing
	task.Error = ""
	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		return errors.Wrapf(err, "重试 %s 任务成功后更新任务状态失败", storedApiCallInfo.KlingApiType)
	}
	e.sendProgressEvent(task)
	return nil
}

// SyncProviderStatus 由外部API状态同步处理器调用
func (e *KlingImage2VideoExecutor) SyncProviderStatus(ctx context.Context, task *model.PictureTask) (updatedTaskStatus pb.WorkflowTaskStatus, err error) {
	if task.Executer != KlingExecutorName || task.ExecuterTaskID == "" {
		// 委托失败处理给 failureHandler
		errorMsg := "无法同步外部任务状态：执行器不匹配或外部任务ID丢失"
		if e.failureHandler != nil {
			return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED,
				e.failureHandler.FailTask(ctx, task, errorMsg)
		}
		// 如果没有 failureHandler，保持原有逻辑
		zlog.LogWithContext(ctx).Error("failureHandler not set, falling back to direct error handling")
		task.Error = errorMsg
		task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
		task.Progress = constants.ProgressFailed
		e.rdb.Del(constants.RedisKeyPictureTaskProgress + ":" + task.TaskID)
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.New(task.Error)
	}

	log := zlog.LogWithContext(ctx).With(zap.String("taskID", task.TaskID), zap.String("executerTaskID", task.ExecuterTaskID))
	status, err := e.klingClient.GetJobStatus(ctx, task.ExecuterTaskID)
	if err != nil {
		retryError := isKlingErrorRetryable(err)
		if !retryError {
			// 委托失败处理给 failureHandler
			if e.failureHandler != nil {
				return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED,
					e.failureHandler.FailTask(ctx, task, fmt.Sprintf("查询可灵任务状态失败且不可重试: %v", err))
			}
			// 如果没有 failureHandler，保持原有逻辑
			log.Error("failureHandler not set, falling back to direct error handling")
			task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			task.Error = fmt.Sprintf("查询可灵任务状态失败且不可重试: %v", err)
			log.Error("查询可灵任务状态失败且不可重试", zap.Error(err))
			e.rdb.Del(constants.RedisKeyPictureTaskProgress + ":" + task.TaskID)
			task.Progress = constants.ProgressFailed
		} else {
			task.ProviderSyncErrorCount++
			task.Error = fmt.Sprintf("查询可灵状态失败(可重试, 第 %d 次): %v", task.ProviderSyncErrorCount, err)
			log.Warn("查询可灵状态失败，将等待下次同步", zap.Error(err), zap.Int32("syncErrorCount", task.ProviderSyncErrorCount))

			const maxSyncErrors = 5
			if task.ProviderSyncErrorCount >= maxSyncErrors {
				// 委托失败处理给 failureHandler
				if e.failureHandler != nil {
					return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED,
						e.failureHandler.FailTask(ctx, task, fmt.Sprintf("查询可灵状态失败次数过多(%d次)，最终失败: %v", task.ProviderSyncErrorCount, err))
				}
				// 如果没有 failureHandler，保持原有逻辑
				log.Error("failureHandler not set, falling back to direct error handling")
				task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
				task.Error = fmt.Sprintf("查询可灵状态失败次数过多(%d次)，最终失败: %v", task.ProviderSyncErrorCount, err)
				log.Error("查询可灵状态失败次数达到上限，任务标记失败", zap.Error(err), zap.Int32("syncErrorCount", task.ProviderSyncErrorCount))
				task.Progress = constants.ProgressFailed
				e.rdb.Del(constants.RedisKeyPictureTaskProgress + ":" + task.TaskID)
				retryError = false
			}
		}

		if retryError {
			return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, errors.Wrap(err, "查询可灵任务状态失败,等待重试")
		}
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.Wrap(err, "查询可灵任务状态失败")
	}

	if task.ProviderSyncErrorCount > 0 {
		task.ProviderSyncErrorCount = 0
	}

	if !status.Done && task.Progress < constants.ProgressProviderMaxSimulated {
		currentProgress := task.Progress
		delta := rand.IntN(5) + 3
		task.Progress = int32(math.Min(float64(currentProgress)+float64(delta), constants.ProgressProviderMaxSimulated))
	}

	if status.Done {
		if status.Error != "" {
			// 委托失败处理给 failureHandler，不直接设置 progress = -1
			if e.failureHandler != nil {
				return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED,
					e.failureHandler.FailTask(ctx, task, "可灵任务失败: "+status.Error)
			}
			// 如果没有 failureHandler，保持原有逻辑但记录错误
			log.Error("failureHandler not set, falling back to direct error handling")
			task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			task.Error = "可灵任务失败: " + status.Error
			task.Progress = constants.ProgressFailed
			e.rdb.Del(constants.RedisKeyPictureTaskProgress + ":" + task.TaskID)
			return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.New(task.Error)
		} else if status.VideoURL == "" {
			// 委托失败处理给 failureHandler
			if e.failureHandler != nil {
				return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED,
					e.failureHandler.FailTask(ctx, task, "可灵任务完成但未返回视频URL")
			}
			// 如果没有 failureHandler，保持原有逻辑
			log.Error("failureHandler not set, falling back to direct error handling")
			task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			task.Error = "可灵任务完成但未返回视频URL"
			task.Progress = constants.ProgressFailed
			e.rdb.Del(constants.RedisKeyPictureTaskProgress + ":" + task.TaskID)
			return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.New(task.Error)
		} else {
			// 成功完成：由结果处理阶段负责下载与上传，并更新进度
			return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED, nil
		}
	}

	// if err := e.taskDao.UpdateTask(ctx, task); err != nil {
	// 	log.Error("同步状态后更新任务失败", zap.Error(err))
	// 	return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.Wrap(err, "同步状态后更新任务失败")
	// }
	// e.sendProgressEvent(task)
	e.updateTaskFieldsAndSendEvent(ctx, task, "", pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION)
	return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil
}

// processSuccessfulKlingResult 处理成功完成的可灵任务
func (e *KlingImage2VideoExecutor) processSuccessfulKlingResult(ctx context.Context, task *model.PictureTask, videoURL string) error {
	task.Progress = int32(math.Max(float64(task.Progress), float64(constants.ProgressDownloading)))
	if err := e.updateTaskFieldsAndSendEvent(ctx, task, "", 0); err != nil {
		return errors.Wrap(err, "更新下载前进度失败")
	}
	projectID := task.ProjectID
	data, err := e.klingClient.DownloadVideo(ctx, videoURL)
	if err != nil {
		return errors.Wrap(err, "下载可灵视频失败")
	}
	frameBytes, frameErr := utils.ExtractFrameFromVideoBytesWithFFmpeg(data)
	if frameErr != nil {
		zlog.LogWithContext(ctx).Error("Failed to extract frame from video", zap.Error(frameErr))
	}

	ossFileName := fmt.Sprintf("%s/generated/%s/%s.mp4", projectID, task.UserID, task.TaskID)
	ossURL, err := utils.UploadToOSS(ossFileName, data)
	if err != nil {
		return errors.Wrap(err, "上传视频到OSS失败")
	}

	var frameOssURL string
	if len(frameBytes) > 0 {
		frameOssFileName := fmt.Sprintf("%s/generated/%s/%s_frame.jpg", projectID, task.UserID, task.TaskID)
		frameOssURL, err = utils.UploadToOSS(frameOssFileName, frameBytes)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("上传首帧到OSS失败，继续完成任务", zap.Error(err))
			frameOssURL = ""
		}
	}
	task.Progress = constants.ProgressUploading
	if err := e.updateTaskFieldsAndSendEvent(ctx, task, "", 0); err != nil {
		return errors.Wrap(err, "update task fields and send event failed")
	}

	var createParams struct {
		Image string `json:"image"`
	}
	unmarshalErr := json.Unmarshal([]byte(task.ExecuterTaskInfo), &createParams)
	if unmarshalErr != nil {
		zlog.LogWithContext(ctx).Error("unmarshal executer task info failed", zap.Error(unmarshalErr))
	}
	return e.completeInternalTask(ctx, task, ossURL, frameOssURL, frameBytes)
}

// completeInternalTask 完成内部任务记录
func (e *KlingImage2VideoExecutor) completeInternalTask(ctx context.Context, task *model.PictureTask, ossURL, frameOssURL string, frameBytes []byte) error {
	resultData := model.PictureTaskResult{
		ResultURL:     ossURL,
		VideoFrameURL: frameOssURL,
	}
	frameImage, _, _ := image.Decode(bytes.NewReader(frameBytes))
	if frameImage != nil {
		resultData.Height, resultData.Width, resultData.VideoFrameAspectRatio = utils.ImageAspectRatio(frameImage)
	} else {
		resultData.Height, resultData.Width, resultData.VideoFrameAspectRatio = 1920, 1080, 1920.0/1080.0
	}

	resultJSON, err := json.Marshal(resultData)
	if err != nil {
		task.Error = "序列化结果失败: " + err.Error()
	}
	task.ResultJSON = string(resultJSON)
	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED)
	task.Progress = constants.ProgressCompleted
	task.UnreadTaskResult = true
	task.CompletedAt = sql.NullTime{Time: time.Now(), Valid: true}

	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		return errors.Wrap(err, "最终更新任务状态失败")
	}
	e.rdb.Del(constants.RedisKeyPictureTaskProgress + ":" + task.TaskID)
	e.sendProgressEvent(task)
	return nil
}

// updateTaskFieldsAndSendEvent 辅助函数
func (e *KlingImage2VideoExecutor) updateTaskFieldsAndSendEvent(ctx context.Context, task *model.PictureTask, errMsg string, status pb.WorkflowTaskStatus) error {
	log := zlog.LogWithContext(ctx).With(zap.String("taskID", task.TaskID))

	// 失败处理已完全委托给 failureHandler
	if status == pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED {
		if e.failureHandler != nil {
			return e.failureHandler.FailTask(ctx, task, errMsg)
		}
		log.Error("failureHandler not set, unable to process task failure")
		// 即使没有handler，也要尝试更新状态
	}

	task.Error = errMsg
	if status != 0 { // 允许只更新错误信息而不改变状态
		task.Status = int32(status)
	}
	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		log.Error("更新任务状态失败", zap.Error(err))
		return err
	}

	e.sendProgressEvent(task)
	return nil
}

// sendProgressEvent 发送进度或结果事件
func (e *KlingImage2VideoExecutor) sendProgressEvent(task *model.PictureTask) {
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

func (e *KlingImage2VideoExecutor) GetName() string {
	return KlingExecutorName
}

// IsSubmissionErrorRetryable 实现TaskExecutorForRetry接口，判断提交错误是否可重试
func (e *KlingImage2VideoExecutor) IsSubmissionErrorRetryable(err error) bool {
	return isKlingErrorRetryable(err)
}

// prepareImage2VideoPayload 为 "image2video" API 准备请求体
func (e *KlingImage2VideoExecutor) prepareImage2VideoPayload(ctx context.Context, inputParams map[string]string, apiconfig model.WorkflowApiConfig) (map[string]interface{}, error) {
	var baseConfig struct {
		ModelName string  `json:"model_name"`
		Mode      string  `json:"mode"`
		Duration  string  `json:"duration"`
		CfgScale  float64 `json:"cfg_scale"`
		Prompt    string  `json:"prompt"`
		Image     string  `json:"image"`
	}
	klingConfigStr, configErr := e.configDao.GetConfigValue(constants.ConfigKeyKlingImage2VideoConfig)
	if configErr != nil {
		return nil, errors.Wrap(configErr, "获取可灵图生视频配置失败")
	}
	if unmarshalErr := json.Unmarshal([]byte(klingConfigStr), &baseConfig); unmarshalErr != nil {
		return nil, errors.Wrap(unmarshalErr, "解析可灵图生视频配置失败")
	}

	if val, ok := inputParams["LoadImage1"]; ok {
		baseConfig.Image = val
	} else if baseConfig.Image == "" {
		return nil, errors.New("图生视频缺少必要的 'LoadImage1' (image) 参数")
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

	return payloadMap, nil
}

// prepareMultiImage2VideoPayload 为 "multi_image2video" API 准备请求体
func (e *KlingImage2VideoExecutor) prepareMultiImage2VideoPayload(ctx context.Context, inputParams map[string]string, apiconfig model.WorkflowApiConfig) (map[string]interface{}, error) {
	// 1) 读取基础配置
	var baseConfig struct {
		ModelName string  `json:"model_name"`
		Mode      string  `json:"mode"`
		Duration  string  `json:"duration"`
		CfgScale  float64 `json:"cfg_scale"`
		Prompt    string  `json:"prompt"`
	}
	klingConfigStr, configErr := e.configDao.GetConfigValue(constants.ConfigKeyKlingImage2VideoConfig)
	if configErr != nil {
		return nil, errors.Wrap(configErr, "获取可灵图生视频配置失败")
	}
	if unmarshalErr := json.Unmarshal([]byte(klingConfigStr), &baseConfig); unmarshalErr != nil {
		return nil, errors.Wrap(unmarshalErr, "解析可灵图生视频配置失败")
	}

	// 2) 从 inputParams 中按序号稳定收集图片
	type imgItem struct {
		order int
		key   string
		url   string
	}
	images := make([]imgItem, 0, 4)
	for k, v := range inputParams {
		if !strings.HasPrefix(k, "LoadImage") {
			continue
		}
		if strings.TrimSpace(v) == "" {
			continue
		}
		// 支持 LoadImage、LoadImage1、LoadImage2...，未带数字的按 1 处理
		ord := 1
		if len(k) > len("LoadImage") {
			// 提取后缀数字
			suffix := k[len("LoadImage"):]
			n := 0
			for i := 0; i < len(suffix); i++ {
				c := suffix[i]
				if c < '0' || c > '9' {
					n = 0
					break
				}
				n = n*10 + int(c-'0')
			}
			if n > 0 {
				ord = n
			}
		}
		images = append(images, imgItem{order: ord, key: k, url: v})
	}
	if len(images) == 0 {
		return nil, errors.New("图生视频缺少必要的 'LoadImage' (image) 参数")
	}
	// 稳定排序，保证与 LoadImageN 的编号顺序一致
	sort.Slice(images, func(i, j int) bool {
		if images[i].order == images[j].order {
			return images[i].key < images[j].key
		}
		return images[i].order < images[j].order
	})

	// 3) 处理 prompt 覆盖
	if val, ok := inputParams["prompt"]; ok {
		baseConfig.Prompt = val
	}
	if val, ok := inputParams["custom_prompt"]; ok {
		baseConfig.Prompt = val
	}
	if val, ok := inputParams["InputPrompt"]; ok {
		baseConfig.Prompt = val
	}

	// 4) 直接构建 payload map，避免 JSON 往返
	payload := map[string]interface{}{
		"model_name": baseConfig.ModelName,
		"mode":       baseConfig.Mode,
		"duration":   baseConfig.Duration,
		"cfg_scale":  baseConfig.CfgScale,
		"prompt":     baseConfig.Prompt,
	}
	imageList := make([]map[string]string, 0, len(images))
	for _, it := range images {
		imageList = append(imageList, map[string]string{"image": it.url})
	}
	payload["image_list"] = imageList

	return payload, nil
}

// prepareEffectsPayload 为 "effects" API 准备请求体
func (e *KlingImage2VideoExecutor) prepareEffectsPayload(ctx context.Context, inputParams map[string]string, apiconfig model.WorkflowApiConfig) (map[string]interface{}, error) {
	requestMap := make(map[string]interface{})
	requestMap["effect_scene"] = apiconfig.EffectScene
	images := []string{}
	for k, v := range inputParams {
		if strings.HasPrefix(k, "LoadImage") {
			images = append(images, v)
		}
	}

	if len(images) == 0 {
		return nil, errors.New("Required parameter 'LoadImage' is missing")
	}

	var baseConfig struct {
		ModelName string  `json:"model_name"`
		Mode      string  `json:"mode"`
		Duration  string  `json:"duration"`
		CfgScale  float64 `json:"cfg_scale"`
		Prompt    string  `json:"prompt"`
		Image     string  `json:"image"`
	}

	klingConfigStr, configErr := e.configDao.GetConfigValue(constants.ConfigKeyKlingEffectsConfig)
	if configErr != nil {
		return nil, errors.Wrap(configErr, "获取可灵图生视频配置失败")
	}
	if unmarshalErr := json.Unmarshal([]byte(klingConfigStr), &baseConfig); unmarshalErr != nil {
		return nil, errors.Wrap(unmarshalErr, "解析可灵图生视频配置失败")
	}

	inputData := make(map[string]interface{})
	inputData["model_name"] = baseConfig.ModelName
	inputData["duration"] = baseConfig.Duration
	switch apiconfig.EffectScene {
	// case KlingEffectScene_Single_BloomBloom,
	// 	KlingEffectScene_Single_DizzyDizzy,
	// 	KlingEffectScene_Single_FuzzyFuzzy,
	// 	KlingEffectScene_Single_Squish,
	// 	KlingEffectScene_Single_Expansion:
	// 	zlog.LogWithContext(ctx).Info("single effect scene", zap.String("effect_scene", apiconfig.EffectScene))
	// 	inputData["image"] = images[0]

	case KlingEffectScene_Double_Hug,
		KlingEffectScene_Double_Kiss,
		KlingEffectScene_Double_HeartGesture,
		KlingEffectScene_Double_Fight,
		KlingEffectScene_Double_FightPro,
		KlingEffectScene_Double_Cheers_2026:
		zlog.LogWithContext(ctx).Info("double effect scene", zap.String("effect_scene", apiconfig.EffectScene))
		inputData["images"] = images
		inputData["mode"] = baseConfig.Mode
	default:
		zlog.LogWithContext(ctx).Info("single effect scene", zap.String("effect_scene", apiconfig.EffectScene))
		inputData["image"] = images[0]
	}
	requestMap["input"] = inputData

	return requestMap, nil
}

// prepareKlingApiCall 准备API调用所需的参数和存储信息 (重构后)
func (e *KlingImage2VideoExecutor) prepareKlingApiCall(
	ctx context.Context,
	inputParams map[string]string,
	apiconfig model.WorkflowApiConfig,
) (payloadBytes []byte, apiCallInfoForStorage *KlingApiCallInfo, err error) {
	preparer, ok := e.paramPreparers[apiconfig.ApiIden]
	if !ok {
		return nil, nil, errors.Errorf("不支持的 klingapi-iden 类型: %s，没有找到对应的参数准备器", apiconfig.ApiIden)
	}

	actualApiPayload, err := preparer(ctx, inputParams, apiconfig)
	if err != nil {
		return nil, nil, err
	}

	payloadBytes, err = json.Marshal(actualApiPayload)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "序列化 %s API参数失败", apiconfig.ApiIden)
	}

	apiCallInfoForStorage = &KlingApiCallInfo{
		KlingApiType: apiconfig.ApiIden,
		Payload:      actualApiPayload,
	}

	return payloadBytes, apiCallInfoForStorage, nil
}

// submitKlingJobToProvider 调用KlingClient提交任务
func (e *KlingImage2VideoExecutor) submitKlingJobToProvider(
	ctx context.Context,
	klingApiType string,
	payloadBytes []byte,
) (*klingai.JobResult, error) {
	switch klingApiType {
	case "image2video":
		return e.klingClient.StartImageToVideo(ctx, payloadBytes)
	case "effects":
		return e.klingClient.StartVideoEffects(ctx, payloadBytes)
	case "multi_image2video":
		return e.klingClient.StartMultiImageToVideo(ctx, payloadBytes)
	default:
		return nil, errors.Errorf("内部错误：尝试提交未知的kling API类型: %s", klingApiType)
	}
}

// ProcessSuccessfulResult a
func (e *KlingImage2VideoExecutor) ProcessSuccessfulResult(ctx context.Context, task *model.PictureTask) error {
	var apiCallInfo KlingApiCallInfo
	if err := json.Unmarshal([]byte(task.ExecuterTaskInfo), &apiCallInfo); err != nil {
		return errors.Wrap(err, "failed to unmarshal ExecuterTaskInfo for result processing")
	}

	// 重新从可灵获取最新的任务状态，确保拿到最新的 videoURL
	status, err := e.klingClient.GetJobStatus(ctx, task.ExecuterTaskID)
	if err != nil {
		return errors.Wrapf(err, "failed to get final job status for task %s before processing result", task.TaskID)
	}

	if !status.Done || status.VideoURL == "" {
		return errors.Errorf("task %s is not truly complete or is missing video URL upon result processing", task.TaskID)
	}
	return e.processSuccessfulKlingResult(ctx, task, status.VideoURL)
}
