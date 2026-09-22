package model

import "gorm.io/gorm"

// AdminAuditLog 审计日志模型
type AdminAuditLog struct {
	gorm.Model
	AdminID    uint   `gorm:"not null;index"`
	AdminName  string `gorm:"type:varchar(64);not null"`
	Action     string `gorm:"type:varchar(64);not null;index"` // add_credits, ban_user, update_config
	Resource   string `gorm:"type:varchar(64);not null"`       // user, credit, config
	ResourceID string `gorm:"type:varchar(255)"`
	Detail     string `gorm:"type:text"` // JSON 详情
	IP         string `gorm:"type:varchar(50)"`
}

func (AdminAuditLog) TableName() string {
	return "va_admin_audit_log"
}
