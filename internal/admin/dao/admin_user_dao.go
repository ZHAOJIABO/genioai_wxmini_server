package dao

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	adminmodel "va_visionai_server/internal/admin/model"
)

type AdminUserDao struct {
	db *gorm.DB
}

func NewAdminUserDao(db *gorm.DB) *AdminUserDao {
	return &AdminUserDao{db: db}
}

// GetByUsername 根据用户名查询管理员
func (d *AdminUserDao) GetByUsername(username string) (*adminmodel.AdminUser, error) {
	var user adminmodel.AdminUser
	err := d.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByID 根据ID查询管理员
func (d *AdminUserDao) GetByID(id uint) (*adminmodel.AdminUser, error) {
	var user adminmodel.AdminUser
	err := d.db.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateLastLogin 更新最后登录信息
func (d *AdminUserDao) UpdateLastLogin(id uint, ip string) error {
	now := time.Now().Unix()
	return d.db.Model(&adminmodel.AdminUser{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_login_at": now,
		"last_login_ip": ip,
	}).Error
}

// UpdatePassword 更新密码
func (d *AdminUserDao) UpdatePassword(id uint, passwordHash string) error {
	return d.db.Model(&adminmodel.AdminUser{}).Where("id = ?", id).Update("password_hash", passwordHash).Error
}

// EnsureDefaultAdmin 确保存在默认管理员账号
func (d *AdminUserDao) EnsureDefaultAdmin() error {
	var count int64
	d.db.Model(&adminmodel.AdminUser{}).Count(&count)
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	admin := &adminmodel.AdminUser{
		Username:     "admin",
		PasswordHash: string(hash),
		DisplayName:  "Super Admin",
		Role:         "super_admin",
		Status:       1,
	}
	return d.db.Create(admin).Error
}
