package dao

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"va_visionai_server/internal/model"
)

type ChatDao struct {
	db *gorm.DB
}

func NewChatDao(db *gorm.DB) *ChatDao {
	return &ChatDao{
		db: db,
	}
}

func (d *ChatDao) UpdateChatTitle(chatID, title string) error {
	return d.db.Model(&model.Chat{}).Where("chat_id = ?", chatID).Update("title", title).Error
}

func (d *ChatDao) UpdateChatLastTime(chatID string, lastTime *time.Time) error {
	return d.db.Model(&model.Chat{}).Where("chat_id = ?", chatID).Update("last_time", lastTime).Error
}

func (d *ChatDao) ChatArchived(projectID, chatID string) error {
	return d.db.Model(&model.Chat{}).Where("project_id = ? and chat_id = ?", projectID, chatID).Update("status", 1).Error
}

func (d *ChatDao) ChatArchivedAll(projectID, userID string) error {
	return d.db.Model(&model.Chat{}).Where("project_id = ? and user_id = ?", projectID, userID).Update("status", 1).Error
}

func (dao *ChatDao) GetByID(projectID, chatID string) (*model.Chat, error) {
	var chat model.Chat
	err := dao.db.Where("project_id = ? and chat_id = ?", projectID, chatID).First(&chat).Error
	if err != nil {
		return nil, err
	}
	return &chat, nil
}

// GetChatList gets the list of chats for a user
func (d *ChatDao) GetChatList(projectID, userId string, count, offset int) ([]*model.Chat, error) {
	if userId == "" {
		return nil, errors.New("userId is required")
	}
	var chats []*model.Chat
	if offset < 1 {
		offset = 1
	}
	err := d.db.Where("project_id = ? and user_id = ? and status = 0 and is_video is false", projectID, userId).Order("id desc").Limit(count).Offset((offset - 1) * count).Find(&chats).Error
	if err != nil {
		return nil, err
	}
	return chats, nil
}

func (d *ChatDao) FirstOrCreate(projectID, chatID string, chat *model.Chat) (bool, error) {
	res := d.db.Where("project_id = ? and chat_id = ?", projectID, chatID).FirstOrCreate(&chat)
	return res.RowsAffected > 0, res.Error
}

// 判断是否是第一次对话
func (d *ChatDao) FirstChat(userId string) bool {
	var count int64
	d.db.Table("va_chat").
		Select("count(*)").
		Where("user_id = ?", userId).
		Count(&count)
	return count == 0
}

// GetAllChatsByUserIDOrdered 获取用户所有 Chat，按创建时间降序排列
func (d *ChatDao) GetAllChatsByUserIDOrdered(ctx context.Context, projectID, userID string) ([]*model.Chat, error) {
	var chats []*model.Chat
	// 确保查询获取所有必要的字段，特别是 create_time 和 chat_id
	// 确保 status = 0 和 is_video is false 等条件符合业务
	err := d.db.WithContext(ctx).Model(&model.Chat{}).
		Select("chat_id", "create_time" /* 其他必要字段 */).
		Where("project_id = ? AND user_id = ? AND status = 0 AND is_video is false", projectID, userID).
		Order("create_time DESC"). // 或 Order("id DESC") 如果 id 能保证顺序
		Find(&chats).Error
	if err != nil {
		return nil, err
	}
	return chats, nil
}

// GetChatsByIDs 批量根据 chat_id 拉取聊天模型，并按创建顺序倒序
func (d *ChatDao) GetChatsByIDs(projectID string, chatIDs []string) ([]*model.Chat, error) {
	if len(chatIDs) == 0 {
		return []*model.Chat{}, nil
	}
	var chats []*model.Chat
	err := d.db.
		Where("project_id = ? AND chat_id IN ? AND status = 0 AND is_video is false", projectID, chatIDs).
		Order("id DESC").
		Find(&chats).Error
	return chats, err
}

// GetAllChatsByUserAndProfileOrdered 获取用户和指定档案共同参与的所有 Chat，按创建时间降序排列
func (d *ChatDao) GetAllChatsByUserAndProfileOrdered(
	ctx context.Context,
	projectID, userID, profileID string,
) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := d.db.WithContext(ctx).
		Model(&model.Chat{}).
		Select("va_chat.chat_id", "va_chat.create_time").
		Joins(`JOIN va_chat_participant cp_prof 
			   ON cp_prof.chat_id = va_chat.chat_id 
			   AND cp_prof.participant_id = ? 
			   AND cp_prof.participant_type = ?`,
			profileID, model.ParticipantTypeCustomProfile).
		Where("project_id = ? AND status = 0 AND is_video = false", projectID).
		Order("create_time DESC").
		Find(&chats).Error
	return chats, err
}
