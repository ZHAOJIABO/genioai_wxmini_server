package google

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/auth/types"
	"va_visionai_server/internal/zlog"
)

var (
	ErrInvalidToken         = errors.New("invalid google token")
	ErrGoogleIDRequired     = errors.New("google id is required")
	ErrJWTClaimsTypeCast    = errors.New("failed to type cast JWT claims")
	ErrMissingIdentityToken = errors.New("missing identity token")
	ErrInvalidAudience      = errors.New("invalid audience")
	ErrInvalidIssuer        = errors.New("invalid issuer")
	ErrTokenExpired         = errors.New("token expired")
	ErrInvalidKeyID         = errors.New("invalid key id")
)

const (
	googleTokenInfoURL = "https://oauth2.googleapis.com/tokeninfo?id_token=%s"
	// 配置键
	googleProviderConfigKey = "auth_provider_google_client_id_map"
)

// GoogleProviderConfig 谷歌登录提供商配置
type GoogleProviderConfig map[string]string // 项目ID到客户端ID的映射
// GoogleClaims 定义Google JWT的声明
type GoogleClaims struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	jwt.RegisteredClaims
}

// GoogleLoginProvider Google登录提供商
type GoogleLoginProvider struct {
	projectID     string
	userDao       *dao.UserDao
	keyManager    *KeyManager
	configService *service.ConfigService
}

// NewGoogleLoginProvider 创建Google登录提供商
func NewGoogleLoginProvider(
	configService *service.ConfigService,
	userDao *dao.UserDao,
	redisClient *redis.Client,
) types.LoginProvider {
	return &GoogleLoginProvider{
		configService: configService,
		userDao:       userDao,
		keyManager:    NewKeyManager(redisClient),
	}
}

// Initialize 初始化
func (p *GoogleLoginProvider) Initialize(ctx context.Context, config types.ProviderConfig) error {
	p.projectID = config.ProjectID

	// 启动密钥刷新器
	p.keyManager.StartKeyRefresher(ctx)

	return nil
}

// GetType 获取类型
func (p *GoogleLoginProvider) GetType() string {
	return "google"
}

// GetProjectID 获取项目ID
func (p *GoogleLoginProvider) GetProjectID() string {
	return p.projectID
}

// PrepareLogin 准备登录
// func (p *GoogleLoginProvider) PrepareLogin(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
// 	// 生成授权URL
// 	authURL := fmt.Sprintf(
// 		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=openid profile email&access_type=offline",
// 		p.clientID, p.redirectURI)

// 	return map[string]interface{}{
// 		"redirect_url": authURL,
// 		"client_id":    p.clientID,
// 	}, nil
// }

func (p *GoogleLoginProvider) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	return nil
}

// verifyGoogleToken 验证Google身份令牌
func (p *GoogleLoginProvider) verifyGoogleToken(ctx context.Context, tokenString string) (*GoogleClaims, error) {
	// 尝试通过本地验证方式处理
	claims, err := p.verifyWithLocalKeys(ctx, tokenString)
	if err != nil {
		// 如果无法获取Google证书或验证失败，尝试使用Google tokeninfo API
		zlog.LogWithContext(ctx).Warn("Failed to verify token with local keys, trying Google tokeninfo API",
			zap.Error(err))

		return p.verifyWithGoogleAPI(ctx, tokenString)
	}

	return claims, nil
}

// verifyWithLocalKeys 使用本地缓存的密钥验证token
func (p *GoogleLoginProvider) verifyWithLocalKeys(ctx context.Context, tokenString string) (*GoogleClaims, error) {
	// 解析未验证的token以获取kid
	token, _ := jwt.Parse(tokenString, nil)
	if token == nil {
		return nil, ErrInvalidToken
	}

	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, ErrInvalidKeyID
	}

	// 获取Google公钥
	jwks, err := p.keyManager.GetKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Google public keys: %w", err)
	}

	// 查找匹配的公钥
	var matchingKey *struct {
		Kid string
		N   string
		E   string
	}
	for _, key := range jwks.Keys {
		if key.Kid == kid {
			matchingKey = &struct {
				Kid string
				N   string
				E   string
			}{
				Kid: key.Kid,
				N:   key.N,
				E:   key.E,
			}
			break
		}
	}

	if matchingKey == nil {
		return nil, ErrInvalidKeyID
	}

	// 构建RSA公钥
	nBytes, err := base64.RawURLEncoding.DecodeString(matchingKey.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(matchingKey.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key exponent: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	publicKey := &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}

	// 验证并解析token
	var claims GoogleClaims
	parsedToken, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims.Audience[0] != p.GetClientIDByProjectID(p.projectID) {
		return nil, ErrInvalidAudience
	}

	if !parsedToken.Valid {
		return nil, ErrInvalidToken
	}

	return &claims, nil
}

// verifyWithGoogleAPI 使用Google TokenInfo API验证token
func (p *GoogleLoginProvider) verifyWithGoogleAPI(ctx context.Context, tokenString string) (*GoogleClaims, error) {
	// 构建请求URL
	url := fmt.Sprintf(googleTokenInfoURL, tokenString)

	// 创建HTTP客户端
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	// 发送请求
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to call Google tokeninfo API: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Google tokeninfo API returned error: %d, body: %s",
			resp.StatusCode, string(body))
	}

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read tokeninfo response: %w", err)
	}

	// 解析响应到临时结构，因为Google API返回的字段与我们的GoogleClaims略有不同
	var tokenInfo struct {
		Iss           string `json:"iss"`
		Azp           string `json:"azp"`
		Aud           string `json:"aud"`
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified string `json:"email_verified"` // API 返回的是字符串 "true" 或 "false"
		AtHash        string `json:"at_hash"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
		Locale        string `json:"locale"`
		Iat           string `json:"iat"`
		Exp           string `json:"exp"`
		Alg           string `json:"alg"`
		Kid           string `json:"kid"`
	}

	if err := json.Unmarshal(body, &tokenInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tokeninfo response: %w", err)
	}

	claims := &GoogleClaims{
		Email:   tokenInfo.Email,
		Name:    tokenInfo.Name,
		Picture: tokenInfo.Picture,
	}

	claims.EmailVerified = tokenInfo.EmailVerified == "true"

	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:  tokenInfo.Iss,
		Subject: tokenInfo.Sub,
	}

	if tokenInfo.Aud != "" {
		claims.Audience = []string{tokenInfo.Aud}
	}

	zlog.LogWithContext(ctx).Info("Successfully verified Google token using tokeninfo API",
		zap.String("subject", claims.Subject),
		zap.String("email", claims.Email))

	return claims, nil
}

// Login 处理登录请求
func (p *GoogleLoginProvider) Login(ctx context.Context, params types.LoginParams) (*types.LoginInfo, error) {
	// 类型断言
	googleParams, ok := params.(*types.GoogleLoginParams)
	if !ok {
		return nil, errors.New("invalid login params type")
	}

	// 验证参数
	if err := googleParams.Validate(); err != nil {
		return nil, err
	}

	// 获取identityToken
	identityToken := googleParams.IdToken
	if identityToken == "" {
		zlog.LogWithContext(ctx).Error("Missing identity token")
		return nil, ErrMissingIdentityToken
	}

	// 验证token
	claims, err := p.verifyGoogleToken(ctx, identityToken)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to verify identity token",
			zap.Error(err))
		return nil, err
	}

	// 构建用户信息
	loginInfo := &types.LoginInfo{
		ProviderUserID: claims.Subject,
		Email:          claims.Email,
		ExtraData: map[string]interface{}{
			"login_method":   "google",
			"identity_token": identityToken,
			"email_verified": claims.EmailVerified,
			"name":           claims.Name,
			"picture":        claims.Picture,
		},
	}

	return loginInfo, nil
}

// RefreshToken 刷新令牌
func (p *GoogleLoginProvider) RefreshToken(ctx context.Context, refreshToken string) (*types.RefreshResult, error) {
	// Google登录的令牌刷新由LoginService统一处理
	return nil, errors.New("refresh token should be handled by login service")
}

// GenerateUserID 生成用户ID
func (p *GoogleLoginProvider) GenerateUserID(ctx context.Context, loginInfo *types.LoginInfo) (string, error) {
	if loginInfo.ProviderUserID == "" {
		zlog.LogWithContext(ctx).Error("Google ID is required")
		return "", ErrGoogleIDRequired
	}

	// 如果有邮箱，优先使用邮箱（去掉@及后面的部分）作为ID前缀
	if loginInfo.Email != "" {
		parts := strings.Split(loginInfo.Email, "@")
		if len(parts) > 0 && parts[0] != "" {
			return fmt.Sprintf("google_%s_%s", parts[0], loginInfo.ProviderUserID[:8]), nil
		}
	}

	// 否则使用google_前缀加上唯一ID
	return "google_" + loginInfo.ProviderUserID, nil
}

// GetClientIDByProjectID 根据项目ID获取Google客户端ID
func (p *GoogleLoginProvider) GetClientIDByProjectID(projectID string) string {
	if projectID == "" {
		return ""
	}
	if p.configService == nil {
		zlog.Logger.Warn("Config service not available",
			zap.String("project_id", projectID))
		return ""
	}

	var config GoogleProviderConfig
	err := p.configService.GetJSONConfig(googleProviderConfigKey, &config)
	if err != nil {
		zlog.Logger.Warn("Failed to get Google provider config",
			zap.String("project_id", projectID),
			zap.Error(err))
		return ""
	}

	if clientID, ok := config[projectID]; ok && clientID != "" {
		return clientID
	}

	return ""
}
