package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/vertexai/genai"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type GeminiClient struct {
	client    *genai.Client
	voiceDao  *dao.VoiceDao
	uploadDao *dao.UploadDao
}

// @deprecated,Please Use genai.NewGenaiClientWithCredentialsFile
// NewGeminiClientWithCredentialsFile 使用凭证文件创建 Gemini 客户端
func NewGeminiClientWithCredentialsFile(credentialsFilePath string, voiceDao *dao.VoiceDao, uploadDao *dao.UploadDao) (*GeminiClient, error) {
	ctx := context.Background()

	if _, err := os.Stat(credentialsFilePath); err != nil {
		return nil, fmt.Errorf("凭证文件不存在或无法访问: %v", err)
	}
	credentials := option.WithCredentialsFile(credentialsFilePath)
	client, err := genai.NewClient(
		ctx,
		"bluemedia-db",
		"us-central1",
		credentials,
	)
	if err != nil {
		return nil, fmt.Errorf("创建 Gemini 客户端失败: %v", err)
	}

	return &GeminiClient{
		client:    client,
		voiceDao:  voiceDao,
		uploadDao: uploadDao,
	}, nil
}

// ChatStream 使用视频数据和提示直接发送到 Gemini API，并返回流式生成的内容。
func (gc *GeminiClient) ChatStream(ctx context.Context, modelName string, prompt string, videoData []byte, urls []string, historyMessage []*genai.Content, lastMessage model.MessageHistory) *genai.GenerateContentResponseIterator {
	mimeType := "text/plain"
	model := gc.client.GenerativeModel(modelName)
	model.SetTopK(40)
	model.SetTopP(0.95)
	model.SetMaxOutputTokens(4096)
	// model.SetTemperature(1)
	model.ResponseMIMEType = "text/plain"
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(prompt)},
		Role:  "model",
	}
	chatSession := model.StartChat()
	chatSession.History = historyMessage
	requestPrompt := []genai.Part{}
	if lastMessage.Content == "" && len(videoData) > 0 {
		requestPrompt = append(requestPrompt, genai.Text(prompt))

	} else {
		requestPrompt = append(requestPrompt, genai.Text(lastMessage.Content))

	}
	// 如果有视频数据，添加到请求中
	if len(videoData) > 0 {
		model.SetTemperature(0.5)
		mimeType = "video/mp4"
		requestPrompt = append(requestPrompt, genai.Blob{
			MIMEType: "video/mp4",
			Data:     videoData,
		})
	}

	// 如果有URLs，获取图片内容并添加到请求中
	if len(urls) > 0 {
		for _, url := range urls {
			var imgData []byte
			var err error
			if strings.HasPrefix(url, "http") {
				// nolint:bodyclose
				_, imgData, err = utils.Get(ctx, url)
				if err != nil {
					zlog.LogWithContext(ctx).Error("读取图片数据失败", zap.Error(err), zap.String("url", url))
					continue // 错误时跳过当前URL
				}
			} else {
				imgData = []byte(url)
			}

			// 正确截断日志数据
			truncateLength := 50
			logData := ""
			if len(imgData) > 0 {
				if len(imgData) > truncateLength {
					logData = string(imgData[:truncateLength])
				} else {
					logData = string(imgData)
				}
			}

			zlog.LogWithContext(ctx).Info("图片数据",
				zap.Int("dataSize", len(imgData)),
				zap.String("data", logData))
			mimeType = "image/jpeg"
			requestPrompt = append(requestPrompt, genai.Blob{
				MIMEType: "image/jpeg",
				Data:     imgData,
			})
		}
	}

	iter := chatSession.SendMessageStream(ctx, requestPrompt...)

	zlog.LogWithContext(ctx).Info("向 Gemini 发送流式请求",
		zap.String("model", modelName),
		zap.String("mimeType", mimeType),
		zap.Int("videoDataSize", len(videoData)),
		zap.Int("urlsCount", len(urls)),
	)

	return iter
}

func ProcessStreamResponse(ctx context.Context, iter *genai.GenerateContentResponseIterator, output model.Output) error {
	defer utils.SafeCloseChan(output)

	for {
		select {
		case <-ctx.Done():
			zlog.LogWithContext(ctx).Warn("Context cancelled during stream processing", zap.Error(ctx.Err()))
			return ctx.Err()
		default:
			resp, err := iter.Next()
			if err == io.EOF {
				zlog.LogWithContext(ctx).Info("Stream finished successfully (EOF)")
				return nil
			}
			if err != nil {
				if err == iterator.Done {
					zlog.LogWithContext(ctx).Info("Stream finished successfully (iteratror.DONE)")
					return nil
				}
				if ctx.Err() != nil {
					zlog.LogWithContext(ctx).Warn("Context cancelled while encountering stream error", zap.Error(ctx.Err()), zap.NamedError("stream_error", err))
					return ctx.Err()
				}
				zlog.LogWithContext(ctx).Error("Stream error during processing", zap.Error(err))
				return fmt.Errorf("stream error: %w", err)
			}

			if len(resp.Candidates) == 0 {
				continue
			}

			candidate := resp.Candidates[0]
			if candidate.FinishReason != genai.FinishReasonUnspecified && candidate.FinishReason != genai.FinishReasonStop {
				zlog.LogWithContext(ctx).Warn("Stream finished with non-STOP reason", zap.String("reason", candidate.FinishReason.String()))
			}

			if candidate.Content != nil {
				for _, part := range candidate.Content.Parts {
					output <- []string{string("")}

					if text, ok := part.(genai.Text); ok {
						select {
						case output <- []string{string(text)}:
						case <-ctx.Done():
							zlog.LogWithContext(ctx).Warn("Context cancelled while sending data to output channel", zap.Error(ctx.Err()))
							return ctx.Err()
						}
					}
				}
			}
		}
	}
}

// Process 实现了 LlmHandler 接口，处理视频数据、Gemini 分析
func (gc *GeminiClient) Process(ctx context.Context, params common.LLMProcessParams) error {
	if params.Output == nil {
		return errors.New("output channel is required")
	}
	processCtx, cancel := context.WithCancel(ctx)

	var message model.MessageHistory
	if len(params.History) > 0 {
		message = params.History[len(params.History)-1]
	} else {
		// 如果没有历史记录，使用当前请求中的消息
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

	var err error
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
	historyMessage := gc.buildChatMessages(params.History, prompt, projectID)
	if len(historyMessage) > 0 {
		historyMessage = historyMessage[:len(historyMessage)-1]
	}
	iterator := gc.ChatStream(processCtx, modelName, prompt, videoData, urls, historyMessage, message)

	// iterator := gc.GenerateContent(processCtx, modelName, prompt, videoData, urls)

	err = ProcessStreamResponse(processCtx, iterator, params.Output)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			zlog.LogWithContext(processCtx).Warn("Gemini 流处理因上下文取消或超时而终止", zap.Error(err))
			return err
		}
		zlog.LogWithContext(processCtx).Error("处理 Gemini 流响应时出错", zap.Error(err))
		return fmt.Errorf("error processing stream response: %w", err)
	}

	zlog.LogWithContext(processCtx).Info("Gemini 流处理成功完成")
	return nil
}

func (m *GeminiClient) buildChatMessages(history []model.MessageHistory, sysPrompt string, projectID string) []*genai.Content {
	historyMessage := []*genai.Content{}

	for _, msg := range history {
		content := m.handleVoiceMessage(msg.VoiceHash, msg.Content)
		senderRole := "user"
		if msg.Sender == "assistant" {
			senderRole = "model"
		}
		if content == "" {
			continue
		}

		message := genai.Content{
			Role:  senderRole,
			Parts: []genai.Part{genai.Text(content)},
		}

		if len(msg.URLs) > 0 {
			switch msg.FileType {
			case vai.MessageType_MT_IMAGE:
				message.Parts = m.handleImageMessage(content, msg.URLs)
			case vai.MessageType_MT_FILE:
				if itemMsg, err := m.handleFileMessage(content, msg.URL, projectID); err == nil {
					message.Parts = itemMsg
				}
			}
		}

		historyMessage = append(historyMessage, &message)
	}

	return historyMessage
}

func (m *GeminiClient) handleVoiceMessage(voiceHash string, content string) string {
	if voiceHash == "" {
		return content
	}

	if voiceInfo, err := m.voiceDao.GetByFileHash(voiceHash); err == nil && voiceInfo.Content != "" {
		return voiceInfo.Content
	}
	return content
}

func (m *GeminiClient) handleImageMessage(content string, urls []string) []genai.Part {
	itemMsg := []genai.Part{
		genai.Text(content),
	}

	for _, v := range urls {
		v = utils.GetLowQualityImageURL(v)
		// nolint:bodyclose
		_, imgData, err := utils.Get(context.TODO(), v)
		if err != nil {
			zlog.Logger.Error("Get Image Error")
			continue
		}
		itemMsg = append(itemMsg, genai.Blob{
			MIMEType: "image/jpeg",
			Data:     imgData,
		})
	}
	return itemMsg
}

func (m *GeminiClient) handleFileMessage(content, url, projectID string) ([]genai.Part, error) {
	fileInfo, err := m.uploadDao.GetByFileUrl(context.TODO(), db.GetDB(), projectID, url, vai.UploaderRole_UPLOADER_ROLE_USER)
	if err != nil {
		zlog.Logger.Error("Get File Error", zap.Error(err), zap.String("url", url))
		return nil, err
	}

	client := http.Client{Timeout: 15 * time.Second}
	fileContentBody, err := client.Get(fileInfo.ContentURL)
	if err != nil {
		zlog.Logger.Error("Get File Txt Error", zap.Error(err))
		return nil, err
	}
	defer fileContentBody.Body.Close()

	fileContentReader := io.LimitReader(fileContentBody.Body, 128*1024)
	fileContent, err := io.ReadAll(fileContentReader)
	if err != nil {
		zlog.Logger.Error("Read File IO Error", zap.Error(err))
		return nil, err
	}

	return []genai.Part{
		genai.Text("解析的文档内容" + string(fileContent)),
		genai.Text(content),
	}, nil
}
