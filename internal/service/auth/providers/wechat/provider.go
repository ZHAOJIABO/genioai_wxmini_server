package wechat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-redis/redis"

	"va_visionai_server/internal/service/auth/types"
)

const ProviderType = "wechat_miniprogram"
const sessionURL = "https://api.weixin.qq.com/sns/jscode2session"

// Provider exchanges a one-use code for identity. Only session signatures are retained.
type Provider struct {
	projectID string
	appID     string
	appSecret string
	client    *http.Client
	redis     *redis.Client
}

var _ types.LoginProvider = (*Provider)(nil)

func NewProvider(redisClient *redis.Client) *Provider {
	return &Provider{redis: redisClient, client: &http.Client{
		Timeout: 5 * time.Second,
		// A redirect must never forward credentials to another endpoint.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (p *Provider) Initialize(_ context.Context, config types.ProviderConfig) error {
	if strings.TrimSpace(config.ProjectID) == "" || config.ProjectID == "default" || strings.TrimSpace(config.ClientID) == "" || strings.TrimSpace(config.ClientSecret) == "" {
		return errors.New("wechat project ID, AppID and AppSecret are required")
	}
	p.projectID, p.appID, p.appSecret = config.ProjectID, config.ClientID, config.ClientSecret
	return nil
}

func (*Provider) GetType() string { return ProviderType }

func (p *Provider) Login(ctx context.Context, params types.LoginParams) (*types.LoginInfo, error) {
	input, ok := params.(*types.WeChatLoginParams)
	if !ok {
		return nil, types.ErrInvalidCredentials
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if input.ProjectID != p.projectID {
		return nil, types.ErrInvalidLoginContext
	}
	query := url.Values{
		"appid": {p.appID}, "secret": {p.appSecret},
		"js_code": {input.Code}, "grant_type": {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sessionURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, errors.New("cannot build wechat login request")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		// net/url errors contain the full URL, including AppSecret and code.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("wechat login request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wechat login HTTP status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if err != nil || len(body) > 64*1024 {
		return nil, errors.New("cannot read wechat login response")
	}
	var result struct {
		OpenID     string `json:"openid"`
		SessionKey string `json:"session_key"`
		ErrCode    int    `json:"errcode"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.New("invalid wechat login response")
	}
	if result.ErrCode != 0 {
		switch result.ErrCode {
		case 40029, 40163, 40226:
			return nil, types.ErrInvalidCredentials
		default:
			return nil, fmt.Errorf("wechat login error %d", result.ErrCode)
		}
	}
	if strings.TrimSpace(result.OpenID) == "" {
		return nil, errors.New("wechat response missing openid")
	}
	identity := ProviderType + ":" + p.appID + ":" + result.OpenID
	if len(identity) > 100 {
		return nil, errors.New("wechat identity exceeds storage limit")
	}
	if result.SessionKey == "" {
		return nil, errors.New("wechat response missing session key")
	}
	if p.redis == nil {
		return nil, errors.New("wechat session storage unavailable")
	}
	if err := p.redis.WithContext(ctx).Set(p.signatureKey(result.OpenID), sessionSignature(result.SessionKey), sessionRetention).Err(); err != nil {
		return nil, errors.New("cannot store wechat session signature")
	}
	return &types.LoginInfo{ProviderUserID: identity}, nil
}

func (*Provider) RefreshToken(context.Context, string) (*types.RefreshResult, error) {
	return nil, errors.New("refresh token is handled by auth service")
}

func (*Provider) GenerateUserID(_ context.Context, info *types.LoginInfo) (string, error) {
	if info == nil || info.ProviderUserID == "" {
		return "", types.ErrInvalidCredentials
	}
	sum := sha256.Sum256([]byte(info.ProviderUserID))
	return hex.EncodeToString(sum[:])[:32], nil
}

func (*Provider) RevokeRefreshToken(context.Context, string) error { return nil }
