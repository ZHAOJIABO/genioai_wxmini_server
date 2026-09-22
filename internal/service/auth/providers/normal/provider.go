package normal

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/auth/types"
	"va_visionai_server/internal/zlog"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrMissingCredentials = errors.New("missing username or password")
	ErrPhoneRequired      = errors.New("phone number is required")
	ErrVerifyCodeFailed   = errors.New("verify code failed")
)

// NormalLoginProvider 普通登录提供商
type NormalLoginProvider struct {
	projectID  string
	userDao    *dao.UserDao
	smsService service.SmsVerifyService
}

// SMSVerifyService 短信验证服务接口
type SMSVerifyService interface {
	Verify(ctx context.Context, phoneNumber, verifyCode string) error
}

// NewNormalLoginProvider 创建普通登录提供商
func NewNormalLoginProvider() types.LoginProvider {
	return &NormalLoginProvider{}
}

// Initialize 初始化
func (p *NormalLoginProvider) Initialize(ctx context.Context, config types.ProviderConfig) error {
	p.projectID = config.ProjectID

	// 获取用户仓库
	userDao, ok := config.ExtraConfig["user_dao"].(*dao.UserDao)
	if !ok {
		return errors.New("user dao not provided")
	}
	p.userDao = userDao

	// 获取短信验证服务
	smsService, ok := config.ExtraConfig["sms_service"].(service.SmsVerifyService)
	if !ok {
		return errors.New("sms service not provided")
	}
	p.smsService = smsService

	return nil
}

// GetType 获取类型
func (p *NormalLoginProvider) GetType() string {
	return "normal"
}

// Login 处理登录
func (p *NormalLoginProvider) Login(ctx context.Context, params types.LoginParams) (*types.LoginInfo, error) {
	// 类型断言
	phoneParams, ok := params.(*types.PhoneVerifyLoginParams)
	if !ok {
		return nil, errors.New("invalid login params type")
	}

	// 验证参数
	if err := phoneParams.Validate(); err != nil {
		return nil, err
	}

	phoneNumber := phoneParams.PhoneNumber
	verifyCode := phoneParams.VerifyCode

	// 特殊测试账号处理
	if phoneNumber != "+8619396357938" || verifyCode != "8888" {
		// 验证短信验证码
		err := p.smsService.Verify(ctx, phoneNumber, verifyCode)
		if err != nil {
			zlog.LogWithContext(ctx).Error("Verify code failed",
				zap.String("phone", phoneNumber),
				zap.Error(err))
			return nil, ErrVerifyCodeFailed
		}
	}

	// 确保手机号格式统一
	phoneWithoutPrefix := types.NormalizePhoneNumber(phoneNumber)

	// 构建用户信息
	loginInfo := &types.LoginInfo{
		ProviderUserID: phoneWithoutPrefix,
		PhoneNumber:    phoneNumber,
		ExtraData: map[string]interface{}{
			"login_method": "phone_verify",
		},
	}

	return loginInfo, nil
}

// RefreshToken 刷新令牌
func (p *NormalLoginProvider) RefreshToken(ctx context.Context, refreshToken string) (*types.RefreshResult, error) {
	// 普通登录的令牌刷新由LoginService统一处理
	return nil, errors.New("refresh token should be handled by login service")
}

// GenerateUserID 生成用户ID
func (p *NormalLoginProvider) GenerateUserID(ctx context.Context, loginInfo *types.LoginInfo) (string, error) {
	// 对于普通登录，使用手机号作为用户ID
	if loginInfo.PhoneNumber == "" {
		zlog.LogWithContext(ctx).Error("Phone number is required for normal login")
		return "", ErrPhoneRequired
	}

	// 确保手机号格式统一（去除+号前缀）
	phoneWithoutPrefix := types.NormalizePhoneNumber(loginInfo.PhoneNumber)

	return phoneWithoutPrefix, nil
}

// RevokeRefreshToken 撤销刷新令牌
func (p *NormalLoginProvider) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	return nil
}
