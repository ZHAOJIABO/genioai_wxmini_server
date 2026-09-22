package model

import (
	"gorm.io/gorm"
)

type UploadFile struct {
	gorm.Model
	ProjectID    string `gorm:"type:varchar(100)"`
	UploaderRole string `gorm:"type:varchar(100)"`
	FileName     string `gorm:"type:varchar(255);not null"`
	FileType     string `gorm:"type:varchar(10);not null"`
	OSSAddr      string `gorm:"type:varchar(255);not null"`
	ContentURL   string `gorm:"type:varchar(255)"`
	Hash         string `gorm:"type:varchar(100);not null"`
	UserID       string `gorm:"type:varchar(100)"`
	TraceId      string `gorm:"type:varchar(100)"`

	//图片宽度
	PhotoWidth int32 `gorm:"-"`
	//图片高度
	PhotoHeight int32 `gorm:"-"`
}

type UploadFileInfo struct {
	Hash          string `gorm:"primaryKey;type:varchar(100);not null" json:"hash"`
	PhotoWidth    int32  `gorm:"type:int" json:"photo_width"`
	PhotoHeight   int32  `gorm:"type:int" json:"photo_height"`
	PhotoQuestion string `gorm:"type:varchar(2048)" json:"photo_question"`
	MoralReview   string `gorm:"type:varchar(255)" json:"moral_review"`
	ImageInfo     string `gorm:"type:varchar(4096)" json:"image_info"`
}
