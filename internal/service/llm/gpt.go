package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"go.uber.org/zap"

	"va_visionai_server/conf"
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

// GPTModel 结构体定义
type GPTModel struct {
	client    *azopenai.Client
	voiceDao  *dao.VoiceDao
	uploadDao *dao.UploadDao
}

// NewGPTModel 创建新的 GPTModel
func NewGPTModel(voiceDao *dao.VoiceDao, uploadDao *dao.UploadDao) (*GPTModel, error) {
	config := conf.GlobalConfig
	client, err := azopenai.NewClientWithKeyCredential(config.LlmConfig.GPT.Endpoint,
		azcore.NewKeyCredential(config.LlmConfig.GPT.APIKey), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI client: %v", err)
	}
	return &GPTModel{
		client:    client,
		voiceDao:  voiceDao,
		uploadDao: uploadDao,
	}, nil
}

// Process 生成响应（根据参数决定是流式还是非流式）
func (m *GPTModel) Process(ctx context.Context, params common.LLMProcessParams) error {
	if params.Output == nil {
		params.Output = make(model.Output)
	}
	systemPrompt := params.Request.GetMessage().GetSystemPrompt()
	projectID := common.GetProjectID(ctx)
	messages := m.buildChatMessages(params.History, systemPrompt, projectID)
	modelName := constants.MappingModelName(params.Request.GetMessage().GetModelId())

	chatOptions := azopenai.ChatCompletionsOptions{
		Messages:       messages,
		N:              to.Ptr[int32](1),
		DeploymentName: &modelName,
	}

	isStream := false
	switch params.Request.GetMessage().GetModelId() {
	case vai.Model_MODEL_GPTO1_PREVIEW, vai.Model_MODEL_GPTO1_MINI:
		messages[0] = &azopenai.ChatRequestAssistantMessage{Content: to.Ptr(systemPrompt)}
		chatOptions.Messages = messages
		// O1 模型暂时不支持流式调用,不支持maxtoken 参数限制,只支持MaxCompletionsTokens 但是azopenai 的SDK暂时没有适配这个参数
		//  Temperature 目前只能为1(默认)
	default:
		// FIXME 这里注意，目前4096大部分情况够用，不过当用户量起来后，关注Token消耗，适当降低
		chatOptions.MaxTokens = to.Ptr[int32](4096)
		chatOptions.Temperature = to.Ptr[float32](0.7)
		isStream = true
	}
	var err error
	if isStream {
		err = m.processStream(ctx, chatOptions, params.Output, params.CancelCh, params.StreamServer)
	} else {
		err = m.processNonStream(chatOptions, params.Output, params.StreamServer)
	}
	if err != nil {
		if rsperr, ok := err.(*azcore.ResponseError); ok {
			if rsperr.ErrorCode == "content_filter" {
				safeContente := constants.GetContentSafePolicyMsg(params.Request.GetRequestHeader().GetDevice().GetLanguage())
				params.Output <- []string{safeContente}
				utils.SafeCloseChan(params.Output)
				return nil
			} else {
				params.Output <- []string{constants.MsgModelErr}
				utils.SafeCloseChan(params.Output)
				return nil
			}
		}
	}

	return err
}

// 流式处理
func (m *GPTModel) processStream(ctx context.Context, chatOptions azopenai.ChatCompletionsOptions, output model.Output, cancelCh model.CancelCh, streamServer vai.ChatService_SendChatMessageStreamServer) error {
	resp, err := m.client.GetChatCompletionsStream(context.TODO(), chatOptions, nil)
	if err != nil {
		zlog.Logger.Error("GPT GetChatCompletionsStream Error", zap.Error(err))
		return err
	}
	defer resp.ChatCompletionsStream.Close()
	gotReply, responseMessages, err := m.processChatCompletionsStream(ctx, resp, output, cancelCh)
	if err != nil {
		return err
	}
	if !gotReply {
		return errors.New("failed to get response from API")
	}

	m.updateTokenUsage(streamServer, chatOptions.Messages, responseMessages)
	return nil
}

// 非流式处理
func (m *GPTModel) processNonStream(chatOptions azopenai.ChatCompletionsOptions, output model.Output, streamServer vai.ChatService_SendChatMessageStreamServer) error {
	resp, err := m.client.GetChatCompletions(context.TODO(), chatOptions, nil)
	if err != nil {
		zlog.Logger.Error("GPT GetChatCompletions Error", zap.Error(err))
		return err
	}

	if len(resp.Choices) == 0 {
		return errors.New("failed to get response from API")
	}

	for _, choice := range resp.Choices {
		if choice.Message.Content != nil {
			output <- []string{*choice.Message.Content}
			close(output)
		}
	}

	// m.updateTokenUsage(streamServer, chatOptions.Messages, resp.Choices)
	return nil
}

// 处理流式响应
func (m *GPTModel) processChatCompletionsStream(ctx context.Context, resp azopenai.GetChatCompletionsStreamResponse, output model.Output, cancelCh model.CancelCh) (bool, []azopenai.ChatRequestMessageClassification, error) {
	gotReply := false
	responseMessages := []azopenai.ChatRequestMessageClassification{}
	var err error
	defer utils.SafeCloseChan(output)
	sendWithTimeoutOrCancel := func(content string) bool {
		select {
		case output <- []string{content}:
			return true
		case <-time.After(30 * time.Second):
			zlog.LogWithContext(ctx).Info("ChatStream Send Token Timeout, Finish ChatStream And Return")
			return false
		case <-cancelCh:
			zlog.LogWithContext(ctx).Info("GPTProcessStream Receive Cancel Signal. Finish Chat Stream And Return")
			return false
		}
	}
	for {
		select {
		case <-cancelCh:
			zlog.LogWithContext(ctx).Info("GPTProcessStream Receive Cancel Signal.Finish Chat Stream And Return")
			return gotReply, responseMessages, err
		default:
			chatCompletions, err := resp.ChatCompletionsStream.Read()
			if errors.Is(err, io.EOF) {
				return gotReply, responseMessages, nil
			}
			if err != nil {
				return gotReply, responseMessages, err
			}

			if len(chatCompletions.Choices) >= 1 {
				choice := chatCompletions.Choices[0]
				gotReply = true
				if choice.FinishReason != nil {
					return gotReply, responseMessages, nil
				}
				if choice.Delta != nil && choice.Delta.Content != nil {
					if !sendWithTimeoutOrCancel(*choice.Delta.Content) {
						return gotReply, responseMessages, nil
					}
					responseMessages = append(responseMessages, &azopenai.ChatRequestAssistantMessage{Content: to.Ptr(*choice.Delta.Content)})
				} else {
					if !sendWithTimeoutOrCancel(constants.MsgModelErr) {
						return gotReply, responseMessages, nil
					}
				}
			}
		}
	}
}

// 更新 token 使用情况
func (m *GPTModel) updateTokenUsage(streamServer vai.ChatService_SendChatMessageStreamServer, inputMessages []azopenai.ChatRequestMessageClassification, outputMessages []azopenai.ChatRequestMessageClassification) {
	inputTotalToken := calculateTotalTokens(inputMessages)
	outputTotalToken := calculateTotalTokens(outputMessages)

	mw := getWrappedStream(streamServer)
	if wss, ok := mw.(*rpc.WrappedServerStream); ok {
		ctx := context.WithValue(wss.Context(), constants.LLMInputToken, inputTotalToken)
		ctx = context.WithValue(ctx, constants.LLMOutputToken, outputTotalToken)
		ctx = context.WithValue(ctx, constants.LLMModelID, 0)
		wss.SetContext(ctx)
	}
}

func calculateTotalTokens(msgs []azopenai.ChatRequestMessageClassification) int {
	totalToken, err := utils.CalculateTokens(msgs)
	if err != nil {
		zlog.Logger.Error("utils.CalculateTokens Request Message Error", zap.Error(err))
		return 0
	}
	return totalToken
}

func (m *GPTModel) buildChatMessages(history []model.MessageHistory, sysPrompt string, projectID string) []azopenai.ChatRequestMessageClassification {
	messages := []azopenai.ChatRequestMessageClassification{
		&azopenai.ChatRequestSystemMessage{Content: to.Ptr(sysPrompt)},
	}
	for _, msg := range history {
		switch azopenai.ChatRole(msg.Sender) {
		case azopenai.ChatRoleAssistant:
			messages = append(messages, &azopenai.ChatRequestAssistantMessage{Content: to.Ptr(msg.Content)})
		case azopenai.ChatRoleUser:
			content := msg.Content
			if msg.VoiceHash != "" {
				voiceInfo, err := m.voiceDao.GetByFileHash(msg.VoiceHash)
				if err != nil {
					zlog.Logger.Error("Get Voice Error", zap.Error(err))
				} else {
					if voiceInfo.Content != "" {
						content = voiceInfo.Content
					}
				}
			}
			if len(msg.URLs) > 0 {
				var contentMsg *azopenai.ChatRequestUserMessageContent
				switch msg.FileType {
				case vai.MessageType_MT_IMAGE:
					itemMsg := []azopenai.ChatCompletionRequestMessageContentPartClassification{}
					if len(msg.URLs) > 0 {
						for _, v := range msg.URLs {
							imagebase64, err := utils.URLToBase64(v)
							if err != nil {
								zlog.Logger.Error("URLToBase64 Error", zap.Error(err))
								continue
							}
							itemMsg = append(itemMsg, &azopenai.ChatCompletionRequestMessageContentPartImage{
								ImageURL: &azopenai.ChatCompletionRequestMessageContentPartImageURL{
									URL: &imagebase64,
								},
							})
						}
					}

					itemMsg = append(itemMsg, &azopenai.ChatCompletionRequestMessageContentPartText{
						Text: to.Ptr(content),
					})
					contentMsg = azopenai.NewChatRequestUserMessageContent(itemMsg)

				case vai.MessageType_MT_FILE:
					fileInfo, err := m.uploadDao.GetByFileUrl(context.TODO(), db.GetDB(), projectID, msg.URL, vai.UploaderRole_UPLOADER_ROLE_USER)
					if err != nil {
						zlog.Logger.Error("Get File Error", zap.Error(err), zap.String("url", msg.URL))
						continue
					}
					client := http.Client{Timeout: 15 * time.Second}
					fileContentBody, err := client.Get(fileInfo.ContentURL)
					if err != nil {
						zlog.Logger.Error("Get File Txt Error", zap.Error(err))
						continue
					}
					defer fileContentBody.Body.Close()
					fileContentReader := io.LimitReader(fileContentBody.Body, 128*1024)
					fileContent, err := io.ReadAll(fileContentReader)
					if err != nil {
						zlog.Logger.Error("Read File IO Error", zap.Error(err))
						continue
					}
					contentMsg = azopenai.NewChatRequestUserMessageContent(
						[]azopenai.ChatCompletionRequestMessageContentPartClassification{
							&azopenai.ChatCompletionRequestMessageContentPartText{
								Text: to.Ptr("解析的文档内容" + string(fileContent)),
							},
							&azopenai.ChatCompletionRequestMessageContentPartText{
								Text: to.Ptr(msg.Content),
							},
						},
					)
				}

				messages = append(messages, &azopenai.ChatRequestUserMessage{Content: contentMsg})
			} else {
				messages = append(messages, &azopenai.ChatRequestUserMessage{Content: azopenai.NewChatRequestUserMessageContent(content)})
			}
		}
	}

	return messages
}

func getWrappedStream(ss interface{}) interface{} {
	if ss == nil {
		return nil
	}
	v := reflect.ValueOf(ss)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	field := v.FieldByName("ServerStream")
	if field.IsValid() && field.CanInterface() {
		return field.Interface()
	}
	return nil
}
