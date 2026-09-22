package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"
	"google.golang.org/genai"
	"gorm.io/gorm"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	chatcommon "va_visionai_server/internal/service/chat/common"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// CustomeUserPersonalInfoLLMHandler 处理常规请求
type CustomeUserPersonalInfoLLMHandler struct {
	BaseBizHandler
	llmService          chatcommon.LLMService
	personalInfoDao     *dao.ProfileDao
	personalInfoChatDao *dao.UserPersonalChatDao
	promptDao           *dao.PromptDao
	chatParticipantDao  *dao.ChatParticipantDao
}

// Name 返回处理器名称
func (h *CustomeUserPersonalInfoLLMHandler) Name() string {
	return "personal_info_llm_handler"
}

// NewCustomUserPersonalInfoLLMHandler 构造函数
func NewCustomUserPersonalInfoLLMHandler(
	llmSvc chatcommon.LLMService,
	msgSvc MessageService,
	personalInfoDao *dao.ProfileDao,
	personalInfoChatDao *dao.UserPersonalChatDao,
	promptDao *dao.PromptDao,
	chatDao *dao.ChatDao,
	chatParticipantDao *dao.ChatParticipantDao,
) *CustomeUserPersonalInfoLLMHandler {
	return &CustomeUserPersonalInfoLLMHandler{
		BaseBizHandler:      NewBaseBizHandler(msgSvc, chatDao),
		llmService:          llmSvc,
		personalInfoDao:     personalInfoDao,
		personalInfoChatDao: personalInfoChatDao,
		promptDao:           promptDao,
		chatParticipantDao:  chatParticipantDao,
	}
}

func (h *CustomeUserPersonalInfoLLMHandler) Match(ctx chatcommon.ChatContext) bool {
	req := ctx.GetRequest()
	if req == nil {
		return false
	}

	if chatReq, ok := req.(interface{ GetChatBizHandlerID() vai.ChatBizHandlerID }); ok {
		return chatReq.GetChatBizHandlerID() == vai.ChatBizHandlerID_CHAT_BIZ_HANDLER_ID_USER_PERSONAL_INFO
	}

	return false
}

func (h *CustomeUserPersonalInfoLLMHandler) Handle(ctx context.Context, chatCtx chatcommon.ChatContext, history []model.MessageHistory) (string, interface{}, error) {
	llmStartTime := time.Now()

	if chatCtx.GetRequest() == nil {
		return "", nil, errors.New("invalid request")
	}

	originalReq := chatCtx.GetRequest().(interface{ GetOriginalRequest() interface{} }).GetOriginalRequest()
	req, ok := originalReq.(*vai.ChatMessageSendRequest)
	if !ok {
		return "", nil, errors.New("invalid request type")
	}

	streamServer, _ := chatCtx.GetStreamServer().(vai.ChatService_SendChatMessageStreamServer)

	// 使用 callbackProcessor 统一管理 suffix/processed/stop
	proc := newCallbackProcessor()

	// 准备一个 Builder 用来累积最终输出
	var sb strings.Builder

	cleanInvalidUTF8 := func(s string) string {
		if utf8.ValidString(s) {
			return s
		}
		return strings.ToValidUTF8(s, "")
	}

	send := func(ctx context.Context, token string) error {
		if streamServer == nil {
			return nil
		}

		cleanToken := cleanInvalidUTF8(token)
		if cleanToken == "" {
			return nil
		}

		res := &vai.ChatMessageStreamResponse{
			MessageToken:   cleanToken,
			ReqMessageId:   req.GetMessage().GetMessageId(),
			ReplyMessageId: chatCtx.GetReplyMessageID(),
			ResponseHeader: &vai.ResponseHeader{
				Code:  0,
				Msg:   "success",
				ReqId: req.GetRequestHeader().GetReqId(),
			},
			IsEnd: false,
		}

		if err := streamServer.Context().Err(); err != nil {
			zlog.LogWithContext(ctx).Error("Stream Context Done", zap.Error(err))
			return ErrRemoteClose
		}

		if err := streamServer.Send(res); err != nil {
			zlog.LogWithContext(ctx).Error("StreamServer send failed",
				zap.Error(err),
				zap.String("token", cleanToken))
			return ErrRemoteClose
		}

		return nil
	}

	// wrappedCallback 委托给 proc 处理，并累积处理后的内容
	wrappedCallback := func(ctx context.Context, token string) error {
		toSend, _ := proc.process(ctx, token)
		if toSend != "" {
			sb.WriteString(toSend) // 累积处理后的片段
			if err := send(ctx, toSend); err != nil {
				return err
			}
		}
		return nil
	}

	ctx, finalModelID, imageContent, _ := h.llmService.ResolveModel(ctx, req, history)

	modelName := req.GetMessage().GetModelName()
	var thinkingConfig *common.ThinkingConfig
	switch modelName {
	case vai.Model_MODEL_GEMINI_2_5_FLASH_THINKING.String():
		thinkingConfig = &common.ThinkingConfig{
			IncludeThoughts: true,
			ThinkingBudget:  genai.Ptr(int32(256)),
		}
	case vai.Model_MODEL_GEMINI_2_5_PRO_THINKING.String():
		thinkingConfig = &common.ThinkingConfig{
			IncludeThoughts: true,
		}
	}

	_, err := h.llmService.StreamProcess(
		ctx,
		req,
		history,
		wrappedCallback,
		chatcommon.LLMOptions{
			InitialModel:   finalModelID,
			GlobalTimeout:  120 * time.Second,
			GapTimeout:     60 * time.Second,
			ImageContent:   imageContent,
			ThinkingConfig: thinkingConfig,
		},
	)

	// 将剩余的 suffix 也累积并发送
	if rem := proc.suffix; rem != "" && !proc.stop {
		sb.WriteString(rem)
		if err := send(ctx, rem); err != nil {
			zlog.LogWithContext(ctx).Error("Failed to send remaining suffix",
				zap.Error(err),
				zap.String("suffix", rem))
		}
	}

	// 解析并发送 follow_questions
	if proc.followQuestionText != "" {
		// 直接按行切分为建议问题数组

		questions := strings.Split(strings.TrimSpace(proc.followQuestionText), "\n")
		zlog.LogWithContext(ctx).Debug("[(p *callbackProcessor) process] FollowQuestionText",
			zap.String("followQuestionText", strings.TrimSpace(proc.followQuestionText)),
			zap.Any("questions", questions),
		)
		followQuestions := make([]string, 0, 3)
		for _, question := range questions {
			if question == "" || question == "**" {
				continue
			}
			question = strings.ReplaceAll(question, "*", "")
			question = strings.ReplaceAll(question, "-", "")
			question = strings.ReplaceAll(question, "•", "")
			question = strings.TrimSpace(question)
			followQuestions = append(followQuestions, question)
			if len(followQuestions) >= 3 {
				break
			}
		}

		// 发送一次带 FollowQuestions 的响应
		if streamServer != nil {
			resp := &vai.ChatMessageStreamResponse{
				ReqMessageId:    req.GetMessage().GetMessageId(),
				ReplyMessageId:  chatCtx.GetReplyMessageID(),
				FollowQuestions: followQuestions,
				IsEnd:           false,
				ResponseHeader: &vai.ResponseHeader{
					Code:  0,
					Msg:   "success",
					ReqId: req.GetRequestHeader().GetReqId(),
				},
			}
			if err := streamServer.Send(resp); err != nil {
				zlog.LogWithContext(ctx).Error("Failed to send follow questions",
					zap.Error(err))
			}
		}
	}

	// 用累积的内容作为最终回复
	processedReply := sb.String()

	llmDuration := time.Since(llmStartTime)
	zlog.LogWithContext(ctx).Info("LLM processing completed",
		zap.Duration("duration", llmDuration),
		zap.Int("replyLength", len(processedReply)),
		zap.String(constants.CtxChatID, req.GetMessage().GetChatId()),
	)

	return processedReply, finalModelID, err
}

func (h *CustomeUserPersonalInfoLLMHandler) StreamResponse(ctx context.Context, chatCtx chatcommon.ChatContext, content string) error {
	// 获取流式服务器
	streamServer, ok := chatCtx.GetStreamServer().(vai.ChatService_SendChatMessageStreamServer)
	if !ok || streamServer == nil {
		return nil
	}

	// 从上下文提取请求
	originalReq := chatCtx.GetRequest().(interface{ GetOriginalRequest() interface{} }).GetOriginalRequest()
	req, ok := originalReq.(*vai.ChatMessageSendRequest)
	if !ok {
		return errors.New("invalid request type")
	}

	reqHeader := req.GetRequestHeader()

	// 发送结束消息
	err := streamServer.Send(&vai.ChatMessageStreamResponse{
		ReqMessageId:   req.GetMessage().GetMessageId(),
		ReplyMessageId: chatCtx.GetReplyMessageID(),
		MessageToken:   "" + AppendLowVersionWarningIfMac(reqHeader),
		IsEnd:          true,
		ResponseHeader: &vai.ResponseHeader{
			Code:           vai.StatusCode_SUCCESS,
			Msg:            "ok",
			ReqId:          reqHeader.GetReqId(),
			ResponseTimeMs: time.Now().UnixMilli(),
			ServerTime:     time.Now().Format(common.TimestampFormat),
		},
	})

	if err != nil {
		zlog.LogWithContext(ctx).Error("ChatStream Send EndTag Msg Error", zap.Error(err))
	}

	return nil
}

func (h *CustomeUserPersonalInfoLLMHandler) PrepareHistory(ctx context.Context, chatCtx chatcommon.ChatContext) ([]model.MessageHistory, error) {

	msgHistory, err := h.msgService.GetMsgHistory(ctx, chatCtx.GetChatID())
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		zlog.LogWithContext(ctx).Error("GetMsgHistory Error", zap.Error(err))
		return nil, err
	}

	// userMsg := model.MessageHistory{}
	// 构建当前用户消息
	userMsg := model.MessageHistory{
		Content: chatCtx.GetContent(),
		// URLs:     chatCtx.GetURLs(),
		Sender:   "user",
		FileType: chatCtx.GetMessageType().(vai.MessageType),
	}
	msgHistory = append(msgHistory, userMsg)

	return msgHistory, nil
}

func (h *CustomeUserPersonalInfoLLMHandler) PrepareSystemPrompt(ctx context.Context, chatCtx chatcommon.ChatContext) error {
	stdReq, ok := chatCtx.GetRequest().(interface{ GetOriginalRequest() interface{} }).GetOriginalRequest().(*vai.ChatMessageSendRequest)
	if !ok {
		return errors.New("invalid request type")
	}
	lang := common.CtxGetStrValue(ctx, constants.CtxLang)
	promptInfo, err := h.promptDao.GetAstroPrompt(constants.SystemConstellationUserProfileChatPrompt, lang)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Get Personal Info Prompt Error",
			zap.Error(err))
		return fmt.Errorf("failed to get system prompt %s: %w", constants.SystemConstellationUserProfileChatPrompt, err)
	}
	var buildLLMPersonalParams struct {
		GenderUser         string   `json:"gender_user"`
		DateBirthUser      string   `json:"date_birth_user"`
		PlaceBirthUser     string   `json:"place_birth_user"`
		SunSignUser        string   `json:"sun_sign_user"`
		MoonSignUser       string   `json:"moon_sign_user"`
		RisingSignUser     string   `json:"rising_sign_user"`
		GenderPartner      string   `json:"gender_partner"`
		DateBirthPartner   string   `json:"date_birth_partner"`
		PlaceBirthPartner  string   `json:"place_birth_partner"`
		SunSignPartner     string   `json:"sun_sign_partner"`
		MoonSignPartner    string   `json:"moon_sign_partner"`
		RisingSignPartner  string   `json:"rising_sign_partner"`
		RelationshipStatus string   `json:"relationship_status"`
		Question           string   `json:"question"`
		UserHouseInfo      []string `json:"user_house_info"`
		ProfileHouseInfo   []string `json:"profile_house_info"`
		UserAspects        []string `json:"user_aspects"`
		ProfileAspects     []string `json:"profile_aspects"`
		// PreQuestion        []string `json:"pre_question"`
	}
	// 基于请求类型调用不同实现
	reqParams := chatCtx.GetRequest()
	if reqParams == nil {
		zlog.LogWithContext(ctx).Error("Request is nil")
		// return errors.New("request is nil")
	}

	originalReq := reqParams.(interface{ GetOriginalRequest() interface{} }).GetOriginalRequest()
	req, ok := originalReq.(*vai.ChatMessageSendRequest)
	if !ok {
		zlog.LogWithContext(ctx).Error("Invalid request type")
		// return errors.New("invalid request type")
	}
	projectID := common.GetProjectID(ctx)
	userID := chatCtx.GetUserID()
	userPersonalInfo, err := h.personalInfoDao.GetUserProfile(ctx, projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetUserPersonalInfo Error", zap.Error(err))
		// return err
	}
	if userPersonalInfo != nil {
		buildLLMPersonalParams.GenderUser = userPersonalInfo.NickName
		buildLLMPersonalParams.DateBirthUser = time.Unix(userPersonalInfo.BirthTimestamp, 0).Format(common.TimestampFormat)
		buildLLMPersonalParams.PlaceBirthUser = userPersonalInfo.BirthAddress
	}
	personalInfoIDs := req.GetMessage().GetCustomProfileIds()
	if len(personalInfoIDs) > 0 {
		partnerPersonalInfos, err := h.personalInfoDao.GetCustomerProfile(ctx, projectID, personalInfoIDs)
		if err != nil {
			zlog.LogWithContext(ctx).Error("GetCustomerUserPersonalInfo Error", zap.Error(err))
			// return err
		}
		if len(partnerPersonalInfos) > 0 {
			partnerPersonalInfo := partnerPersonalInfos[0]
			buildLLMPersonalParams.GenderPartner = partnerPersonalInfo.NickName
			buildLLMPersonalParams.DateBirthPartner = time.Unix(partnerPersonalInfo.BirthTimestamp, 0).Format(common.TimestampFormat)
			buildLLMPersonalParams.PlaceBirthPartner = partnerPersonalInfo.BirthAddress
			buildLLMPersonalParams.SunSignPartner = partnerPersonalInfo.Constellation
			buildLLMPersonalParams.RelationshipStatus = partnerPersonalInfo.Relation
		}

	}
	buildLLMPersonalParams.Question = chatCtx.GetContent()

	// 星座服务已删除，相关功能已禁用
	// userChart, err := h.constellationService.GetConstellationChart(ctx, projectID, userID)
	// if err != nil {
	// 	zlog.LogWithContext(ctx).Error("GetConstellationChart Error", zap.Error(err))
	// }
	if len(personalInfoIDs) > 0 {
		// 星座服务已删除，相关功能已禁用
		// profileChart, err := h.constellationService.GetConstellationChartByProfileID(ctx, projectID, personalInfoIDs[0])
		// if err != nil {
		// 	zlog.LogWithContext(ctx).Error("GetConstellationChartByProfileID Error", zap.Error(err))
		// }
		// if profileChart != nil {
		// 	profileHouseParams := make([]string, 0, len(profileChart.NatalChart.Houses))
		// 	for _, house := range profileChart.NatalChart.Houses {
		// 		item := fmt.Sprintf("%s %s", house.Sign, house.Name)
		// 		profileHouseParams = append(profileHouseParams, item)
		// 	}
		// 	buildLLMPersonalParams.ProfileHouseInfo = profileHouseParams
		//
		// 	aspects := make([]string, 0, len(profileChart.TransitChart.TransitAspects))
		// 	for _, aspect := range profileChart.TransitChart.TransitAspects {
		// 		item := fmt.Sprintf("%s %s %s %d", aspect.P1Name, aspect.Aspect, aspect.P2Name, int(aspect.AspectDegrees))
		// 		aspects = append(aspects, item)
		// 	}
		// 	buildLLMPersonalParams.ProfileAspects = aspects
		// }
		// customProfileBasicConstellationInfo, err := h.constellationService.GetCustomProfileBasicConstellationInfo(ctx, projectID, personalInfoIDs[0])
		// if err != nil {
		// 	zlog.LogWithContext(ctx).Error("GetCustomProfileBasicConstellationInfo Error", zap.Error(err))
		// }
		// if customProfileBasicConstellationInfo != nil {
		// 	buildLLMPersonalParams.MoonSignPartner = customProfileBasicConstellationInfo.Moon
		// 	buildLLMPersonalParams.SunSignPartner = customProfileBasicConstellationInfo.Sun
		// 	buildLLMPersonalParams.RisingSignPartner = customProfileBasicConstellationInfo.Ascendant
		// }
	}
	// 星座服务已删除，相关功能已禁用
	// if userChart != nil {
	// 	userHouseParams := make([]string, 0, len(userChart.NatalChart.Houses))
	// 	for _, house := range userChart.NatalChart.Houses {
	// 		item := fmt.Sprintf("%s %s", house.Sign, house.Name)
	// 		userHouseParams = append(userHouseParams, item)
	// 	}
	// 	buildLLMPersonalParams.UserHouseInfo = userHouseParams
	// 	aspects := make([]string, 0, len(userChart.TransitChart.TransitAspects))
	// 	for _, aspect := range userChart.TransitChart.TransitAspects {
	// 		item := fmt.Sprintf("%s %s %s %d", aspect.P1Name, aspect.Aspect, aspect.P2Name, int(aspect.AspectDegrees))
	// 		aspects = append(aspects, item)
	// 	}
	// 	buildLLMPersonalParams.UserAspects = aspects
	//
	// }

	// 星座服务已删除，相关功能已禁用
	// basicConstellationInfo, err := h.constellationService.GetBasicConstellationInfo(ctx, projectID, userID)
	// if err != nil {
	// 	zlog.LogWithContext(ctx).Error("GetBasicConstellationInfo Error", zap.Error(err))
	// }
	// if basicConstellationInfo != nil {
	// 	buildLLMPersonalParams.MoonSignUser = basicConstellationInfo.Moon
	// 	buildLLMPersonalParams.SunSignUser = basicConstellationInfo.Sun
	// 	buildLLMPersonalParams.RisingSignUser = basicConstellationInfo.Ascendant
	// }
	personalParamsJson, err := json.Marshal(buildLLMPersonalParams)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Marshal BuildLLMPersonalParams Error", zap.Error(err))
		return err
	}
	systemPrompt := promptInfo.Content + "\n User Input Data: " + string(personalParamsJson)
	stdReq.Message.SystemPrompt = systemPrompt
	zlog.LogWithContext(ctx).Info("Successfully prepared personal info system prompt",
		zap.String("systemPrompt", systemPrompt),
	)
	return nil
}

// EnsureChatSession 实现默认的会话创建/获取逻辑
func (h *CustomeUserPersonalInfoLLMHandler) EnsureChatSession(ctx context.Context, chatCtx chatcommon.ChatContext) (isNew bool, err error) {
	isNew, err = h.BaseBizHandler.EnsureChatSession(ctx, chatCtx)
	if err != nil {
		zlog.LogWithContext(ctx).Error("EnsureChatSession Error", zap.Error(err))
		return false, err
	}
	if isNew {
		participantIDs := chatCtx.GetParticipantIDs()
		// 添加参与者
		for _, participantID := range participantIDs {
			h.chatParticipantDao.AddParticipants(db.GetDB(), []*model.ChatParticipant{
				{
					ChatID:          chatCtx.GetChatID(),
					ParticipantID:   participantID,
					ParticipantType: model.ParticipantTypeCustomProfile,
				},
			})
		}
	}

	return isNew, nil
}
