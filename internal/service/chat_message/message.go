package chat_message

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"

	"va_visionai_server/conf"
	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/utils"
	"va_visionai_server/internal/zlog"
)

type ChatMessageService struct {
	msgDao      *dao.MsgDao
	chatDao     *dao.ChatDao
	mongoCli    *mongo.Client
	mongoDbName string
}

func NewChatMessageService(mongoCli *mongo.Client, dbName string, msgDao *dao.MsgDao, chatDao *dao.ChatDao) *ChatMessageService {
	return &ChatMessageService{
		msgDao:      msgDao,
		chatDao:     chatDao,
		mongoCli:    mongoCli,
		mongoDbName: dbName,
	}
}

type SequentialChatCursor struct {
	NextChatID           string     `json:"next_chat_id"`
	NextMessageTimestamp *time.Time `json:"next_msg_ts,omitempty"`
	NextMessageID        string     `json:"next_msg_id,omitempty"`
	IsDone               bool       `json:"is_done"`
}

type SequentialMessageBatch struct {
	Messages   []*model.Message `json:"messages"`
	NextCursor string           `json:"next_cursor"`
}

func encodeSeqCursor(cursor *SequentialChatCursor) (string, error) {
	if cursor == nil {
		return "", nil
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func decodeSeqCursor(cursorStr string) (*SequentialChatCursor, error) {
	if cursorStr == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(cursorStr)
	if err != nil {
		return nil, fmt.Errorf("invalid sequential cursor format: %w", err)
	}
	var cursor SequentialChatCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, fmt.Errorf("invalid sequential cursor data: %w", err)
	}
	return &cursor, nil
}

func getMsgCollectionHandle(chat *model.Chat) *mongo.Collection {
	config := conf.GlobalConfig.MongoDB
	client := db.GetMongo()
	collectionName := fmt.Sprintf("%s%s", config.Collection, time.Unix(chat.CreateTime, 0).Format(constants.DateOnly))
	return client.Database(config.Db).Collection(collectionName)
}

func (s *ChatMessageService) findTargetChatIndex(ctx context.Context, allChats []*model.Chat, targetChatID string, userID string) int {
	targetChatIndex := -1
	for i, chat := range allChats {
		if chat.ChatID == targetChatID {
			targetChatIndex = i
			break
		}
	}

	if targetChatIndex == -1 {
		if targetChatID != "" {
			zlog.LogWithContext(ctx).Warn("Target chat ID not found for user, starting from newest chat", zap.String("targetChatID", targetChatID), zap.String("userID", userID))
		}
		targetChatIndex = 0
	}
	return targetChatIndex
}

func (s *ChatMessageService) determineInitialCursorState(ctx context.Context, allChats []*model.Chat, targetChatIndex int, cursor *SequentialChatCursor, targetChatID string, userID string) (int, *time.Time, string) {
	startChatIndex := targetChatIndex
	var startMsgTime *time.Time
	var startMsgID string

	if cursor != nil && cursor.NextChatID != "" {
		foundCursorChat := false
		cursorChatIndex := -1
		for i, chat := range allChats {
			if chat.ChatID == cursor.NextChatID {
				cursorChatIndex = i
				foundCursorChat = true
				break
			}
		}

		if foundCursorChat {
			if cursorChatIndex >= targetChatIndex {
				startChatIndex = cursorChatIndex
				startMsgTime = cursor.NextMessageTimestamp
				startMsgID = cursor.NextMessageID
			} else {
				zlog.LogWithContext(ctx).Warn("Cursor chat ID is newer than target chat ID, ignoring cursor and starting from target chat", zap.String("cursor.NextChatID", cursor.NextChatID), zap.String("targetChatID", targetChatID), zap.String("userID", userID))
			}
		} else {
			zlog.LogWithContext(ctx).Warn("Cursor chat ID not found, ignoring cursor and starting from target chat", zap.String("cursor.NextChatID", cursor.NextChatID), zap.String("targetChatID", targetChatID), zap.String("userID", userID))
		}
	}
	return startChatIndex, startMsgTime, startMsgID
}

func (s *ChatMessageService) accumulateMessagesAcrossChats(
	ctx context.Context,
	allChats []*model.Chat,
	startChatIndex int,
	initialMsgTime *time.Time,
	initialMsgID string,
	limit int,
) ([]*model.Message, *SequentialChatCursor) {
	accumulatedMessages := make([]*model.Message, 0, limit)
	currentChatIndex := startChatIndex
	currentMsgTime := initialMsgTime
	currentMsgID := initialMsgID
	var nextCursor *SequentialChatCursor

	for currentChatIndex < len(allChats) {
		chat := allChats[currentChatIndex]
		remainingLimit := limit - len(accumulatedMessages)
		if remainingLimit <= 0 {
			break
		}

		collection := getMsgCollectionHandle(chat)

		messages, daoErr := s.msgDao.GetMessagesInCollectionBeforeCursor(
			ctx,
			collection,
			chat.ChatID,
			remainingLimit,
			currentMsgTime,
			currentMsgID,
		)
		if daoErr != nil {
			zlog.LogWithContext(ctx).Error("Failed to get messages for chat", zap.String("chat.ChatID", chat.ChatID), zap.String("collection.Name()", collection.Name()), zap.Error(daoErr))
			currentChatIndex++
			currentMsgTime = nil
			currentMsgID = ""
			continue
		}

		accumulatedMessages = append(accumulatedMessages, messages...)

		if len(accumulatedMessages) >= limit {
			lastMessage := accumulatedMessages[len(accumulatedMessages)-1]
			parsedTime, parseErr := time.Parse(time.DateTime, lastMessage.CreateTime)
			if parseErr != nil {
				zlog.LogWithContext(ctx).Error("Failed to parse time for last message to create next cursor", zap.String("lastMessage.MessageID", lastMessage.MessageID), zap.String("chat.ChatID", chat.ChatID), zap.Error(parseErr))
				nextCursor = &SequentialChatCursor{IsDone: true}
				break
			}

			nextCursor = &SequentialChatCursor{
				NextChatID:           chat.ChatID,
				NextMessageTimestamp: &parsedTime,
				NextMessageID:        lastMessage.ID,
				IsDone:               false,
			}
			break
		}

		currentChatIndex++
		currentMsgTime = nil
		currentMsgID = ""
	}

	if nextCursor == nil {
		nextCursor = &SequentialChatCursor{IsDone: true}
	}

	return accumulatedMessages, nextCursor
}

func (s *ChatMessageService) decodeMessageContents(ctx context.Context, messages []*model.Message) {
	for _, msg := range messages {
		var decodeErr error
		msg.Content, decodeErr = utils.Base64Decode(msg.Content)
		if decodeErr != nil {
			zlog.LogWithContext(ctx).Error("Failed to decode message content", zap.String("msg.MessageID", msg.MessageID), zap.Error(decodeErr))
		}
	}
}

func (s *ChatMessageService) loadChats(
	ctx context.Context,
	projectID, userID, profileID string,
) ([]*model.Chat, error) {
	if profileID == "" {
		return s.chatDao.GetAllChatsByUserIDOrdered(ctx, projectID, userID)
	}
	return s.chatDao.GetAllChatsByUserAndProfileOrdered(ctx, projectID, userID, profileID)
}

func (s *ChatMessageService) GetUserMessagesSequentially(
	ctx context.Context,
	userID string,
	targetChatID string,
	cursorStr string,
	limit int,
	profileID string,
) (*SequentialMessageBatch, error) {
	cursor, err := decodeSeqCursor(cursorStr)
	if err != nil {
		return nil, err
	}
	if cursor != nil && cursor.IsDone {
		return &SequentialMessageBatch{Messages: []*model.Message{}, NextCursor: cursorStr}, nil
	}
	projectID := common.GetProjectID(ctx)
	allChats, err := s.loadChats(ctx, projectID, userID, profileID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to get ordered chats for user", zap.String("userID", userID), zap.Error(err))
		return nil, fmt.Errorf("database error fetching chats: %w", err)
	}

	if len(allChats) == 0 {
		doneCursor := &SequentialChatCursor{IsDone: true}
		doneCursorStr, _ := encodeSeqCursor(doneCursor)
		return &SequentialMessageBatch{Messages: []*model.Message{}, NextCursor: doneCursorStr}, nil
	}

	targetChatIndex := s.findTargetChatIndex(ctx, allChats, targetChatID, userID)

	startChatIndex, startMsgTime, startMsgID := s.determineInitialCursorState(ctx, allChats, targetChatIndex, cursor, targetChatID, userID)

	accumulatedMessages, nextCursorState := s.accumulateMessagesAcrossChats(
		ctx,
		allChats,
		startChatIndex,
		startMsgTime,
		startMsgID,
		limit,
	)
	s.decodeMessageContents(ctx, accumulatedMessages)
	nextCursorStr, encErr := encodeSeqCursor(nextCursorState)
	if encErr != nil {
		zlog.LogWithContext(ctx).Error("Failed to encode next sequential cursor", zap.Error(encErr))
		return &SequentialMessageBatch{Messages: accumulatedMessages, NextCursor: ""}, fmt.Errorf("failed to encode cursor: %w", encErr)
	}

	return &SequentialMessageBatch{
		Messages:   accumulatedMessages,
		NextCursor: nextCursorStr,
	}, nil
}
