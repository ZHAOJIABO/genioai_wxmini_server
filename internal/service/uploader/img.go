package uploader

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"

	"va_visionai_server/conf"
	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/llm"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"

	_ "golang.org/x/image/webp"
)

type imgUploader struct {
	*BaseUploader
	Body        []byte
	FileType    vai.FileType
	UserID      string
	fileName    string
	width       int32
	height      int32
	llmFactory  *llm.LLMFactoryImpl
	promptDao   *dao.PromptDao
	configDao   *dao.ConfigDao
	AskQuestion bool
}

func NewImgUploader(ctx context.Context, body []byte, fileType vai.FileType, userID string, fileName string, uploadDao *dao.UploadDao, llmFactory *llm.LLMFactoryImpl, askQuestion bool) *imgUploader {
	return &imgUploader{
		BaseUploader: NewBaseUploader(ctx, uploadDao),
		Body:         body,
		FileType:     fileType,
		UserID:       userID,
		fileName:     fileName,
		llmFactory:   llmFactory,
		promptDao:    dao.NewPromptDao(uploadDao.DB),
		configDao:    dao.NewConfigDao(),
		AskQuestion:  askQuestion,
	}
}

func (h *imgUploader) Process(ctx context.Context) (*vai.UploadResponse, error) {
	// Generate MD5 checksum of the original file
	hasher := md5.New()
	hasher.Write(h.Body)
	fileMd5 := hex.EncodeToString(hasher.Sum(nil))

	// 检查是否已存在该图片的记录及问题和道德审查信息
	fileInfo, err := h.uploadDao.GetFileInfoByHash(ctx, fileMd5)
	var questions []string
	var review float64 = 10.0 // 默认值
	var photoQuestion string
	var needGenerateQuestions bool = true
	var threshold float64 = 0.8       // 默认阈值
	var isContentApproved bool = true // 默认通过审核

	if err == nil && fileInfo != nil {
		// 文件信息存在
		if fileInfo.PhotoQuestion != "" && fileInfo.MoralReview != "" {
			// 已有问题和道德审查信息，直接使用
			needGenerateQuestions = false
			photoQuestion = fileInfo.PhotoQuestion
			if err := json.Unmarshal([]byte(photoQuestion), &questions); err != nil {
				zlog.LogWithContext(ctx).Error("unmarshal questions failed", zap.Error(err))
				needGenerateQuestions = true
			}

			if reviewScore, err := strconv.ParseFloat(fileInfo.MoralReview, 64); err == nil {
				review = reviewScore
			}
		}
	}

	// 获取道德审查阈值（无论是否需要生成问题）
	thresholdStr, err := h.configDao.GetConfigValue("moral_review_threshold")
	if err == nil {
		if tempThreshold, err := strconv.ParseFloat(thresholdStr, 64); err == nil {
			threshold = tempThreshold
		}
	}

	// 如果是图片类型且需要生成问题，检查道德审查
	if needGenerateQuestions && h.AskQuestion {
		var err error
		language := common.CtxGetStrValue(ctx, constants.CtxLang)
		if language == "" {
			language = "en"
		}
		questions, review, err = h.generateQuestionsAndReview(ctx, string(h.Body), language)
		if err != nil {
			// 只记录错误，不影响上传流程
			zlog.LogWithContext(ctx).Error("generate questions and review failed, but continue upload", zap.Error(err))
		} else {
			// 存储问题为JSON字符串
			questionsJSON, err := json.Marshal(questions)
			if err != nil {
				zlog.LogWithContext(ctx).Error("marshal questions failed", zap.Error(err))
			} else {
				photoQuestion = string(questionsJSON)
			}
		}
	}

	// review为0时，默认通过审核
	if review == 0 {
		isContentApproved = true
	} else {
		isContentApproved = review >= threshold
	}
	if !isContentApproved {
		zlog.LogWithContext(ctx).Warn("Content did not pass moral review",
			zap.Float64("review", review),
			zap.Float64("threshold", threshold),
			zap.String("fileMd5", fileMd5))
	}

	currentDate := time.Now().Format("200601")
	projectID := h.projectID
	filename := fmt.Sprintf("%s/upload/%s/img/%s.jpg", projectID, currentDate, fileMd5)
	filenameLowQuality := fmt.Sprintf("%s/upload/%s/img/%s-low.jpg", projectID, currentDate, fileMd5)

	var originalFileUrl string
	var imageWidth, imageHeight int32
	var lowQualityUrl string

	// 只有通过道德审查的内容才上传到OSS
	if isContentApproved {
		originalFileUrl, err = utils.UploadToOSS(filename, h.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to upload original file to OSS: %v", err)
		}

		if h.FileType == vai.FileType_FT_IMAGE {
			imageWidth, imageHeight, lowQualityUrl, err = h.processImage(filenameLowQuality)
			if err != nil {
				return nil, fmt.Errorf("failed to process image: %v", err)
			}
		}
	} else {
		// 对于未通过审核的图片，只获取尺寸信息，不上传
		if h.FileType == vai.FileType_FT_IMAGE {
			// 解码图片以获取尺寸
			img, err := imaging.Decode(bytes.NewReader(h.Body))
			if err != nil {
				zlog.LogWithContext(ctx).Error("failed to decode image for dimensions", zap.Error(err))
			} else {
				imageWidth = int32(img.Bounds().Dx())
				imageHeight = int32(img.Bounds().Dy())
			}
		}
	}

	filetypeStr := convertFileType(h.FileType)
	var imgTraceId string
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		traceIDList := md.Get(constants.CtxTraceID)
		if len(traceIDList) > 0 {
			imgTraceId = traceIDList[0]
		}
	}
	uploaderRole := common.CtxGetStrValue(ctx, constants.CtxUploaderRole)
	// 无论是否通过审核，都更新文件附加信息
	if err := h.upsertFileInfo(context.Background(), fileMd5, imageWidth, imageHeight, photoQuestion, review); err != nil {
		zlog.LogWithContext(ctx).Error("UpsertFileInfoError", zap.Error(err))
	}
	// 持久化数据，包括审核状态
	uploadFile := &model.UploadFile{
		ProjectID:    h.projectID,
		UploaderRole: uploaderRole,
		FileName:     h.fileName,
		FileType:     filetypeStr,
		OSSAddr:      originalFileUrl, // 未通过审核时为空字符串
		Hash:         fileMd5,
		UserID:       h.UserID,
		TraceId:      imgTraceId,
	}
	if err := h.uploadDao.CreateUploadFile(context.Background(), nil, uploadFile, uploaderRole); err != nil {
		zlog.LogWithContext(ctx).Error("PersistenceImgError", zap.Error(err))
	}
	response := &vai.UploadResponse{
		FileUrl:       originalFileUrl,
		FileName:      fileMd5,
		FileMd5:       fileMd5,
		ImageWidth:    imageWidth,
		ImageHeight:   imageHeight,
		LowQualityUrl: lowQualityUrl,
		Question:      questions,
		FileId:        strconv.FormatUint(uint64(uploadFile.ID), 10),
	}

	// 未通过审核时，添加响应头信息
	if !isContentApproved {
		response.ResponseHeader = &vai.ResponseHeader{
			Code:               vai.StatusCode_UPLOAD_CONTENT_SENSITIVE,
			ResponseStatusCode: vai.ResponseStatusCode_RESPONSE_STATUS_CODE_UPLOAD_CONTENT_SENSITIVE,
			Msg:                constants.ErrMsgContentSafePolicy,
		}
	}

	return response, nil
}

// Helper function to process image, get dimensions, and upload low-quality image
func (h *imgUploader) processImage(fileName string) (int32, int32, string, error) {
	// Decode the image
	img, err := imaging.Decode(bytes.NewReader(h.Body))
	if err != nil {
		return 0, 0, "", fmt.Errorf("failed to decode image: %v", err)
	}

	// Get original dimensions
	h.width = int32(img.Bounds().Dx())
	h.height = int32(img.Bounds().Dy())

	// Resize image if necessary
	var lowQualityUrl string
	// 生成webp格式
	webpFileName := strings.TrimSuffix(fileName, "-low.jpg") + "-low.webp"
	webpBuf := new(bytes.Buffer)
	err = webp.Encode(webpBuf, img, &webp.Options{Lossless: false, Quality: 80})
	if err != nil {
		return 0, 0, "", fmt.Errorf("failed to encode webp image: %v", err)
	}

	// 上传webp格式到OSS
	_, err = utils.UploadToOSS(webpFileName, webpBuf.Bytes())
	if err != nil {
		return 0, 0, "", fmt.Errorf("failed to upload webp image to OSS: %v", err)
	}

	lowQualityImg := imaging.Resize(img, 1080, 0, imaging.Lanczos)

	// Encode the low-quality image to buffer
	buf := new(bytes.Buffer)
	err = imaging.Encode(buf, lowQualityImg, imaging.JPEG, imaging.JPEGQuality(80))
	if err != nil {
		return 0, 0, "", fmt.Errorf("failed to encode low-quality image: %v", err)
	}

	// Upload the low-quality image to OSS
	lowQualityUrl, err = utils.UploadToOSS(fileName, buf.Bytes())
	if err != nil {
		return 0, 0, "", fmt.Errorf("failed to upload low-quality image to OSS: %v", err)
	}

	return h.width, h.height, lowQualityUrl, nil
}

// func (h *imgUploader) persistence(fileType, fileUrl, hash, traceId, uploaderRole string) error {
// 	uploadFile := &model.UploadFile{
// 		ProjectID:    h.projectID,
// 		UploaderRole: uploaderRole,
// 		FileName:     h.fileName,
// 		FileType:     fileType,
// 		OSSAddr:      fileUrl,
// 		Hash:         hash,
// 		UserID:       h.UserID,
// 		TraceId:      traceId,
// 	}
// 	return h.uploadDao.CreateUploadFile(h.ctx, nil, uploadFile, uploaderRole)
// }

func (h *imgUploader) upsertFileInfo(ctx context.Context, fileMd5 string, imageWidth, imageHeight int32, photoQuestion string, review float64) error {
	fileInfo := &model.UploadFileInfo{
		Hash:          fileMd5,
		PhotoWidth:    imageWidth,
		PhotoHeight:   imageHeight,
		PhotoQuestion: photoQuestion,
		MoralReview:   fmt.Sprintf("%.2f", review),
	}
	return h.uploadDao.UpsertFileInfo(ctx, fileInfo)
}

// generateQuestionsAndReview 生成问题和道德审查结果
func (h *imgUploader) generateQuestionsAndReview(ctx context.Context, imagePath string, language string) (questions []string, review float64, err error) {
	// 使用工具函数清理URL中的查询参数
	if h.llmFactory == nil {
		return nil, 0, errors.New("llmFactory is not initialized")
	}
	serverRegion := conf.GlobalConfig.VisionAiServerConfig.Region
	//  从库中获取模型
	photoQuestionModel, err := h.configDao.GetConfigValue(serverRegion + "_generate_photo_question_model")
	if err != nil {
		zlog.LogWithContext(ctx).Error("get photo question model failed", zap.Error(err))
		return nil, 0, fmt.Errorf("get model failed: %w", err)
	}

	if strings.Contains(strings.ToLower(photoQuestionModel), "doubao") {
		imagePath = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(h.Body)
	} else if strings.Contains(strings.ToLower(photoQuestionModel), "gemini") {
		imagePath = string(h.Body)
	} else {
		imagePath = utils.CleanURL(imagePath)
	}

	ctx, llmModel, err := h.llmFactory.CreateHandler(ctx, vai.Model(vai.Model_value[photoQuestionModel]))
	if err != nil {
		return nil, 0, fmt.Errorf("create model failed: %w", err)
	}
	zlog.LogWithContext(ctx).Info("generate questions and review model", zap.String("model", photoQuestionModel))

	prompt, err := h.promptDao.GetPrompt(constants.SystemPictureQuestions, language)
	if err != nil {
		// 如果指定语言获取不到，则降级到 en
		if language != constants.EN {
			prompt, err = h.promptDao.GetPrompt(constants.SystemPictureQuestions, constants.EN)
			if err != nil {
				return nil, 0, fmt.Errorf("get prompt failed for default en: %w", err)
			}
		} else {
			return nil, 0, fmt.Errorf("get prompt failed: %w", err)
		}
	}
	prompt.Content = strings.ReplaceAll(prompt.Content, "{visionai-replace-lang}", language)

	promptId := common.CtxGetStrValue(ctx, constants.CtxPromptID)
	if promptId != "" {
		uploadPrompt, err := h.promptDao.GetPrompt(promptId, language)
		if err != nil {
			prompt.Content = strings.ReplaceAll(prompt.Content, "{visionai-replace-prompt-describe}", "")
		} else {
			prompt.Content = strings.ReplaceAll(prompt.Content, "{visionai-replace-prompt-describe}", uploadPrompt.Describe)
			zlog.LogWithContext(ctx).Info("generate questions and review prompt replace")
		}
	}

	history := []model.MessageHistory{{
		Sender:   "user",
		URLs:     []string{imagePath},
		FileType: vai.MessageType_MT_IMAGE,
		Content:  prompt.Content,
	}}

	output := make(chan []string)
	cancelCh := make(chan struct{})
	defer func() {
		utils.SafeCloseChan(cancelCh)
		utils.SafeCloseChan(output)
	}()

	req := &vai.ChatMessageSendRequest{
		Message: &vai.Message{SystemPrompt: prompt.Content},
	}

	var (
		modelResp struct {
			Questions []string `json:"questions"`
			Review    float64  `json:"review"`
		}
		fullResponse strings.Builder
		gotReply     bool
	)
	errCh := make(chan error, 1)
	go func() {
		errCh <- llmModel.Process(ctx, common.LLMProcessParams{
			Request:      req,
			StreamServer: nil,
			History:      history,
			Output:       output,
			CancelCh:     cancelCh,
		})
	}()

	timer := time.NewTimer(115 * time.Second)
	defer timer.Stop()

ProcessLoop:
	for {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-timer.C:
			return nil, 0, errors.New("generate questions timeout after 30s")
		case err := <-errCh:
			if err != nil {
				return nil, 0, errors.New("process image failed")
			}
			if !gotReply {
				return nil, 0, errors.New("no response received")
			}
			break ProcessLoop
		case chunks := <-output:
			gotReply = true
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(30 * time.Second)
			for _, chunk := range chunks {
				fullResponse.WriteString(chunk)
			}
		}
	}

	responseStr := fullResponse.String()
	if responseStr == "" {
		return nil, 0, errors.New("empty response from model")
	}
	zlog.LogWithContext(ctx).Info("Generate questions and review response", zap.String("response", responseStr), zap.Any("body size", len(h.Body)))
	responseStr, err = utils.ExtractJSONFromLLMResponse(responseStr)
	zlog.LogWithContext(ctx).Info("Generate questions and review response after extract json", zap.String("response", responseStr))
	if err != nil {
		zlog.LogWithContext(ctx).Error("extract json from response failed", zap.Error(err), zap.String("response", responseStr))
		return nil, 0, fmt.Errorf("extract json from response failed: %w", err)
	}
	zlog.LogWithContext(ctx).Info("Generate questions and review response", zap.String("response", responseStr))
	if err := json.Unmarshal([]byte(responseStr), &modelResp); err != nil {
		zlog.LogWithContext(ctx).Error("unmarshal response failed", zap.Error(err), zap.String("response", responseStr))
	}

	if len(modelResp.Questions) == 0 {
		return nil, 0, errors.New("no valid questions generated")
	}
	if len(modelResp.Questions) > 3 {
		modelResp.Questions = modelResp.Questions[:3]
	}

	return modelResp.Questions, modelResp.Review, nil
}

// GenerateQuestionsAndReview 是一个导出方法，用于生成问题和道德审查结果
// 此方法为测试提供方便，可以直接调用内部的generateQuestionsAndReview方法
func (h *imgUploader) GenerateQuestionsAndReview(ctx context.Context, imagePath string, language string) ([]string, float64, error) {
	return h.generateQuestionsAndReview(ctx, imagePath, language)
}
