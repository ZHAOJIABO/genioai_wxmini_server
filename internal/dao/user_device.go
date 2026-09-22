package dao

import (
	"gorm.io/gorm"

	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
)

type userDeviceDao struct{ db *gorm.DB }

var UserDevice userDeviceDao

func NewUserDeviceDao() userDeviceDao {
	return userDeviceDao{db: db.GetDB()}
}

func (d *userDeviceDao) CreateOrUpdate(info model.UserDeviceInfo) (*model.UserDeviceInfo, error) {
	err := d.db.Model(&model.UserDeviceInfo{}).
		Where("user_id = ? AND os = ? AND app_store = ?", info.UserID, info.OS, info.AppStore).
		Assign(info).
		FirstOrCreate(&info).Error
	return &info, err
}
