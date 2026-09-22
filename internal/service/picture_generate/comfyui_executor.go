package picture_generate

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/dundunHa/comfy2go/client"
	"github.com/dundunHa/comfy2go/graphapi"
	"github.com/go-redis/redis"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/conf"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/utils"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

const ComfyUIExecutorName = "comfyui_executor"

// ComfyUIExecutor 实现ComfyUI图片生成执行器
type ComfyUIExecutor struct {
	taskDao         *dao.PictureTaskDao
	uploadDao       *dao.UploadDao
	pictureForgeDao *dao.PictureForgeDao
	rdb             *redis.Client
	httpClient      *http.Client
	evaluateService *EvaluateService
	failureHandler  TaskFailureHandler
	configDao       *dao.ConfigDao
}

// NewComfyUIExecutor 创建ComfyUI执行器实例
func NewComfyUIExecutor(
	taskDao *dao.PictureTaskDao,
	uploadDao *dao.UploadDao,
	pictureForgeDao *dao.PictureForgeDao,
	rdb *redis.Client,
	httpClient *http.Client,
	evaluateService *EvaluateService,
	configDao *dao.ConfigDao,
) *ComfyUIExecutor {
	return &ComfyUIExecutor{
		taskDao:         taskDao,
		uploadDao:       uploadDao,
		pictureForgeDao: pictureForgeDao,
		rdb:             rdb,
		httpClient:      httpClient,
		evaluateService: evaluateService,
		configDao:       configDao,
	}
}

// SetFailureHandler 设置失败处理器
func (e *ComfyUIExecutor) SetFailureHandler(handler TaskFailureHandler) {
	e.failureHandler = handler
}

// GetName 返回执行器名称
func (e *ComfyUIExecutor) GetName() string {
	return ComfyUIExecutorName
}

// Match 判断是否匹配ComfyUI任务
func (e *ComfyUIExecutor) Match(params map[string]string) bool {
	// 优先检查provider字段
	if provider, exists := params["provider"]; exists {
		return provider == constants.ComfyUIExecutorName
	}

	// 如果没有provider字段，回退到原来的逻辑（向后兼容）
	workflowType := params["workflow_type"]
	return workflowType != pb.WorkflowKindType_WORKFLOW_KIND_TYPE_VIDEO.String()
}

// Process 处理ComfyUI任务初始提交
func (e *ComfyUIExecutor) Process(ctx context.Context, taskID string, params map[string]string) error {

	task, err := e.taskDao.GetTask(ctx, taskID)
	if err != nil {
		return errors.Wrap(err, "获取任务失败")
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		zlog.LogWithContext(ctx).Info("ComfyUI任务已完成或失败，跳过处理",
			zap.String("taskID", taskID),
			zap.Int32("status", task.Status))
		return nil
	}

	// 设置执行器名称和状态
	task.Executer = ComfyUIExecutorName
	task.RetryCount = 0
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 5, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING); err != nil {
		return errors.Wrap(err, "更新任务初始状态失败")
	}

	// 获取工作流信息
	workflowJson, err := e.getWorkflowJson(ctx, task)
	if err != nil {
		return e.handleSubmissionError(ctx, task, err, false)
	}

	// 准备工作流图
	workflowJson = e.setWorkflowParams(ctx, workflowJson, params)

	workflowJson, err = e.uploadComfyUIImage(ctx, params, taskID, workflowJson)
	if err != nil {
		return errors.Wrap(err, "上传ComfyUI图片失败")
	}

	// 更新进度
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 10, 0); err != nil {
		zlog.LogWithContext(ctx).Error("更新任务进度失败", zap.Error(err))
	}
	workflowParam := map[string]interface{}{}
	err = json.Unmarshal([]byte(workflowJson), &workflowParam)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "反序列化ComfyUI请求失败"), true)
	}

	comfyReq := map[string]interface{}{
		"prompt":    workflowParam,
		"client_id": uuid.New().String(),
	}
	comfyReqBytes, err := json.Marshal(comfyReq)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "序列化ComfyUI请求失败"), true)
	}

	// 提交任务到ComfyUI
	promptID, err := e.InvokeFunction(ctx, comfyReqBytes)
	if err != nil {
		return e.handleSubmissionError(ctx, task, errors.Wrap(err, "提交任务到ComfyUI失败"), true)
	}

	// 保存PromptID到ExecuterTaskID字段
	task.ExecuterTaskID = promptID
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 15, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION); err != nil {
		return errors.Wrap(err, "保存PromptID失败")
	}

	zlog.LogWithContext(ctx).Info("ComfyUI任务提交成功",
		zap.String("taskID", taskID),
		zap.String("promptID", promptID))

	return nil
}

// InvokeFunction 异步调用ComfyUI函数
func (c *ComfyUIExecutor) InvokeFunction(ctx context.Context, request []byte) (string, error) {
	var response struct {
		PromptID   string            `json:"prompt_id"`
		Number     int               `json:"number"`
		NodeErrors map[string]string `json:"node_errors"`
	}
	comfyUIAddr := "https://" + conf.GlobalConfig.ComfyUI.Server
	// 构造请求URL
	url := comfyUIAddr + "/api/prompt"

	// 创建HTTP请求
	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(request))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应（但不解析，因为是异步任务）
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("function invoke failed with status %d", resp.StatusCode)
	}
	bodyContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	err = json.Unmarshal(bodyContent, &response)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if len(response.NodeErrors) > 0 {
		return "", fmt.Errorf("function invoke failed with node errors: %v", response.NodeErrors)
	}

	return response.PromptID, nil
}

func (e *ComfyUIExecutor) uploadComfyUIImage(ctx context.Context, params map[string]string, taskID string, workflowJson string) (string, error) {
	comfyUIAddr := "https://" + conf.GlobalConfig.ComfyUI.Server

	for key, imagePath := range params {
		if !strings.HasPrefix(imagePath, "http") {
			continue
		}

		fileName, err := e.uploadSingleImage(ctx, comfyUIAddr, key, imagePath, taskID)
		if err != nil {
			return workflowJson, err
		}

		workflowJson = strings.ReplaceAll(workflowJson, "{"+key+"}", fileName)
	}

	return workflowJson, nil
}

// uploadSingleImage 上传单个图片到ComfyUI
func (e *ComfyUIExecutor) uploadSingleImage(ctx context.Context, comfyUIAddr, key, imagePath, taskID string) (string, error) {
	// 下载图片
	//nolint:bodyclose
	_, body, err := utils.Get(ctx, imagePath)
	if err != nil {
		return "", errors.Wrap(err, "获取图片失败")
	}

	// 构建multipart表单
	fileName := fmt.Sprintf("%s_%s.jpg", taskID, key)
	extraFields := map[string]string{
		"type": "input",
		"name": fileName,
	}

	formData, contentType, err := utils.BuildMultipartForm(body, fileName, extraFields)
	if err != nil {
		return "", errors.Wrap(err, "构建multipart表单失败")
	}

	// 执行上传
	if err := e.doUploadRequest(ctx, comfyUIAddr+"/api/upload/image", formData, contentType, key, fileName); err != nil {
		return "", err
	}

	return fileName, nil
}

// doUploadRequest 执行上传请求
func (e *ComfyUIExecutor) doUploadRequest(ctx context.Context, url string, formData []byte, contentType, key, fileName string) error {
	uploadOptions := utils.HTTPClientOptions{
		Timeout:       60 * time.Second,
		MaxRetries:    5,
		RetryInterval: 2 * time.Second,
	}
	resp, respBody, err := utils.NewHTTPRequest("POST", url).
		WithBody(formData).
		WithHeader("Content-Type", contentType).
		WithOptions(uploadOptions).
		Do(ctx)
	if err != nil {
		return errors.Wrapf(err, "上传失败，key: %s, fileName: %s", key, fileName)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.Errorf("上传失败，状态码: %d，响应: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// SyncProviderStatus 同步ComfyUI任务状态
func (e *ComfyUIExecutor) SyncProviderStatus(ctx context.Context, task *model.PictureTask) (pb.WorkflowTaskStatus, error) {
	if task.Executer != ComfyUIExecutorName {
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED,
			errors.New("执行器不匹配，无法同步状态")
	}

	if task.ExecuterTaskID == "" {
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED,
			errors.New("PromptID为空，无法同步状态")
	}

	// 创建ComfyUI客户端
	comfyClient, err := e.createComfyClient(ctx)
	if err != nil {
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION,
			errors.Wrap(err, "创建ComfyUI客户端失败")
	}
	defer comfyClient.HttpClient().CloseIdleConnections()

	// 获取任务历史记录
	promptID := task.ExecuterTaskID
	historyItem, err := comfyClient.GetPromptHistory(promptID)
	if err != nil {
		// 任务尚未出现在历史记录中，或者获取失败，都视为仍在处理中
		zlog.LogWithContext(ctx).Debug("ComfyUI任务尚未出现在历史记录中或获取失败",
			zap.String("taskID", task.TaskID),
			zap.String("promptID", promptID),
			zap.Error(err))

		// 模拟进度更新：计算随机增量（8% 到 12%），确保不超过85%
		if task.Progress < 85 {
			increment := int32(8 + (task.Progress % 5)) // 简单的伪随机增量：8-12%
			newProgress := task.Progress + increment
			if newProgress > 85 {
				newProgress = 85
			}

			// 调用通用进度更新函数，状态参数传0表示不改变状态
			if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, newProgress, 0); err != nil {
				zlog.LogWithContext(ctx).Error("更新ComfyUI模拟进度失败", zap.Error(err))
			}
		}

		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION, nil
	}

	// 检查是否有输出结果
	if len(historyItem.Outputs) > 0 {
		// 有输出，任务成功完成
		zlog.LogWithContext(ctx).Info("ComfyUI任务已完成",
			zap.String("taskID", task.TaskID),
			zap.String("promptID", promptID))
		return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED, nil
	}

	// 检查是否有错误信息（这里需要根据ComfyUI的实际返回结构来判断）
	// 如果历史记录存在但没有输出，可能是失败了
	zlog.LogWithContext(ctx).Warn("ComfyUI任务可能失败",
		zap.String("taskID", task.TaskID),
		zap.String("promptID", promptID))
	return pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED,
		errors.New("ComfyUI任务没有输出结果")
}

// ProcessSuccessfulResult 处理成功的任务结果
func (e *ComfyUIExecutor) ProcessSuccessfulResult(ctx context.Context, task *model.PictureTask) error {
	if task.ExecuterTaskID == "" {
		return errors.New("PromptID为空，无法处理结果")
	}

	// 处理开始时：更新进度至85%
	// if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 85, 0); err != nil {
	// 	return errors.Wrap(err, "更新处理开始进度失败")
	// }

	// 创建ComfyUI客户端
	comfyClient, err := e.createComfyClient(ctx)
	if err != nil {
		return errors.Wrap(err, "创建ComfyUI客户端失败")
	}
	defer comfyClient.HttpClient().CloseIdleConnections()

	// 获取最新的历史记录
	promptID := task.ExecuterTaskID
	historyItem, err := comfyClient.GetPromptHistory(promptID)
	if err != nil {
		return errors.Wrap(err, "获取ComfyUI历史记录失败")
	}

	// 从ComfyUI下载完结果文件后：更新进度至90%
	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 90, 0); err != nil {
		zlog.LogWithContext(ctx).Error("更新下载完成进度失败", zap.Error(err))
	}

	// 处理输出结果
	var resultURL, webpURL string
	var width, height int
	var aspectRatio float64

	// 遍历输出节点
	for nodeID, outputs := range historyItem.Outputs {
		zlog.LogWithContext(ctx).Info("处理输出节点",
			zap.String("taskID", task.TaskID),
			zap.Int("nodeID", nodeID))

		for _, output := range outputs {
			if output.Type == "temp" {
				continue
			}
			// 根据文件类型判断是图片还是视频
			if strings.HasSuffix(strings.ToLower(output.Filename), ".gif") {
				// 处理GIF
				return e.handleGifOutput(ctx, comfyClient, output, task)
			} else {
				// 处理图片
				imageData, err := comfyClient.GetImage(output)
				if err != nil {
					return errors.Wrapf(err, "下载图片失败: %s", output.Filename)
				}

				// 解码图片获取尺寸
				img, _, err := image.Decode(bytes.NewReader(*imageData))
				if err != nil {
					return errors.Wrap(err, "解码图片失败")
				}
				width, height = img.Bounds().Dx(), img.Bounds().Dy()
				aspectRatio = float64(height) / float64(width)

				// 上传到OSS
				fileName := fmt.Sprintf("comfyui_%s_%s", task.TaskID, output.Filename)
				ossURL, err := utils.UploadToOSS(fileName, *imageData)
				if err != nil {
					return errors.Wrap(err, "上传图片到OSS失败")
				}
				resultURL = ossURL

				// 将主文件上传到OSS成功后：更新进度至95%
				if err := updateTaskProgressAndSendEvent(context.Background(), e.taskDao, e.rdb, task, 95, 0); err != nil {
					zlog.LogWithContext(ctx).Error("更新上传完成进度失败", zap.Error(err))
				}

				// 生成WebP缩略图
				webpData, err := utils.CompressImageWithMaxSize(img, 512, imaging.JPEG)
				if err != nil {
					zlog.LogWithContext(ctx).Warn("生成WebP缩略图失败", zap.Error(err))
				} else {
					webpFileName := fmt.Sprintf("comfyui_%s_%s.webp", task.TaskID,
						strings.TrimSuffix(output.Filename, filepath.Ext(output.Filename)))
					webpURL, err = utils.UploadToOSS(webpFileName, webpData)
					if err != nil {
						zlog.LogWithContext(ctx).Warn("上传WebP缩略图失败", zap.Error(err))
					}
				}

				// 只处理第一个成功的输出
				break
			}
		}
		if resultURL != "" {
			break
		}
	}

	if resultURL == "" {
		return errors.New("未能从ComfyUI输出中找到任何有效的图片结果")
	}

	// 构建任务结果
	resultData := model.PictureTaskResult{
		ResultURL:        resultURL,
		UserShowImageURL: webpURL,
		Width:            width,
		Height:           height,
		AspectRatio:      aspectRatio,
	}
	resultJSON, err := json.Marshal(resultData)
	if err != nil {
		return errors.Wrap(err, "序列化结果失败")
	}

	// 所有处理完成，准备写入最终结果时：更新进度至100%，并将任务状态设置为COMPLETED
	task.ResultJSON = string(resultJSON)
	task.UnreadTaskResult = true
	task.CompletedAt = sql.NullTime{Time: time.Now(), Valid: true}
	if err := updateTaskProgressAndSendEvent(context.Background(), e.taskDao, e.rdb, task, 100, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED); err != nil {
		return errors.Wrap(err, "更新最终任务结果失败")
	}

	// 评估图片
	// if e.evaluateService != nil {
	// 	if err := e.evaluateService.EvaluatePicture(ctx, task.TaskID, resultURL); err != nil {
	// 		zlog.LogWithContext(ctx).Error("评估图片失败",
	// 			zap.String("taskID", task.TaskID),
	// 			zap.Error(err))
	// 	}
	// }

	return nil
}

// RetrySubmission 重试任务提交
func (e *ComfyUIExecutor) RetrySubmission(ctx context.Context, task *model.PictureTask) error {
	if task.Executer != ComfyUIExecutorName {
		return errors.New("任务执行器不匹配，无法重试")
	}

	// 解析原始参数
	var taskParams map[string]string
	if err := json.Unmarshal([]byte(task.ParamJSON), &taskParams); err != nil {
		return errors.Wrap(err, "解析任务参数失败")
	}

	// 重新执行Process逻辑
	return e.Process(ctx, task.TaskID, taskParams)
}

// IsSubmissionErrorRetryable 判断提交错误是否可重试
func (e *ComfyUIExecutor) IsSubmissionErrorRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// 网络相关错误可重试
	if strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "初始化ComfyUI客户端失败") ||
		strings.Contains(errStr, "提交任务到ComfyUI失败") {
		return true
	}

	// 工作流相关错误不可重试
	if strings.Contains(errStr, "加载工作流失败") ||
		strings.Contains(errStr, "工作流参数") {
		return false
	}

	// 默认可重试
	return true
}

// 辅助方法

// createComfyClient 创建ComfyUI客户端
func (e *ComfyUIExecutor) createComfyClient(ctx context.Context) (*client.ComfyClient, error) {
	comfyUIConf := conf.GlobalConfig.ComfyUI
	auth := &client.Auth{
		Username: comfyUIConf.Username,
		Password: comfyUIConf.Password,
	}

	// 优先从Redis获取 object_info JSON 缓存
	redisKey := constants.ConfigKeyComfyUIObjectInfo
	objectInfoJSON, err := e.rdb.Get(redisKey).Result()
	if err == redis.Nil {
		// Redis缓存未命中，从数据库获取
		zlog.LogWithContext(ctx).Info("ComfyUI object_info Redis缓存未命中，从数据库获取")
		objectInfoJSON, err = e.configDao.GetConfigValue(constants.ConfigKeyComfyUIObjectInfo)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("从数据库获取 comfyui_object_info 配置失败, 将回退到API调用", zap.Error(err))
			objectInfoJSON = ""
		} else {
			// 异步将数据写入Redis缓存
			go func() {
				err := e.rdb.Set(redisKey, objectInfoJSON, 24*time.Hour).Err()
				if err != nil {
					// 此处只记录日志，不影响主流程
					zlog.Logger.Error("设置ComfyUI object_info到Redis失败", zap.Error(err))
				}
			}()
		}
	} else if err != nil {
		zlog.LogWithContext(ctx).Error("从Redis获取ComfyUI object_info失败", zap.Error(err))
		// 如果Redis出错，直接从数据库降级获取
		objectInfoJSON, err = e.configDao.GetConfigValue(constants.ConfigKeyComfyUIObjectInfo)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("Redis失败后，从数据库获取 comfyui_object_info 配置也失败", zap.Error(err))
			objectInfoJSON = ""
		}
	}

	options := &client.ComfyClientOptions{
		UseHttps:       true,
		Timeout:        600,
		MaxRetry:       30,
		BaseDelay:      15 * time.Second,
		MaxDelay:       20 * time.Second,
		Auth:           auth,
		ObjectInfoJSON: objectInfoJSON,
	}

	return client.NewComfyClientWithOptions(comfyUIConf.Server, comfyUIConf.Port, nil, options), nil
}

// getWorkflowJson 获取工作流JSON
func (e *ComfyUIExecutor) getWorkflowJson(ctx context.Context, task *model.PictureTask) (string, error) {
	workflow, err := e.pictureForgeDao.GetWorkflow(ctx, db.GetDB(), task.WorkflowID)
	if err != nil {
		return "", errors.Wrap(err, "获取工作流信息失败")
	}

	if workflow.WorkflowJson == "" {
		return "", errors.New("工作流JSON为空")
	}

	return workflow.WorkflowJson, nil
}

// setWorkflowParams 设置工作流参数
func (e *ComfyUIExecutor) setWorkflowParams(ctx context.Context, workflowJson string, params map[string]string) string {
	// 设置随机种子
	randomSeed := utils.GenerateRandomNumber(16)
	workflowJson = strings.ReplaceAll(workflowJson, "{random_seed}", strconv.Itoa(int(randomSeed)))
	workflowJson = strings.ReplaceAll(workflowJson, "{InputPrompt}", params["InputPrompt"])
	// 处理图片尺寸参数
	for _, imagePath := range params {
		if !strings.HasPrefix(imagePath, "http") {
			continue
		}
		imagePath = utils.CleanURL(imagePath)
		uploadInfo, err := e.uploadDao.GetImgUrlWithout(ctx, db.GetDB(), imagePath)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to get upload info",
				zap.String("imagePath", imagePath),
				zap.Error(err))
			continue
		}
		newWidth, newHeight := utils.ScaleImageDimensions(int(uploadInfo.PhotoWidth), int(uploadInfo.PhotoHeight))
		if uploadInfo != nil && uploadInfo.PhotoWidth > 0 {
			workflowJson = strings.ReplaceAll(workflowJson, "{img_width}", strconv.Itoa(newWidth))
			workflowJson = strings.ReplaceAll(workflowJson, "{img_height}", strconv.Itoa(newHeight))
			break
		}
	}

	// 兜底处理，设置默认尺寸
	workflowJson = strings.ReplaceAll(workflowJson, "{img_width}", "1080")
	workflowJson = strings.ReplaceAll(workflowJson, "{img_height}", "1080")

	return workflowJson
}

// processInputImages 处理输入图片
func (e *ComfyUIExecutor) processInputImages(ctx context.Context, comfyClient *client.ComfyClient, graph *graphapi.Graph, params map[string]string, taskID string) error {
	for key, imagePath := range params {
		if strings.HasPrefix(imagePath, "http") {
			if err := e.uploadImageToComfyUI(ctx, comfyClient, graph, key, imagePath, taskID); err != nil {
				return errors.Wrapf(err, "上传图片%s失败", key)
			}
		}
	}
	return nil
}

// uploadImageToComfyUI 上传图片到ComfyUI
func (e *ComfyUIExecutor) uploadImageToComfyUI(ctx context.Context, comfyClient *client.ComfyClient, graph *graphapi.Graph, key, imagePath, taskID string) error {
	// 下载图片
	resp, err := e.httpClient.Get(imagePath)
	if err != nil {
		return errors.Wrap(err, "下载图片失败")
	}
	defer resp.Body.Close()

	// 解码图片
	img, err := imaging.Decode(resp.Body)
	if err != nil {
		return errors.Wrap(err, "解码图片失败")
	}

	// 压缩图片
	imgCompressBytes, err := utils.CompressImageWithMaxSize(img, 1080, imaging.PNG)
	if err != nil {
		return errors.Wrap(err, "压缩图片失败")
	}

	img, err = imaging.Decode(bytes.NewReader(imgCompressBytes))
	if err != nil {
		return errors.Wrap(err, "解码压缩图片失败")
	}

	// 找到对应的LoadImage节点
	loadImageNode := graph.GetFirstNodeWithTitle(key)
	if loadImageNode == nil {
		return errors.Errorf("找不到LoadImage节点: %s", key)
	}

	prop := loadImageNode.GetPropertyWithName("choose file to upload")
	if prop == nil {
		return errors.New("找不到图片上传属性")
	}

	uploadProp, _ := prop.ToImageUploadProperty()
	if uploadProp == nil {
		return errors.New("无法获取上传属性")
	}

	filename := fmt.Sprintf("%s_%s.png", taskID, key)
	_, err = comfyClient.UploadImage(img, filename, false, client.InputImageType, "", uploadProp)
	if err != nil {
		return errors.Wrap(err, "上传图片到ComfyUI失败")
	}

	return nil
}

// handleGifOutput 处理GIF输出
func (e *ComfyUIExecutor) handleGifOutput(ctx context.Context, comfyClient *client.ComfyClient, output client.DataOutput, task *model.PictureTask) error {
	// 下载GIF
	gifData, err := comfyClient.GetVideo(output)
	if err != nil {
		return errors.Wrap(err, "下载GIF失败")
	}

	// 上传到OSS
	fileName := fmt.Sprintf("comfyui_%s_%s", task.TaskID, output.Filename)
	ossURL, err := utils.UploadToOSS(fileName, *gifData)
	if err != nil {
		return errors.Wrap(err, "上传GIF到OSS失败")
	}

	// 生成首帧作为缩略图
	frameURL, err := e.generateGifThumbnail(ctx, *gifData, task.TaskID)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("生成GIF缩略图失败", zap.Error(err))
	}

	// 构建结果
	resultData := model.PictureTaskResult{
		ResultURL:             ossURL,
		VideoFrameURL:         frameURL,
		VideoFrameAspectRatio: 1.0, // 默认比例，实际应该计算
	}

	resultJSON, err := json.Marshal(resultData)
	if err != nil {
		return errors.Wrap(err, "序列化GIF结果失败")
	}

	// 更新任务
	task.ResultJSON = string(resultJSON)
	task.UnreadTaskResult = true

	if err := updateTaskProgressAndSendEvent(ctx, e.taskDao, e.rdb, task, 100, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED); err != nil {
		return errors.Wrap(err, "更新GIF任务结果失败")
	}

	return nil
}

// generateGifThumbnail 生成GIF缩略图
func (e *ComfyUIExecutor) generateGifThumbnail(ctx context.Context, gifData []byte, taskID string) (string, error) {
	// 解析GIF获取第一帧
	img, err := utils.ExtractFirstFrameFromGIF(gifData)
	if err != nil {
		return "", errors.Wrap(err, "提取GIF首帧失败")
	}

	// 压缩首帧
	frameData, err := utils.CompressImageWithMaxSize(img, 512, imaging.JPEG)
	if err != nil {
		return "", errors.Wrap(err, "压缩首帧失败")
	}

	// 上传首帧
	frameFileName := fmt.Sprintf("comfyui_%s_frame.jpg", taskID)
	frameURL, err := utils.UploadToOSS(frameFileName, frameData)
	if err != nil {
		return "", errors.Wrap(err, "上传首帧失败")
	}

	return frameURL, nil
}

// handleSubmissionError 处理提交错误
func (e *ComfyUIExecutor) handleSubmissionError(ctx context.Context, task *model.PictureTask, err error, isRetryable bool) error {
	zlog.LogWithContext(ctx).Error("ComfyUI任务提交失败",
		zap.String("taskID", task.TaskID),
		zap.Error(err),
		zap.Bool("isRetryable", isRetryable))

	task.Error = err.Error()

	if isRetryable {
		task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING_SUBMISSION_RETRY)
	} else {
		if e.failureHandler != nil {
			if failErr := e.failureHandler.FailTask(context.Background(), task, err.Error()); failErr != nil {
				zlog.LogWithContext(ctx).Error("调用失败处理器失败", zap.Error(failErr))
			}
		} else {
			task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			task.Progress = -1
		}
	}

	if updateErr := e.taskDao.UpdateTask(context.Background(), task); updateErr != nil {
		zlog.LogWithContext(ctx).Error("更新任务状态失败", zap.Error(updateErr))
		return errors.Wrapf(err, "提交失败后更新任务状态也失败: %v", updateErr)
	}

	SendProgressEvent(task)
	return err
}
