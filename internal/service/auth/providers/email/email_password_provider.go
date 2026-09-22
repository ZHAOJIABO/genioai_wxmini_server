package email

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service/auth/types"
	"va_visionai_server/internal/zlog"
)

var (
	ErrPasswordRequired      = errors.New("password is required")
	ErrInvalidEmailOrPassword = errors.New("invalid email or password")
	ErrUserNotFound          = errors.New("user not found")
	ErrPasswordNotSet        = errors.New("password not set for this account")
)

// EmailPasswordProvider 邮箱密码登录提供商
type EmailPasswordProvider struct {
	projectID string
	userDao   *dao.UserDao
}

// NewEmailPasswordProvider 创建邮箱密码登录提供商
func NewEmailPasswordProvider(userDao *dao.UserDao) types.LoginProvider {
	return &EmailPasswordProvider{
		userDao: userDao,
	}
}

// Initialize 初始化
func (p *EmailPasswordProvider) Initialize(ctx context.Context, config types.ProviderConfig) error {
	p.projectID = config.ProjectID
	return nil
}

// GetType 获取类型
func (p *EmailPasswordProvider) GetType() string {
	return "email_password"
}

// Login 处理登录请求
func (p *EmailPasswordProvider) Login(ctx context.Context, params types.LoginParams) (*types.LoginInfo, error) {
	// 类型断言
	emailParams, ok := params.(*types.EmailPasswordLoginParams)
	if !ok {
		return nil, errors.New("invalid login params type")
	}

	// 验证参数
	if err := emailParams.Validate(); err != nil {
		return nil, err
	}

	email := emailParams.Email
	password := emailParams.Password

	// 使用运行时的ProjectID，而不是初始化时的projectID
	projectID := emailParams.ProjectID

	logFields := []zap.Field{
		zap.String("project_id", projectID),
		zap.String("email", email),
		zap.String("provider_type", "email_password"),
	}

	zlog.LogWithContext(ctx).Info("EmailPasswordProvider.Login start", logFields...)

	// 根据邮箱查询用户
	user, err := p.userDao.GetUserByEmail(projectID, email)
	if err != nil {
		zlog.LogWithContext(ctx).Error("User not found by email",
			append(logFields, zap.Error(err))...)
		return nil, ErrInvalidEmailOrPassword
	}

	// 检查用户是否设置了密码
	if user.PasswordHash == "" {
		zlog.LogWithContext(ctx).Warn("User password not set",
			append(logFields, zap.String("user_id", user.UserId))...)
		return nil, ErrPasswordNotSet
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		zlog.LogWithContext(ctx).Warn("Password verification failed",
			append(logFields, zap.String("user_id", user.UserId), zap.Error(err))...)
		return nil, ErrInvalidEmailOrPassword
	}

	// 构建用户信息
	loginInfo := &types.LoginInfo{
		ProviderUserID: email,
		Email:          email,
		NickName:       user.UserName,
		Avatar:         user.AvatarUrl,
		ExtraData: map[string]interface{}{
			"login_method": "email_password",
			"user_id":      user.UserId,
		},
	}

	zlog.LogWithContext(ctx).Info("EmailPasswordProvider.Login success",
		append(logFields, zap.String("user_id", user.UserId))...)

	return loginInfo, nil
}

// RefreshToken 刷新令牌
func (p *EmailPasswordProvider) RefreshToken(ctx context.Context, refreshToken string) (*types.RefreshResult, error) {
	// 邮箱密码登录的令牌刷新由AuthService统一处理
	return nil, errors.New("refresh token should be handled by auth service")
}

// GenerateUserID 生成用户ID
func (p *EmailPasswordProvider) GenerateUserID(ctx context.Context, loginInfo *types.LoginInfo) (string, error) {
	if loginInfo.Email == "" {
		zlog.LogWithContext(ctx).Error("Email is required")
		return "", ErrEmailRequired
	}

	// 如果ExtraData中已经有user_id（表示是老用户），直接返回
	if userID, ok := loginInfo.ExtraData["user_id"].(string); ok && userID != "" {
		return userID, nil
	}

	// 新用户：使用邮箱生成用户ID
	return fmt.Sprintf("email_%s", loginInfo.Email), nil
}

// RevokeRefreshToken 撤销刷新令牌
func (p *EmailPasswordProvider) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	// 邮箱密码登录不需要撤销令牌
	return nil
}

// HashPassword 对密码进行哈希加密
func HashPassword(password string) (string, error) {
	// 使用bcrypt加密，成本因子为10（默认值，平衡安全性和性能）
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hashedBytes), nil
}

// VerifyPassword 验证密码是否正确
func VerifyPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
