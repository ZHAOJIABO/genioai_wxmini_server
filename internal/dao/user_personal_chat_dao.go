package dao

import (
	"context"

	"gorm.io/gorm"

	"va_visionai_server/internal/model"
)

// UserPersonalChatDao 用户个人聊天数据访问对象
type UserPersonalChatDao struct {
	db *gorm.DB
}

// NewUserPersonalChatDao 创建用户个人聊天数据访问对象
func NewUserPersonalChatDao(db *gorm.DB) *UserPersonalChatDao {
	return &UserPersonalChatDao{
		db: db,
	}
}

// Save 保存 UserPersonalChat 记录
func (d *UserPersonalChatDao) Save(ctx context.Context, chat *model.UserPersonalChat) error {
	return d.db.Save(chat).Error
}

// GetChatListByUserDoc 获取用户文档相关的聊天历史记录
func (d *UserPersonalChatDao) GetChatListByUserDoc(
	ctx context.Context,
	userID, userDocID string,
	count, offset int,
) ([]*model.UserPersonalChat, error) {
	var chats []*model.UserPersonalChat

	if offset < 1 {
		offset = 1
	}

	query := d.db.Where("user_id = ? AND user_doc_id = ?", userID, userDocID)
	err := query.Order("updated_at desc").
		Limit(count).
		Offset((offset - 1) * count).
		Find(&chats).Error

	return chats, err
}

// GetByID 根据ID获取 UserPersonalChat 记录
func (d *UserPersonalChatDao) GetByID(
	ctx context.Context,
	chatID string,
) (*model.UserPersonalChat, error) {
	var chat model.UserPersonalChat
	err := d.db.Where("chat_id = ?", chatID).First(&chat).Error
	if err != nil {
		return nil, err
	}
	return &chat, nil
}

// DeleteByChatID 删除用户的聊天记录
func (d *UserPersonalChatDao) DeleteByChatID(
	ctx context.Context,
	userID, chatID string,
) error {
	return d.db.Where("user_id = ? AND chat_id = ?", userID, chatID).
		Delete(&model.UserPersonalChat{}).Error
}

// GetChatsByUserID 获取用户所有文档聊天
func (d *UserPersonalChatDao) GetChatsByUserID(
	ctx context.Context,
	userID string,
	count, offset int,
) ([]*model.UserPersonalChat, error) {
	var chats []*model.UserPersonalChat

	if offset < 1 {
		offset = 1
	}

	err := d.db.Where("user_id = ?", userID).
		Order("updated_at desc").
		Limit(count).
		Offset((offset - 1) * count).
		Find(&chats).Error

	return chats, err
}
