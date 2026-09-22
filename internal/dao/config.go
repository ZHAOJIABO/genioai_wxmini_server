package dao

import (
	"gorm.io/gorm"

	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
)

type ConfigDao struct {
	db *gorm.DB
}

func NewConfigDao() *ConfigDao {
	return &ConfigDao{db: db.GetDB()}
}

func (d *ConfigDao) GetConfigValue(key string) (string, error) {
	var config model.Config
	if err := d.db.Model(&model.Config{}).Where("config_name = ?", key).First(&config).Error; err != nil {
		return "", err
	}
	return config.Value, nil
}

func (d *ConfigDao) SetConfigValue(key string, value string) error {
	// 与模型字段保持一致：ConfigName / Value
	return d.db.Model(&model.Config{}).Where("config_name = ?", key).Update("value", value).Error
}
