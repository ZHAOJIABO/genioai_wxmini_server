package picture_generate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/dundunHa/comfy2go/client"
	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/conf"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/event"
	"va_visionai_server/internal/utils"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type ImageGenerator struct {
	taskDao         *dao.PictureTaskDao
	evaluateService *EvaluateService
	rdb             *redis.Client
	httpClient      *http.Client
	uploadDao       *dao.UploadDao
}

func NewImageGenerator(taskDao *dao.PictureTaskDao, evaluateService *EvaluateService, rdb *redis.Client, httpClient *http.Client, uploadDao *dao.UploadDao) *ImageGenerator {
	return &ImageGenerator{
		taskDao:         taskDao,
		evaluateService: evaluateService,
		rdb:             rdb,
		httpClient:      httpClient,
		uploadDao:       uploadDao,
	}
}

func (g *ImageGenerator) setWorkflowParams(ctx context.Context, workflowJson string, params map[string]string) string {
	randomSeed := utils.GenerateRandomNumber(16)
	workflowJson = strings.ReplaceAll(workflowJson, "{random_seed}", strconv.Itoa(int(randomSeed)))

	for _, imagePath := range params {
		if !strings.HasPrefix(imagePath, "http") {
			continue
		}
		imagePath = utils.CleanURL(imagePath)
		uploadInfo, err := g.uploadDao.GetImgUrlWithout(ctx, g.uploadDao.DB, imagePath)
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
	// 兜底去除占位
	workflowJson = strings.ReplaceAll(workflowJson, "{img_width}", "1080")
	workflowJson = strings.ReplaceAll(workflowJson, "{img_height}", "1080")

	return workflowJson
}
func (g *ImageGenerator) ApiGenerate(ctx context.Context, workflowJson string, taskID string, params map[string]string) error {
	task, err := g.taskDao.GetTask(context.TODO(), taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	paramJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}
	task.ParamJSON = string(paramJSON)
	if err := g.taskDao.UpdateTask(context.TODO(), task); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	projectID := task.ProjectID

	go func() {
		ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				zlog.LogWithContext(ctx).Error("generate image panic", zap.Any("recover", r))
				task.Error = fmt.Sprintf("internal error: %v", r)
				_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			}
		}()
		// 添加进度控制
		const (
			stageDownload = 2  // 下载阶段占总进度的20%
			stageProcess  = 2  // 处理阶段占总进度的40%
			stageGenerate = 86 // 生成阶段占总进度的30%
			stageUpload   = 10 // 上传阶段占总进度的10%
		)
		//currentStage := 0
		lastProgress := int32(0)
		// 生成阶段的计时器，用于动态更新进度
		var generateTimer *time.Timer
		// 进度更新函数
		updateProgress := func(stageProgress float64, stage int) {
			// 如果是失败状态，直接设置进度为-1
			if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
				if lastProgress != -1 {
					lastProgress = -1
					task.Progress = -1
					_ = g.taskDao.UpdateTask(context.Background(), task)
					DispatchUserEvent(pb.WatchEventType_WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS,
						&pb.TaskProgressEventData{
							PictureTaskId: task.TaskID,
							Progress:      -1,
						},
						projectID, task.UserID,
					)
				}
				return
			}
			baseProgress := float64(0)
			switch stage {
			case 1: // 下载阶段
				baseProgress = stageProgress * float64(stageDownload) / 100
			case 2: // 处理阶段
				baseProgress = float64(stageDownload) + stageProgress*float64(stageProcess)/100
			case 3: // 生成阶段
				if stageProgress >= 100 {
					// 生成完成，直接设置为90%
					baseProgress = float64(stageDownload+stageProcess) + float64(stageGenerate)*0.9
					// 停止计时器
					if generateTimer != nil {
						generateTimer.Stop()
					}
				} else {
					// 生成过程中，在4%到90%之间动态更新
					baseProgress = float64(stageDownload+stageProcess) + stageProgress*float64(stageGenerate)*0.9/100
				}
			case 4: // 上传阶段
				baseProgress = float64(stageDownload+stageProcess+stageGenerate) + stageProgress*float64(stageUpload)/100
			}

			newProgress := int32(baseProgress)
			if newProgress > lastProgress {
				lastProgress = newProgress
				task.Progress = newProgress
				_ = g.taskDao.UpdateTask(context.Background(), task)

				if newProgress < 100 {
					DispatchUserEvent(pb.WatchEventType_WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS,
						&pb.TaskProgressEventData{
							PictureTaskId: task.TaskID,
							Progress:      newProgress,
						},
						projectID, task.UserID,
					)
				}
			}
		}
		// 启动生成阶段的进度更新计时器
		generateTimer = time.NewTimer(time.Millisecond * 5000)
		go func() {
			startTime := time.Now()
			for range generateTimer.C {
				elapsed := time.Since(startTime)
				if elapsed >= time.Second*90 {
					generateTimer.Stop()
					return
				}
				// 计算当前进度百分比（0-100）
				progress := (elapsed.Seconds() / 90.0) * 100
				updateProgress(progress, 3) // 更新生成阶段进度
				generateTimer.Reset(time.Millisecond * 10000)
			}
		}()
		// 更新任务状态为处理中
		// task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING)
		// task.Progress = 10
		// if err := g.taskDao.UpdateTask(context.TODO(), task); err != nil {
		// 	zlog.LogWithContext(ctx).Error("failed to update task", zap.Error(err))
		// 	return
		// }
		// if task.Progress < 100 {
		// 	DispatchUserEvent(pb.WatchEventType_WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS,
		// 		&pb.TaskProgressEventData{
		// 			PictureTaskId: task.TaskID,
		// 			Progress:      task.Progress,
		// 		},
		// 		projectID, task.UserID,
		// 	)
		// }
		// 初始进度
		updateProgress(0, 1)
		// 从params获取原始图片并下载到本地
		var localImagePath string
		for _, imagePath := range params {
			if strings.HasPrefix(imagePath, "http") {
				// 创建临时文件
				tmpFileName := fmt.Sprintf("/tmp/image_%s.png", taskID)
				tmpFile, err := os.Create(tmpFileName)
				if err != nil {
					task.Error = fmt.Sprintf("failed to create temp file: %v", err)
					_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
					return
				}
				defer os.Remove(tmpFileName) // 确保临时文件会被删除

				// 下载图片
				resp, err := g.httpClient.Get(imagePath)
				updateProgress(20, 1) // 开始下载
				if err != nil {
					task.Error = fmt.Sprintf("failed to download image: %v", err)
					_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
					return
				}
				defer resp.Body.Close()

				// 保存到临时文件
				if _, err := io.Copy(tmpFile, resp.Body); err != nil {
					task.Error = fmt.Sprintf("failed to save image: %v", err)
					_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
					return
				}
				tmpFile.Close()
				localImagePath = tmpFileName
				updateProgress(100, 1) // 下载完成
				break
			}
		}

		if localImagePath == "" {
			task.Error = "no valid image found in params"
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)

		// 打开并添加图片文件
		imageFile, err := os.Open(localImagePath)
		updateProgress(0, 2) // 开始处理
		if err != nil {
			task.Error = fmt.Sprintf("failed to open image file: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		defer imageFile.Close()

		if err = utils.CreateFormFileWithMIME(writer, "image", localImagePath, imageFile); err != nil {
			task.Error = fmt.Sprintf("failed create image form file with MIME: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		//生成蒙版图片
		localMaskPath := fmt.Sprintf("/tmp/image_%s.png", taskID)
		err = utils.GenerateMaskImage(localImagePath, localMaskPath)
		if err != nil {
			task.Error = fmt.Sprintf("failed to generate mask image: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		defer os.Remove(localMaskPath)
		// 打开并添加蒙版文件
		maskFile, err := os.Open(localMaskPath)
		updateProgress(50, 2) // 处理到一半
		if err != nil {
			task.Error = fmt.Sprintf("failed to open mask file: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		defer maskFile.Close()

		if err = utils.CreateFormFileWithMIME(writer, "mask", localMaskPath, maskFile); err != nil {
			task.Error = fmt.Sprintf("failed create mask form file with MIME: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		updateProgress(75, 2) // 处理完成
		// 添加提示词
		writer.WriteField("prompt", workflowJson)

		// 读取原始图片尺寸
		origImg, err := imaging.Open(localImagePath)
		if err != nil {
			task.Error = fmt.Sprintf("failed to open image for size detection: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}

		widthImg := origImg.Bounds().Dx()
		heightImg := origImg.Bounds().Dy()
		aspectRatioImg := float64(widthImg) / float64(heightImg)

		// 根据宽高比决定输出尺寸
		var size string
		if aspectRatioImg >= 1.5 { // 横向图片
			size = "1536x1024"
		} else if aspectRatioImg <= 0.67 { // 纵向图片 (1/1.5 ≈ 0.67)
			size = "1024x1536"
		} else { // 接近正方形的图片
			size = "1024x1024"
		}

		writer.WriteField("size", size)
		writer.WriteField("quality", "high")
		updateProgress(100, 2) // 处理完成
		// 关闭writer
		writer.Close()

		req, err := http.NewRequestWithContext(ctxWithTimeout, "POST", conf.GlobalConfig.LlmConfig.GptImage.Endpoint, &body)
		if err != nil {
			task.Error = fmt.Sprintf("failed to create request: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		req.Header.Set("Authorization", "Bearer "+conf.GlobalConfig.LlmConfig.GptImage.APIKey)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		// 发送请求
		// 生成图片时
		//updateProgress(0, 3) // 开始生成
		resp, err := g.httpClient.Do(req)
		if err != nil {
			if ctxWithTimeout.Err() == context.DeadlineExceeded {
				task.Error = "request timeout"
			} else {
				task.Error = fmt.Sprintf("failed to send request: %v", err)
			}
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		defer resp.Body.Close()

		//updateProgress(50, 3) // 生成到一半
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			task.Error = fmt.Sprintf("failed to read response body: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		//updateProgress(100, 3) // 生成完成
		// 再解析 JSON
		var result struct {
			Data []struct {
				B64JSON string `json:"b64_json"`
			} `json:"data"`
		}
		if err := json.Unmarshal(bodyBytes, &result); err != nil {
			task.Error = fmt.Sprintf("failed to decode response: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		updateProgress(0, 4) // 开始上传
		if len(result.Data) == 0 {
			task.Error = "no image generated"
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}

		// 解码 base64 图片数据
		imgData, err := base64.StdEncoding.DecodeString(result.Data[0].B64JSON)
		if err != nil {
			task.Error = fmt.Sprintf("failed to decode base64 image: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}

		// 处理图片并上传
		img, err := imaging.Decode(bytes.NewReader(imgData))
		if err != nil {
			task.Error = fmt.Sprintf("failed to decode image: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		updateProgress(25, 4) // 上传到一半
		width, height, aspectRatio := utils.ImageAspectRatio(img)
		aspectRatio = math.Round(aspectRatio*100) / 100

		// 生成缩略图
		compressedImgData, err := utils.CompressImageWithMaxSize(img, 360, imaging.JPEG)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to compress image", zap.Error(err))
		}

		// 上传原图
		fileName := fmt.Sprintf("com.domob.piclib/generated/%s/%s.jpg", task.UserID, taskID)
		// 检查图片大小是否超过1MB
		imgData, err = utils.CompressImage(imgData)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to zip image", zap.Error(err))
		}

		ossURL, err := utils.UploadToOSS(fileName, imgData)
		if err != nil {
			task.Error = fmt.Sprintf("failed to upload to oss: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		updateProgress(50, 4) // 上传完成
		// 上传缩略图
		compressedFileName := fmt.Sprintf("com.domob.piclib/generated/%s/%s-low.jpg", task.UserID, taskID)
		_, err = utils.UploadToOSS(compressedFileName, compressedImgData)
		if err != nil {
			task.Error = fmt.Sprintf("failed to upload to oss: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}

		// 生成并上传 WebP 格式
		webpFileName := strings.TrimSuffix(compressedFileName, ".jpg") + ".webp"
		webpCompressedImgData, err := utils.ConvertToWebp(img)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to convert to webp", zap.Error(err))
		}
		updateProgress(75, 4)
		webpOssURL, err := utils.UploadToOSS(webpFileName, webpCompressedImgData)
		if err != nil {
			task.Error = fmt.Sprintf("failed to upload to oss: %v", err)
			_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, -1, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}
		updateProgress(100, 4) // 上传完成
		// 更新任务结果
		resultData := model.PictureTaskResult{
			ResultURL:   ossURL,
			Width:       width,
			Height:      height,
			AspectRatio: aspectRatio,
		}
		resultJSON, _ := json.Marshal(resultData)
		task.ResultJSON = string(resultJSON)
		task.UnreadTaskResult = true
		_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, 100, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED)

		// 评估图片
		if err := g.evaluateService.EvaluatePicture(ctxWithTimeout, taskID, resultData.ResultURL); err != nil {
			zlog.LogWithContext(ctx).Error("Failed to evaluate picture",
				zap.String("TaskID", taskID),
				zap.Error(err))
		}
		// 发送完成事件
		DispatchUserEvent(pb.WatchEventType_WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS,
			&pb.TaskProgressEventData{
				PictureTaskId: task.TaskID,
				Progress:      100,
				PictureInfo: &pb.PictureInfo{
					Url:          ossURL,
					ThumbnailUrl: webpOssURL,
					AspectRatio:  float32(aspectRatio),
				},
			},
			projectID, task.UserID,
		)
	}()

	return nil
}

func (g *ImageGenerator) Generate(ctx context.Context, workflowJson string, taskID string, params map[string]string) error {
	workflowJson = g.setWorkflowParams(ctx, workflowJson, params)
	comfyUIConf := conf.GlobalConfig.ComfyUI
	auth := &client.Auth{
		Username: comfyUIConf.Username,
		Password: comfyUIConf.Password,
	}

	options := &client.ComfyClientOptions{
		UseHttps:  true,
		Timeout:   600,
		MaxRetry:  30,
		BaseDelay: 15 * time.Second,
		MaxDelay:  20 * time.Second,
		Auth:      auth,
	}

	c := client.NewComfyClientWithOptions(comfyUIConf.Server, comfyUIConf.Port, nil, options)
	task, err := g.taskDao.GetTask(context.TODO(), taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	paramJSON, err := json.Marshal(params)
	projectID := task.ProjectID
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}
	task.ParamJSON = string(paramJSON)
	if err := g.taskDao.UpdateTask(context.TODO(), task); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	progressKey := constants.RedisKeyPictureTaskProgress + ":" + taskID

	go func() {
		defer func() {
			c.HttpClient().CloseIdleConnections()
			if r := recover(); r != nil {
				zlog.LogWithContext(ctx).Error("generate image panic", zap.Any("recover", r))
				task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
				task.Error = fmt.Sprintf("internal error: %v", r)
				_ = g.taskDao.UpdateTask(context.Background(), task)
			}
		}()

		if !c.IsInitialized() {
			if err := c.Init(); err != nil {
				zlog.LogWithContext(ctx).Error("InitComfyClientError", zap.Error(err))
				task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
				task.Error = fmt.Sprintf("failed to initialize client: %v", err)
				_ = g.taskDao.UpdateTask(context.Background(), task)
				return
			}
		}

		graph, missing, err := c.NewGraphFromJsonString(workflowJson)
		if err != nil {
			task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			task.Error = fmt.Sprintf("failed to load workflow: %v", err)
			_ = g.taskDao.UpdateTask(context.Background(), task)
			// 发送完成事件
			DispatchUserEvent(pb.WatchEventType_WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS,
				&pb.TaskProgressEventData{
					PictureTaskId: task.TaskID,
					Progress:      -1,
				},
				projectID, task.UserID,
			)
			return
		}

		totalNodes := len(graph.Nodes)
		completedNodes := 0
		samplerProgress := float64(0)
		var currentNodeTitle string
		calculateTotalProgress := func() int32 {
			nodeProgress := float64(completedNodes) / float64(totalNodes) * 50.0
			samplerComponent := samplerProgress * 0.5
			totalProgress := nodeProgress + samplerComponent

			if totalProgress > 99 {
				totalProgress = 99
			}

			return int32(totalProgress)
		}

		if missing != nil && len(*missing) > 0 {
			zlog.LogWithContext(ctx).Warn("workflow has missing nodes", zap.Strings("missing", *missing))
		}

		for key, imagePath := range params {
			if strings.HasPrefix(imagePath, "http") {
				resp, err := g.httpClient.Get(imagePath)
				if err != nil {
					task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
					task.Error = fmt.Sprintf("failed to download image: %v", err)
					_ = g.taskDao.UpdateTask(context.Background(), task)
					return
				}
				defer resp.Body.Close()

				img, err := imaging.Decode(resp.Body)
				if err != nil {
					task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
					task.Error = fmt.Sprintf("failed to decode image: %v", err)
					_ = g.taskDao.UpdateTask(context.Background(), task)
					return
				}
				imgCompressBytes, err := utils.CompressImageWithMaxSize(img, 1080, imaging.PNG)
				if err != nil {
					task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
					task.Error = fmt.Sprintf("failed to compress image: %v", err)
					_ = g.taskDao.UpdateTask(context.Background(), task)
					return
				}
				img, err = imaging.Decode(bytes.NewReader(imgCompressBytes))
				if err != nil {
					task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
					task.Error = fmt.Sprintf("failed to decode image: %v", err)
					_ = g.taskDao.UpdateTask(context.Background(), task)
					return
				}

				loadImageNode := graph.GetFirstNodeWithTitle(key)
				if loadImageNode == nil {
					task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
					task.Error = "missing Load Image node"
					_ = g.taskDao.UpdateTask(context.Background(), task)
					return
				}

				prop := loadImageNode.GetPropertyWithName("choose file to upload")
				if prop == nil {
					task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
					task.Error = "missing image property in Load Image node"
					_ = g.taskDao.UpdateTask(context.Background(), task)
					return
				}

				uploadProp, _ := prop.ToImageUploadProperty()
				if uploadProp == nil {
					task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
					task.Error = fmt.Sprintf("failed to get upload property: %v", err)
					_ = g.taskDao.UpdateTask(context.Background(), task)
					return
				}

				filename := fmt.Sprintf("%s_%s.png", taskID, key)
				_, err = c.UploadImage(img, filename, false, client.InputImageType, "", uploadProp)
				if err != nil {
					task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
					task.Error = fmt.Sprintf("failed to upload image: %v", err)
					_ = g.taskDao.UpdateTask(context.Background(), task)
					return
				}
			}
		}
		item, err := c.QueuePrompt(graph)
		if err != nil {
			task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			task.Error = fmt.Sprintf("failed to queue prompt: %v", err)
			_ = g.taskDao.UpdateTask(context.Background(), task)
			return
		}

		timeout := time.After(10 * time.Minute)

		for continueLoop := true; continueLoop; {
			if err := c.CheckConnection(); err != nil {
				zlog.LogWithContext(ctx).Error("CheckComfyClientError", zap.Error(err))
			}
			c.HttpClient().CloseIdleConnections()

			select {
			case msg, ok := <-item.Messages:
				if !ok {
					task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
					task.Error = "message channel closed unexpectedly"
					_ = g.taskDao.UpdateTask(context.Background(), task)
					return
				}

				switch msg.Type {
				case "started":
					qm := msg.ToPromptMessageStarted()
					zlog.LogWithContext(ctx).Info("workflow started",
						zap.String("promptID", qm.PromptID))
					task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING)
					g.taskDao.UpdateTask(context.TODO(), task)

				case "executing":
					qm := msg.ToPromptMessageExecuting()
					currentNodeTitle = qm.Title
					zlog.LogWithContext(ctx).Debug("executing node", zap.String("title", currentNodeTitle))
					completedNodes++
					progress := calculateTotalProgress()
					if progress > task.Progress {
						task.Progress = progress
						if err := g.taskDao.UpdateTask(context.TODO(), task); err != nil {
							zlog.LogWithContext(ctx).Error("failed to update task", zap.Error(err))
						}

						if progress < 100 {
							DispatchUserEvent(pb.WatchEventType_WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS,
								&pb.TaskProgressEventData{
									PictureTaskId: task.TaskID,
									Progress:      progress,
								},
								projectID, task.UserID,
							)
						}
					}

				case "progress":
					qm := msg.ToPromptMessageProgress()
					if strings.Contains(currentNodeTitle, "KSampler") {
						samplerProgress = (float64(qm.Value) / float64(qm.Max)) * 100
						progress := calculateTotalProgress()

						if progress > task.Progress {
							task.Progress = progress
							if err := g.taskDao.UpdateTask(context.TODO(), task); err != nil {
								zlog.LogWithContext(ctx).Error("failed to update task", zap.Error(err))
							}

							if progress < 100 {
								DispatchUserEvent(pb.WatchEventType_WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS,
									&pb.TaskProgressEventData{
										PictureTaskId: task.TaskID,
										Progress:      progress,
									},
									projectID, task.UserID,
								)
							}
						}
					}

					if err := g.rdb.Set(progressKey, task.Progress, time.Hour*24).Err(); err != nil {
						zlog.LogWithContext(ctx).Error("failed to set progress in redis", zap.Error(err))
					}
				case "stopped":
					qm := msg.ToPromptMessageStopped()
					if qm.Exception != nil {
						task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
						task.Error = fmt.Sprintf("workflow error: %v", qm.Exception)
						_ = g.taskDao.UpdateTask(context.TODO(), task)
						return
					}
					continueLoop = false

				case "data":
					qm := msg.ToPromptMessageData()
					_ = g.handleDataMessage(ctx, c, qm, task, projectID, progressKey)
				}

			case <-timeout:
				task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
				task.Error = "workflow execution timed out"
				_ = g.taskDao.UpdateTask(context.Background(), task)
				return
			}
		}
	}()

	return nil
}

func DispatchUserEvent(eventType pb.WatchEventType, data interface{}, projectID, userID string) {
	event.GetEventRegistry().DispatchToUser(eventType, data, projectID, userID)
}

// handleDataMessage 根据 key 分别处理 images 和 gifs
func (g *ImageGenerator) handleDataMessage(
	ctx context.Context,
	c *client.ComfyClient,
	qm *client.PromptMessageData,
	task *model.PictureTask,
	projectID, progressKey string,
) error {
	if imgs, ok := qm.Data["images"]; ok {
		if err := g.handleImageOutputs(ctx, c, imgs, task, projectID, progressKey); err != nil {
			return err
		}
	}
	if gifs, ok := qm.Data["gifs"]; ok {
		if err := g.handleGifOutputs(ctx, c, gifs, task, projectID, progressKey); err != nil {
			return err
		}
	}
	return nil
}

// handleImageOutputs 处理静态图片：下载、解码、压缩、上传、更新任务、发送事件
func (g *ImageGenerator) handleImageOutputs(
	ctx context.Context,
	c *client.ComfyClient,
	outputs []client.DataOutput,
	task *model.PictureTask,
	projectID, progressKey string,
) error {
	for _, output := range outputs {
		data, err := c.GetImage(output)
		if err != nil {
			continue
		}
		img, _, err := image.Decode(bytes.NewReader(*data))
		if err != nil {
			zlog.LogWithContext(ctx).Error("解码图片失败", zap.Error(err))
			continue
		}
		// 压缩到1080再重解码
		buf1080, err := utils.CompressImageWithMaxSize(img, 1080, imaging.JPEG)
		if err != nil {
			zlog.LogWithContext(ctx).Error("压缩(1080)失败", zap.Error(err))
			continue
		}
		img1080, err := imaging.Decode(bytes.NewReader(buf1080))
		if err != nil {
			zlog.LogWithContext(ctx).Error("重解码(1080)失败", zap.Error(err))
			continue
		}
		w, h, ar := utils.ImageAspectRatio(img1080)
		ar = math.Round(ar*100) / 100

		// 上传原图
		origName := fmt.Sprintf("%s/generated/%s/%s.jpg", constants.ProjectIdPicLib, task.UserID, task.TaskID)
		url, err := utils.UploadToOSS(origName, *data)
		if err != nil {
			g.updateTaskError(ctx, task, fmt.Sprintf("上传原图失败: %v", err))
			return err
		}
		// 上传缩略图
		buf360, _ := utils.CompressImageWithMaxSize(img1080, 360, imaging.JPEG)
		thumbName := fmt.Sprintf("%s/generated/%s/%s-low.jpg", constants.ProjectIdPicLib, task.UserID, task.TaskID)
		_, err = utils.UploadToOSS(thumbName, buf360)
		if err != nil {
			g.updateTaskError(ctx, task, fmt.Sprintf("上传缩略图失败: %v", err))
			return err
		}
		// 上传 WebP 缩略图
		webpBuf, _ := utils.ConvertToWebp(img1080)
		webpName := strings.TrimSuffix(thumbName, ".jpg") + ".webp"
		webpURL, err := utils.UploadToOSS(webpName, webpBuf)
		if err != nil {
			g.updateTaskError(ctx, task, fmt.Sprintf("上传WebP失败: %v", err))
			return err
		}

		// 更新任务状态并发送事件
		result := model.PictureTaskResult{
			ResultURL:   url,
			Width:       w,
			Height:      h,
			AspectRatio: ar,
		}
		bj, _ := json.Marshal(result)
		task.ResultJSON = string(bj)
		task.UnreadTaskResult = true
		_ = updateTaskProgressAndSendEvent(context.Background(), g.taskDao, g.rdb, task, 100, pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED)
		_ = g.evaluateService.EvaluatePicture(context.Background(), task.TaskID, url)
		DispatchUserEvent(
			pb.WatchEventType_WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS,
			&pb.TaskProgressEventData{
				PictureTaskId: task.TaskID,
				Progress:      100,
				PictureInfo: &pb.PictureInfo{
					Url:          url,
					ThumbnailUrl: webpURL,
					AspectRatio:  float32(ar),
				},
			},
			projectID, task.UserID,
		)
		return nil
	}
	return nil
}

// handleGifOutputs 处理 GIF：直接上传、读取第一帧获取尺寸、更新任务、发送事件
func (g *ImageGenerator) handleGifOutputs(
	ctx context.Context,
	c *client.ComfyClient,
	outputs []client.DataOutput,
	task *model.PictureTask,
	projectID, progressKey string,
) error {
	for _, output := range outputs {
		data, err := c.GetVideo(output)
		if err != nil {
			continue
		}
		name := fmt.Sprintf("%s/generated/%s/%s.mp4", constants.ProjectIdPicLib, task.UserID, task.TaskID)
		url, err := utils.UploadToOSS(name, *data)
		if err != nil {
			g.updateTaskError(ctx, task, fmt.Sprintf("上传MP4失败: %v", err))
			return err
		}
		// 尝试用第一帧获取尺寸和比例
		img, _, err := image.Decode(bytes.NewReader(*data))
		var w, h int
		var ar float64
		if err == nil {
			w, h, ar = utils.ImageAspectRatio(img)
			ar = math.Round(ar*100) / 100
		}
		result := model.PictureTaskResult{
			ResultURL:   url,
			Width:       w,
			Height:      h,
			AspectRatio: ar,
		}
		bj, _ := json.Marshal(result)
		task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED)
		task.ResultJSON = string(bj)
		task.Progress = 100
		task.UnreadTaskResult = true
		_ = g.taskDao.UpdateTask(context.Background(), task)
		g.rdb.Del(progressKey)
		_ = g.evaluateService.EvaluatePicture(context.Background(), task.TaskID, url)
		DispatchUserEvent(
			pb.WatchEventType_WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS,
			&pb.TaskProgressEventData{
				PictureTaskId: task.TaskID,
				Progress:      100,
				PictureInfo: &pb.PictureInfo{
					Url:          url,
					ThumbnailUrl: "",
					AspectRatio:  float32(ar),
				},
			},
			projectID, task.UserID,
		)
		return nil
	}
	return nil
}

// updateTaskError 统一更新 task 错误状态
func (g *ImageGenerator) updateTaskError(ctx context.Context, task *model.PictureTask, msg string) {
	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
	task.Error = msg
	_ = g.taskDao.UpdateTask(ctx, task)
}
