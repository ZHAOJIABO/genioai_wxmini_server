package dao

import (
	"gorm.io/gorm"

	adminmodel "va_visionai_server/internal/admin/model"
)

type AuditLogDao struct {
	db *gorm.DB
}

func NewAuditLogDao(db *gorm.DB) *AuditLogDao {
	return &AuditLogDao{db: db}
}

// Create 创建审计日志
func (d *AuditLogDao) Create(log *adminmodel.AdminAuditLog) error {
	return d.db.Create(log).Error
}

// List 分页获取审计日志
func (d *AuditLogDao) List(page, size int) ([]adminmodel.AdminAuditLog, int64, error) {
	var logs []adminmodel.AdminAuditLog
	var total int64

	d.db.Model(&adminmodel.AdminAuditLog{}).Count(&total)

	offset := (page - 1) * size
	err := d.db.Order("created_at DESC").Offset(offset).Limit(size).Find(&logs).Error
	return logs, total, err
}
