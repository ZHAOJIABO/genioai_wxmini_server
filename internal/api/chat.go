package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"va_visionai_server/conf"
	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/chat"
	"va_visionai_server/internal/service/chat_message"
	"va_visionai_server/internal/service/prompt"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type ChatServer struct {
	chatService            *chat.ChatService
	userService            *service.UserService
	msgService             *service.MessageService
	subscribeService       *service.SubscribeService
	promptService          *prompt.PromptService
	modelService           *service.ModelService
	chatDao                *dao.ChatDao
	chatParticipantService *chat.ChatParticipantService
	chatMessageService     *chat_message.ChatMessageService

	vai.UnimplementedChatServiceServer
}

func NewChatServer(chatService *chat.ChatService, userService *service.UserService,
	msgService *service.MessageService, subscribeService *service.SubscribeService,
	promptService *prompt.PromptService, chatDao *dao.ChatDao,
	chatParticipantService *chat.ChatParticipantService,
	chatMessageService *chat_message.ChatMessageService) *ChatServer {
	return &ChatServer{
		chatService:            chatService,
		userService:            userService,
		msgService:             msgService,
		subscribeService:       subscribeService,
		promptService:          promptService,
		modelService:           service.NewModelService(db.GetDB()),
		chatDao:                chatDao,
		chatParticipantService: chatParticipantService,
		chatMessageService:     chatMessageService,
	}
}

func (s *ChatServer) GetChatList(ctx context.Context, req *vai.ChatListRequest) (*vai.ChatListResponse, error) {
	reqHeader := req.GetRequestHeader()
	var err error
	var rsp *vai.ChatListResponse
	defer func() {
		zlog.LogWithContext(ctx).Info("GetChatList",
			zap.Int32("Offset", req.GetOffset()),
			zap.Int32("Count", req.GetCount()),
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
			zap.Any("ChatListLen", len(rsp.GetChats())),
			zap.Error(err))
	}()

	code, err := isVerifyAccessToken(s.userService, reqHeader)
	if err != nil {
		rsp = &vai.ChatListResponse{}
		rsp, err = BuildErrorResponse[vai.ChatListResponse](ctx, code, err, constants.CodeMsg(code))
		return rsp, nil
	}

	lang := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	replyChatList, err := s.chatService.GetChatList(ctx, reqHeader.GetUserId(), lang, int(req.GetCount()), int(req.GetOffset()))
	if err != nil {
		rsp = &vai.ChatListResponse{}
		rsp, err = BuildErrorResponse[vai.ChatListResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.ChatListResponse{
		Chats: replyChatList,
	})
	return rsp, nil
}

func (s *ChatServer) GetChatMessage(ctx context.Context, req *vai.ChatMessageListRequest) (*vai.ChatMessageListResponse, error) {
	reqHeader := req.GetRequestHeader()
	var err error
	var rsp *vai.ChatMessageListResponse
	defer func() {
		zlog.LogWithContext(ctx).Info("GetChatMessage",
			zap.String("ChatID", req.GetChatId()),
			zap.Int32("Offset", req.GetOffset()),
			zap.Int32("Count", req.GetCount()),
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
			zap.Any("MessageLen", len(rsp.GetMessages())),
			zap.Error(err))
	}()

	code, err := isVerifyAccessToken(s.userService, reqHeader)
	if err != nil {
		rsp = &vai.ChatMessageListResponse{}
		rsp, err = BuildErrorResponse[vai.ChatMessageListResponse](ctx, code, err, constants.CodeMsg(code))
		return rsp, nil
	}

	if req.GetChatId() == "" {
		rsp = &vai.ChatMessageListResponse{}
		rsp, err = BuildErrorResponse[vai.ChatMessageListResponse](ctx, vai.StatusCode_INVALID_PARAM, errors.New("chat id is empty"), constants.ErrMsgInvalidParam)
		return rsp, nil
	}

	_, err = s.chatService.GetChatInfo(ctx, common.GetProjectID(ctx), req.GetChatId())
	if err != nil {
		rsp = &vai.ChatMessageListResponse{}
		rsp, err = BuildErrorResponse[vai.ChatMessageListResponse](ctx, vai.StatusCode_INVALID_REQUEST, err, constants.ErrMsgInvalidRequest)
		return rsp, nil
	}

	replyMsgList, err := s.msgService.GetMessage(ctx, req.GetChatId(), req.GetOffset(), req.GetCount(), true)
	if err != nil {
		rsp = &vai.ChatMessageListResponse{}
		rsp, err = BuildErrorResponse[vai.ChatMessageListResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.ChatMessageListResponse{
		Messages: replyMsgList,
	})
	return rsp, nil
}

func (s *ChatServer) SendChatMessageStream(req *vai.ChatMessageSendRequest, streamServer vai.ChatService_SendChatMessageStreamServer) error {
	ctx := streamServer.Context()
	userID := req.GetRequestHeader().GetUserId()
	msgType := req.GetMessage().GetMessageType()
	ctx = context.WithValue(ctx, constants.CtxModelID, req.GetMessage().GetModelId().Number())
	ctx = context.WithValue(ctx, constants.CtxModelName, req.GetMessage().GetModelName())
	s.logStreamMetrics(ctx, req, "SendChatMessageStream Request", constants.EventSendChatMessageStream)
	// Defer logging
	defer func(streamServer vai.ChatService_SendChatMessageStreamServer) {
		s.logStreamMetrics(streamServer.Context(), req, "SendChatMessageStream Request Complete", constants.EventSendChatMessageStreamComplete)
	}(streamServer)

	// Verify access and subscription
	if err := s.verifyAccessAndSubscription(ctx, req, streamServer, userID, msgType); err != nil {
		zlog.LogWithContext(ctx).Error("verifyAccessAndSubscription", zap.Error(err))
		return err
	}
	if s.chatDao.FirstChat(req.GetRequestHeader().GetUserId()) {
		attributionService := service.NewattributionService()
		_, err := attributionService.RecordEventInfo(req.GetRequestHeader(), constants.QnACompletionFirst)
		if err != nil {
			zlog.LogWithContext(ctx).Error("Record EventInfo Error", zap.Error(err))
		}
	}

	// Process chat message
	if _, err := s.chatService.Process(ctx, req, streamServer); err != nil {
		return s.handleStreamError(streamServer, req.GetRequestHeader(), err)
	}

	return nil
}

func (s *ChatServer) handleStreamError(streamServer vai.ChatService_SendChatMessageStreamServer, reqHeader *vai.RequestHeader, err error) error {
	resp := &vai.ChatMessageStreamResponse{
		MessageToken: "",
		IsEnd:        true,
		ResponseHeader: &vai.ResponseHeader{
			Code:           vai.StatusCode_REQUEST_FAILED,
			Msg:            constants.CodeMsg(vai.StatusCode_REQUEST_FAILED),
			ReqId:          reqHeader.GetReqId(),
			ResponseTimeMs: time.Now().UnixMilli(),
			ServerTime:     time.Now().Format(common.TimestampFormat),
		},
	}

	if sendErr := streamServer.Send(resp); sendErr != nil {
		zlog.Logger.Error("Failed to send error message",
			zap.Error(sendErr),
		)
		return sendErr
	}
	return err
}

func (s *ChatServer) ChatArchive(ctx context.Context, req *vai.ChatArchiveRequest) (*vai.ChatArchiveResponse, error) {
	reqHeader := req.GetRequestHeader()
	var err error
	var rsp *vai.ChatArchiveResponse
	defer func() {
		zlog.LogWithContext(ctx).Info("ChatArchive",
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
			zap.Error(err))
	}()

	code, err := isVerifyAccessToken(s.userService, reqHeader)
	if err != nil {
		rsp = &vai.ChatArchiveResponse{}
		rsp, err = BuildErrorResponse[vai.ChatArchiveResponse](ctx, code, err, constants.CodeMsg(code))
		return rsp, nil
	}

	err = s.chatDao.ChatArchived(common.GetProjectID(ctx), req.GetChatId())
	if err != nil {
		rsp = &vai.ChatArchiveResponse{}
		rsp, err = BuildErrorResponse[vai.ChatArchiveResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.ChatArchiveResponse{})
	return rsp, nil
}

func (s *ChatServer) ClearChatHistory(ctx context.Context, req *vai.ClearChatHistoryRequest) (*vai.ClearChatHistoryResponse, error) {
	reqHeader := req.GetRequestHeader()
	userID := reqHeader.GetUserId()
	var err error
	var rsp *vai.ClearChatHistoryResponse
	defer func() {
		zlog.LogWithContext(ctx).Info("ClearChatHistory",
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
			zap.Error(err))
	}()

	code, err := isVerifyAccessToken(s.userService, reqHeader)
	if err != nil {
		rsp = &vai.ClearChatHistoryResponse{}
		rsp, err = BuildErrorResponse[vai.ClearChatHistoryResponse](ctx, code, err, constants.CodeMsg(code))
		return rsp, nil
	}

	err = s.chatDao.ChatArchivedAll(common.GetProjectID(ctx), userID)
	if err != nil {
		rsp = &vai.ClearChatHistoryResponse{}
		rsp, err = BuildErrorResponse[vai.ClearChatHistoryResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.ClearChatHistoryResponse{})
	return rsp, nil
}

func (s *ChatServer) logStreamMetrics(ctx context.Context, req *vai.ChatMessageSendRequest, msg string, eventType constants.ServiceEventType) {
	inToken, outToken := utils.GetInOutTokenCount(ctx)

	zlog.LogWithContext(ctx).Info(msg,
		zap.String("ChatID", req.GetMessage().GetChatId()),
		zap.Int("InputToken", inToken),
		zap.Int("OutputToken", outToken),
		zap.String("URL", req.GetMessage().GetUrl()),
		zap.String("MsgType", req.GetMessage().GetMessageType().String()),
		zap.Any("VoiceHash", req.GetMessage().GetVoiceInfo().GetMd5()),
		zap.Any(constants.ServiceEvent, eventType),
		zap.String(constants.CtxPromptID, req.GetMessage().GetPromptId()))
}

func (s *ChatServer) verifyAccessAndSubscription(ctx context.Context, req *vai.ChatMessageSendRequest,
	streamServer vai.ChatService_SendChatMessageStreamServer, userID string, msgType vai.MessageType) error {
	os := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())
	packageName := req.GetRequestHeader().GetApp().GetPackageName()
	if common.IsLimited(packageName, os) {
		return errors.New("Not Support")
	}
	// Build system prompt
	if err := s.promptService.BuildReqSystemPrompt(ctx, req); err != nil {
		return s.handleStreamError(streamServer, req.GetRequestHeader(), err)
	}

	if conf.IsLocal() {
		return nil
	}

	appVersion := req.GetRequestHeader().GetApp().GetAppVersion()
	if os == constants.IOS && s.isNoLimitVersion(appVersion) {
		zlog.LogWithContext(ctx).Info("current app version is no limit version",
			zap.String("appVersion", appVersion),
			zap.String("os", os),
		)
	} else if os == constants.ANDROID && s.isGoogleNoLimitVersion(appVersion) && packageName == constants.ProjectIdVisualAI {
		zlog.LogWithContext(ctx).Info("current app version is google no limit version",
			zap.String("appVersion", appVersion),
			zap.String("os", os),
		)
	} else {
		if !s.subscribeService.CheckAndDeductUserAmount(ctx, userID, msgType) {
			zlog.LogWithContext(ctx).Info("user amount is not enough",
				zap.String("userID", userID),
				zap.String("msgType", msgType.String()),
			)
			return s.sendLimitExceededResponse(streamServer)
		}
	}

	// Verify access token
	if code, err := isVerifyAccessToken(s.userService, req.GetRequestHeader()); err != nil {
		return s.sendAuthErrorResponse(streamServer, req.GetRequestHeader(), code)
	}

	return nil
}

func (s *ChatServer) sendLimitExceededResponse(streamServer vai.ChatService_SendChatMessageStreamServer) error {
	resp := &vai.ChatMessageStreamResponse{
		MessageToken: "",
		IsEnd:        true,
		ResponseHeader: &vai.ResponseHeader{
			Code:           vai.StatusCode_DAILY_TOKEN_LIMIT_EXCEEDED,
			Msg:            constants.CodeMsg(vai.StatusCode_DAILY_TOKEN_LIMIT_EXCEEDED),
			ResponseTimeMs: time.Now().UnixMilli(),
			ServerTime:     time.Now().Format(common.TimestampFormat),
		},
	}
	if err := streamServer.Send(resp); err != nil {
		zlog.LogWithContext(streamServer.Context()).Error("Failed to send limit exceeded error",
			zap.Error(err))
		return err
	}
	return constants.ERR_DAILY_TOLEN_LIMIT
}

func (s *ChatServer) sendAuthErrorResponse(streamServer vai.ChatService_SendChatMessageStreamServer,
	reqHeader *vai.RequestHeader, code vai.StatusCode) error {

	resp := &vai.ChatMessageStreamResponse{
		MessageToken: "",
		IsEnd:        true,
		ResponseHeader: &vai.ResponseHeader{
			Code:           code,
			Msg:            constants.CodeMsg(code),
			ReqId:          reqHeader.GetReqId(),
			ResponseTimeMs: time.Now().UnixMilli(),
			ServerTime:     time.Now().Format(common.TimestampFormat),
		},
	}

	if err := streamServer.Send(resp); err != nil {
		zlog.LogWithContext(streamServer.Context()).Error("Failed to send auth error",
			zap.Error(err))
		return err
	}
	return errors.New(constants.CodeMsg(code))
}

func (s *ChatServer) isNoLimitVersion(appVersion string) bool {
	compareVersions := func(v1, v2 string) (int, error) {
		ver1, err := version.NewVersion(v1)
		if err != nil {
			return 0, err
		}
		ver2, err := version.NewVersion(v2)
		if err != nil {
			return 0, err
		}

		return ver1.Compare(ver2), nil
	}
	lastNolimitVersion := "1.5.0"
	compare, err := compareVersions(appVersion, lastNolimitVersion)
	if err != nil {
		return false
	}
	return compare < 0
}

func (s *ChatServer) isGoogleNoLimitVersion(appVersion string) bool {
	compareVersions := func(v1, v2 string) (int, error) {
		ver1, err := version.NewVersion(v1)
		if err != nil {
			return 0, err
		}
		ver2, err := version.NewVersion(v2)
		if err != nil {
			return 0, err
		}

		return ver1.Compare(ver2), nil
	}
	lastNolimitVersion := "3.0.0"
	compare, err := compareVersions(appVersion, lastNolimitVersion)
	if err != nil {
		return false
	}
	return compare < 0
}

func (s *ChatServer) ListModels(ctx context.Context, req *vai.ListModelsRequest) (*vai.ListModelsResponse, error) {
	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.ListModelsResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}

	language := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	projectID := common.GetProjectID(ctx)
	if projectID == constants.ProjectIdVisualAI {
		projectID = constants.ProjectIdVisionAI
	}

	models, err := s.modelService.ListModelsWithLang(ctx, projectID, language)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to list models", zap.Error(err))
	}
	if len(models) == 0 {
		models, err = s.modelService.ListModelsWithLang(ctx, projectID, constants.EN)
		if err != nil {
			return BuildErrorResponse[vai.ListModelsResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list models")
		}
	}

	modelInfos := make([]*vai.ModelInfo, 0, len(models))
	for _, m := range models {
		modelInfos = append(modelInfos, m.ToProto())
	}

	return BuildSuccessResponse(&vai.ListModelsResponse{
		Models: modelInfos,
	})
}

func (s *ChatServer) ListUserProfileChat(ctx context.Context, req *vai.ListUserProfileChatRequest) (*vai.ListUserProfileChatResponse, error) {
	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.ListUserProfileChatResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}

	code, err := isVerifyAccessToken(s.userService, req.GetRequestHeader())
	if err != nil {
		return BuildErrorResponse[vai.ListUserProfileChatResponse](ctx, code, err, constants.CodeMsg(code))
	}

	chatList, err := s.chatParticipantService.ListUserChat(ctx, req.GetProfileId(), int(req.GetOffset()), int(req.GetCount()))
	if err != nil {
		return BuildErrorResponse[vai.ListUserProfileChatResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list user profile chat")
	}

	return BuildSuccessResponse(&vai.ListUserProfileChatResponse{
		Chats: chatList,
	})
}

func (s *ChatServer) SuggestQuestionsWithProfile(ctx context.Context, req *vai.SuggestQuestionsWithProfileRequest) (*vai.SuggestQuestionsWithProfileResponse, error) {
	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.SuggestQuestionsWithProfileResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}

	code, err := isVerifyAccessToken(s.userService, req.GetRequestHeader())
	if err != nil {
		return BuildErrorResponse[vai.SuggestQuestionsWithProfileResponse](ctx, code, err, constants.CodeMsg(code))
	}

	lang := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	questions, err := s.chatParticipantService.SuggestQuestions(ctx, req.GetChatId(), req.GetProfileId(), lang)
	if err != nil {
		return BuildErrorResponse[vai.SuggestQuestionsWithProfileResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to suggest questions")
	}

	return BuildSuccessResponse(&vai.SuggestQuestionsWithProfileResponse{
		Questions: questions,
	})
}

func (s *ChatServer) ScrollChatMessage(ctx context.Context, req *vai.ScrollChatMessageRequest) (*vai.ScrollChatMessageResponse, error) {
	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.ScrollChatMessageResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}

	chatID := req.GetChatId()
	cursor := req.GetCursor()
	limit := req.GetLimit()
	userID := req.GetRequestHeader().GetUserId()
	profileID := req.GetProfileId()
	messages, err := s.chatMessageService.GetUserMessagesSequentially(ctx, userID, chatID, cursor, int(limit), profileID)
	if err != nil {
		return BuildErrorResponse[vai.ScrollChatMessageResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to scroll chat message")
	}

	replyMessages := make([]*vai.Message, 0, len(messages.Messages))
	for _, msg := range messages.Messages {
		rspMsg := msg.ToProto()
		if msg.ModelId == vai.Model_MODEL_DEEPSEEK_R1.String() || strings.Contains(msg.ModelId, "THINKING") {
			rspMsg.IsReasoning = true
		}
		replyMessages = append(replyMessages, rspMsg)
	}

	return BuildSuccessResponse(&vai.ScrollChatMessageResponse{
		Messages: replyMessages,
		Cursor:   messages.NextCursor,
	})

}

func (s *ChatServer) DeleteChatMessage(ctx context.Context, req *vai.DeleteChatMessageRequest) (*vai.DeleteChatMessageResponse, error) {
	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.DeleteChatMessageResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}

	code, err := isVerifyAccessToken(s.userService, req.GetRequestHeader())
	if err != nil {
		return BuildErrorResponse[vai.DeleteChatMessageResponse](ctx, code, err, constants.CodeMsg(code))
	}

	err = s.msgService.DeleteMessage(ctx, req.GetMessageId())
	if err != nil {
		return BuildErrorResponse[vai.DeleteChatMessageResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to delete chat message")
	}

	return BuildSuccessResponse(&vai.DeleteChatMessageResponse{})
}

func (s *ChatServer) StarlitQuestionsWithProfile(req *vai.StarlitQuestionsWithProfileRequest, streamServer vai.ChatService_StarlitQuestionsWithProfileServer) error {
	// 星座服务已删除，该功能已禁用
	return status.Errorf(codes.Unimplemented, "Starlit service has been removed")
}
