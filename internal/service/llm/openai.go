package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/dundunHa/go-openai"
	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/rpc"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// LLMProcessRequest 包含处理LLM请求所需的所有参数
type LLMProcessRequest struct {
	Ctx          context.Context
	Request      *vai.ChatMessageSendRequest
	StreamServer vai.ChatService_SendChatMessageStreamServer
	History      []model.MessageHistory
	Output       model.Output
	CancelCh     model.CancelCh
}

// OpenAIModelConfig holds configuration for OpenAIModel
type OpenAIModelConfig struct {
	APIKey     string
	Endpoint   string
	ModelName  string
	ServerName string
}

// OpenAIModel represents an OpenAI API client
type OpenAIModel struct {
	client     *openai.Client
	modelName  string
	serverName string
	voiceDao   *dao.VoiceDao
	uploadDao  *dao.UploadDao
}

// NewOpenAIModel creates a new OpenAIModel instance
func NewOpenAIModel(config OpenAIModelConfig, voiceDao *dao.VoiceDao, uploadDao *dao.UploadDao) (*OpenAIModel, error) {
	if config.APIKey == "" || config.Endpoint == "" {
		return nil, errors.New("missing required OpenAI configuration")
	}

	clientConfig := openai.DefaultConfig(config.APIKey)
	if config.Endpoint != "" {
		clientConfig.BaseURL = config.Endpoint
	}

	client := openai.NewClientWithConfig(clientConfig)
	return &OpenAIModel{
		client:     client,
		modelName:  config.ModelName,
		serverName: config.ServerName,
		voiceDao:   voiceDao,
		uploadDao:  uploadDao,
	}, nil
}

// Process handles the chat completion request
func (m *OpenAIModel) Process(ctx context.Context, params common.LLMProcessParams) (err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			zlog.Logger.Error("OpenAIModel.Process panic",
				zap.Any("error", r),
				zap.ByteString("stack", stack))
			err = fmt.Errorf("internal error: %v", r)
		}
	}()

	if params.Output == nil {
		params.Output = make(model.Output)
	}
	defer close(params.Output)

	projectID := common.GetProjectID(ctx)
	messages := m.buildChatMessages(params.History, params.Request.GetMessage().GetSystemPrompt(), projectID)
	messages = m.buildMessagesWithImageInfoInSystemPrompt(params.ImageContent, messages)

	chatReq := openai.ChatCompletionRequest{
		Model:    m.modelName,
		Messages: messages,
		Stream:   true,
		StreamOptions: &openai.StreamOptions{
			IncludeUsage: true,
		},
		Temperature: 0.7,
	}

	stream, err := m.client.CreateChatCompletionStream(ctx, chatReq)
	if err != nil {
		zlog.LogWithContext(ctx).Error("create chat completion stream failed", zap.Error(err))
		return fmt.Errorf("create chat completion stream failed: %v", err)
	}
	defer stream.Close()

	var (
		inputTotalToken  int
		outputTotalToken int
		gotReply         bool
		thinkingStart    bool
		thinkingEnd      bool
	)
	detectBuffer := strings.Builder{}
	enableDetect := false
	for {
		select {
		case <-params.CancelCh:
			zlog.LogWithContext(ctx).Info("OpenAIModel.Process cancelCh")
			return nil
		default:
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				goto END
			}
			if err != nil {
				zlog.Logger.Error("Error receiving from stream", zap.Error(err))
				utils.SafeCloseChan(params.Output)
				return fmt.Errorf("stream receive error: %w", err)
			}

			// Use actual token counts from API response
			if response.Usage != nil {
				inputTotalToken = response.Usage.PromptTokens
				outputTotalToken = response.Usage.CompletionTokens
			}

			if len(response.Choices) > 0 {
				gotReply = true
				choice := response.Choices[0]
				var outputContent string
				var formulas []string
				outputContent = choice.Delta.Content
				if !enableDetect {
					var needDetect string
					outputContent, needDetect = m.detectContent(outputContent)
					if needDetect != "" {
						detectBuffer.WriteString(needDetect)
						enableDetect = true
					}
				} else {
					detectBuffer.WriteString(outputContent)
					outputContent, formulas = m.extractLatexContent(detectBuffer.String())
					if len(formulas) > 0 {
						for i, formula := range formulas {
							outputContent = strings.ReplaceAll(outputContent, "{{MATH"+strconv.Itoa(i)+"}}", "$$"+formula+"$$")
							zlog.Logger.Info("OpenAIModel.Process latex", zap.String("formula", formula), zap.String("outputContent", outputContent))
						}
						enableDetect = false
						detectBuffer.Reset()
						formulas = nil
					} else {
						outputContent = ""
					}
				}
				if !params.HiddenReasoning {
					if choice.Delta.ReasoningContent != "" {
						if !thinkingStart {
							outputContent = ">" + choice.Delta.ReasoningContent
							thinkingStart = true
						} else {
							outputContent = strings.ReplaceAll(choice.Delta.ReasoningContent, "\n\n", "\n>\n>")
						}
					} else if thinkingStart && !thinkingEnd {
						outputContent = "\n" + outputContent
						thinkingEnd = true
					}
				} else {
					params.Output <- []string{""}
				}

				if detectBuffer.Len() > 200 && outputContent == "" {
					outputContent = detectBuffer.String()
					detectBuffer.Reset()
					enableDetect = false
				}

				if outputContent != "" {
					params.Output <- []string{outputContent}
				}
				if enableDetect {
					params.Output <- []string{""}
				}

			} else {
				if detectBuffer.Len() > 0 {
					params.Output <- []string{detectBuffer.String()}
					detectBuffer.Reset()
				}
			}
		}
	}

END:
	if !gotReply {
		return errors.New("failed to get response from API")
	}

	// Update token usage in context
	if mw := getWrappedStream(params.StreamServer); mw != nil {
		if wss, ok := mw.(*rpc.WrappedServerStream); ok {
			ctx := context.WithValue(ctx, constants.LLMInputToken, inputTotalToken)
			ctx = context.WithValue(ctx, constants.LLMOutputToken, outputTotalToken)
			wss.SetContext(ctx)
		}
	}

	return nil
}

func (m *OpenAIModel) handleVoiceMessage(voiceHash string, content string) string {
	if voiceHash == "" {
		return content
	}

	if voiceInfo, err := m.voiceDao.GetByFileHash(voiceHash); err == nil && voiceInfo.Content != "" {
		return voiceInfo.Content
	}
	return content
}

func (m *OpenAIModel) handleImageMessage(content string, urls []string) []openai.ChatMessagePart {
	itemMsg := make([]openai.ChatMessagePart, 0, 1+len(urls))
	itemMsg = append(itemMsg, openai.ChatMessagePart{
		Type: openai.ChatMessagePartTypeText,
		Text: content,
	})

	for _, v := range urls {
		v = utils.GetLowQualityImageURL(v)
		itemMsg = append(itemMsg, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL: v,
			},
		})
	}
	return itemMsg
}

func (m *OpenAIModel) handleFileMessage(content, url, projectID string) ([]openai.ChatMessagePart, error) {
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

	return []openai.ChatMessagePart{
		{
			Type: openai.ChatMessagePartTypeText,
			Text: "解析的文档内容" + string(fileContent),
		},
		{
			Type: openai.ChatMessagePartTypeText,
			Text: content,
		},
	}, nil
}

func (m *OpenAIModel) buildChatMessages(history []model.MessageHistory, sysPrompt string, projectID string) []openai.ChatCompletionMessage {
	messages := []openai.ChatCompletionMessage{}
	if sysPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: sysPrompt,
		})
	}

	for _, msg := range history {
		content := m.handleVoiceMessage(msg.VoiceHash, msg.Content)
		message := openai.ChatCompletionMessage{
			Role:    msg.Sender,
			Content: content,
		}

		if len(msg.URLs) > 0 {
			switch msg.FileType {
			case vai.MessageType_MT_IMAGE:
				message.MultiContent = m.handleImageMessage(content, msg.URLs)
			case vai.MessageType_MT_FILE:
				if itemMsg, err := m.handleFileMessage(content, msg.URL, projectID); err == nil {
					message.MultiContent = itemMsg
				}
			}
		}

		if len(message.MultiContent) > 0 {
			message.Content = ""
		}
		messages = append(messages, message)
	}

	return messages
}

func (m *OpenAIModel) buildMessagesWithImageInfoInSystemPrompt(imageInfo string, messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	if imageInfo == "" {
		return messages
	}

	newMessages := make([]openai.ChatCompletionMessage, 0, len(messages))

	hasSystemMessage := false
	for i, msg := range messages {
		if msg.Role == "system" {
			hasSystemMessage = true

			newSystemContent := fmt.Sprintf("以下是图片的详细描述信息，请在回答问题时参考此信息：\n\n%s\n\n原系统提示：%s",
				imageInfo, msg.Content)

			newMsg := msg
			newMsg.Content = newSystemContent
			newMessages = append(newMessages, newMsg)

			newMessages = append(newMessages, messages[i+1:]...)

			break
		}

		newMessages = append(newMessages, msg)
	}

	if !hasSystemMessage {
		systemMsg := openai.ChatCompletionMessage{
			Role:    "system",
			Content: "以下是图片的详细描述信息，请在回答问题时参考此信息：\n\n" + imageInfo,
		}

		newMessages = append([]openai.ChatCompletionMessage{systemMsg}, newMessages...)
	}

	// 将图片类型的消息转换为普通文本消息，移除URL信息
	for i, msg := range newMessages {
		if len(msg.MultiContent) > 0 {
			// 检查是否包含图片URL
			hasImageURL := false
			textContent := ""

			var textContentSb strings.Builder
			for _, part := range msg.MultiContent {
				if part.Type == openai.ChatMessagePartTypeText {
					textContentSb.WriteString(part.Text)
				}
				if part.Type == openai.ChatMessagePartTypeImageURL {
					hasImageURL = true
				}
			}
			textContent += textContentSb.String()

			// 如果消息包含图片URL，则将其转换为纯文本消息
			if hasImageURL {
				newMessages[i].MultiContent = nil
				newMessages[i].Content = textContent
			}
		}
	}

	return newMessages
}

func (m *OpenAIModel) detectContent(content string) (string, string) {
	// 特殊字符集
	specialChars := []byte{'\\', '$', '['}

	// 单次扫描找到第一个特殊字符
	for i := 0; i < len(content); i++ {
		for _, char := range specialChars {
			if content[i] == char {
				return content[:i], content[i:]
			}
		}
	}
	return content, ""
}

func (m *OpenAIModel) extractLatexContent(content string) (string, []string) {
	formulas := make([]string, 0)

	// 定义不同格式的数学公式模式
	patterns := []struct {
		regex    string
		startLen int
		endLen   int
	}{
		{`\$\$(.*?)\$\$`, 2, 2},     // $$...$$ 格式
		{`\$([^\$\r\n]+?)\$`, 1, 1}, // $...$ 格式
		{`\\\((.*?)\\\)`, 2, 2},     // \(...\) 格式 - 关键修正
		{`\[([^\]\r\n]*?(?:\\pi|\\sum|\\frac|_|\^)[^\]\r\n]*?)\]`, 1, 1}, // 数学方括号
		{`\\\[(?s).*?\\begin\{(.+?)\}.*?\\end.*?\\\]`, 0, 0},
	}

	// 处理每种格式
	replacedText := content
	for _, p := range patterns {
		re := regexp.MustCompile(p.regex)
		replacedText = re.ReplaceAllStringFunc(replacedText, func(match string) string {
			// 提取公式内容
			formula := match[p.startLen : len(match)-p.endLen]
			formulas = append(formulas, formula)
			return fmt.Sprintf("{{MATH%d}}", len(formulas)-1)
		})
	}

	return replacedText, formulas
}
