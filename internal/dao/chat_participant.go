package dao

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"va_visionai_server/internal/model"
)

type ChatParticipantDao struct {
	db *gorm.DB
}

func NewChatParticipantDao(db *gorm.DB) *ChatParticipantDao {
	return &ChatParticipantDao{db: db}
}

// AddParticipants 批量添加参与者 (事务安全)
func (d *ChatParticipantDao) AddParticipants(tx *gorm.DB, participants []*model.ChatParticipant) error {
	db := d.db
	if tx != nil {
		db = tx
	}
	if len(participants) == 0 {
		return nil
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&participants).Error
}

// RemoveParticipant 移除单个参与者
func (d *ChatParticipantDao) RemoveParticipant(tx *gorm.DB, chatID string, participantID string, participantType model.ParticipantType) error {
	db := d.db
	if tx != nil {
		db = tx
	}
	return db.Where("chat_id = ? AND participant_id = ? AND participant_type = ?", chatID, participantID, participantType).Delete(&model.ChatParticipant{}).Error
}

// GetParticipantsByChatID 获取聊天的所有参与者信息
func (d *ChatParticipantDao) GetParticipantsByChatID(chatID string) ([]*model.ChatParticipant, error) {
	var participants []*model.ChatParticipant
	err := d.db.Where("chat_id = ?", chatID).Find(&participants).Error
	return participants, err
}

// GetChatIDsByParticipant 查询特定参与者参与的所有 ChatID (分页)
func (d *ChatParticipantDao) GetChatIDsByParticipant(participantID string, participantType model.ParticipantType, count, offset int) ([]string, error) {
	var chatIDs []string
	query := d.db.Model(&model.ChatParticipant{}).
		Where("participant_id = ? AND participant_type = ?", participantID, participantType)

	if count > 0 {
		query = query.Limit(count)
	}
	if count > 0 && offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Pluck("chat_id", &chatIDs).Error
	return chatIDs, err
}

// GetCommonChatIDs 查询多个参与者共同参与的 ChatID (精确匹配所有指定的参与者)
func (d *ChatParticipantDao) GetCommonChatIDs(participants []model.ChatParticipant, limit, offset int) ([]string, error) {
	if len(participants) == 0 {
		return []string{}, nil
	}

	var chatIDs []string
	participantCount := len(participants)

	subQuery := d.db.Model(&model.ChatParticipant{}).Select("chat_id")
	whereClause := d.db
	for i, p := range participants {
		clause := d.db.Where("participant_id = ? AND participant_type = ?", p.ParticipantID, p.ParticipantType)
		if i == 0 {
			whereClause = clause
		} else {
			whereClause = whereClause.Or(clause)
		}
	}
	subQuery = subQuery.Where(whereClause).Group("chat_id").Having("COUNT(*) = ?", participantCount)

	query := d.db.Table("(?) as t", subQuery).Order("t.chat_id DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if limit > 0 && offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Pluck("chat_id", &chatIDs).Error
	return chatIDs, err
}

// IsParticipant 检查某个参与者是否在聊天中
func (d *ChatParticipantDao) IsParticipant(chatID string, participantID string, participantType model.ParticipantType) (bool, error) {
	var count int64
	err := d.db.Model(&model.ChatParticipant{}).
		Where("chat_id = ? AND participant_id = ? AND participant_type = ?", chatID, participantID, participantType).
		Count(&count).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return count > 0, nil
}
