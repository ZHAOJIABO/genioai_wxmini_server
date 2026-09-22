package apple

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/Timothylock/go-signin-with-apple/apple"
	"github.com/go-redis/redis"
	"github.com/golang-jwt/jwt/v4"
	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service/auth/types"
	"va_visionai_server/internal/zlog"
)

const (
	appleTokenURL = "https://appleid.apple.com/auth/token" // Apple token 交换接口

	UserAgent    = "go-signin-with-apple"
	AcceptHeader = "application/json"
)

var (
	ErrInvalidToken         = errors.New("invalid apple token")
	ErrAppleIDRequired      = errors.New("apple id is required")
	ErrJWTClaimsTypeCast    = errors.New("failed to type cast JWT claims")
	ErrMissingIdentityToken = errors.New("missing identity token")
	ErrInvalidAudience      = errors.New("invalid audience")
	ErrInvalidIssuer        = errors.New("invalid issuer")
	ErrTokenExpired         = errors.New("token expired")
)

// AppleClaims 定义 Apple JWT 的声明
type AppleClaims struct {
	Iss string `json:"iss"` // 发行者，应为 "https://appleid.apple.com"
	Sub string `json:"sub"` // 用户唯一标识符
	Aud string `json:"aud"` // 受众，应匹配你的 App Bundle ID
	Exp int64  `json:"exp"` // 过期时间
	Iat int64  `json:"iat"` // 发行时间
	jwt.RegisteredClaims
}

// appleTokenResponse 对应 Apple Token 接口的返回 JSON
type appleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	IdToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// AppleLoginProvider Apple 登录提供者
type AppleLoginProvider struct {
	teamID     string
	clientID   string
	privateKey string

	mu          sync.RWMutex
	secretCache string    // JWT token 缓存
	secretExp   time.Time // JWT token 过期时间

	projectID    string
	kid          string
	clientSecret string
	redirectURI  string
	userDao      *dao.UserDao
	keyManager   *KeyManager
}

// NewAppleLoginProvider 创建新的 Apple 登录提供者
func NewAppleLoginProvider(privateKey, kid, teamID string, redisClient *redis.Client) *AppleLoginProvider {
	return &AppleLoginProvider{
		kid:        kid,
		privateKey: privateKey,
		teamID:     teamID,
		keyManager: NewKeyManager(redisClient),
	}
}

// Initialize 初始化
func (p *AppleLoginProvider) Initialize(ctx context.Context, config types.ProviderConfig) error {
	p.projectID = config.ProjectID
	p.clientID = config.ClientID
	p.clientSecret = config.ClientSecret
	p.redirectURI = config.RedirectURI

	userDao, ok := config.ExtraConfig["user_dao"].(*dao.UserDao)
	if !ok {
		return errors.New("user dao not provided")
	}
	p.userDao = userDao

	// 启动 key manager 的后台刷新
	p.keyManager.StartKeyRefresher(ctx)

	return nil
}

// GetType 获取类型
func (p *AppleLoginProvider) GetType() string {
	return "apple"
}

// GetProjectID 获取项目ID
func (p *AppleLoginProvider) GetProjectID() string {
	return p.projectID
}

// RefreshToken 刷新令牌
func (p *AppleLoginProvider) RefreshToken(ctx context.Context, refreshToken string) (*types.RefreshResult, error) {
	// Apple登录的令牌刷新由LoginService统一处理
	return nil, errors.New("refresh token should be handled by login service")
}

// GenerateUserID 生成用户ID
func (p *AppleLoginProvider) GenerateUserID(ctx context.Context, loginInfo *types.LoginInfo) (string, error) {
	if loginInfo.ProviderUserID == "" {
		zlog.LogWithContext(ctx).Error("Apple ID is required")
		return "", ErrAppleIDRequired
	}

	// 如果有邮箱，优先使用邮箱（去掉@及后面的部分）作为ID前缀
	if loginInfo.Email != "" {
		parts := strings.Split(loginInfo.Email, "@")
		if len(parts) > 0 && parts[0] != "" {
			return fmt.Sprintf("apple_%s_%s", parts[0], loginInfo.ProviderUserID[:8]), nil
		}
	}

	// 否则使用apple_前缀加上唯一ID
	return "apple_" + loginInfo.ProviderUserID, nil
}

// fetchRefreshToken 使用 authorizationCode 交换并返回 Apple 的 refresh_token
func (p *AppleLoginProvider) fetchRefreshToken(ctx context.Context, identityToken string, authorizationCode string) (string, error) {
	// 构造表单
	projectID := common.GetProjectID(ctx)
	secret, _ := apple.GenerateClientSecret(p.privateKey, p.teamID, projectID, p.kid)
	client := apple.New()
	vReq := apple.AppValidationTokenRequest{
		ClientID:     projectID,
		ClientSecret: secret,
		Code:         authorizationCode,
	}

	var resp apple.ValidationResponse

	err := client.VerifyAppToken(context.Background(), vReq, &resp)
	if err != nil {
		return "", fmt.Errorf("读取 token 响应失败: %w", err)
	}
	return resp.RefreshToken, nil
}

// verifyIdentityToken 验证Apple身份令牌
func (p *AppleLoginProvider) verifyIdentityToken(tokenString string) (string, string, error) {
	keys, err := p.keyManager.GetKeys(context.Background())
	if err != nil {
		return "", "", fmt.Errorf("failed to get apple public keys: %w", err)
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("kid not found in token header")
		}

		pubKey, err := p.getPublicKey(kid, keys)
		if err != nil {
			return nil, err
		}

		return pubKey, nil
	})

	if err != nil {
		zlog.LogWithContext(context.Background()).Error("failed to parse token", zap.Error(err))
		return "", "", fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", ErrJWTClaimsTypeCast
	}

	iss, ok := claims["iss"].(string)
	if !ok || iss != "https://appleid.apple.com" {
		return "", "", ErrInvalidIssuer
	}

	exp, ok := claims["exp"].(float64)
	if !ok || int64(exp) < time.Now().Unix() {
		return "", "", ErrTokenExpired
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return "", "", errors.New("subject claim not found or invalid")
	}

	email, _ := claims["email"].(string)

	return sub, email, nil
}

// getPublicKey 根据kid获取对应的公钥
func (p *AppleLoginProvider) getPublicKey(kid string, keys ApplePublicKey) (*rsa.PublicKey, error) {
	for _, key := range keys.Keys {
		if key.Kid == kid {
			// 解码模数(N)
			nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
			if err != nil {
				return nil, errors.New("failed to decode modulus: " + err.Error())
			}
			n := new(big.Int).SetBytes(nBytes)

			// 解码指数(E)
			eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
			if err != nil {
				return nil, errors.New("failed to decode exponent: " + err.Error())
			}
			e := new(big.Int).SetBytes(eBytes)

			// 创建RSA公钥
			publicKey := &rsa.PublicKey{
				N: n,
				E: int(e.Int64()),
			}

			return publicKey, nil
		}
	}

	return nil, errors.New("no matching key found for kid: " + kid)
}

// Login 处理登录请求
func (p *AppleLoginProvider) Login(ctx context.Context, params types.LoginParams) (*types.LoginInfo, error) {
	appleParams, ok := params.(*types.AppleLoginParams)
	if !ok {
		return nil, errors.New("invalid login params type")
	}

	if err := appleParams.Validate(); err != nil {
		return nil, err
	}

	identityToken := appleParams.IdToken
	if identityToken == "" {
		zlog.LogWithContext(ctx).Error("Missing identity token")
		return nil, ErrMissingIdentityToken
	}

	authorizationCode := appleParams.AuthorizationCode
	if authorizationCode == "" {
		zlog.LogWithContext(ctx).Error("Missing authorization code")
		return nil, errors.New("missing authorization code")
	}

	appleID, email, err := p.verifyIdentityToken(identityToken)
	if err != nil {
		zlog.LogWithContext(ctx).Error("验证 identity token 失败", zap.Error(err))
		return nil, err
	}

	refreshToken, err := p.fetchRefreshToken(ctx, identityToken, authorizationCode)
	if err != nil {
		zlog.LogWithContext(ctx).Error("交换 refresh token 失败", zap.Error(err))
		return nil, err
	}

	loginInfo := &types.LoginInfo{
		ProviderUserID: appleID,
		Email:          email,
		ExtraData: map[string]interface{}{
			"login_method":   "apple",
			"identity_token": identityToken,
			"refresh_token":  refreshToken,
		},
	}

	return loginInfo, nil
}

func (p *AppleLoginProvider) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	projectID := common.GetProjectID(ctx)
	secret, _ := apple.GenerateClientSecret(p.privateKey, p.teamID, projectID, p.kid)
	client := apple.New()

	vReq := apple.RevokeRefreshTokenRequest{
		ClientID:     projectID,
		ClientSecret: secret,
		RefreshToken: refreshToken,
	}

	var resp apple.RevokeResponse
	err := client.RevokeRefreshToken(ctx, vReq, &resp)
	if err != nil && err.Error() != "EOF" {
		zlog.LogWithContext(ctx).Error("revoke refresh token 失败", zap.Error(err))
		return err
	}
	if resp.Error != "" {
		zlog.LogWithContext(ctx).Error("revoke refreshtoken Response Error", zap.String("error", resp.Error), zap.String("error_description", resp.ErrorDescription))
		return errors.New(resp.Error + " - " + resp.ErrorDescription)
	}

	return nil
}
