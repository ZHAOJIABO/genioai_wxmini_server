package picture_generate

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/go-redis/redis"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/ai_clients/gpt4o"
	"va_visionai_server/internal/utils"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

const Gpt4oExecutorNameV2 = "gpt4o_image2image_v2"

// Gpt4oImage2ImageExecutorV2 实现gpt4o图生图执行器 v2
type Gpt4oImage2ImageExecutorV2 struct {
	taskDao        *dao.PictureTaskDao
	uploadDao      *dao.UploadDao
	gpt4oClient    *gpt4o.Gpt4oClient
	rdb            *redis.Client
	failureHandler TaskFailureHandler
}

// NewGpt4oImage2ImageExecutorV2 创建执行器实例
func NewGpt4oImage2ImageExecutorV2(
	taskDao *dao.PictureTaskDao,
	uploadDao *dao.UploadDao,
	gpt4oClient *gpt4o.Gpt4oClient,
	rdb *redis.Client,
) *Gpt4oImage2ImageExecutorV2 {
	return &Gpt4oImage2ImageExecutorV2{
		taskDao:     taskDao,
		uploadDao:   uploadDao,
		gpt4oClient: gpt4oClient,
		rdb:         rdb,
	}
}

// SetFailureHandler 设置失败处理器
func (e *Gpt4oImage2ImageExecutorV2) SetFailureHandler(handler TaskFailureHandler) {
	e.failureHandler = handler
}

// Match 判断是否匹配4o图生图任务 v2
func (e *Gpt4oImage2ImageExecutorV2) Match(params map[string]string) bool {
	// V2执行器只匹配明确指定的provider，避免与原版执行器冲突
	if provider, exists := params["provider"]; exists {
		return provider == constants.Gpt4oExecutorNameV2
	}
	return false // V2执行器只处理明确指定的请求
}

// Process 处理图生视频任务
func (e *Gpt4oImage2ImageExecutorV2) Process(ctx context.Context, taskID string, params map[string]string) error {
	task, err := e.taskDao.GetTask(ctx, taskID)
	if err != nil {
		return errors.Wrap(err, "获取任务失败")
	}
	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		return nil
	}
	task.Executer = Gpt4oExecutorNameV2
	task.RetryCount = 0
	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING)
	task.Progress = 1
	// 1. 更新任务状态为处理中
	if err := e.updateTaskProgress(ctx, task, 2, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		return err
	}

	createParams := struct {
		Image  string `json:"image"`
		Prompt string `json:"prompt"`
	}{
		Image:  params["LoadImage1"],
		Prompt: params["prompt"],
	}

	if val, ok := params["custom_prompt"]; ok {
		createParams.Prompt = val
	}
	paramsBytes, err := json.Marshal(createParams)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "4o生图任务失败,创建任务参数失败"), false)
	}
	task.ExecuterTaskInfo = string(paramsBytes)
	imgData, err := e.startGpt4oTask(ctx, task, paramsBytes)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "4o生图任务失败"), false) // 可重试
	}
	if len(imgData) == 0 {
		return e.handleSubmissionError(ctx, task, errors.New("重试调用4o API 返回了空的 imgData"), false)
	}
	if err = e.updateTaskProgress(ctx, task, 90, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		e.releaseUserProcessingLock(ctx, task.UserID, taskID)
		return err
	}
	ossURL, photoWidth, photoHeight, _, webpOssURL, err := e.UploadImage(ctx, task, imgData)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, false)
	}

	// 5. 完成任务
	err = e.completeTask(ctx, task, ossURL, webpOssURL, photoWidth, photoHeight)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, false)
	}
	e.releaseUserProcessingLock(ctx, task.UserID, taskID)
	return nil
}

// updateTaskProgress 更新任务进度
func (e *Gpt4oImage2ImageExecutorV2) updateTaskProgress(ctx context.Context, task *model.PictureTask, progress int32, status pb.WorkflowTaskStatus) error {
	task.Progress = progress
	if status != 0 {
		task.Status = int32(status)
	}

	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		zlog.LogWithContext(ctx).Error("更新任务状态失败", zap.Error(err))
		return err
	}

	SendProgressEvent(task)
	return nil
}

// startKlingTask 启动4o任务
func (e *Gpt4oImage2ImageExecutorV2) startGpt4oTask(ctx context.Context, task *model.PictureTask, paramsBytes []byte) ([]byte, error) {
	taskParams := struct {
		Image  string `json:"image"`
		Prompt string `json:"prompt"`
	}{}
	err := json.Unmarshal(paramsBytes, &taskParams)
	if err != nil {
		// 处理反序列化错误
		return nil, errors.Wrap(err, "处理反序列化错误")
	}
	if err = e.updateTaskProgress(ctx, task, 5, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		return nil, err
	}
	localImagePath, localMaskPath, err := e.createImageAndMask(ctx, task.TaskID, taskParams.Image)
	if err != nil {
		return nil, err
	}
	// 创建一个取消的上下文，用于控制定时进度更新协程
	updateCtx, cancel := context.WithCancel(ctx)

	// 启动进度更新函数
	go e.updateProgress(updateCtx, task, cancel)
	imgData, err := e.gpt4oClient.StartImageToImage(ctx, task.TaskID, taskParams.Prompt, localImagePath, localMaskPath)
	defer os.Remove(localImagePath)
	defer os.Remove(localMaskPath)
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "调用4o 生图 API 失败")
	}
	cancel()
	if imgData == nil {
		return nil, errors.New("4o API 返回了空的 imgData")
	}
	return imgData, nil
}

// 上传图片
func (e *Gpt4oImage2ImageExecutorV2) UploadImage(ctx context.Context, task *model.PictureTask, imgData []byte) (string, int, int, string, string, error) {
	// 处理图片并上传
	var imageUrl string
	var imageUrlLow string
	var webpOssURL string
	var photoWidth, photoHeight int

	// 上传原图
	fileName := fmt.Sprintf("com.domob.piclib/generated/%s/%s.jpg", task.UserID, task.TaskID)
	// 检查图片大小是否超过1MB
	imgData, err := utils.CompressImage(imgData)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to zip image", zap.Error(err))
	}

	imageUrl, err = utils.UploadToOSS(fileName, imgData)
	if err != nil {
		zlog.LogWithContext(ctx).Error("上传图片失败", zap.Error(err))
		return imageUrl, photoWidth, photoHeight, imageUrlLow, webpOssURL, err
	}
	img, err := imaging.Decode(bytes.NewReader(imgData))
	if err != nil {
		zlog.LogWithContext(ctx).Error("解析图片失败", zap.Error(err))
		return imageUrl, photoWidth, photoHeight, imageUrlLow, webpOssURL, err
	}
	photoWidth, photoHeight, _ = utils.ImageAspectRatio(img)
	// 生成缩略图
	compressedImgData, err := utils.CompressImageWithMaxSize(img, 360, imaging.JPEG)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to compress image", zap.Error(err))
		return imageUrl, photoWidth, photoHeight, imageUrlLow, webpOssURL, err
	}
	compressedFileName := fmt.Sprintf("com.domob.piclib/generated/%s/%s-low.jpg", task.UserID, task.TaskID)
	imageUrlLow, err = utils.UploadToOSS(compressedFileName, compressedImgData)
	if err != nil {
		zlog.LogWithContext(ctx).Error("上传缩略图失败", zap.Error(err))
		return imageUrl, photoWidth, photoHeight, imageUrlLow, webpOssURL, err
	}

	// 生成并上传 WebP 格式
	webpFileName := strings.TrimSuffix(compressedFileName, ".jpg") + ".webp"
	webpCompressedImgData, err := utils.ConvertToWebp(img)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to convert to webp", zap.Error(err))
	}
	webpOssURL, err = utils.UploadToOSS(webpFileName, webpCompressedImgData)
	if err != nil {
		zlog.LogWithContext(ctx).Error("上传缩略图失败", zap.Error(err))
		return imageUrl, photoWidth, photoHeight, imageUrlLow, webpOssURL, err
	}

	// update file info
	fileMd5 := utils.GetMD5FromBytes(imgData)
	err = e.uploadDao.CreateUploadFile(ctx, e.uploadDao.DB, &model.UploadFile{
		OSSAddr:   imageUrl,
		Hash:      fileMd5,
		UserID:    task.UserID,
		ProjectID: task.ProjectID,
		FileType:  "jpg",
	}, pb.UploaderRole_UPLOADER_ROLE_SYSTEM.String())
	if err != nil {
		zlog.LogWithContext(ctx).Error("[gpt4o_image2image_executor_v2]上传图片失败", zap.Error(err))
	}
	fileInfo := model.UploadFileInfo{
		PhotoWidth:  int32(photoWidth),
		PhotoHeight: int32(photoHeight),
		Hash:        fileMd5,
	}
	if err := e.uploadDao.UpsertFileInfo(ctx, &fileInfo); err != nil {
		zlog.LogWithContext(ctx).Error("[gpt4o_image2image_executor_v2]更新文件信息失败", zap.Error(err))
	}

	return imageUrl, photoWidth, photoHeight, imageUrlLow, webpOssURL, err
}

// completeTask 完成任务
func (e *Gpt4oImage2ImageExecutorV2) completeTask(ctx context.Context, task *model.PictureTask, ossURL, webpOssURL string, photoWidth, photoHeight int) error {

	// 更新任务结果
	resultData := model.PictureTaskResult{
		ResultURL:        ossURL,
		Width:            photoWidth,
		Height:           photoHeight,
		AspectRatio:      math.Round(float64(photoHeight)/float64(photoWidth)*100) / 100,
		UserShowImageURL: webpOssURL,
	}

	resultJSON, err := json.Marshal(resultData)
	if err != nil {
		return errors.Wrap(err, "序列化结果失败")
	}

	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED)
	task.ResultJSON = string(resultJSON)
	task.Progress = 100
	task.UnreadTaskResult = true
	task.CompletedAt = sql.NullTime{Time: time.Now(), Valid: true}
	if err := e.taskDao.UpdateTask(ctx, task); err != nil {
		return err
	}
	e.rdb.Del(constants.RedisKeyPictureTaskProgress + ":" + task.TaskID)
	SendProgressEvent(task)

	// 触发资源清理（释放并发槽位）
	TriggerOnTaskTerminated(ctx, task)
	// 触发任务完成回调（链式编排等）
	TriggerOnTaskCompleted(task)

	return nil
}

// handleSubmissionError 处理提交任务时的错误，更新任务状态
func (e *Gpt4oImage2ImageExecutorV2) handleSubmissionError(ctx context.Context, task *model.PictureTask, err error, isRetryable bool) error {
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
		e.releaseUserProcessingLock(ctx, task.UserID, task.TaskID)
		return err // 返回原始错误
	}

	if updateErr := e.taskDao.UpdateTask(ctx, task); updateErr != nil {
		log.Error("更新任务状态失败(handleSubmissionError)", zap.Error(updateErr), zap.Int32("targetStatus", task.Status))
		e.releaseUserProcessingLock(ctx, task.UserID, task.TaskID)
		return errors.Wrapf(err, "提交失败后更新任务状态也失败: %v", updateErr)
	}

	SendProgressEvent(task)
	e.releaseUserProcessingLock(ctx, task.UserID, task.TaskID)
	return err
}

// 构造图片
func (e *Gpt4oImage2ImageExecutorV2) createImageAndMask(ctx context.Context, taskID string, imageUrl string) (string, string, error) {
	// 从params获取原始图片并下载到本地
	var localImagePath string
	var localMaskPath string
	// 创建临时文件
	tmpFileName := fmt.Sprintf("/tmp/image_%s.png", taskID)
	tmpFile, err := os.Create(tmpFileName)
	if err != nil {
		zlog.LogWithContext(ctx).Error("创建临时文件失败", zap.Error(err))
		return localImagePath, localMaskPath, err
	}
	httpClient := &http.Client{}
	resp, err := httpClient.Get(imageUrl)
	if err != nil {
		zlog.LogWithContext(ctx).Error("图片下载失败", zap.Error(err))
		return "", "", err
	}
	defer resp.Body.Close()

	// 保存到临时文件
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		zlog.LogWithContext(ctx).Error("保存到临时图片失败", zap.Error(err))
		return "", "", err
	}
	tmpFile.Close()
	localImagePath = tmpFileName

	if localImagePath == "" {
		zlog.LogWithContext(ctx).Error("临时图片为空")
		return localImagePath, localMaskPath, errors.New("临时图片为空")
	}
	//生成蒙版图片
	localMaskPath = fmt.Sprintf("/tmp/imageMask_%s.png", taskID)
	err = utils.GenerateMaskImage(localImagePath, localMaskPath)
	if err != nil {
		zlog.LogWithContext(ctx).Error("生成蒙版图片失败", zap.Error(err))
		return localImagePath, localMaskPath, err
	}

	return localImagePath, localMaskPath, nil
}

// 进度更新函数
func (e *Gpt4oImage2ImageExecutorV2) updateProgress(ctx context.Context, task *model.PictureTask, cancel context.CancelFunc) {
	// 设置任务的最大执行时长（比如90秒）
	maxDuration := time.Second * 90
	startTime := time.Now()

	for {
		select {
		case <-ctx.Done(): // 如果上下文被取消，退出
			return
		case <-time.After(time.Second * 10): // 每隔 10 秒更新一次进度
			// 计算任务已执行的时间
			elapsed := time.Since(startTime)

			// 如果任务已经完成，则停止更新
			if elapsed > maxDuration {
				// 在最大时长内完成任务，更新到 100%
				//if err := e.updateTaskProgress(ctx, task, 100, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
				//	log.Printf("Error updating task progress to 100: %v", err)
				//}
				return
			}

			// 计算当前进度（在0到100之间）
			progress := int(float64(elapsed)/float64(maxDuration)*90) + 5 // 进度从5%到85%
			if err := e.updateTaskProgress(ctx, task, int32(progress), pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION); err != nil {
				// 如果更新进度失败，则记录日志并退出
				log.Printf("Error updating task progress: %v", err)
				return
			}
		}
	}
}

// RetrySubmission 由重试处理器调用
func (e *Gpt4oImage2ImageExecutorV2) RetrySubmission(ctx context.Context, task *model.PictureTask) error {
	if task.Executer != Gpt4oExecutorNameV2 {
		e.releaseUserProcessingLock(ctx, task.UserID, task.TaskID)
		return errors.New("任务执行器不匹配，无法重试")
	}
	if task.ExecuterTaskInfo == "" {
		e.releaseUserProcessingLock(ctx, task.UserID, task.TaskID)
		return errors.New("无法重试：ExecuterTaskInfo 为空")
	}

	paramsBytes := []byte(task.ExecuterTaskInfo)
	imgData, err := e.startGpt4oTask(ctx, task, paramsBytes)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "重试提交4o任务失败"), true) // 可重试
	}
	if len(imgData) == 0 {
		return e.handleSubmissionError(ctx, task, errors.New("重试调用4o API 返回了空的 imgData"), false)
	}
	if err = e.updateTaskProgress(ctx, task, 90, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		e.releaseUserProcessingLock(ctx, task.UserID, task.TaskID)
		return err
	}
	ossURL, photoWidth, photoHeight, _, webpOssURL, err := e.UploadImage(ctx, task, imgData)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, false)
	}

	task.CompletedAt = sql.NullTime{Time: time.Now(), Valid: true}
	task.UnreadTaskResult = true

	// 5. 完成任务
	err = e.completeTask(ctx, task, ossURL, webpOssURL, photoWidth, photoHeight)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, false)
	}
	e.releaseUserProcessingLock(ctx, task.UserID, task.TaskID)
	return nil
}

func (e *Gpt4oImage2ImageExecutorV2) GetName() string {
	return Gpt4oExecutorNameV2
}

// IsSubmissionErrorRetryable 实现TaskExecutorForRetry接口，判断提交错误是否可重试
func (e *Gpt4oImage2ImageExecutorV2) IsSubmissionErrorRetryable(err error) bool {
	//return isKlingErrorRetryable(err)
	//默认可以重试
	return true
}

// SyncProviderStatus 由外部API状态同步处理器调用
func (e *Gpt4oImage2ImageExecutorV2) SyncProviderStatus(ctx context.Context, task *model.PictureTask) (updatedTaskStatus pb.WorkflowTaskStatus, err error) {
	if task.Executer != Gpt4oExecutorNameV2 {
		errorMsg := "无法同步外部任务状态：执行器不匹配"
		if e.failureHandler != nil {
			if failErr := e.failureHandler.FailTask(ctx, task, errorMsg); failErr != nil {
				zlog.LogWithContext(ctx).Error("调用 failureHandler.FailTask 失败", zap.Error(failErr))
			}
		} else {
			// 降级处理：直接更新任务状态
			task.Error = errorMsg
			task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			task.Progress = -1
			if updateErr := e.taskDao.UpdateTask(ctx, task); updateErr != nil {
				zlog.LogWithContext(ctx).Error("更新任务失败状态失败(SyncProviderStatus/preCheck)",
					zap.String("taskID", task.TaskID), zap.Error(updateErr))
				return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.Wrapf(errors.New(task.Error), "更新任务数据库失败: %v", updateErr)
			}
			SendProgressEvent(task)
		}
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.New(errorMsg)
	}
	task, err = e.taskDao.GetTask(ctx, task.TaskID)
	if err != nil {
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, errors.Wrap(err, "获取任务失败")
	}
	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) {
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED, nil
	} else if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED, nil
	}

	return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil
}
func (p *Gpt4oImage2ImageExecutorV2) releaseUserProcessingLock(ctx context.Context, userID string, taskID string) {
	lockKey := fmt.Sprintf("%s%s", userProcessingLockKeyPrefix, userID)
	if err := p.rdb.Del(lockKey).Err(); err != nil {
		zlog.LogWithContext(ctx).Error("Failed to release user processing lock",
			zap.String("user_id", userID),
			zap.String("task_id", taskID),
			zap.String("lock_key", lockKey),
			zap.Error(err))
	} else {
		zlog.LogWithContext(ctx).Info("Successfully released user processing lock",
			zap.String("user_id", userID),
			zap.String("task_id", taskID),
			zap.String("lock_key", lockKey),
		)
	}
}

func (e *Gpt4oImage2ImageExecutorV2) ProcessSuccessfulResult(ctx context.Context, task *model.PictureTask) error {
	return nil
}
