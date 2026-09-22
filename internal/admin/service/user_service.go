package service

import (
	"context"

	"gorm.io/gorm"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
)

type UserService struct {
	db      *gorm.DB
	userDao *dao.UserDao
}

func NewUserService(db *gorm.DB, userDao *dao.UserDao) *UserService {
	return &UserService{db: db, userDao: userDao}
}

type AppUser struct {
	ID        uint   `json:"id"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Status    int    `json:"status"`
	LoginType string `json:"login_type"`
	Country   string `json:"country"`
	CreatedAt int64  `json:"created_at"`
}

// ListUsers 分页获取用户列表
func (s *UserService) ListUsers(ctx context.Context, page, size int, search string) ([]AppUser, int64, error) {
	var users []model.UserRecord
	var total int64

	query := s.db.Model(&model.UserRecord{})
	if search != "" {
		query = query.Where("user_name LIKE ? OR email LIKE ? OR user_id LIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	query.Count(&total)

	offset := (page - 1) * size
	err := query.Order("id DESC").Offset(offset).Limit(size).Find(&users).Error
	if err != nil {
		return nil, 0, err
	}

	result := make([]AppUser, 0, len(users))
	for _, u := range users {
		result = append(result, AppUser{
			ID:        u.ID,
			UserID:    u.UserId,
			UserName:  u.UserName,
			Email:     u.Email,
			Phone:     u.PhoneNumber,
			Status:    u.Status,
			LoginType: u.LoginType,
			Country:   u.Country,
			CreatedAt: u.CreateTime,
		})
	}

	return result, total, nil
}

// GetUser 获取用户详情
func (s *UserService) GetUser(ctx context.Context, userID string) (*AppUser, error) {
	var user model.UserRecord
	err := s.db.Where("user_id = ?", userID).First(&user).Error
	if err != nil {
		return nil, err
	}

	return &AppUser{
		ID:        user.ID,
		UserID:    user.UserId,
		UserName:  user.UserName,
		Email:     user.Email,
		Phone:     user.PhoneNumber,
		Status:    user.Status,
		LoginType: user.LoginType,
		Country:   user.Country,
		CreatedAt: user.CreateTime,
	}, nil
}

// BanUser 封禁用户
func (s *UserService) BanUser(ctx context.Context, userID string) error {
	return s.db.Model(&model.UserRecord{}).Where("user_id = ?", userID).Update("status", 1).Error
}

// UnbanUser 解封用户
func (s *UserService) UnbanUser(ctx context.Context, userID string) error {
	return s.db.Model(&model.UserRecord{}).Where("user_id = ?", userID).Update("status", 0).Error
}
