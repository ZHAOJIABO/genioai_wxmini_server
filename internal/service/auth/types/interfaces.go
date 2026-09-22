package types

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrProviderNotFound    = errors.New("login provider not found")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidToken        = errors.New("invalid token")
	ErrInvalidLoginContext = errors.New("invalid login context")
	ErrUserCreateFailed    = errors.New("user create failed")
	ErrTokenRefreshFailed  = errors.New("token refresh failed")
	ErrProviderInitFailed  = errors.New("provider initialization failed")
)

// LoginInfo 包含登录过程中收集的用户信息
type LoginInfo struct {
	ProviderUserID string                 // 提供商的用户ID
	NickName       string                 // 用户昵称
	PhoneNumber    string                 // 电话号码
	Email          string                 // 电子邮件
	Avatar         string                 // 头像URL
	ExtraData      map[string]interface{} // 额外的数据
}

// RefreshResult 包含刷新令牌的结果
type RefreshResult struct {
	UserID                string // 用户ID
	AccessToken           string // 访问令牌
	RefreshToken          string // 刷新令牌
	AccessTokenExpiresIn  int64  // 访问令牌有效期（秒）
	RefreshTokenExpiresIn int64  // 刷新令牌有效期（秒）
}

// ProviderConfig 存储登录提供商的配置
type ProviderConfig struct {
	ProjectID    string                 // 项目ID
	ProviderType string                 // 提供商类型 (normal, apple, google, 等)
	ClientID     string                 // OAuth客户端ID
	ClientSecret string                 // OAuth客户端密钥
	Kid          string                 // OAuth客户端密钥ID
	PrivateKey   string                 // OAuth客户端私钥
	TeamID       string                 // OAuth团队ID
	RedirectURI  string                 // OAuth重定向URI
	ExtraConfig  map[string]interface{} // 额外配置
}

// LoginProvider 定义登录提供商需要实现的接口
type LoginProvider interface {
	// Initialize 初始化提供商
	Initialize(ctx context.Context, config ProviderConfig) error

	// GetType 获取登录提供商类型
	GetType() string

	// Login 处理登录请求，返回用户信息
	Login(ctx context.Context, params LoginParams) (*LoginInfo, error)

	// RefreshToken 刷新访问令牌
	RefreshToken(ctx context.Context, refreshToken string) (*RefreshResult, error)

	// GenerateUserID 根据登录信息生成唯一用户ID
	GenerateUserID(ctx context.Context, loginInfo *LoginInfo) (string, error)

	// RevokeRefreshToken 撤销刷新令牌
	RevokeRefreshToken(ctx context.Context, refreshToken string) error
}

// VerifyService 定义验证服务接口
type VerifyService interface {
	// Verify 验证码验证
	Verify(ctx context.Context, phone, code string) (bool, error)
}

// UserProvider 定义用户信息提供接口
type UserProvider interface {
	// GetUserByUID 根据用户ID获取用户信息
	GetUserByUID(ctx context.Context, uid string) (interface{}, error)

	// GetUserByPhone 根据手机号获取用户信息
	GetUserByPhone(ctx context.Context, phone string) (interface{}, error)

	// GetUserByEmail 根据邮箱获取用户信息
	GetUserByEmail(ctx context.Context, email string) (interface{}, error)

	// CreateUser 创建用户
	CreateUser(ctx context.Context, user interface{}) error

	// UpdateUser 更新用户信息
	UpdateUser(ctx context.Context, uid string, updates map[string]interface{}) error
}

// LoginResult 登录结果
type LoginResult struct {
	// 用户信息
	UserInfo *UserInfo
	// 令牌信息
	TokenInfo *TokenInfo
	// 是否是新用户
	IsNewUser bool
	// 额外数据
	ExtraData map[string]interface{}
}

// LoginPrepareResult 登录准备结果
type LoginPrepareResult struct {
	// 重定向URL（OAuth流程使用）
	RedirectURL string
	// 额外数据
	ExtraData map[string]interface{}
}

// UserInfo 用户信息
type UserInfo struct {
	// 提供商用户ID
	ProviderUserID string
	// 系统用户ID
	SystemUserID string
	// 用户名
	Username string
	// 邮箱
	Email string
	// 手机号
	Phone string
	// 头像
	Avatar string
	// 元数据（JSON格式）
	Metadata map[string]interface{}
}

// TokenInfo 令牌信息
type TokenInfo struct {
	// 访问令牌
	AccessToken string
	// 刷新令牌
	RefreshToken string
	// 过期时间（秒）
	ExpiresIn int64
	// 令牌类型
	TokenType string
}

// UserIDGenerator 用户ID生成器接口
type UserIDGenerator interface {
	// Generate 生成用户ID
	Generate(ctx context.Context, providerType string, providerUserID string, userInfo map[string]interface{}) (string, error)
}

// DefaultUserIDGenerator 默认用户ID生成器
type DefaultUserIDGenerator struct{}

// Generate 默认的用户ID生成方法（兼容现有的手机号格式）
func (g *DefaultUserIDGenerator) Generate(ctx context.Context, providerType string, providerUserID string, userInfo map[string]interface{}) (string, error) {
	// 如果有手机号，使用手机号作为userID
	if phone, ok := userInfo["phone"].(string); ok && phone != "" {
		// 确保手机号格式符合期望
		if len(phone) > 0 && phone[0] == '+' {
			phone = phone[1:]
		}
		return phone, nil
	}

	// 否则使用提供商类型作为前缀，拼接提供商用户ID
	return providerType + "_" + providerUserID, nil
}

func CurrentTimeInSeconds() int64 {
	return Time.Now().Unix()
}

// 暴露依赖以便于单元测试时替换
var (
	Time TimeInterface = &DefaultTime{}
	JSON JSONInterface = &DefaultJSON{}
)

// TimeInterface 时间接口
type TimeInterface interface {
	Now() time.Time
}

// DefaultTime 默认时间实现
type DefaultTime struct{}

// Now 获取当前时间
func (t *DefaultTime) Now() time.Time {
	return time.Now()
}

// JSONInterface JSON接口
type JSONInterface interface {
	Marshal(v interface{}) ([]byte, error)
	Unmarshal(data []byte, v interface{}) error
}

// DefaultJSON 默认JSON实现
type DefaultJSON struct{}

// Marshal 将对象转换为JSON字节流
func (j *DefaultJSON) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal 将JSON字节流转换为对象
func (j *DefaultJSON) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
