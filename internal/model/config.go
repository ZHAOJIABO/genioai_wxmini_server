package model

import "gorm.io/gorm"

type Config struct {
	gorm.Model

	ConfigName string `gorm:"config_name;type:varchar(64);uniqueIndex:va_config_unique_key"`
	Value      string `gorm:"value"`
	Comment    string `gorm:"comment"`
}

type FeecBackConfig struct {
	PopupInterval uint32 `json:"popup_interval"`
	PopupCount    uint32 `json:"popup_count"`
}
