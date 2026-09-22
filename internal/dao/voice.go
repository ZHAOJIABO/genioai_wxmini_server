package dao

import (
	"gorm.io/gorm"

	"va_visionai_server/internal/model"
)

type VoiceDao struct {
	DB *gorm.DB
}

func NewVoiceDAO(db *gorm.DB) *VoiceDao {
	return &VoiceDao{
		DB: db,
	}
}

func (dao *VoiceDao) Create(voice *model.Voice) error {

	return dao.DB.Model(&model.Voice{}).Create(&voice).Error
}

func (dao *VoiceDao) GetByFileHash(fileHash string) (model.Voice, error) {
	var voice model.Voice
	if err := dao.DB.Model(&model.Voice{}).Where("file_hash = ?", fileHash).First(&voice).Error; err != nil {
		return voice, err
	}
	return voice, nil
}

func (dao *VoiceDao) GetBatchByFileHash(fileHash []string) ([]model.Voice, error) {
	var voice []model.Voice
	if err := dao.DB.Model(&model.Voice{}).Where("file_hash in (?)", fileHash).Find(&voice).Error; err != nil {
		return voice, err
	}
	return voice, nil
}

func (dao *VoiceDao) GetByID(id int64) (*model.Voice, error) {
	var voice model.Voice
	if err := dao.DB.Model(&model.Voice{}).Where("id = ?", id).First(&voice).Error; err != nil {
		return nil, err
	}
	return &voice, nil
}

func (dao *VoiceDao) UpdateStatus(voiceInfo model.Voice) error {
	updateFields := []string{"status", "err_msg"}
	if voiceInfo.TaskID != "" {
		updateFields = append(updateFields, "task_id")
	}
	if voiceInfo.Content != "" {
		updateFields = append(updateFields, "content")
	}
	if voiceInfo.ParseStartTime.Valid && voiceInfo.ParseStartTime.Time.Unix() > 0 {
		updateFields = append(updateFields, "parse_start_time")
	}
	if voiceInfo.ParseEndTime.Valid && voiceInfo.ParseEndTime.Time.Unix() > 0 {
		updateFields = append(updateFields, "parse_end_time")
	}
	return dao.DB.Model(&model.Voice{}).
		Select(updateFields).
		Where("id = ?", voiceInfo.ID).
		Updates(voiceInfo).Error
}
