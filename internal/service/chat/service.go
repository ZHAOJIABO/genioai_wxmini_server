package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/chat/biz"
	chatcommon "va_visionai_server/internal/service/chat/common"
	"va_visionai_server/internal/service/prompt"
	"va_visionai_server/internal/task"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

var (
	ErrRemoteClose = errors.New("remote close conn")
)

// ChatService 提供聊天相关的业务服务
type ChatService struct {
	llmService    *ChatLLMService
	msgService    *service.MessageService
	voiceService  *service.VoiceService
	promptTracker *prompt.HeatTracker
	uploadService *service.UploadService
	db            *gorm.DB
	uploadDao     *dao.UploadDao
	modelDao      *dao.ModelDao
	promptDao     *dao.PromptDao
	chatDao       *dao.ChatDao
}

// NewChatService 创建新的ChatService实例
func NewChatService(
	llmService *ChatLLMService,
	msgService *service.MessageService,
	voiceService *service.VoiceService,
	promptTracker *prompt.HeatTracker,
	uploadService *service.UploadService,
	db *gorm.DB,
	uploadDao *dao.UploadDao,
	modelDao *dao.ModelDao,
	promptDao *dao.PromptDao,
	repos *dao.Repositories,
) *ChatService {
	svc := &ChatService{
		llmService:    llmService,
		msgService:    msgService,
		voiceService:  voiceService,
		promptTracker: promptTracker,
		uploadService: uploadService,
		db:            db,
		uploadDao:     uploadDao,
		modelDao:      modelDao,
		promptDao:     promptDao,
		chatDao:       repos.Chat,
	}
	return svc
}

func (s *ChatService) Process(
	ctx context.Context,
	req *vai.ChatMessageSendRequest,
	streamServer vai.ChatService_SendChatMessageStreamServer,
) (*vai.Message, error) {
	// 包装为标准请求并交由通用处理流程
	switch req.GetMessage().GetChatBizHandlerId() {
	case vai.ChatBizHandlerID_CHAT_BIZ_HANDLER_ID_USER_PERSONAL_INFO:
		return s.ProcessRequest(ctx, &CustomPersonalInfoChatRequest{Req: req}, streamServer)
	default:
		return s.ProcessRequest(ctx, &StandardRequest{Req: req}, streamServer)
	}
}

// 请求上下文，用于传递请求相关数据
type chatRequestContext struct {
	UserID         string
	ChatID         string
	Content        string
	MessageType    vai.MessageType
	MessageID      string
	ReplyMessageID string
	PromptID       string
	URLs           []string
	Blobs          []*vai.Blob
	VoiceContent   string
	VoiceHash      string
	FrameURL       string
}

// startAsyncTasks 启动异步任务（更新标题和提交任务）
func (s *ChatService) startAsyncTasks(
	ctx context.Context,
	req *vai.ChatMessageSendRequest,
	chatID string,
	title string,
	msgHistory []model.MessageHistory,
) {
	// 仅当不是星座应用时更新标题
	// if req.GetRequestHeader().GetApp().getp() != "com.bluex.astrox" {
	titleCtx := context.Background()
	titleCtx = utils.NewContextWithValue(titleCtx, constants.CtxProjectID, common.GetProjectID(ctx))
	go s.updateChatTitle(titleCtx, req, chatID, title, msgHistory)
	// }

	// 提交聊天任务
	now := time.Now()
	go task.Chat.Submit(chatID, &now)
}

// processVideoFrame 处理视频帧提取
func (s *ChatService) processVideoFrame(ctx context.Context, header *vai.RequestHeader, messageId string, videoData []byte) string {
	frameBytes, frameErr := utils.ExtractFrameFromVideoBytesWithFFmpeg(videoData)
	if frameErr != nil {
		zlog.LogWithContext(ctx).Error("Failed to extract frame from video", zap.Error(frameErr))
		return ""
	}

	zlog.LogWithContext(ctx).Info("Successfully extracted video frame", zap.Int("frame_size", len(frameBytes)))
	uploadReq := &vai.UploadRequest{
		RequestHeader: header,
		FileName:      fmt.Sprintf("frame_%s.jpg", messageId),
		FileType:      vai.FileType_FT_IMAGE,
		Data:          frameBytes,
		AskQuestion:   false,
	}

	uploadResp, uploadErr := s.uploadService.SysUpload(ctx, uploadReq)
	if uploadErr != nil {
		zlog.LogWithContext(ctx).Error("Failed to sys-upload video frame", zap.Error(uploadErr))
		return ""
	}

	if uploadResp != nil && uploadResp.GetFileUrl() != "" {
		frameUrl := uploadResp.GetFileUrl()
		zlog.LogWithContext(ctx).Info("Successfully uploaded video frame", zap.String("frame_url", frameUrl))
		return frameUrl
	}

	return ""
}

// processVoice 处理语音内容
func (s *ChatService) processVoice(ctx context.Context, voiceHash string) string {
	voiceStartTime := time.Now()
	voiceInfo, err := s.voiceService.WaitAndGetVoice(voiceHash)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to get voice content",
			zap.String("voiceHash", voiceHash),
			zap.Error(err))
		return ""
	}

	zlog.LogWithContext(ctx).Info("Voice processing completed",
		zap.Duration("duration", time.Since(voiceStartTime)),
		zap.String("voiceHash", voiceHash))

	return voiceInfo.Content
}

// validateRequest 验证请求参数
func (s *ChatService) validateRequest(ctx context.Context, userID, chatID, content, voiceContent string,
	blobs []*vai.Blob, messageType vai.MessageType, urls []string, personalInfoID []string, chatBizHandlerID vai.ChatBizHandlerID) error {

	if userID == "" || chatID == "" || (content == "" && voiceContent == "" && len(blobs) == 0) {
		zlog.LogWithContext(ctx).Error("userId, chatID, and content are required")
		return constants.ERR_INVALID_PARAM
	}

	switch chatBizHandlerID {
	case vai.ChatBizHandlerID_CHAT_BIZ_HANDLER_ID_DEFAULT:
		if messageType == vai.MessageType_MT_VIDEO && len(blobs) == 0 {
			zlog.LogWithContext(ctx).Error("Video Message Type Should Have Blob")
			return constants.ERR_INVALID_PARAM
		}
	case vai.ChatBizHandlerID_CHAT_BIZ_HANDLER_ID_USER_PERSONAL_INFO:

	}

	return nil
}

// updateChatTitle 更新聊天标题
func (s *ChatService) updateChatTitle(
	ctx context.Context,
	req *vai.ChatMessageSendRequest,
	chatID string,
	title string,
	msgHistory []model.MessageHistory,
) {
	if title != constants.NewChatTitle && title != constants.MsgModelErr {
		return
	}
	defer func() {
		if title == constants.MsgModelErr {
			return
		}
		title = strings.ReplaceAll(title, "\"", "")
		if err := s.chatDao.UpdateChatTitle(chatID, title); err != nil {
			zlog.Logger.Error("[UpdateChatTitle Error] DAO UpdateChatTitle Error", zap.Error(err))
			return
		}
	}()

	lang := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	projectID := common.GetProjectID(ctx)

	var promptInfo *model.Prompt
	var err error
	switch projectID {
	case constants.ProjectIdSolacex:
		promptInfo, err = s.promptDao.GetAstroPrompt(constants.SystemConstellationChatTitlePrompt, lang)
	default:
		promptInfo, err = s.promptDao.GetPrompt(constants.SystemPromptTitle, lang)
	}
	if err != nil {
		fallbackLangs := []string{"en", "zh"}
		var foundPrompt bool
		for _, fallbackLang := range fallbackLangs {
			if fallbackLang == lang {
				continue
			}
			switch projectID {
			case constants.ProjectIdSolacex:
				promptInfo, err = s.promptDao.GetAstroPrompt(constants.SystemConstellationChatTitlePrompt, fallbackLang)
			default:
				promptInfo, err = s.promptDao.GetPrompt(constants.SystemPromptTitle, fallbackLang)
			}

			if err == nil {
				foundPrompt = true
				zlog.Logger.Info("Using fallback language for title prompt",
					zap.String("requestedLang", lang),
					zap.String("fallbackLang", fallbackLang))
				lang = fallbackLang
				break
			}
		}
		if !foundPrompt {
			zlog.Logger.Error("Failed to get title prompt in any language", zap.Error(err))
			return
		}
	}

	titlePrompt := promptInfo.Content
	titlePrompt = strings.ReplaceAll(titlePrompt, "{visionai-replace-lang}", lang)
	titleHistory := append(msgHistory, model.MessageHistory{Content: titlePrompt, Sender: "user"})

	// 复制请求并修改模型
	// nolint:govet
	titleReq := *req
	titleReq.Message.ModelId = vai.Model_MODEL_GPT4O_MINI
	titleReq.Message.ModelName = vai.Model_MODEL_GPT4O_MINI.String()

	// 调用LLM生成标题
	llmRsp, err := s.llmService.StreamProcess(
		ctx,
		&titleReq,
		titleHistory,
		nil,
		chatcommon.LLMOptions{
			InitialModel:  vai.Model_MODEL_GPT4O_MINI,
			GlobalTimeout: 120 * time.Second,
		},
	)

	if err != nil {
		zlog.Logger.Error("[UpdateChatTitle Error] LLM Process Error", zap.Error(err))
		return
	}

	if llmRsp != "" {
		title = llmRsp
	}
}

// generateMsgID 生成新的消息ID
func generateMsgID() string {
	newUUID := uuid.New()
	prefix := "om"
	return fmt.Sprintf("%s-%s", prefix, newUUID.String())
}

// ProcessRequest 通用处理入口
func (s *ChatService) ProcessRequest(
	ctx context.Context,
	chatReq ChatRequest,
	streamServer interface{},
) (*vai.Message, error) {

	chatCtx, err := s.prepareContext(ctx, chatReq, streamServer)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to prepare context", zap.Error(err))
		return nil, fmt.Errorf("failed to prepare context: %w", err)
	}

	if err := s.validateRequest(ctx, chatCtx.UserID, chatCtx.ChatID,
		chatCtx.Content, chatCtx.VoiceContent, chatCtx.Blobs,
		chatCtx.MessageType, chatCtx.URLs, chatCtx.ParticipantIDs, chatCtx.ChatBizHandlerID); err != nil {
		return nil, err
	}

	// 4. 遍历所有处理器
	for _, handler := range biz.Handlers {
		if !handler.Match(chatCtx) {
			continue
		}
		// 记录当前处理器
		handlerName := handler.Name()
		zlog.LogWithContext(ctx).Debug("Matched handler", zap.String("handler", handlerName))

		// 3. 由匹配的 Handler 负责创建或获取会话
		isNew, err := handler.EnsureChatSession(ctx, chatCtx)
		if err != nil {
			zlog.LogWithContext(ctx).Error("EnsureChatSession failed",
				zap.String("handler", handlerName),
				zap.Error(err))
			return nil, fmt.Errorf("failed to ensure chat session: %w", err)
		}
		if chatCtx.GetChat() == nil {
			zlog.LogWithContext(ctx).Error("Handler did not set chat context",
				zap.String("handler", handlerName))
			return nil, fmt.Errorf("handler %s failed to set chat session", handlerName)
		}
		zlog.LogWithContext(ctx).Info("Chat session ensured",
			zap.String("handler", handlerName),
			zap.Bool("isNew", isNew))

		// 如果是新会话且有 PromptID，则跟踪
		if isNew && chatCtx.PromptID != "" {
			if s.promptTracker != nil {
				go s.promptTracker.Track(utils.CloneContext(ctx), chatCtx.PromptID)
			} else {
				zlog.LogWithContext(ctx).Warn("Prompt tracker is nil, skipping tracking")
			}
		}

		// 5. 准备消息历史
		history, err := handler.PrepareHistory(ctx, chatCtx)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			zlog.LogWithContext(ctx).Error("PrepareHistory Error",
				zap.String("handler", handlerName),
				zap.Error(err))
			history = []model.MessageHistory{}
		}

		// 5.1 处理系统提示词
		if err := handler.PrepareSystemPrompt(ctx, chatCtx); err != nil {
			zlog.LogWithContext(ctx).Error("PrepareSystemPrompt Error",
				zap.String("handler", handlerName),
				zap.Error(err))
		}

		// 6. 执行业务处理
		content, modelID, err := handler.Handle(ctx, chatCtx, history)
		if err != nil {
			if errors.Is(err, ErrRemoteClose) {
				zlog.LogWithContext(ctx).Warn("Remote connection closed")
				return nil, nil
			}
			return nil, err
		}

		// 7. 流式响应处理
		if err := handler.StreamResponse(ctx, chatCtx, content); err != nil {
			zlog.LogWithContext(ctx).Error("Stream response failed",
				zap.String("handler", handlerName),
				zap.Error(err))
		}

		// 8. 归档与异步任务
		return s.finalizeResponse(ctx, chatCtx, content, modelID, history)
	}

	zlog.LogWithContext(ctx).Warn("No suitable handler found for the request")
	return nil, errors.New("no suitable handler found")
}

// prepareContext 准备处理上下文
func (s *ChatService) prepareContext(ctx context.Context, req ChatRequest, streamServer interface{}) (*ChatContext, error) {
	switch req.GetRequestType() {
	case RequestTypeStandard, RequestTypeCustomPersonalInfoQA:
		stdReq := req.GetOriginalRequest().(*vai.ChatMessageSendRequest)
		// 临时在程序中隐藏推理内容
		if req.GetRequestType() == RequestTypeCustomPersonalInfoQA {
			stdReq.Message.HiddenReasoning = true
		}
		reqHeader := stdReq.GetRequestHeader()
		reqMsg := stdReq.GetMessage()
		userID := reqHeader.GetUserId()
		chatID := reqMsg.GetChatId()
		content := reqMsg.GetContent()
		messageType := reqMsg.GetMessageType()
		chatBizHandlerID := reqMsg.GetChatBizHandlerId()
		messageID := reqMsg.GetMessageId()
		promptID := reqMsg.GetPromptId()
		replyMessageID := generateMsgID()

		// 处理URL
		urls := reqMsg.GetUrls()
		blobs := reqMsg.GetBlobs()
		if reqMsg.GetUrl() != "" {
			urls = append(urls, reqMsg.GetUrl())
		}

		// 处理视频帧
		var frameURL string
		if messageType == vai.MessageType_MT_VIDEO && len(blobs) > 0 {
			frameURL = s.processVideoFrame(ctx, reqHeader, messageID, blobs[0].GetData())
		}

		// 处理语音
		var voiceContent string
		var voiceHash string
		if reqMsg.GetVoiceInfo() != nil {
			voiceHash = reqMsg.GetVoiceInfo().GetMd5()
			if voiceHash != "" {
				voiceContent = s.processVoice(ctx, voiceHash)
			}
		}
		// 处理个人档案信息ID
		personalInfoID := reqMsg.GetCustomProfileIds()

		// 确保消息ID不为空
		if strings.TrimSpace(messageID) == "" {
			messageID = generateMsgID()
		}

		return &ChatContext{
			UserID:           userID,
			ChatID:           chatID,
			Content:          content,
			MessageType:      messageType,
			ChatBizHandlerID: chatBizHandlerID,
			MessageID:        messageID,
			ReplyMessageID:   replyMessageID,
			PromptID:         promptID,
			URLs:             urls,
			Blobs:            blobs,
			VoiceContent:     voiceContent,
			VoiceHash:        voiceHash,
			FrameURL:         frameURL,
			ParticipantIDs:   personalInfoID,
			Request:          req,
			StreamServer:     streamServer,
		}, nil
	default:
		return nil, errors.New("unsupported request type")
	}
}

// finalizeResponse 完成响应处理
func (s *ChatService) finalizeResponse(ctx context.Context, chatCtx *ChatContext, content string, modelID interface{}, history []model.MessageHistory) (*vai.Message, error) {
	// 归档消息
	finalModelID, ok := modelID.(vai.Model)
	if !ok {
		finalModelID = vai.Model_MODEL_GPT4O // 默认模型
	}

	// 处理帧URL
	urls := chatCtx.URLs
	if chatCtx.FrameURL != "" {
		urls = append(urls, chatCtx.FrameURL)
	}

	// 归档用户消息
	stdReq := chatCtx.Request.GetOriginalRequest().(*vai.ChatMessageSendRequest)
	_, err := s.msgService.ArchiveChatMessage(
		chatCtx.ChatID,
		chatCtx.MessageID,
		chatCtx.UserID,
		chatCtx.Chat.CreateTime,
		chatCtx.Content,
		urls,
		chatCtx.VoiceHash,
		vai.MessageSender_USER,
		chatCtx.MessageType,
		finalModelID.String(),
	)
	if err != nil {
		return nil, err
	}

	// 归档系统回复
	replyMsgID, err := s.msgService.ArchiveChatMessage(
		chatCtx.ChatID,
		chatCtx.ReplyMessageID,
		chatCtx.UserID,
		chatCtx.Chat.CreateTime,
		content,
		[]string{},
		"",
		vai.MessageSender_ASSISTANT,
		vai.MessageType_MT_TEXT,
		finalModelID.String(),
	)
	if err != nil {
		zlog.LogWithContext(ctx).Error("ArchiveChatMessage Error", zap.Error(err))
		return nil, nil
	}

	// 构建回复消息
	responseMsg := &vai.Message{
		ChatId:      chatCtx.ChatID,
		MessageId:   replyMsgID,
		Content:     content,
		CreateTime:  time.Now().Format(common.TimestampFormat),
		MessageType: vai.MessageType_MT_TEXT,
		Sender:      vai.MessageSender_ASSISTANT,
	}
	if chatCtx.GetMessageType() != vai.MessageType_MT_VIDEO {
		// 启动异步任务
		s.startAsyncTasks(ctx, stdReq, chatCtx.ChatID, chatCtx.Chat.Title,
			append(history, model.MessageHistory{Content: content, Sender: "assistant"}))
	}

	return responseMsg, nil
}

func (s *ChatService) GetChatInfo(ctx context.Context, projectID, chatID string) (*vai.Chat, error) {
	if chatID == "" {
		zlog.Logger.Error("ChatID Is Required")
		return nil, constants.ERR_INVALID_REQUEST
	}
	chat, err := s.chatDao.GetByID(projectID, chatID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetChatByID", zap.Error(err))
		return nil, err
	}
	if chat == nil {
		zlog.LogWithContext(ctx).Error("chat not found")
		return nil, constants.ERR_INVALID_REQUEST
	}
	if chat.Status == 1 {
		zlog.LogWithContext(ctx).Error("Chat Has Been Archived")
		return nil, constants.ERR_INVALID_REQUEST
	}
	return &vai.Chat{
		ChatId:     chat.ChatID,
		Title:      chat.Title,
		Image:      "",
		CreateTime: time.Unix(chat.CreateTime, 0).Format(common.TimestampFormat),
	}, nil
}

// buildBasicChat 将 model.Chat 转换为基础的 vai.Chat
func (s *ChatService) buildBasicChat(ctx context.Context, chat *model.Chat, lang string) *vai.Chat {
	vaiChat := &vai.Chat{
		ChatId:     chat.ChatID,
		Title:      chat.Title,
		CreateTime: time.Unix(chat.CreateTime, 0).Format(common.TimestampFormat),
		Image:      chat.Image,
	}

	if chat.LastTime != nil {
		vaiChat.LastTime = chat.LastTime.Format(time.DateTime)
	}

	// 获取 Prompt 信息
	if chat.PromptID != "" {
		promptDao := dao.NewPromptDao(s.db)
		prompt, err := promptDao.GetPrompt(chat.PromptID, lang)
		if err == nil && prompt != nil {
			vaiChat.Prompt = prompt.ConvertToVAIPrompt()
		}
	}

	return vaiChat
}

// // enrichChatWithProfiles 为聊天添加档案和用户档案信息
// func (s *ChatService) enrichChatWithProfiles(ctx context.Context, chat *vai.Chat, userID string) error {
// 	// 查询聊天参与者中是否有档案ID
// 	participantDao := dao.NewChatParticipantDao(s.db)
// 	participants, err := participantDao.GetParticipantsByChatID(chat.GetChatId())
// 	if err != nil {
// 		zlog.LogWithContext(ctx).Error("Failed to get chat participants",
// 			zap.String("chatID", chat.GetChatId()),
// 			zap.Error(err))
// 		return err
// 	}

// 	profileDao := dao.NewProfileDao(s.db)
// 	projectID := common.GetProjectID(ctx)

// 	// 查找档案参与者
// 	var customProfileIDs []string
// 	for _, participant := range participants {
// 		if participant.ParticipantType == model.ParticipantTypeCustomProfile {
// 			customProfileIDs = append(customProfileIDs, participant.ParticipantID)
// 		}
// 	}

// 	// 1. 无论如何先查询并加入当前用户的 UserProfile
// 	userProfile, err := profileDao.GetUserProfile(ctx, projectID, userID)
// 	if err != nil {
// 		zlog.LogWithContext(ctx).Error("Failed to get user profile",
// 			zap.String("userID", userID),
// 			zap.Error(err))
// 		return err
// 	}
// 	// 预留空间 customProfileIDs + 1
// 	chat.Profiles = make([]*vai.UserProfile, 0, len(customProfileIDs)+1)
// 	if userProfile != nil {
// 		chat.Profiles = append(chat.Profiles, userProfile.ToGRPC())
// 	}

// 	// 2. 再查询并追加自定义档案
// 	if len(customProfileIDs) > 0 {
// 		customProfiles, err := profileDao.GetCustomerProfile(ctx, projectID, customProfileIDs)
// 		if err != nil {
// 			zlog.LogWithContext(ctx).Error("Failed to get custom profiles",
// 				zap.Strings("profileIDs", customProfileIDs),
// 				zap.Error(err))
// 		} else if len(customProfiles) > 0 {
// 			for _, p := range customProfiles {
// 				chat.Profiles = append(chat.Profiles, p.CustomProfileToGRPC().GetProfile())
// 			}
// 		}
// 	}

// 	return nil
// }

// GetChatList 获取聊天列表
func (s *ChatService) GetChatList(ctx context.Context, userID, lang string, count, offset int) ([]*vai.Chat, error) {
	projectID := common.GetProjectID(ctx)
	// 获取基础聊天列表
	chats, err := s.chatDao.GetChatList(projectID, userID, count, offset)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to get chat list",
			zap.String("userID", userID),
			zap.Error(err))
		return nil, err
	}

	// 转换并丰富聊天信息
	vaiChats := make([]*vai.Chat, 0, len(chats))
	for _, chat := range chats {
		vaiChat := s.buildBasicChat(ctx, chat, lang)

		// // 添加档案和用户档案信息
		// if err := s.enrichChatWithProfiles(ctx, vaiChat, userID); err != nil {
		// 	zlog.LogWithContext(ctx).Error("Failed to enrich chat with profiles",
		// 		zap.String("chatID", chat.ChatID),
		// 		zap.Error(err))
		// 	continue
		// }

		vaiChats = append(vaiChats, vaiChat)
	}

	return vaiChats, nil
}

// ArchiveAllChats 归档用户的所有对话
func (s *ChatService) ArchiveAllChats(ctx context.Context, projectID, userID string) error {
	if userID == "" {
		zlog.LogWithContext(ctx).Error("userID is required")
		return constants.ERR_INVALID_PARAM
	}

	if projectID == "" {
		zlog.LogWithContext(ctx).Error("projectID is required")
		return constants.ERR_INVALID_PARAM
	}

	if err := s.chatDao.ChatArchivedAll(projectID, userID); err != nil {
		zlog.LogWithContext(ctx).Error("Failed to archive all chats",
			zap.String("userID", userID),
			zap.String("projectID", projectID),
			zap.Error(err))
		return err
	}

	zlog.LogWithContext(ctx).Info("Successfully archived all chats",
		zap.String("userID", userID),
		zap.String("projectID", projectID))
	return nil
}
