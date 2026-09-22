package model

import "gorm.io/gorm"

// AdminUser 管理员用户模型
type AdminUser struct {
	gorm.Model
	Username     string `gorm:"type:varchar(64);uniqueIndex;not null"`
	PasswordHash string `gorm:"type:varchar(255);not null"`
	DisplayName  string `gorm:"type:varchar(100)"`
	Role         string `gorm:"type:varchar(32);not null;default:'admin'"` // super_admin, admin
	Status       int    `gorm:"type:tinyint(1);not null;default:1"`       // 1=active, 0=disabled
	LastLoginAt  *int64 `gorm:"column:last_login_at"`
	LastLoginIP  string `gorm:"type:varchar(50)"`
}

func (AdminUser) TableName() string {
	return "va_admin_user"
}
