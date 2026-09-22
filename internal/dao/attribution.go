package dao

import (
	"gorm.io/gorm"

	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
)

type AttributionDao struct {
	DB *gorm.DB
}

func AttributionDAO() *AttributionDao {
	return &AttributionDao{DB: db.GetDB()}
}
func (d *AttributionDao) CreateAttribution(attributionInfo *model.AttributionEvent) error {
	return d.DB.Model(&model.AttributionEvent{}).Create(&attributionInfo).Error
}
func (d *AttributionDao) CreateAsaEvent(event *model.AsaEvent) error {
	return d.DB.Model(&model.AsaEvent{}).Create(&event).Error
}
