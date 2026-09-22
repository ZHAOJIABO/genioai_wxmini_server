package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	chatcommon "va_visionai_server/internal/service/chat/common"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// LLMOptions 配置LLM调用参数
type LLMOptions = chatcommon.LLMOptions

// 默认LLM调用选项
var DefaultLLMOptions = LLMOptions{
	GlobalTimeout: 120 * time.Second,
	GapTimeout:    30 * time.Second,
}

// StreamCallback 定义流式回调函数
type StreamCallback func(ctx context.Context, token string) error

// ChatLLMService 聊天LLM服务
type ChatLLMService struct {
	llmFactory         common.LLMFactory
	modelDao           *dao.ModelDao
	uploadDao          *dao.UploadDao
	promptDao          *dao.PromptDao
	transformers       []ModelTransformer  // 模型转换器列表
	modelSelector      *ModelSelector      // 模型选择器
	modelResolverChain *ModelResolverChain // 模型解析器链
}

// NewChatLLMService 创建聊天LLM服务
func NewChatLLMService(
	llmFactory common.LLMFactory,
	modelDao *dao.ModelDao,
	uploadDao *dao.UploadDao,
	promptDao *dao.PromptDao,
	transformers ...ModelTransformer,
) *ChatLLMService {
	// 创建模型选择器
	modelSelector := NewModelSelector(modelDao)

	// 创建模型解析器
	defaultResolver := NewDefaultModelResolver(modelSelector)
	nameResolver := NewModelNameResolver()
	videoResolver := NewVideoModelResolver()

	// 构建模型解析器链
	resolverChain := NewModelResolverChain(
		videoResolver,   // 首先处理视频消息
		nameResolver,    // 然后处理模型名称
		defaultResolver, // 最后使用默认模型
	)

	return &ChatLLMService{
		llmFactory:         llmFactory,
		modelDao:           modelDao,
		uploadDao:          uploadDao,
		promptDao:          promptDao,
		transformers:       transformers,
		modelSelector:      modelSelector,
		modelResolverChain: resolverChain,
	}
}

// StreamProcess 执行LLM流式处理并通过callback返回结果
func (s *ChatLLMService) StreamProcess(
	ctx context.Context,
	req *vai.ChatMessageSendRequest,
	history []model.MessageHistory,
	callback func(ctx context.Context, token string) error,
	opts chatcommon.LLMOptions,
) (string, error) {
	// 确保超时设置有效
	if opts.GlobalTimeout == 0 {
		opts.GlobalTimeout = DefaultLLMOptions.GlobalTimeout
	}
	if opts.GapTimeout == 0 {
		opts.GapTimeout = DefaultLLMOptions.GapTimeout
	}

	// 视频消息强制使用Gemini Flash
	messageType := req.GetMessage().GetMessageType()
	modelID := opts.InitialModel
	if messageType == vai.MessageType_MT_VIDEO {
		modelID = vai.Model_MODEL_GEMINI_2_0_FLASH
		zlog.LogWithContext(ctx).Info("视频消息检测到，强制使用 Gemini Flash 模型",
			zap.String(constants.CtxModelName, modelID.String()))
	}

	// 创建LLM处理器
	ctx, llmHandler, err := s.llmFactory.CreateHandler(ctx, modelID)
	if err != nil {
		return "", err
	}

	// 清理URL
	cleanURLs := make([]string, 0, len(req.GetMessage().GetUrls()))
	for _, url := range req.GetMessage().GetUrls() {
		cleanURLs = append(cleanURLs, utils.CleanURL(url))
	}
	req.GetMessage().Urls = cleanURLs

	// 包装回调函数
	wrappedCallback := func(ctx context.Context, outputs []string) error {
		outInfo := strings.Join(outputs, "")
		// 如果回调函数为空，则不执行回调
		// 如果输出为空，则不执行回调
		if outInfo == "" || callback == nil {
			return nil
		}
		return callback(ctx, outInfo)
	}

	// 设置LLM查询选项
	llmOpts := utils.LLMQueryOptions{
		GlobalTimeout:     opts.GlobalTimeout,
		MessageGapTimeout: opts.GapTimeout,
		ImageContent:      opts.ImageContent,
		Callback:          wrappedCallback,
		ThinkingConfig:    opts.ThinkingConfig,
	}

	// 执行LLM查询
	return utils.ExecuteLLMQueryWithGapMonitor(ctx, llmHandler, req, history, llmOpts)
}
func (s *ChatLLMService) ResolveModel(
	ctx context.Context,
	req *vai.ChatMessageSendRequest,
	history []model.MessageHistory,
) (context.Context, vai.Model, string, error) {
	for _, transformer := range s.transformers {
		transformer.Transform(ctx, req)
	}

	lang := common.CtxGetStrValue(ctx, constants.CtxLang)
	if lang == "" {
		lang = constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	}

	transformCtx := NewTransformContext(ctx, req.GetMessage(), lang)

	isVision := isVisionChat(history)
	transformCtx.SetIsVisual(isVision)

	chainErr := s.modelResolverChain.ResolveModel(transformCtx)
	if chainErr != nil {
		zlog.LogWithContext(ctx).Error("模型解析链执行过程中出现错误", zap.Error(chainErr))
	}

	finalModelID := transformCtx.GetModelResult()
	ctx = context.WithValue(ctx, constants.CtxModelID, finalModelID)
	ctx = context.WithValue(ctx, constants.CtxModelName, finalModelID.String())

	if req.GetMessage().GetMessageType() == vai.MessageType_MT_VIDEO {
		return ctx, finalModelID, "", nil
	}

	imageContent := ""
	var err error

	if isVision {
		imageContent, err = s.processImageForVisionChat(ctx, history, finalModelID)
		if err != nil {
			zlog.LogWithContext(ctx).Error("处理图片失败", zap.Error(err))
			return ctx, finalModelID, "", nil
		}
	}

	return ctx, finalModelID, imageContent, nil
}

// processImageForVisionChat 处理视觉聊天中的图片
func (s *ChatLLMService) processImageForVisionChat(
	ctx context.Context,
	history []model.MessageHistory,
	modelID vai.Model,
) (string, error) {
	// 获取最后一条图片消息的URL
	var lastImageURL string
	var userPrompt string
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].FileType == vai.MessageType_MT_IMAGE && len(history[i].URLs) > 0 {
			lastImageURL = history[i].URLs[0]
			// 查找与图片消息配对的用户文本消息作为提示
			if i+1 < len(history) && history[i+1].Sender == "user" && history[i+1].FileType == vai.MessageType_MT_TEXT {
				userPrompt = history[i+1].Content
			} else if history[i].Content != "" {
				userPrompt = history[i].Content
			}
			break
		}
	}

	// 如果没有图片URL，返回空字符串
	if lastImageURL == "" {
		return "", nil
	}

	// 处理图片内容
	return s.checkAndProcessImageUrl(ctx, modelID, lastImageURL, userPrompt)
}

// checkAndProcessImageUrl 处理图片URL并返回处理后的内容
func (s *ChatLLMService) checkAndProcessImageUrl(
	ctx context.Context,
	modelID vai.Model,
	imageURL string,
	userPrompt string,
) (string, error) {
	// // 验证图片URL
	// if imageURL == "" {
	// 	return "", errors.New("图片URL为空")
	// }

	// // 清理URL
	// cleanURL := utils.CleanURL(imageURL)

	// // 获取图片内容
	// imageBytes, err := s.uploadDao.GetImageContentByURL(ctx, cleanURL)
	// if err != nil {
	// 	return "", err
	// }

	// // 根据模型类型处理图片内容
	// // 这里可以针对不同模型有不同的处理逻辑
	// zlog.LogWithContext(ctx).Info("处理图片",
	// 	zap.String("model", modelID.String()),
	// 	zap.String("url", cleanURL),
	// 	zap.Int("image_size", len(imageBytes)))

	// // 根据图片内容和用户提示生成的处理后内容
	// // 具体实现可能涉及图片编码、格式转换等
	// processedContent := utils.EncodeImageContentForLLM(imageBytes, modelID, userPrompt)

	// return processedContent, nil
	// 如果URL为空，直接返回
	if imageURL == "" {
		zlog.LogWithContext(ctx).Info("Please Re-upload the image")
		return "", errors.New("Please Re-upload the image")
	}

	// 通过model_dao首先检查模型是否支持视觉
	modelInfo, err := s.modelDao.GetModel(ctx, modelID.String())
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取模型信息失败",
			zap.String("model_id", modelID.String()),
			zap.Error(err))
		return "", fmt.Errorf("获取模型信息失败: %w", err)
	}

	// 如果模型支持视觉，直接返回
	if modelInfo.IsVision {
		zlog.LogWithContext(ctx).Info("模型支持视觉能力，无需预处理图片",
			zap.String("model_id", modelID.String()),
			zap.String("model_name", modelInfo.ModelID))
		return "", nil
	}

	// 通过upload_dao检查URL有效性，并获取可能已缓存的图片内容
	imageURL = utils.CleanURL(imageURL)
	imgInfo, err := s.uploadDao.GetImgUrlWithout(ctx, s.uploadDao.DB, imageURL)
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取图片信息失败",
			zap.String("url", imageURL),
			zap.Error(err))
		return "", fmt.Errorf("获取图片信息失败: %w", err)
	}
	info, err := s.uploadDao.GetFileInfoByHash(ctx, imgInfo.Hash)
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取图片信息失败",
			zap.String("url", imageURL),
			zap.Error(err))
		return "", fmt.Errorf("获取图片信息失败: %w", err)
	}
	// 如果已有缓存的图片内容，直接返回
	if info.ImageInfo != "" {
		zlog.LogWithContext(ctx).Info("使用缓存的图片分析内容",
			zap.String("url", imageURL))
		return info.ImageInfo, nil
	}

	// 模型不支持视觉且没有缓存内容，调用processImageContent获取图片信息
	zlog.LogWithContext(ctx).Info("模型不支持视觉能力且无缓存内容，开始预处理图片",
		zap.String("model_id", modelID.String()),
		zap.String("model_name", modelInfo.ModelName),
		zap.String("url", imageURL))

	imageContent, err := s.processImageContent(ctx, imageURL, userPrompt, modelID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("处理图片内容失败",
			zap.String("url", imageURL),
			zap.Error(err))
		return "", fmt.Errorf("处理图片内容失败: %w", err)
	}

	// 将处理结果更新到数据库
	if imageContent != "" {
		go func() {
			updateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			fileInfo := &model.UploadFileInfo{
				Hash:      imgInfo.Hash,
				ImageInfo: imageContent,
			}
			if err := s.uploadDao.UpsertFileInfo(updateCtx, fileInfo); err != nil {
				zlog.Logger.Error("更新图片内容到数据库失败",
					zap.String("url", imageURL),
					zap.Error(err))
			} else {
				zlog.Logger.Info("成功更新图片内容到数据库",
					zap.String("url", imageURL))
			}
		}()
	}

	return imageContent, nil
}

// isVisionChat 检查消息历史是否包含图片
func isVisionChat(history []model.MessageHistory) bool {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].FileType == vai.MessageType_MT_IMAGE || history[i].FileType == vai.MessageType_MT_VIDEO {
			return true
		}
	}
	return false
}

// isMacOSRequest 检查请求是否来自macOS设备
func isMacOSRequest(header *vai.RequestHeader) bool {
	if header == nil || header.GetDevice() == nil {
		return false
	}
	device := header.GetDevice()
	// 检查Os字段是否为3（macOS）
	return device.GetOs() == 3 || strings.Contains(strings.ToLower(device.GetOsv()), "macos")
}

// processImageContent 使用视觉模型获取图片信息，然后将信息传递给非视觉模型进行回答
func (s *ChatLLMService) processImageContent(
	ctx context.Context,
	imagePath string,
	userPrompt string,
	nonVisionModelID vai.Model,
) (fullResponse string, err error) {
	// 第一步：使用视觉模型获取图片信息
	ctx, visionModel, err := s.llmFactory.CreateHandler(ctx, vai.Model_MODEL_GEMINI_2_0_FLASH)
	if err != nil {
		return "", fmt.Errorf("创建视觉模型失败: %w", err)
	}

	// 从数据库中读取视觉分析提示词
	prompt, err := s.promptDao.GetPrompt(constants.SystemPictureVisionAnalysis, "zh")
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取视觉分析提示词失败",
			zap.Error(err),
			zap.String(constants.CtxPromptID, constants.SystemPictureVisionAnalysis))
		return "", fmt.Errorf("获取视觉分析提示词失败: %w", err)
	}

	// 构建提示词，指导视觉模型详细描述图像
	visionPrompt := prompt.Content

	if userPrompt != "" {
		visionPrompt = fmt.Sprintf("%s\n同时，请特别关注与以下问题相关的内容：%s", visionPrompt, userPrompt)
	}

	history := []model.MessageHistory{{
		Sender:   "user",
		URLs:     []string{imagePath},
		FileType: vai.MessageType_MT_IMAGE,
		Content:  visionPrompt,
	}}

	visionOutput := make(chan []string)
	visionCancelCh := make(chan struct{})
	defer func() {
		utils.SafeCloseChan(visionCancelCh)
		utils.SafeCloseChan(visionOutput)
	}()

	req := &vai.ChatMessageSendRequest{
		Message: &vai.Message{SystemPrompt: visionPrompt},
	}

	var visionResponseBuilder strings.Builder
	var gotVisionReply bool

	visionErrCh := make(chan error, 1)
	go func() {
		visionErrCh <- visionModel.Process(ctx, common.LLMProcessParams{
			Request:      req,
			StreamServer: nil,
			History:      history,
			Output:       visionOutput,
			CancelCh:     visionCancelCh,
		})
	}()

	visionTimer := time.NewTimer(60 * time.Second)
	defer visionTimer.Stop()

ProcessVisionLoop:
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-visionTimer.C:
			return "", errors.New("获取图像信息超时（60秒）")
		case err := <-visionErrCh:
			if err != nil {
				return "", fmt.Errorf("处理图像失败: %w", err)
			}
			if !gotVisionReply {
				return "", errors.New("未收到视觉模型响应")
			}
			break ProcessVisionLoop
		case chunks := <-visionOutput:
			gotVisionReply = true
			if !visionTimer.Stop() {
				select {
				case <-visionTimer.C:
				default:
				}
			}
			visionTimer.Reset(60 * time.Second)
			for _, chunk := range chunks {
				visionResponseBuilder.WriteString(chunk)
			}
		}
	}

	imageInfo := visionResponseBuilder.String()
	if imageInfo == "" {
		return "", errors.New("视觉模型返回的图像信息为空")
	}

	zlog.LogWithContext(ctx).Info("视觉模型分析图像结果", zap.String("image_info", imageInfo))

	return imageInfo, nil
}
