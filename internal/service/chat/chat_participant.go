package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service"
	chatcommon "va_visionai_server/internal/service/chat/common"
	"va_visionai_server/internal/service/chat_message"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"

	_ "va_visionai_server/internal/service/chat/biz"
)

type ChatParticipantService struct {
	chatParticipantDao *dao.ChatParticipantDao
	chatDao            *dao.ChatDao
	msgService         *service.MessageService
	llmFactory         common.LLMFactory
	profileDao         *dao.ProfileDao
	configDao          *dao.ConfigDao
	promptDao          *dao.PromptDao
	chatMessageService *chat_message.ChatMessageService
	// 添加 llmService 用于流式处理
	llmService chatcommon.LLMService
}

// MessageCursor 用于在获取用户消息时进行分页和追踪
type MessageCursor struct {
	ChatID    string    `json:"chat_id"`
	Timestamp time.Time `json:"timestamp"`
}

// MessageBatch 表示一批获取到的消息
type MessageBatch struct {
	Messages   []string       `json:"messages"`
	NextCursor *MessageCursor `json:"next_cursor,omitempty"`
}

// NewMessageCursor 创建一个新的消息游标
func NewMessageCursor(chatID string, timestamp time.Time) *MessageCursor {
	return &MessageCursor{
		ChatID:    chatID,
		Timestamp: timestamp,
	}
}

func NewChatParticipantService(
	chatParticipantDao *dao.ChatParticipantDao,
	chatDao *dao.ChatDao,
	msgService *service.MessageService,
	llmFactory common.LLMFactory,
	profileDao *dao.ProfileDao,
	configDao *dao.ConfigDao,
	chatMessageService *chat_message.ChatMessageService,
	promptDao *dao.PromptDao,
	llmService chatcommon.LLMService, // 新增参数
) *ChatParticipantService {
	return &ChatParticipantService{
		chatParticipantDao: chatParticipantDao,
		chatDao:            chatDao,
		msgService:         msgService,
		llmFactory:         llmFactory,
		profileDao:         profileDao,
		configDao:          configDao,
		chatMessageService: chatMessageService,
		promptDao:          promptDao,
		llmService:         llmService, // 新增字段
	}
}

// buildChatWithProfiles 会为单条 model.Chat 组装对应的 *vai.Chat，并填充 Profiles 字段
func (s *ChatParticipantService) buildChatWithProfiles(
	ctx context.Context, chat *model.Chat,
) (*vai.Chat, error) {
	grpcChat := chat.ChatToGRPC()

	projectID := common.GetProjectID(ctx)
	userID := common.GetUserID(ctx)

	// 1. 查询所有参与者
	participants, err := s.chatParticipantDao.GetParticipantsByChatID(chat.ChatID)
	if err != nil {
		return grpcChat, err
	}
	var customProfileIDs []string
	for _, p := range participants {
		if p.ParticipantType == model.ParticipantTypeCustomProfile {
			customProfileIDs = append(customProfileIDs, p.ParticipantID)
		}
	}

	// 2. 加载当前用户档案
	var profiles []*vai.UserProfile
	if userProf, err := s.profileDao.GetUserProfile(ctx, projectID, userID); err == nil && userProf != nil {
		profiles = append(profiles, userProf.ToGRPC())
	}

	// 3. 批量加载自定义档案
	if len(customProfileIDs) > 0 {
		custs, err := s.profileDao.GetCustomerProfile(ctx, projectID, customProfileIDs)
		if err == nil {
			for _, cp := range custs {
				profiles = append(profiles, cp.CustomProfileToGRPC().GetProfile())
			}
		} else {
			zlog.LogWithContext(ctx).Warn("buildChatWithProfiles: 获取自定义档案失败", zap.Error(err))
		}
	}

	grpcChat.Profiles = profiles
	return grpcChat, nil
}

// ListUserChat 根据 profileID（为空时表示查询用户所有会话，不为空时表示查询与某档案的会话）
// 先从 DAO 层拉取 model.Chat，再统一组装 Profiles
func (s *ChatParticipantService) ListUserChat(
	ctx context.Context, profileID string, offset, count int,
) ([]*vai.Chat, error) {
	projectID := common.GetProjectID(ctx)
	userID := common.GetUserID(ctx)

	var chatModels []*model.Chat
	var err error

	if profileID == "" {
		// 查询当前用户所有会话
		chatModels, err = s.chatDao.GetChatList(projectID, userID, count, offset)
	} else {
		// 查询与指定档案的会话（既包含 user，又包含 custom_profile）
		participants := []model.ChatParticipant{
			{ParticipantID: userID, ParticipantType: model.ParticipantTypeUser},
			{ParticipantID: profileID, ParticipantType: model.ParticipantTypeCustomProfile},
		}
		chatIDs, err2 := s.chatParticipantDao.GetCommonChatIDs(participants, count, offset)
		if err2 != nil {
			return nil, err2
		}
		chatModels, err = s.chatDao.GetChatsByIDs(projectID, chatIDs)
	}
	if err != nil {
		zlog.LogWithContext(ctx).Error("ListUserChat 拉取会话失败", zap.Error(err))
		return nil, err
	}

	// 统一组装 gRPC 对象
	result := make([]*vai.Chat, 0, len(chatModels))
	for _, cm := range chatModels {
		grpcChat, err := s.buildChatWithProfiles(ctx, cm)
		if err != nil {
			zlog.LogWithContext(ctx).Error("ListUserChat 组装 Profiles 失败", zap.Error(err))
			continue
		}
		result = append(result, grpcChat)
	}
	return result, nil
}

type LLMInputData struct {
	GenderUser         string   `json:"gender_user,omitempty"`
	DateBirthUser      string   `json:"date_birth_user,omitempty"`
	PlaceBirthUser     string   `json:"place_birth_user,omitempty"`
	SunSignUser        string   `json:"sun_sign_user,omitempty"`
	GenderPartner      string   `json:"gender_partner,omitempty"`
	DateBirthPartner   string   `json:"date_birth_partner,omitempty"`
	PlaceBirthPartner  string   `json:"place_birth_partner,omitempty"`
	SunSignPartner     string   `json:"sun_sign_partner,omitempty"`
	RelationshipStatus string   `json:"relationship_status,omitempty"`
	PreQuestion        []string `json:"pre_question"`
}
type LLMInputDataForStarlit struct {
	GenderUser         string   `json:"gender_user,omitempty"`
	DateBirthUser      string   `json:"date_birth_user,omitempty"`
	PlaceBirthUser     string   `json:"place_birth_user,omitempty"`
	SunSignUser        string   `json:"sun_sign_user,omitempty"`
	MoonSignUser       string   `json:"moon_sign_user"`
	RisingSignUser     string   `json:"rising_sign_user"`
	GenderPartner      string   `json:"gender_partner,omitempty"`
	DateBirthPartner   string   `json:"date_birth_partner,omitempty"`
	PlaceBirthPartner  string   `json:"place_birth_partner,omitempty"`
	SunSignPartner     string   `json:"sun_sign_partner,omitempty"`
	MoonSignPartner    string   `json:"moon_sign_partner"`
	RisingSignPartner  string   `json:"rising_sign_partner"`
	RelationshipStatus string   `json:"social_relationship,omitempty"`
	PreQuestion        []string `json:"question"`
}

// getRecentUserMessagesFromSpecificChats 从指定的一组聊天ID中获取用户最近发送的消息
func (s *ChatParticipantService) getRecentUserMessagesFromSpecificChats(ctx context.Context, userID string, targetChatIDs []string, maxCount int) ([]string, error) {
	if len(targetChatIDs) == 0 {
		return []string{}, nil
	}

	allUserMessages := make([]*vai.Message, 0)
	currentCursor := ""
	maxIterations := 10                   // 最大迭代次数，防止无限循环
	fetchBatchSize := 20                  // 每次从 GetUserMessagesSequentially 拉取的数量
	desiredMessagesBuffer := maxCount * 5 // 期望收集到的候选消息数量，以便有足够数据排序和筛选
	if desiredMessagesBuffer < fetchBatchSize*2 {
		desiredMessagesBuffer = fetchBatchSize * 2 // 保证至少拉取两批
	}

	targetChatIDSet := make(map[string]struct{}, len(targetChatIDs))
	for _, id := range targetChatIDs {
		targetChatIDSet[id] = struct{}{}
	}

	for i := 0; i < maxIterations && len(allUserMessages) < desiredMessagesBuffer; i++ {
		batch, err := s.chatMessageService.GetUserMessagesSequentially(ctx, userID, "", currentCursor, fetchBatchSize, "")
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				zlog.LogWithContext(ctx).Info("getRecentUserMessagesFromSpecificChats: reached end of user message history.", zap.String("userID", userID))
			} else {
				zlog.LogWithContext(ctx).Error("getRecentUserMessagesFromSpecificChats: error fetching messages sequentially", zap.String("userID", userID), zap.Error(err))
			}
			break
		}

		if batch == nil || len(batch.Messages) == 0 {
			zlog.LogWithContext(ctx).Info("getRecentUserMessagesFromSpecificChats: received empty batch.", zap.String("userID", userID))
			break
		}

		for _, msg := range batch.Messages {
			if _, ok := targetChatIDSet[msg.ChatID]; ok {
				if msg.Sender == int(vai.MessageSender_USER) {
					allUserMessages = append(allUserMessages, msg.ToProto())
				}
			}
		}

		currentCursor = batch.NextCursor
		if currentCursor == "" {
			zlog.LogWithContext(ctx).Info("getRecentUserMessagesFromSpecificChats: no next cursor, reached end of history.", zap.String("userID", userID))
			break
		}
	}

	sort.Slice(allUserMessages, func(i, j int) bool {
		return allUserMessages[i].GetCreateTime() > allUserMessages[j].GetCreateTime()
	})

	recentUserMessagesContent := make([]string, 0, maxCount)
	for k := 0; k < len(allUserMessages) && k < maxCount; k++ {
		recentUserMessagesContent = append(recentUserMessagesContent, allUserMessages[k].GetContent())
	}

	if len(recentUserMessagesContent) == 0 {
		zlog.LogWithContext(ctx).Info("getRecentUserMessagesFromSpecificChats: no relevant messages found after filtering for specified chats.",
			zap.String("userID", userID),
			zap.Int("targetChatIDCount", len(targetChatIDs)),
			zap.Int("collectedMessagesBeforeSortAndLimit", len(allUserMessages)))
	}

	return recentUserMessagesContent, nil
}

// getRecentUserMessagesWithProfileID 获取用户与指定 partnerProfileID 相关的最新N条用户提问
func (s *ChatParticipantService) getRecentUserMessagesWithProfileID(ctx context.Context, userID string, partnerProfileID string, maxCount int) ([]string, error) {
	if partnerProfileID == "" { // 防御性检查
		zlog.LogWithContext(ctx).Error("getRecentUserMessagesWithProfileID: partnerProfileID is empty")
		return nil, errors.New("partnerProfileID cannot be empty for getRecentUserMessagesWithProfileID")
	}

	participants := []model.ChatParticipant{
		{ParticipantID: userID, ParticipantType: model.ParticipantTypeUser},
		{ParticipantID: partnerProfileID, ParticipantType: model.ParticipantTypeCustomProfile},
	}
	// 获取所有共同的 chatID，不分页 (limit=0, offset=0)
	commonChatIDs, err := s.chatParticipantDao.GetCommonChatIDs(participants, 0, 0)
	if err != nil {
		zlog.LogWithContext(ctx).Error("getRecentUserMessagesWithProfileID: failed to get common chat IDs",
			zap.String("userID", userID),
			zap.String("partnerProfileID", partnerProfileID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get common chat IDs: %w", err)
	}

	if len(commonChatIDs) == 0 {
		zlog.LogWithContext(ctx).Info("getRecentUserMessagesWithProfileID: no common chats found",
			zap.String("userID", userID),
			zap.String("partnerProfileID", partnerProfileID))
		return []string{}, nil // 没有相关聊天内容，返回空消息
	}

	return s.getRecentUserMessagesFromSpecificChats(ctx, userID, commonChatIDs, maxCount)
}

// getRecentUserMessagesWithSelf 获取用户在不与任何 CustomProfile 关联的聊天中的最新N条用户提问
func (s *ChatParticipantService) getRecentUserMessagesWithSelf(ctx context.Context, userID string, maxCount int) ([]string, error) {
	// 1. 获取用户参与的所有 ChatID
	userChatIDs, err := s.chatParticipantDao.GetChatIDsByParticipant(userID, model.ParticipantTypeUser, 0, 0) // 获取所有
	if err != nil {
		zlog.LogWithContext(ctx).Error("getRecentUserMessagesWithSelf: failed to get user chat IDs",
			zap.String("userID", userID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get user chat IDs for %s: %w", userID, err)
	}

	if len(userChatIDs) == 0 {
		zlog.LogWithContext(ctx).Info("getRecentUserMessagesWithSelf: no chats found for user", zap.String("userID", userID))
		return []string{}, nil
	}

	// 2. 筛选出不包含 CustomProfile 参与者的 ChatID
	var validChatIDs []string
	if len(userChatIDs) > 0 {
		for _, chatID := range userChatIDs {
			participants, err := s.chatParticipantDao.GetParticipantsByChatID(chatID)
			if err != nil {
				zlog.LogWithContext(ctx).Warn("getRecentUserMessagesWithSelf: failed to get participants for chat, skipping chatID",
					zap.String("chatID", chatID),
					zap.Error(err))
				continue
			}

			hasCustomProfile := false
			for _, p := range participants {
				if p.ParticipantType == model.ParticipantTypeCustomProfile {
					hasCustomProfile = true
					break
				}
			}
			if !hasCustomProfile {
				validChatIDs = append(validChatIDs, chatID)
			}
		}
	}

	if len(validChatIDs) == 0 {
		zlog.LogWithContext(ctx).Info("getRecentUserMessagesWithSelf: no chats without custom profile found after filtering",
			zap.String("userID", userID),
			zap.Int("totalUserChatsFound", len(userChatIDs)))
		return []string{}, nil
	}

	return s.getRecentUserMessagesFromSpecificChats(ctx, userID, validChatIDs, maxCount)
}

// SuggestQuestions 生成问题建议
func (s *ChatParticipantService) SuggestQuestions(ctx context.Context, chatID, partnerProfileID, lang string) ([]string, error) {
	userID := common.GetUserID(ctx)
	projectID := common.GetProjectID(ctx)
	if userID == "" {
		return nil, errors.New("user ID not found in context")
	}
	if projectID == "" {
		return nil, errors.New("project ID not found in context")
	}

	llmData, err := s.getProfileDataForLLM(ctx, projectID, userID, partnerProfileID)
	if err != nil {
		return nil, err
	}

	var recentUserMessages []string
	const maxRecentMessages = 3

	if partnerProfileID != "" {
		recentUserMessages, err = s.getRecentUserMessagesWithProfileID(ctx, userID, partnerProfileID, maxRecentMessages)
		if err != nil {
			zlog.LogWithContext(ctx).Error("SuggestQuestions: failed to get recent user messages with profileID, proceeding with empty history",
				zap.String("userID", userID),
				zap.String("partnerProfileID", partnerProfileID),
				zap.Error(err))
			recentUserMessages = []string{}
		}
	} else {
		recentUserMessages, err = s.getRecentUserMessagesWithSelf(ctx, userID, maxRecentMessages)
		if err != nil {
			zlog.LogWithContext(ctx).Error("SuggestQuestions: failed to get recent user messages with self, proceeding with empty history",
				zap.String("userID", userID),
				zap.Error(err))
			recentUserMessages = []string{}
		}
	}

	llmData.PreQuestion = recentUserMessages
	if llmData.PreQuestion == nil {
		llmData.PreQuestion = []string{}
	}

	prompt, err := s.promptDao.GetAstroPrompt(constants.SystemConstellationUserSuggestQuestionsPrompt, lang)
	if err != nil {
		prompt, err = s.promptDao.GetAstroPrompt(constants.SystemConstellationUserSuggestQuestionsPrompt, constants.EN)
		if err != nil {
			zlog.LogWithContext(ctx).Error("SuggestQuestions: failed to get prompt", zap.Error(err))
			return nil, err
		}
	}

	fixedSystemPrompt := prompt.Content
	fixedSystemPrompt = strings.ReplaceAll(fixedSystemPrompt, "{astro_replace_lang}", lang)
	suggestions, err := s.callLLMForSuggestions(ctx, fixedSystemPrompt, llmData, chatID, userID)
	if err != nil {
		return nil, err
	}

	zlog.LogWithContext(ctx).Info("SuggestQuestions: successfully generated suggestions",
		zap.String("chatID", chatID),
		zap.String("partnerProfileID", partnerProfileID),
		zap.Int("suggestion_count", len(suggestions)))

	return suggestions, nil
}

func (s *ChatParticipantService) getProfileDataForLLM(ctx context.Context, projectID, userID, partnerProfileID string) (LLMInputData, error) {
	llmData := LLMInputData{}

	userProfile, err := s.profileDao.GetUserProfile(ctx, projectID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			zlog.LogWithContext(ctx).Warn("getProfileDataForLLM: user profile not found", zap.String("userID", userID), zap.String("projectID", projectID))
		} else {
			zlog.LogWithContext(ctx).Error("getProfileDataForLLM: failed to get user profile", zap.String("userID", userID), zap.Error(err))
			return llmData, fmt.Errorf("failed to retrieve user profile for user %s: %w", userID, err)
		}
	} else if userProfile != nil {
		llmData.GenderUser = constants.MapGenderToString(userProfile.Sex)
		if userProfile.BirthTimestamp > 0 {
			llmData.DateBirthUser = time.Unix(userProfile.BirthTimestamp, 0).Format(common.TimestampFormat)
		}
		llmData.PlaceBirthUser = userProfile.BirthAddress
		llmData.SunSignUser = userProfile.Constellation
	}

	if partnerProfileID != "" {
		partnerProfiles, err := s.profileDao.GetCustomerProfile(ctx, projectID, []string{partnerProfileID})
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				zlog.LogWithContext(ctx).Warn("getProfileDataForLLM: partner profile not found", zap.String("partnerProfileID", partnerProfileID), zap.String("projectID", projectID))
			} else {
				zlog.LogWithContext(ctx).Error("getProfileDataForLLM: failed to get partner profile", zap.String("partnerProfileID", partnerProfileID), zap.Error(err))
				return llmData, fmt.Errorf("failed to retrieve partner profile %s: %w", partnerProfileID, err)
			}
		} else if len(partnerProfiles) > 0 && partnerProfiles[0] != nil {
			partnerProfile := partnerProfiles[0]
			llmData.GenderPartner = constants.MapGenderToString(partnerProfile.Sex)
			if partnerProfile.BirthTimestamp > 0 {
				llmData.DateBirthPartner = time.Unix(partnerProfile.BirthTimestamp, 0).Format(common.TimestampFormat)
			}
			llmData.PlaceBirthPartner = partnerProfile.BirthAddress
			llmData.SunSignPartner = partnerProfile.Constellation
			llmData.RelationshipStatus = partnerProfile.Relation
		} else {
			zlog.LogWithContext(ctx).Warn("getProfileDataForLLM: partner profile query returned no results", zap.String("partnerProfileID", partnerProfileID), zap.String("projectID", projectID))
		}
	}

	return llmData, nil
}

func (s *ChatParticipantService) callLLMForSuggestions(ctx context.Context, fixedSystemPrompt string, llmData LLMInputData, chatID, userID string) ([]string, error) {
	jsonData, err := json.Marshal(llmData)
	if err != nil {
		zlog.LogWithContext(ctx).Error("callLLMForSuggestions: failed to marshal LLM input data", zap.Error(err))
		return nil, fmt.Errorf("failed to construct LLM input JSON: %w", err)
	}
	llmInput := fixedSystemPrompt + "\n User Input Data:\n " + string(jsonData)
	modelNameStr, err := s.configDao.GetConfigValue(constants.SystemSuggestQuestionModel)
	if err != nil {
		zlog.LogWithContext(ctx).Error("callLLMForSuggestions: failed to get suggest question model from config",
			zap.String("config_key", constants.SystemSuggestQuestionModel),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get suggest question model config '%s': %w", constants.SystemSuggestQuestionModel, err)
	}

	modelValue, ok := vai.Model_value[modelNameStr]
	if !ok {
		zlog.LogWithContext(ctx).Error("callLLMForSuggestions: invalid model name configured",
			zap.String("config_key", constants.SystemSuggestQuestionModel),
			zap.String("configured_model_name", modelNameStr))
		return nil, fmt.Errorf("invalid model name '%s' configured for key '%s'", modelNameStr, constants.SystemSuggestQuestionModel)
	}
	modelID := vai.Model(modelValue)

	var llmHandler common.LlmHandler
	ctx, llmHandler, err = s.llmFactory.CreateHandler(ctx, modelID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("callLLMForSuggestions: failed to get LLM handler", zap.String("modelName", modelNameStr), zap.Error(err))
		return nil, fmt.Errorf("failed to get LLM handler for model %s: %w", modelNameStr, err)
	}

	tempMessageID := utils.GenerateMsgID()
	req := &vai.ChatMessageSendRequest{
		RequestHeader: &vai.RequestHeader{UserId: userID},
		Message: &vai.Message{
			ChatId:       chatID,
			MessageId:    tempMessageID,
			Content:      llmInput,
			Sender:       vai.MessageSender_USER,
			SystemPrompt: fixedSystemPrompt,
		},
	}

	llmResponse, err := utils.ExecuteLLMQuery(ctx, llmHandler, req, nil)
	if err != nil {
		zlog.LogWithContext(ctx).Error("callLLMForSuggestions: LLM query failed",
			zap.String("chatID", chatID),
			zap.String("modelName", modelNameStr),
			zap.Error(err))
		return nil, fmt.Errorf("LLM query execution failed for model %s: %w", modelNameStr, err)
	}

	var suggestedQuestionsResp struct {
		Questions []string `json:"questions"`
	}
	if errUnmarshal := json.Unmarshal([]byte(llmResponse), &suggestedQuestionsResp); errUnmarshal != nil {
		cleanedResponse := utils.RemoveMarkdownCodeBlock(llmResponse)
		if errRetry := json.Unmarshal([]byte(cleanedResponse), &suggestedQuestionsResp); errRetry != nil {
			zlog.LogWithContext(ctx).Error("callLLMForSuggestions: failed to unmarshal LLM response even after cleaning",
				zap.String("original_response", llmResponse),
				zap.String("cleaned_response", cleanedResponse),
				zap.NamedError("initial_error", errUnmarshal),
				zap.NamedError("retry_error", errRetry))
			return nil, fmt.Errorf("failed to parse LLM response as JSON array: %w", errRetry)
		}
		zlog.LogWithContext(ctx).Debug("callLLMForSuggestions: successfully unmarshaled LLM response after cleaning markdown",
			zap.String("original_response", llmResponse),
			zap.String("cleaned_response", cleanedResponse))
	}

	zlog.LogWithContext(ctx).Info("callLLMForSuggestions: successfully parsed suggestions",
		zap.Int("suggestion_count", len(suggestedQuestionsResp.Questions)))

	return suggestedQuestionsResp.Questions, nil
}

// getProfileDataForStarlitLLM starlit获取用户档案信息
// func (s *ChatParticipantService) getProfileDataForStarlitLLM(ctx context.Context, projectID, userID, partnerProfileID string) (LLMInputDataForStarlit, error) {
// 	llmDataForStarlit := LLMInputDataForStarlit{}

// 	userProfile, err := s.profileDao.GetUserProfile(ctx, projectID, userID)
// 	if err != nil {
// 		if errors.Is(err, gorm.ErrRecordNotFound) {
// 			zlog.LogWithContext(ctx).Warn("getProfileDataForStarlitLLM: user profile not found", zap.String("userID", userID), zap.String("projectID", projectID))
// 		} else {
// 			zlog.LogWithContext(ctx).Error("getProfileDataForStarlitLLM: failed to get user profile", zap.String("userID", userID), zap.Error(err))
// 			return llmDataForStarlit, fmt.Errorf("failed to retrieve user profile for starlit user %s: %w", userID, err)
// 		}
// 	} else if userProfile != nil {
// 		llmDataForStarlit.GenderUser = constants.MapGenderToString(userProfile.Sex)
// 		if userProfile.BirthTimestamp > 0 {
// 			llmDataForStarlit.DateBirthUser = time.Unix(userProfile.BirthTimestamp, 0).Format(common.TimestampFormat)
// 		}
// 		llmDataForStarlit.PlaceBirthUser = userProfile.BirthAddress
// 		llmDataForStarlit.SunSignUser = userProfile.Constellation
// 		llmDataForStarlit.RelationshipStatus = userProfile.EmotionalState
// 	}
// 	//获取月亮和上升星座
// 	constellationBasic, err := s.constellationBasicDao.GetByUserID(ctx, userID)
// 	if err != nil {
// 		if errors.Is(err, gorm.ErrRecordNotFound) {
//// 			zlog.LogWithContext(ctx).Warn("getProfileDataForStarlitLLM: user constellationBasic not found", zap.String("userID", userID), zap.String("projectID", projectID))
//// 		} else {
//// 			zlog.LogWithContext(ctx).Error("getProfileDataForStarlitLLM: failed to get user constellationBasic", zap.String("userID", userID), zap.Error(err))
//// 			return llmDataForStarlit, fmt.Errorf("failed to retrieve user constellationBasic for starlit user %s: %w", userID, err)
//// 		}
//// 	} else if constellationBasic != nil {
//// 		llmDataForStarlit.MoonSignUser = constellationBasic.Moon
//// 		llmDataForStarlit.RisingSignUser = constellationBasic.Ascendant
//// 	}
//
//// 	if partnerProfileID != "" {
//// 		partnerProfiles, err := s.profileDao.GetCustomerProfile(ctx, projectID, []string{partnerProfileID})
// 		if err != nil {
// 			if errors.Is(err, gorm.ErrRecordNotFound) {
// 				zlog.LogWithContext(ctx).Warn("getProfileDataForLLM: partner profile not found", zap.String("partnerProfileID", partnerProfileID), zap.String("projectID", projectID))
// 			} else {
// 				zlog.LogWithContext(ctx).Error("getProfileDataForLLM: failed to get partner profile", zap.String("partnerProfileID", partnerProfileID), zap.Error(err))
// 				return llmDataForStarlit, fmt.Errorf("failed to retrieve partner profile %s: %w", partnerProfileID, err)
// 			}
// 		} else if len(partnerProfiles) > 0 && partnerProfiles[0] != nil {
// 			partnerProfile := partnerProfiles[0]
// 			llmDataForStarlit.GenderPartner = constants.MapGenderToString(partnerProfile.Sex)
// 			if partnerProfile.BirthTimestamp > 0 {
// 				llmDataForStarlit.DateBirthPartner = time.Unix(partnerProfile.BirthTimestamp, 0).Format(common.TimestampFormat)
// 			}
// 			llmDataForStarlit.PlaceBirthPartner = partnerProfile.BirthAddress
// 			llmDataForStarlit.SunSignPartner = partnerProfile.Constellation
// 			llmDataForStarlit.RelationshipStatus = partnerProfile.Relation
// 		} else {
// 			zlog.LogWithContext(ctx).Warn("getProfileDataForLLM: partner profile query returned no results", zap.String("partnerProfileID", partnerProfileID), zap.String("projectID", projectID))
// 		}

// 		//获取伴侣等月亮和上升星座
// 		customConstellationBasic, err := s.customConstellationBasicDao.GetByProfileID(ctx, partnerProfileID)
// 		if err != nil {
// 			if errors.Is(err, gorm.ErrRecordNotFound) {
//// 				zlog.LogWithContext(ctx).Warn("getProfileDataForStarlitLLM: user customConstellationBasic not found", zap.String("partnerProfileID", partnerProfileID), zap.String("projectID", projectID))
//// 			} else {
//// 				zlog.LogWithContext(ctx).Error("getProfileDataForStarlitLLM: failed to get user customConstellationBasic", zap.String("partnerProfileID", partnerProfileID), zap.Error(err))
//// 				return llmDataForStarlit, fmt.Errorf("failed to retrieve user customConstellationBasic for starlit partner %s: %w", partnerProfileID, err)
//// 			}
//// 		} else if customConstellationBasic != nil {
//// 			llmDataForStarlit.MoonSignPartner = customConstellationBasic.Moon
//// 			llmDataForStarlit.RisingSignPartner = customConstellationBasic.Ascendant
//// 		}
//// 	}
//
//// 	return llmDataForStarlit, nil
// }

// // StarlitQuestionsStream 函数，添加流式输出支持
// func (s *ChatParticipantService) StarlitQuestionsStream(
// 	ctx context.Context,
// 	partnerProfileID, lang string,
// 	streamServer vai.ChatService_SendChatMessageStreamServer,
// ) ([]string, error) {
// 	userID := common.GetUserID(ctx)
// 	projectID := common.GetProjectID(ctx)
// 	//默认单人档案
// 	promptStarlit := constants.SystemConstellationSingleUserStarlitQuestionsPrompt
// 	if userID == "" {
// 		return nil, errors.New("user ID not found in context")
// 	}
// 	if projectID == "" {
// 		return nil, errors.New("project ID not found in context")
// 	}

// 	llmDataForStarlit, err := s.getProfileDataForStarlitLLM(ctx, projectID, userID, partnerProfileID)
// 	if err != nil {
// 		return nil, err
// 	}

// 	var recentUserMessages []string
// 	const maxRecentMessages = 3

// 	if partnerProfileID != "" {
// 		//多人
// 		promptStarlit = constants.SystemConstellationMultiUserStarlitQuestionsPrompt
// 		recentUserMessages, err = s.getRecentUserMessagesWithProfileID(ctx, userID, partnerProfileID, maxRecentMessages)
// 		if err != nil {
// 			zlog.LogWithContext(ctx).Error("StarlitQuestionsStream: failed to get recent user messages with profileID, proceeding with empty history",
// 				zap.String("userID", userID),
// 				zap.String("partnerProfileID", partnerProfileID),
// 				zap.Error(err))
// 			recentUserMessages = []string{}
// 		}
// 	} else {
// 		recentUserMessages, err = s.getRecentUserMessagesWithSelf(ctx, userID, maxRecentMessages)
// 		if err != nil {
// 			zlog.LogWithContext(ctx).Error("StarlitQuestionsStream: failed to get recent user messages with self, proceeding with empty history",
// 				zap.String("userID", userID),
// 				zap.Error(err))
// 			recentUserMessages = []string{}
// 		}
// 	}

// 	llmDataForStarlit.PreQuestion = recentUserMessages
// 	if llmDataForStarlit.PreQuestion == nil {
// 		llmDataForStarlit.PreQuestion = []string{}
// 	}

// 	prompt, err := s.promptDao.GetAstroPrompt(promptStarlit, lang)
// 	if err != nil {
// 		prompt, err = s.promptDao.GetAstroPrompt(promptStarlit, constants.EN)
// 		if err != nil {
// 			zlog.LogWithContext(ctx).Error("StarlitQuestionsStream: failed to get prompt", zap.Error(err))
// 			return nil, err
// 		}
// 	}

// 	fixedSystemPrompt := prompt.Content
// 	fixedSystemPrompt = strings.ReplaceAll(fixedSystemPrompt, "{astro_replace_lang}", lang)
// 	// 记录当前分析的档案类型
// 	profileType := "single"
// 	if partnerProfileID != "" {
// 		profileType = "double"
// 	}
// 	zlog.LogWithContext(ctx).Info("StarlitQuestionsStream: 开始分析档案",
// 		zap.String("profileType", profileType),
// 		zap.String("userID", userID),
// 		zap.String("partnerProfileID", partnerProfileID),
// 		zap.String("language", lang))
// 	// 使用流式版本
// 	suggestions, err := s.callLLMForStarlitSuggestionsStream(ctx, fixedSystemPrompt, llmDataForStarlit, userID, streamServer)
// 	if err != nil {
// 		return nil, err
// 	}

// 	zlog.LogWithContext(ctx).Info("StarlitQuestionsStream: successfully generated suggestions",
// 		zap.String("partnerProfileID", partnerProfileID),
// 		zap.Int("Starlit_suggestion_count", len(suggestions)))

// 	return suggestions, nil
// }

// // callLLMForStarlitSuggestionsStream 添加新的流式版本函数
// func (s *ChatParticipantService) callLLMForStarlitSuggestionsStream(
// 	ctx context.Context,
// 	fixedSystemPrompt string,
// 	llmDataForStarlit LLMInputDataForStarlit,
// 	userID string,
// 	streamServer vai.ChatService_SendChatMessageStreamServer,
// ) ([]string, error) {
// 	jsonData, err := json.Marshal(llmDataForStarlit)
// 	if err != nil {
// 		zlog.LogWithContext(ctx).Error("callLLMForStarlitSuggestionsStream: failed to marshal LLM input data", zap.Error(err))
// 		return nil, fmt.Errorf("failed to construct LLM input JSON: %w", err)
// 	}
// 	llmInput := fixedSystemPrompt + "\n User Input Data:\n " + string(jsonData)
// 	zlog.LogWithContext(ctx).Debug("Starlit llmInput Json",
// 		zap.String("Starlit llmInput Json", string(jsonData)))

// 	modelNameStr, err := s.configDao.GetConfigValue(constants.SystemStarlitQuestionModel)
// 	if err != nil {
// 		zlog.LogWithContext(ctx).Error("callLLMForStarlitSuggestionsStream: failed to get starlit question model from config",
// 			zap.String("config_key", constants.SystemStarlitQuestionModel),
// 			zap.Error(err))
// 		return nil, fmt.Errorf("failed to get starlit question model config '%s': %w", constants.SystemStarlitQuestionModel, err)
// 	}

// 	modelValue, ok := vai.Model_value[modelNameStr]
// 	if !ok {
// 		zlog.LogWithContext(ctx).Error("callLLMForStarlitSuggestionsStream: invalid model name configured",
// 			zap.String("config_key", constants.SystemSuggestQuestionModel),
// 			zap.String("configured_model_name", modelNameStr))
// 		return nil, fmt.Errorf("invalid model name '%s' configured for key '%s'", modelNameStr, constants.SystemSuggestQuestionModel)
// 	}
// 	modelID := vai.Model(modelValue)

// 	tempMessageID := utils.GenerateMsgID()
// 	req := &vai.ChatMessageSendRequest{
// 		RequestHeader: &vai.RequestHeader{UserId: userID},
// 		Message: &vai.Message{
// 			MessageId:    tempMessageID,
// 			Content:      llmInput,
// 			Sender:       vai.MessageSender_USER,
// 			SystemPrompt: fixedSystemPrompt,
// 			ModelName:    modelNameStr,
// 			ModelId:      modelID,
// 		},
// 	}

// 	// 使用 callbackProcessorWithStarlit 处理流式输出
// 	proc := newCallbackProcessorWithStarlit()

// 	// 获取问题channel
// 	questionChan := proc.GetQuestionChannel()

// 	// 启动goroutine来处理问题发送
// 	var suggestions []string
// 	var suggestionsMutex sync.Mutex
// 	var processedCount int32 = 0 // 使用原子计数器跟踪处理的问题数量

// 	// 创建一个context，用于取消goroutine
// 	ctxWithCancel, cancel := context.WithCancel(ctx)
// 	defer cancel()

// 	// 创建WaitGroup来等待所有goroutine完成
// 	var wg sync.WaitGroup
// 	wg.Add(1) // 只需要一个goroutine

// 	cleanInvalidUTF8 := func(s string) string {
// 		if utf8.ValidString(s) {
// 			return s
// 		}
// 		return strings.ToValidUTF8(s, "")
// 	}

// 	// 批量发送计数器
// 	batchCount := 0
// 	batchSize := 5 // 每5个问题一批
// 	// 启动goroutine来直接从questionChan读取并发送问题到客户端
// 	go func() {
// 		defer wg.Done()

// 		for {
// 			select {
// 			case <-ctxWithCancel.Done():
// 				return
// 			case question, ok := <-questionChan:
// 				if !ok {
// 					// channel已关闭
// 					return
// 				}

// 				// 添加到建议列表
// 				suggestionsMutex.Lock()
// 				suggestions = append(suggestions, question)
// 				currentCount := len(suggestions)
// 				suggestionsMutex.Unlock()

// 				// 更新处理计数
// 				atomic.AddInt32(&processedCount, 1)

// 				// 发送到客户端
// 				if streamServer != nil {
// 					cleanToken := cleanInvalidUTF8(question)
// 					if cleanToken != "" {
// 						res := &vai.ChatMessageStreamResponse{
// 							MessageToken:   cleanToken,
// 							ReqMessageId:   req.GetMessage().GetMessageId(),
// 							ReplyMessageId: tempMessageID,
// 							ResponseHeader: &vai.ResponseHeader{
// 								Code:  0,
// 								Msg:   "success",
// 								ReqId: req.GetRequestHeader().GetReqId(),
// 							},
// 							IsEnd: false,
// 						}

// 						if err := streamServer.Send(res); err != nil {
// 							zlog.LogWithContext(ctx).Error("StreamServer send failed",
// 								zap.Error(err),
// 								zap.String("token", cleanToken))
// 						} else {
// 							zlog.LogWithContext(ctx).Debug("Sent question to client",
// 								zap.String("question", cleanToken),
// 								zap.Int("question_count", currentCount))
// 						}
// 					}
// 				}

// 				// 批量控制发送间隔
// 				batchCount++
// 				if batchCount >= batchSize {
// 					batchCount = 0
// 					// 每批问题后休眠一次，而不是每个问题都休眠
// 					time.Sleep(questionDelay / 2) // 减少间隔时间
// 				}
// 			}
// 		}
// 	}()

// 	// 添加变量来累积原始输出
// 	var rawOutput strings.Builder

// 	// wrappedCallback 处理流式token
// 	wrappedCallback := func(ctx context.Context, token string) error {
// 		// 累积原始输出
// 		rawOutput.WriteString(token)

// 		// 处理token，解析问题会通过channel发送
// 		_, _ = proc.process(ctx, token)
// 		return nil
// 	}

// 	ctx, finalModelID, imageContent, _ := s.llmService.ResolveModel(ctx, req, nil)

// 	_, err = s.llmService.StreamProcess(
// 		ctx,
// 		req,
// 		nil, // 没有历史消息
// 		wrappedCallback,
// 		chatcommon.LLMOptions{
// 			InitialModel:  finalModelID,
// 			GlobalTimeout: 120 * time.Second,
// 			GapTimeout:    60 * time.Second,
// 			ImageContent:  imageContent,
// 		},
// 	)

// 	if err != nil {
// 		zlog.LogWithContext(ctx).Error("callLLMForStarlitSuggestionsStream: LLM stream processing failed",
// 			zap.String("modelName", modelNameStr),
// 			zap.Error(err))
// 		return nil, fmt.Errorf("LLM stream processing failed for model %s: %w", modelNameStr, err)
// 	}

// 	// 输出完整的原始输出
// 	completeRawOutput := rawOutput.String()
// 	zlog.LogWithContext(ctx).Info("Model complete raw output",
// 		zap.String("raw_output", completeRawOutput),
// 		zap.String("modelName", modelNameStr),
// 		zap.Int("output_length", len(completeRawOutput)))

// 	// 流式处理完成后，确保所有问题都被处理完
// 	// 先调用Finish确保所有问题都被解析并发送到channel
// 	proc.Finish(ctx)

// 	// 等待一段时间，确保channel中的所有问题都被处理
// 	// 这里不需要依赖GetQuestionCount方法，而是给予足够的时间
// 	time.Sleep(2 * time.Second) // 先等待一小段时间让问题进入channel

// 	// 然后再取消goroutine
// 	cancel()

// 	// 使用WaitGroup等待所有goroutine完成
// 	wgDone := make(chan struct{})
// 	go func() {
// 		wg.Wait()
// 		close(wgDone)
// 	}()

// 	// 设置更长的超时，避免无限等待
// 	// 根据当前已处理的问题数量动态计算等待时间
// 	currentProcessed := atomic.LoadInt32(&processedCount)
// 	waitTime := time.Duration(currentProcessed/int32(batchSize)+1)*(questionDelay/2) + 10*time.Second

// 	select {
// 	case <-wgDone:
// 		// 所有goroutine已完成
// 		zlog.LogWithContext(ctx).Info("All questions processed and sent",
// 			zap.Int("total_questions", len(suggestions)))
// 	case <-time.After(waitTime):
// 		// 超时，继续执行
// 		zlog.LogWithContext(ctx).Warn("Timeout waiting for question processing goroutines to complete",
// 			zap.Duration("wait_time", waitTime),
// 			zap.Int("processed_questions", len(suggestions)),
// 			zap.Int32("processed_count", atomic.LoadInt32(&processedCount)))
// 	}

// 	// 如果没有通过流式获取到建议，尝试解析累积的文本
// 	suggestionsMutex.Lock()
// 	suggestionCount := len(suggestions)
// 	suggestionsMutex.Unlock()

// 	if suggestionCount == 0 {
// 		var suggestedQuestionsResp struct {
// 			Questions []string `json:"questions"`
// 		}
// 		if errUnmarshal := json.Unmarshal([]byte(completeRawOutput), &suggestedQuestionsResp); errUnmarshal != nil {
// 			cleanedResponse := utils.RemoveMarkdownCodeBlock(completeRawOutput)
// 			if errRetry := json.Unmarshal([]byte(cleanedResponse), &suggestedQuestionsResp); errRetry != nil {
// 				zlog.LogWithContext(ctx).Error("callLLMForStarlitSuggestionsStream: failed to unmarshal LLM response even after cleaning",
// 					zap.String("original_response", completeRawOutput),
// 					zap.String("cleaned_response", cleanedResponse),
// 					zap.NamedError("initial_error", errUnmarshal),
// 					zap.NamedError("retry_error", errRetry))
// 			} else {
// 				suggestions = suggestedQuestionsResp.Questions
// 			}
// 		} else {
// 			suggestions = suggestedQuestionsResp.Questions
// 		}
// 	}

// 	zlog.LogWithContext(ctx).Info("callLLMForStarlitSuggestionsStream: successfully parsed suggestions",
// 		zap.Int("suggestion_count", len(suggestions)),
// 		zap.Strings("suggestions", suggestions))

// 	return suggestions, nil
// }
