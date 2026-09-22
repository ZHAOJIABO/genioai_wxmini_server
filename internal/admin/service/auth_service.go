package service

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"va_visionai_server/conf"
	admindao "va_visionai_server/internal/admin/dao"
	adminmodel "va_visionai_server/internal/admin/model"
)

type AuthService struct {
	adminUserDao *admindao.AdminUserDao
}

func NewAuthService(adminUserDao *admindao.AdminUserDao) *AuthService {
	return &AuthService{adminUserDao: adminUserDao}
}

// AdminClaims JWT claims for admin
type AdminClaims struct {
	AdminID  uint   `json:"admin_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Login 管理员登录
func (s *AuthService) Login(username, password, ip string) (string, *adminmodel.AdminUser, error) {
	user, err := s.adminUserDao.GetByUsername(username)
	if err != nil {
		return "", nil, errors.New("用户名或密码错误")
	}

	if user.Status != 1 {
		return "", nil, errors.New("账号已被禁用")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", nil, errors.New("用户名或密码错误")
	}

	// 生成JWT token
	token, err := s.generateToken(user)
	if err != nil {
		return "", nil, errors.New("生成token失败")
	}

	// 更新最后登录信息
	_ = s.adminUserDao.UpdateLastLogin(user.ID, ip)

	return token, user, nil
}

// ChangePassword 修改密码
func (s *AuthService) ChangePassword(adminID uint, oldPassword, newPassword string) error {
	user, err := s.adminUserDao.GetByID(adminID)
	if err != nil {
		return errors.New("用户不存在")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return errors.New("旧密码错误")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("密码加密失败")
	}

	return s.adminUserDao.UpdatePassword(adminID, string(hash))
}

// ValidateToken 验证JWT token
func (s *AuthService) ValidateToken(tokenString string) (*AdminClaims, error) {
	secret := conf.GlobalConfig.Admin.JWTSecret
	if secret == "" {
		secret = "default-admin-jwt-secret"
	}

	token, err := jwt.ParseWithClaims(tokenString, &AdminClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*AdminClaims)
	if !ok || !token.Valid {
		return nil, errors.New("无效的token")
	}

	return claims, nil
}

func (s *AuthService) generateToken(user *adminmodel.AdminUser) (string, error) {
	secret := conf.GlobalConfig.Admin.JWTSecret
	if secret == "" {
		secret = "default-admin-jwt-secret"
	}

	expiryStr := conf.GlobalConfig.Admin.TokenExpiry
	if expiryStr == "" {
		expiryStr = "24h"
	}
	expiry, err := time.ParseDuration(expiryStr)
	if err != nil {
		expiry = 24 * time.Hour
	}

	claims := &AdminClaims{
		AdminID:  user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
