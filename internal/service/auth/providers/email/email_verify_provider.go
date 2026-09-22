package email

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/auth/types"
	"va_visionai_server/internal/zlog"
)

var (
	ErrEmailRequired       = errors.New("email is required")
	ErrVerifyCodeRequired  = errors.New("verify code is required")
	ErrVerifyCodeInvalid   = errors.New("verify code is invalid")
	ErrEmailVerifyFailed   = errors.New("email verification failed")
)

// EmailVerifyProvider 邮箱验证码登录提供商
type EmailVerifyProvider struct {
	projectID         string
	userDao           *dao.UserDao
	emailVerifyService *service.EmailVerifyService
}

// NewEmailVerifyProvider 创建邮箱验证码登录提供商
func NewEmailVerifyProvider(
	userDao *dao.UserDao,
	emailVerifyService *service.EmailVerifyService,
) types.LoginProvider {
	return &EmailVerifyProvider{
		userDao:            userDao,
		emailVerifyService: emailVerifyService,
	}
}

// Initialize 初始化
func (p *EmailVerifyProvider) Initialize(ctx context.Context, config types.ProviderConfig) error {
	p.projectID = config.ProjectID
	return nil
}

// GetType 获取类型
func (p *EmailVerifyProvider) GetType() string {
	return "email"
}

// Login 处理登录请求
func (p *EmailVerifyProvider) Login(ctx context.Context, params types.LoginParams) (*types.LoginInfo, error) {
	// 类型断言
	emailParams, ok := params.(*types.EmailVerifyLoginParams)
	if !ok {
		return nil, errors.New("invalid login params type")
	}

	// 验证参数
	if err := emailParams.Validate(); err != nil {
		return nil, err
	}

	email := emailParams.Email
	verifyCode := emailParams.VerifyCode

	// 使用运行时的ProjectID，而不是初始化时的projectID
	projectID := emailParams.ProjectID

	logFields := []zap.Field{
		zap.String("project_id", projectID),
		zap.String("email", email),
		zap.String("provider_type", "email"),
	}

	zlog.LogWithContext(ctx).Info("EmailVerifyProvider.Login start", logFields...)

	// 验证邮箱验证码
	if err := p.emailVerifyService.Verify(ctx, email, verifyCode); err != nil {
		zlog.LogWithContext(ctx).Error("Email verify code validation failed",
			append(logFields, zap.Error(err))...)
		return nil, ErrVerifyCodeInvalid
	}

	// 构建用户信息
	loginInfo := &types.LoginInfo{
		ProviderUserID: email, // 使用邮箱作为提供商用户ID
		Email:          email,
		ExtraData: map[string]interface{}{
			"login_method": "email_verify",
		},
	}

	zlog.LogWithContext(ctx).Info("EmailVerifyProvider.Login success", logFields...)

	return loginInfo, nil
}

// RefreshToken 刷新令牌
func (p *EmailVerifyProvider) RefreshToken(ctx context.Context, refreshToken string) (*types.RefreshResult, error) {
	// 邮箱登录的令牌刷新由AuthService统一处理
	return nil, errors.New("refresh token should be handled by auth service")
}

// GenerateUserID 生成用户ID
func (p *EmailVerifyProvider) GenerateUserID(ctx context.Context, loginInfo *types.LoginInfo) (string, error) {
	if loginInfo.Email == "" {
		zlog.LogWithContext(ctx).Error("Email is required")
		return "", ErrEmailRequired
	}

	// 使用邮箱前缀作为用户ID（去掉@后面的部分）
	// 例如: user@example.com -> user
	// 为了全局唯一性，添加email_前缀
	return fmt.Sprintf("email_%s", loginInfo.Email), nil
}

// RevokeRefreshToken 撤销刷新令牌
func (p *EmailVerifyProvider) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	// 邮箱验证码登录不需要撤销令牌
	return nil
}
