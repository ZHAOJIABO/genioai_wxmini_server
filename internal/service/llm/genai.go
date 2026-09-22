package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
	"go.uber.org/zap"
	"google.golang.org/genai"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type GenaiClient struct {
	client    *genai.Client
	voiceDao  *dao.VoiceDao
	uploadDao *dao.UploadDao
}

// NewGenaiClientWithCredentialsFile 使用凭证文件创建 Gemini 客户端
func NewGenaiClientWithCredentialsFile(credentialsFilePath string, voiceDao *dao.VoiceDao, uploadDao *dao.UploadDao) (*GenaiClient, error) {
	ctx := context.Background()

	if _, err := os.Stat(credentialsFilePath); err != nil {
		return nil, fmt.Errorf("凭证文件不存在或无法访问: %v", err)
	}

	scopes := []string{
		"https://www.googleapis.com/auth/cloud-platform",
	}

	tokenProvider, err := credentials.DetectDefault(&credentials.DetectOptions{
		CredentialsFile: credentialsFilePath,
		Scopes:          scopes,
	})
	if err != nil {
		zlog.LogWithContext(ctx).Error("使用 credentials.DetectDefault 加载凭据失败", zap.Error(err))
		return nil, fmt.Errorf("加载凭据失败: %w", err)
	}

	jsonBytes, err := os.ReadFile(credentialsFilePath)
	if err != nil {
		zlog.LogWithContext(ctx).Error("读取凭据文件JSON内容失败", zap.Error(err))
		return nil, fmt.Errorf("读取凭据文件失败: %w", err)
	}

	authOptions := &auth.CredentialsOptions{
		TokenProvider: tokenProvider,
		JSON:          jsonBytes,
	}

	authCredentials := auth.NewCredentials(authOptions)

	clientConfig := &genai.ClientConfig{
		Project:     "bluemedia-db",
		Location:    "us-central1",
		Backend:     genai.BackendVertexAI,
		Credentials: authCredentials,
	}

	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		zlog.LogWithContext(ctx).Error("创建 genai 客户端失败", zap.Error(err))
		return nil, fmt.Errorf("创建 genai 客户端失败: %w", err)
	}

	return &GenaiClient{
		client:    client,
		voiceDao:  voiceDao,
		uploadDao: uploadDao,
	}, nil
}
func (gc *GenaiClient) ChatStream(ctx context.Context, modelName string, prompt string, videoData []byte, urls []string, historyMessage []*genai.Content, lastMessage model.MessageHistory, thinkingConfig *common.ThinkingConfig) iter.Seq2[*genai.GenerateContentResponse, error] {
	var systemInstructionContent *genai.Content
	if prompt != "" {
		systemInstructionContent = &genai.Content{
			Parts: []*genai.Part{{Text: prompt}},
		}
	}

	chatConfig := &genai.GenerateContentConfig{
		TopK:              genai.Ptr(float32(40)),
		TopP:              genai.Ptr(float32(0.95)),
		MaxOutputTokens:   4096,
		ResponseMIMEType:  "text/plain",
		SystemInstruction: systemInstructionContent,
	}
	if thinkingConfig != nil {
		chatConfig.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: thinkingConfig.IncludeThoughts,
			ThinkingBudget:  thinkingConfig.ThinkingBudget,
		}
	}

	if len(videoData) > 0 {
		chatConfig.Temperature = genai.Ptr(float32(0.5))
	}

	currentUserParts := []*genai.Part{}
	var userTextForCurrentMessage string

	if lastMessage.Content == "" && len(videoData) > 0 {
		userTextForCurrentMessage = prompt
	} else {
		userTextForCurrentMessage = lastMessage.Content
	}

	currentUserParts = append(currentUserParts, &genai.Part{Text: userTextForCurrentMessage})

	overallMimeTypeHint := "text/plain"

	if len(videoData) > 0 {
		currentUserParts = append(currentUserParts, &genai.Part{
			InlineData: &genai.Blob{
				MIMEType: "video/mp4",
				Data:     videoData,
			},
		})
		overallMimeTypeHint = "video/mp4"
	}

	if len(urls) > 0 {
		for _, url := range urls {
			var imgData []byte
			var err error
			if strings.HasPrefix(url, "http") {
				// nolint:bodyclose // Ensure body is closed in utils.Get if it opens one.
				_, imgData, err = utils.Get(ctx, url)
				if err != nil {
					zlog.LogWithContext(ctx).Error("读取图片数据失败", zap.Error(err), zap.String("url", url))
					continue
				}
			} else {
				imgData = []byte(url)
				zlog.LogWithContext(ctx).Warn("将非HTTP URL作为原始数据处理（可能不是图片数据）")
			}

			truncateLength := 50
			logDataPreview := ""
			if len(imgData) > 0 {
				if len(imgData) > truncateLength {
					logDataPreview = string(imgData[:truncateLength]) + "..."
				} else {
					logDataPreview = string(imgData)
				}
			}
			zlog.LogWithContext(ctx).Info("图片数据", zap.Int("dataSize", len(imgData)), zap.String("dataPreview", logDataPreview))

			currentUserParts = append(currentUserParts, &genai.Part{
				InlineData: &genai.Blob{
					MIMEType: "image/jpeg",
					Data:     imgData,
				},
			})
			if overallMimeTypeHint == "text/plain" {
				overallMimeTypeHint = "image/jpeg"
			}
		}
	}

	allContents := make([]*genai.Content, 0, len(historyMessage)+1)
	allContents = append(allContents, historyMessage...)

	if len(currentUserParts) > 0 {
		currentContent := &genai.Content{
			Parts: currentUserParts,
			Role:  "user",
		}
		allContents = append(allContents, currentContent)
	} else {
		zlog.LogWithContext(ctx).Warn("当前用户消息部分为空 (currentUserParts is empty)", zap.Any("lastMessage", lastMessage))
	}

	zlog.LogWithContext(ctx).Info("向 Gemini 发送流式请求 (google.golang.org/genai)",
		zap.String("model", modelName),
		zap.String("overallMimeTypeHint", overallMimeTypeHint),
		zap.Int("videoDataSize", len(videoData)),
		zap.Int("urlsCount", len(urls)),
		zap.Int("historyMessagesCount", len(historyMessage)),
		zap.Int("currentUserPartsCount", len(currentUserParts)),
	)

	return gc.client.Models.GenerateContentStream(ctx, modelName, allContents, chatConfig)
}

// ThinkingProcessor 思考内容处理器
type ThinkingProcessor struct {
	thinkingStart   bool
	thinkingEnd     bool
	hiddenReasoning bool
}

// ProcessThinkingContent 处理思考内容格式化
func (tp *ThinkingProcessor) ProcessThinkingContent(content string, isThought bool) string {
	if isThought {
		if tp.hiddenReasoning {
			return ""
		}
		if !tp.thinkingStart {
			tp.thinkingStart = true
			return ">" + strings.ReplaceAll(content, "\n\n", "\n>\n>")
		} else {
			return strings.ReplaceAll(content, "\n\n", "\n>\n>")
		}
	} else if tp.thinkingStart && !tp.thinkingEnd {
		tp.thinkingEnd = true
		return "\n" + content
	}

	return content
}

// ProcessStreamResponse 使用 iter.Seq2 进行流式处理
func (gc *GenaiClient) ProcessStreamResponse(ctx context.Context, streamIter iter.Seq2[*genai.GenerateContentResponse, error], output model.Output) error {
	defer utils.SafeCloseChan(output)
	thinkingProcessor := &ThinkingProcessor{}
	// 辅助函数：检查上下文并发送数据
	sendWithContext := func(data []string) error {
		select {
		case output <- data:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	for resp, err := range streamIter {
		// 优先检查上下文取消
		if ctx.Err() != nil {
			zlog.LogWithContext(ctx).Warn("Context cancelled during stream processing", zap.Error(ctx.Err()))
			return ctx.Err()
		}

		if err != nil {
			zlog.LogWithContext(ctx).Error("Stream error during processing", zap.Error(err))
			return fmt.Errorf("stream error: %w", err)
		}

		// 跳过无效响应
		if resp == nil || len(resp.Candidates) == 0 {
			continue
		}

		candidate := resp.Candidates[0]
		if candidate.Content == nil {
			continue
		}

		// 处理内容部分
		for _, part := range candidate.Content.Parts {
			// 检查上下文取消
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// 发送空字符串标记（如果业务需要）
			if err := sendWithContext([]string{""}); err != nil {
				return err
			}

			// 处理文本内容，支持思考部分格式化
			if textualContent := part.Text; textualContent != "" {
				// 检查是否是思考部分并进行格式化
				processedContent := thinkingProcessor.ProcessThinkingContent(textualContent, part.Thought)

				if processedContent != "" {
					if err := sendWithContext([]string{processedContent}); err != nil {
						return err
					}
				}
			}
		}
	}

	zlog.LogWithContext(ctx).Info("Stream finished successfully")
	return nil
}

// Process 实现了 LlmHandler 接口，处理视频数据、Gemini 分析
func (gc *GenaiClient) Process(ctx context.Context, params common.LLMProcessParams) error {
	if params.Output == nil {
		return errors.New("output channel is required")
	}
	processCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var message model.MessageHistory
	if len(params.History) > 0 {
		message = params.History[len(params.History)-1]
	} else {
		message = model.MessageHistory{
			Content: params.Request.GetMessage().GetContent(),
		}
	}

	prompt := params.Request.GetMessage().GetSystemPrompt()
	modelName := common.GetModelDeploymentName(processCtx)

	var videoData []byte
	if len(message.VideoBlob) > 0 {
		videoData = message.VideoBlob
	}
	urls := message.URLs
	if message.URL != "" {
		urls = append(urls, message.URL)
	}

	zlog.LogWithContext(processCtx).Info("开始调用 Gemini 进行流式分析 (直接传输数据)",
		zap.String("model", modelName),
		zap.Int("videoSize", len(videoData)),
		zap.Int("urlSize", len(urls)),
	)

	go func() {
		select {
		case <-params.CancelCh:
			zlog.LogWithContext(processCtx).Info("接收到外部取消信号 (CancelCh), 取消 Process 上下文")
			cancel()
		case <-processCtx.Done():
			zlog.LogWithContext(processCtx).Debug("Process 上下文已完成或被取消，取消监听 goroutine 退出", zap.Error(processCtx.Err()))
		}
	}()

	projectID := common.GetProjectID(ctx)
	// historyMessage := gc.buildChatMessages(params.History, projectID)

	var chatHistoryForStream []*genai.Content
	if len(params.History) > 0 {
		chatHistoryForStream = gc.buildChatMessages(params.History[:len(params.History)-1], projectID)
	} else {
		chatHistoryForStream = []*genai.Content{}
	}
	thinkingConfig := params.ThinkingConfig
	streamIterator := gc.ChatStream(processCtx, modelName, prompt, videoData, urls, chatHistoryForStream, message, thinkingConfig)

	err := gc.ProcessStreamResponse(processCtx, streamIterator, params.Output)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			zlog.LogWithContext(processCtx).Warn("Gemini 流处理因上下文取消或超时而终止", zap.Error(err))
			return err
		}
		if processCtx.Err() != nil {
			zlog.LogWithContext(processCtx).Warn("Gemini 流处理因 Process 上下文取消而终止", zap.Error(processCtx.Err()), zap.NamedError("underlying_stream_error", err))
			return processCtx.Err()
		}
		zlog.LogWithContext(processCtx).Error("处理 Gemini 流响应时出错", zap.Error(err))
		return fmt.Errorf("error processing stream response: %w", err)
	}

	zlog.LogWithContext(processCtx).Info("Gemini 流处理成功完成")
	return nil
}

func (m *GenaiClient) buildChatMessages(history []model.MessageHistory, projectID string) []*genai.Content {
	historyMessage := []*genai.Content{}

	for _, msg := range history {
		content := m.handleVoiceMessage(msg.VoiceHash, msg.Content)
		senderRole := "user"
		if msg.Sender == "assistant" {
			senderRole = "model"
		}

		if content == "" && len(msg.URLs) == 0 && msg.URL == "" {
			zlog.Logger.Debug("Skipping history message with empty text content and no media", zap.String("voiceHash", msg.VoiceHash))
			continue
		}

		currentMsgParts := []*genai.Part{}
		currentMsgParts = append(currentMsgParts, &genai.Part{Text: content})

		if len(msg.URLs) > 0 {
			switch msg.FileType {
			case vai.MessageType_MT_IMAGE:
				imageParts := m.handleImageMessage(content, msg.URLs)
				if len(imageParts) > 0 {
					currentMsgParts = imageParts
				}
			case vai.MessageType_MT_FILE:
				if msg.URL != "" {
					fileParts, err := m.handleFileMessage(content, msg.URL, projectID)
					if err == nil && len(fileParts) > 0 {
						currentMsgParts = fileParts
					} else if err != nil {
						zlog.Logger.Error("Failed to handle file message in history, using original parts",
							zap.Error(err), zap.String("url", msg.URL))
					}
				}
			}
		} else if msg.URL != "" && msg.FileType == vai.MessageType_MT_FILE {
			fileParts, err := m.handleFileMessage(content, msg.URL, projectID)
			if err == nil && len(fileParts) > 0 {
				currentMsgParts = fileParts
			} else if err != nil {
				zlog.Logger.Error("Failed to handle single file message in history, using original parts",
					zap.Error(err), zap.String("url", msg.URL))
			}
		}

		if len(currentMsgParts) > 0 {
			hasActualContent := false
			for _, p := range currentMsgParts {
				if p.Text != "" || p.InlineData != nil {
					hasActualContent = true
					break
				}
			}

			if hasActualContent {
				message := genai.Content{
					Role:  senderRole,
					Parts: currentMsgParts,
				}
				historyMessage = append(historyMessage, &message)
			} else {
				zlog.Logger.Debug("Skipping history message as it resolved to no actual content parts", zap.Any("msg", msg))
			}
		}
	}
	return historyMessage
}

func (m *GenaiClient) handleVoiceMessage(voiceHash string, content string) string {
	if voiceHash == "" {
		return content
	}
	if voiceInfo, err := m.voiceDao.GetByFileHash(voiceHash); err == nil && voiceInfo.Content != "" {
		return voiceInfo.Content
	}
	return content
}

func (m *GenaiClient) handleImageMessage(content string, urls []string) []*genai.Part {
	itemMsgParts := []*genai.Part{}
	itemMsgParts = append(itemMsgParts, &genai.Part{Text: content})

	for _, v := range urls {
		v = utils.GetLowQualityImageURL(v)
		// nolint:bodyclose // Ensure body is closed in utils.Get if it opens one.
		_, imgData, err := utils.Get(context.TODO(), v)
		if err != nil {
			zlog.Logger.Error("Get Image Error for history message", zap.Error(err), zap.String("url", v))
			continue
		}
		itemMsgParts = append(itemMsgParts, &genai.Part{
			InlineData: &genai.Blob{
				MIMEType: "image/jpeg",
				Data:     imgData,
			},
		})
	}
	return itemMsgParts
}

func (m *GenaiClient) handleFileMessage(content, url, projectID string) ([]*genai.Part, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	fileInfo, err := m.uploadDao.GetByFileUrl(ctx, db.GetDB(), projectID, url, vai.UploaderRole_UPLOADER_ROLE_USER)
	if err != nil {
		zlog.Logger.Error("Get File Error for history message", zap.Error(err), zap.String("url", url))
		return nil, err
	}

	httpClient := http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", fileInfo.ContentURL, nil)
	if err != nil {
		zlog.Logger.Error("Create File Request Error for history", zap.Error(err), zap.String("url", fileInfo.ContentURL))
		return nil, err
	}

	fileContentBody, err := httpClient.Do(req)
	if err != nil {
		zlog.Logger.Error("Get File Content Error for history message", zap.Error(err), zap.String("contentURL", fileInfo.ContentURL))
		return nil, err
	}
	defer fileContentBody.Body.Close()

	fileContentReader := io.LimitReader(fileContentBody.Body, 128*1024)
	fileData, err := io.ReadAll(fileContentReader)
	if err != nil {
		zlog.Logger.Error("Read File IO Error for history message", zap.Error(err))
		return nil, err
	}

	fileParts := make([]*genai.Part, 0, 2)
	fileParts = append(fileParts, &genai.Part{Text: "解析的文档内容" + string(fileData)})
	fileParts = append(fileParts, &genai.Part{Text: content})
	return fileParts, nil
}
