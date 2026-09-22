package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"

	"va_visionai_server/conf"
	"va_visionai_server/internal/common"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type MessageService struct {
	mongoClient  *mongo.Client
	voiceService *VoiceService
	chatDao      *dao.ChatDao
}

func NewMessageService(mongoClient *mongo.Client, voiceService *VoiceService, repos *dao.Repositories) *MessageService {
	return &MessageService{
		mongoClient:  mongoClient,
		voiceService: voiceService,
		chatDao:      repos.Chat,
	}
}

// GetMessage 获取聊天的消息列表
func (s *MessageService) GetMessage(ctx context.Context, chatId string, offset, count int32, sortByTimeDesc bool) ([]*vai.Message, error) {
	if chatId == "" {
		zlog.LogWithContext(ctx).Error("chatID is Required")
		return nil, errors.New("chatId is required")
	}
	projectID := common.GetProjectID(ctx)
	offset, count = s.normalizeOffsetAndCount(offset, count)
	chat, err := s.chatDao.GetByID(projectID, chatId)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetChatByID", zap.Error(err))
		return nil, err
	}
	if chat == nil {
		zlog.LogWithContext(ctx).Error("GetChatByID Not Found", zap.Error(err))
		return nil, errors.New("chat not found")
	}
	collection := getCollection(time.Unix(chat.CreateTime, 0).Format("20060102"))
	mdao := dao.NewMsgDao()
	messages, err := mdao.GetMessages(collection, chatId, offset, count, sortByTimeDesc)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetMessages", zap.Error(err))
		return nil, err
	}
	return s.buildOutputMessages(ctx, messages)
}

// buildOutputMessages 构建输出消息列表
func (s *MessageService) buildOutputMessages(ctx context.Context, messages []*model.Message) ([]*vai.Message, error) {
	outMsgList := make([]*vai.Message, 0, len(messages))
	for _, message := range messages {
		content, err := s.DecodeMessageContent(ctx, message.Content)
		if err != nil {
			zlog.LogWithContext(ctx).Error("Failed to decode message content", zap.Error(err))
		}
		urls := []string{}
		if message.URL != "" {
			tempUrls := strings.Split(message.URL, ",")
			for _, url := range tempUrls {
				if url == "" {
					continue
				}
				urls = append(urls, url)
			}
		}
		outMsg := &vai.Message{
			ChatId:      message.ChatID,
			MessageId:   message.MessageID,
			Content:     content,
			CreateTime:  message.CreateTime,
			Url:         message.URL,
			Urls:        urls,
			MessageType: vai.MessageType(message.MessageType),
			Sender:      vai.MessageSender(message.Sender),
			VoiceInfo:   &vai.VoiceInfo{Md5: message.VoiceHash},
			Quality:     message.Quality,
		}
		outMsgList = append(outMsgList, outMsg)
	}
	if err := s.voiceService.FillMsgVoiceInfo(outMsgList); err != nil {
		return nil, err
	}
	return outMsgList, nil
}

// DecodeMessageContent 解码消息内容
func (s *MessageService) DecodeMessageContent(ctx context.Context, content string) (string, error) {
	decodedContent, err := utils.Base64Decode(content)
	if err != nil {
		return content, nil
	}
	return decodedContent, nil
}

// normalizeOffsetAndCount 规范化 offset 和 count
func (s *MessageService) normalizeOffsetAndCount(offset, count int32) (int32, int32) {
	if offset <= 0 {
		offset = 1
	}
	if count <= 0 || count > 100 {
		count = 100
	}
	return offset, count
}

// ChatMessageArchive 归档聊天消息
func (s *MessageService) ChatMessageArchive(chatId, messageId, userId string, chatCreateTime int64, content string, urls []string, voiceHash string, sender vai.MessageSender, msgType vai.MessageType, modelID string) (string, error) {
	if chatId == "" || (content == "" && voiceHash == "" && msgType != vai.MessageType_MT_VIDEO) {
		return "", errors.New("chatId and content are required")
	}
	date := time.Unix(chatCreateTime, 0).Format("20060102")
	message := s.buildMessageArchive(chatId, messageId, userId, chatCreateTime, content, urls, voiceHash, sender, msgType, modelID)
	collection := getCollection(date)
	mdao := dao.NewMsgDao()
	err := mdao.AddMessage(collection, message)
	if err != nil {
		return "", err
	}
	return messageId, nil
}

func (s *MessageService) buildMessageArchive(chatId, messageId, userId string, chatCreateTime int64, content string, urls []string, voiceHash string, sender vai.MessageSender, msgType vai.MessageType, modelID string) *model.Message {
	date := time.Unix(chatCreateTime, 0).Format("20060102")
	if voiceHash != "" {
		content = ""
	}
	message := &model.Message{
		MessageID:   messageId,
		ChatID:      chatId,
		Content:     utils.Base64Encode(content),
		URL:         strings.Join(urls, ","),
		MessageType: int(msgType),
		Sender:      int(sender),
		UserID:      userId,
		Date:        date,
		CreateTime:  time.Now().Format(common.TimestampFormat),
		VoiceHash:   voiceHash,
		ModelId:     modelID,
	}

	return message
}

func (s *MessageService) DeleteMessage(ctx context.Context, messageID string) error {
	collection := getCollection(time.Now().Format("20060102"))
	mdao := dao.NewMsgDao()
	return mdao.DeleteMessage(collection, messageID)
}

// GetMsgHistory 获取聊天的消息历史
func (s *MessageService) GetMsgHistory(ctx context.Context, ChatID string) ([]model.MessageHistory, error) {
	msgListTotal, err := s.GetMessage(ctx, ChatID, int32(1), 20, true)
	if err != nil {
		return nil, err
	}
	slices.Reverse(msgListTotal)
	msgHistory := make([]model.MessageHistory, 0)
	for idx, msg := range msgListTotal {
		history := model.MessageHistory{
			Content:   msg.GetContent(),
			FileType:  msg.GetMessageType(),
			URL:       msg.GetUrl(),
			VoiceHash: msg.GetVoiceInfo().GetMd5(),
		}
		msgURLs := msg.GetUrls()
		messageType := msg.GetMessageType()
		sender := msg.GetSender()
		if messageType != vai.MessageType_MT_TEXT && sender == vai.MessageSender_USER && len(msgURLs) > 0 {
			// 如果遇到了新的图片消息，则需要清空历史记录，避免拿历史的聊天内容进行提问
			msgHistory = []model.MessageHistory{}
			history.URLs = msgURLs
			// 第十轮对话改为空素材
			if len(msgListTotal)-idx > 20 {
				history.URLs = []string{}
			}
		}
		if msg.GetSender() == vai.MessageSender_USER {
			history.Sender = "user"
		} else {
			history.Sender = "assistant"
		}
		msgHistory = append(msgHistory, history)
	}
	return msgHistory, nil
}

// GetMsgHistoryBefore 获取指定时间戳之前的聊天消息历史
func (s *MessageService) GetMsgHistoryBefore(ctx context.Context, chatID string, timestamp time.Time) ([]model.MessageHistory, error) {
	if chatID == "" {
		zlog.LogWithContext(ctx).Error("chatID is Required")
		return nil, errors.New("chatId is required")
	}
	projectID := common.GetProjectID(ctx)
	chat, err := s.chatDao.GetByID(projectID, chatID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetChatByID", zap.Error(err))
		return nil, err
	}
	if chat == nil {
		zlog.LogWithContext(ctx).Error("GetChatByID Not Found", zap.Error(err))
		return nil, errors.New("chat not found")
	}

	// 格式化时间戳为字符串，用于过滤
	timeStr := timestamp.Format(common.TimestampFormat)
	collection := getCollection(time.Unix(chat.CreateTime, 0).Format("20060102"))

	// 创建过滤条件，获取创建时间小于指定时间戳的消息
	filter := bson.M{
		"chatId":      chatID,
		"messageType": bson.M{"$ne": 4},
		"createTime":  bson.M{"$lt": timeStr},
	}

	// 从数据库查询消息
	findOptions := options.Find()
	findOptions.SetSort(bson.D{
		{Key: "createTime", Value: -1},
		{Key: "_id", Value: -1},
	})
	findOptions.SetLimit(20) // 限制返回条数

	cur, err := collection.Find(context.Background(), filter, findOptions)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetMessagesBefore", zap.Error(err))
		return nil, err
	}
	defer cur.Close(context.Background())

	// 解析查询结果
	var messages []*model.Message
	for cur.Next(context.Background()) {
		var message model.Message
		if err := cur.Decode(&message); err != nil {
			return nil, err
		}
		messages = append(messages, &message)
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	// 转换为 vai.Message 格式
	vaiMessages, err := s.buildOutputMessages(ctx, messages)
	if err != nil {
		return nil, err
	}

	// 构建历史消息
	slices.Reverse(vaiMessages)
	msgHistory := make([]model.MessageHistory, 0, len(vaiMessages))
	for idx, msg := range vaiMessages {
		history := model.MessageHistory{
			Content:   msg.GetContent(),
			FileType:  msg.GetMessageType(),
			URL:       msg.GetUrl(),
			VoiceHash: msg.GetVoiceInfo().GetMd5(),
		}
		msgURLs := msg.GetUrls()
		messageType := msg.GetMessageType()
		sender := msg.GetSender()
		if messageType != vai.MessageType_MT_TEXT && sender == vai.MessageSender_USER && len(msgURLs) > 0 {
			// 如果遇到了新的图片消息，则需要清空历史记录，避免拿历史的聊天内容进行提问
			msgHistory = []model.MessageHistory{}
			history.URLs = msgURLs
			// 第十轮对话改为空素材
			if len(vaiMessages)-idx > 20 {
				history.URLs = []string{}
			}
		}
		if msg.GetSender() == vai.MessageSender_USER {
			history.Sender = "user"
		} else {
			history.Sender = "assistant"
		}
		msgHistory = append(msgHistory, history)
	}

	return msgHistory, nil
}

// ArchiveChatMessage 对外导出版本的消息归档方法
func (s *MessageService) ArchiveChatMessage(chatId, messageId, userId string, chatCreateTime int64, content string, urls []string, voiceHash string, sender vai.MessageSender, msgType vai.MessageType, modelID string) (string, error) {
	return s.ChatMessageArchive(chatId, messageId, userId, chatCreateTime, content, urls, voiceHash, sender, msgType, modelID)
}

// getCollection gets the MongoDB collection based on chat creation date
func getCollection(chatCreateDate string) *mongo.Collection {
	config := conf.GlobalConfig.MongoDB
	client := db.GetMongo()
	collectionName := fmt.Sprintf("%s%s", config.Collection, chatCreateDate)
	return client.Database(config.Db).Collection(collectionName)
}
