package temp

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service/auth/types"
	"va_visionai_server/internal/zlog"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrMissingDeviceID    = errors.New("missing device identifier")
)

// TempLoginProvider 临时用户登录提供商
type TempLoginProvider struct {
	projectID string
	userDao   *dao.UserDao
}

// NewTempLoginProvider 创建临时用户登录提供商
func NewTempLoginProvider() types.LoginProvider {
	return &TempLoginProvider{}
}

// Initialize 初始化
func (p *TempLoginProvider) Initialize(ctx context.Context, config types.ProviderConfig) error {
	p.projectID = config.ProjectID

	// 获取用户仓库
	userDao, ok := config.ExtraConfig["user_dao"].(*dao.UserDao)
	if !ok {
		return errors.New("user dao not provided")
	}
	p.userDao = userDao

	return nil
}

// GetType 获取类型
func (p *TempLoginProvider) GetType() string {
	return "temp"
}

// Login 处理登录
func (p *TempLoginProvider) Login(ctx context.Context, params types.LoginParams) (*types.LoginInfo, error) {
	// 类型断言
	tempParams, ok := params.(*types.TempUserLoginParams)
	if !ok {
		return nil, errors.New("invalid login params type")
	}

	// 验证参数
	if err := tempParams.Validate(); err != nil {
		return nil, err
	}

	tempUserIden := tempParams.TempUserIden

	// 构建用户信息
	loginInfo := &types.LoginInfo{
		ProviderUserID: tempUserIden,
		ExtraData: map[string]interface{}{
			"login_method": "temp_user",
			"device_id":    tempParams.BaseLoginParams.DeviceInfo.DeviceID,
			"os":           tempParams.BaseLoginParams.DeviceInfo.OS,
		},
	}

	zlog.LogWithContext(ctx).Info("临时用户登录",
		zap.String("temp_user_id", tempUserIden),
		zap.String("device_id", tempParams.BaseLoginParams.DeviceInfo.DeviceID))

	return loginInfo, nil
}

// RefreshToken 刷新令牌
func (p *TempLoginProvider) RefreshToken(ctx context.Context, refreshToken string) (*types.RefreshResult, error) {
	// 临时用户的令牌刷新由LoginService统一处理
	return nil, errors.New("refresh token should be handled by login service")
}

// GenerateUserID 生成用户ID
func (p *TempLoginProvider) GenerateUserID(ctx context.Context, loginInfo *types.LoginInfo) (string, error) {
	// 对于临时用户登录，直接使用提供商用户ID作为用户ID
	if loginInfo.ProviderUserID == "" {
		zlog.LogWithContext(ctx).Error("Provider user ID is required for temp user login")
		return "", ErrMissingDeviceID
	}

	return loginInfo.ProviderUserID, nil
}

// RevokeRefreshToken 撤销刷新令牌
func (p *TempLoginProvider) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	return nil
}
