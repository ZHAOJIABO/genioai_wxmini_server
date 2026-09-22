package dao

import (
	"gorm.io/gorm"

	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
)

type TTSDao struct {
	db *gorm.DB
}

func NewTTSDao() *TTSDao {
	return &TTSDao{db: db.GetDB()}
}

func (d *TTSDao) Create(r model.TTS) error {
	return d.db.Model(&model.TTS{}).Create(r).Error
}

func (d *TTSDao) FindAudio(chatID, msgID string) (*model.TTS, error) {
	r := &model.TTS{}
	err := d.db.Model(&model.TTS{}).Where("chat_id = ? and msg_id = ?", chatID, msgID).First(&r).Error
	return r, err
}
