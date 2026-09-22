package model

import "gorm.io/gorm"

type UserDeviceInfo struct {
	gorm.Model

	UserID    string `gorm:"type:varchar(100);not null"`
	OS        string `gorm:"type:varchar(16);not null"`
	IDFV      string `gorm:"type:varchar(128);not null"`
	IDFA      string `gorm:"type:varchar(128);not null"`
	AndroidID string `gorm:"type:varchar(128);not null"`
	OAID      string `gorm:"type:varchar(128);not null"`
	OpenID    string `gorm:"type:varchar(128);not null"`
	AppStore  string `gorm:"type:varchar(128);not null"`

	DeviceInfoJson string `gorm:"type:text;not null"`
}
