package wechat

import (
	"bytes"
	"context"
	"crypto/hmac"
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

// This is a local retention limit, not WeChat's session expiry time.
const sessionRetention = 30 * 24 * time.Hour
const resetCooldown = 30 * time.Second

var ErrSessionUnavailable = errors.New("wechat session unavailable; login again")
var ErrResetTooFrequent = errors.New("wechat session reset too frequent")
var ErrSessionChanged = errors.New("wechat session changed; check session again")

func sessionSignature(key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	return hex.EncodeToString(mac.Sum(nil))
}

func (p *Provider) signatureKey(openID string) string {
	// Shared by projects using the same AppID: WeChat has one active key per user/app.
	sum := sha256.Sum256([]byte(p.appID + ":" + openID))
	return "wechat:session_signature:" + hex.EncodeToString(sum[:])
}

func (p *Provider) tokenKey() string { return "wechat:stable_token:" + p.appID }

// Compare-and-delete cannot remove a newer login's signature or token.
func (p *Provider) discard(ctx context.Context, key, value string) error {
	return p.redis.WithContext(ctx).Eval(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0`, []string{key}, value).Err()
}

func (p *Provider) replaceSignature(ctx context.Context, key, old, next string) error {
	updated, err := p.redis.WithContext(ctx).Eval(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
local ttl = redis.call('PTTL', KEYS[1])
if ttl <= 0 then return 0 end
redis.call('PSETEX', KEYS[1], ttl, ARGV[2])
return 1`, []string{key}, old, next).Int64()
	if err != nil {
		return errors.New("cannot update wechat session signature")
	}
	if updated != 1 {
		return ErrSessionChanged
	}
	return nil
}

// requestJSON never includes URLs, request bodies or raw responses in returned errors.
func (p *Provider) requestJSON(ctx context.Context, method, endpoint string, body []byte, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("cannot build wechat request")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("wechat request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("wechat HTTP status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
	if err != nil || len(data) > 64*1024 {
		return errors.New("cannot read wechat response")
	}
	if err := json.Unmarshal(data, result); err != nil {
		return errors.New("invalid wechat response")
	}
	return nil
}

func (p *Provider) accessToken(ctx context.Context) (string, error) {
	token, err := p.redis.WithContext(ctx).Get(p.tokenKey()).Result()
	if err == nil && token != "" {
		return token, nil
	}
	if err != nil && !errors.Is(err, redis.Nil) {
		return "", errors.New("cannot read wechat access token cache")
	}
	body, _ := json.Marshal(map[string]interface{}{
		"grant_type": "client_credential", "appid": p.appID,
		"secret": p.appSecret, "force_refresh": false,
	})
	var result struct {
		Token     string `json:"access_token"`
		ExpiresIn int64  `json:"expires_in"`
		ErrCode   int    `json:"errcode"`
	}
	if err := p.requestJSON(ctx, http.MethodPost, "https://api.weixin.qq.com/cgi-bin/stable_token", body, &result); err != nil {
		return "", err
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("wechat access token error %d", result.ErrCode)
	}
	if result.Token == "" || result.ExpiresIn <= 0 || result.ExpiresIn > 7200 {
		return "", errors.New("invalid wechat access token response")
	}
	// For a short-lived response, use the token once without caching it.
	if result.ExpiresIn > 60 {
		if err := p.redis.WithContext(ctx).Set(p.tokenKey(), result.Token, time.Duration(result.ExpiresIn-60)*time.Second).Err(); err != nil {
			return "", errors.New("cannot cache wechat access token")
		}
	}
	return result.Token, nil
}

// Session checks/rotates the caller's stored identity, never a client-supplied OpenID.
// It returns false only for a missing or invalid session; upstream failures remain errors.
func (p *Provider) Session(ctx context.Context, projectID, identity string, reset bool) (bool, error) {
	if projectID != p.projectID {
		return false, types.ErrInvalidLoginContext
	}
	prefix := ProviderType + ":" + p.appID + ":"
	if !strings.HasPrefix(identity, prefix) || len(identity) <= len(prefix) {
		return false, types.ErrInvalidLoginContext
	}
	if p.redis == nil {
		return false, errors.New("wechat session storage unavailable")
	}
	openID := strings.TrimPrefix(identity, prefix)
	key := p.signatureKey(openID)
	signature, err := p.redis.WithContext(ctx).Get(key).Result()
	if errors.Is(err, redis.Nil) || (err == nil && signature == "") {
		if reset {
			return false, ErrSessionUnavailable
		}
		return false, nil
	}
	if err != nil {
		return false, errors.New("cannot read wechat session signature")
	}
	token, err := p.accessToken(ctx)
	if err != nil {
		return false, err
	}
	path := "/wxa/checksession"
	if reset {
		allowed, err := p.redis.WithContext(ctx).SetNX(key+":reset_cooldown", "1", resetCooldown).Result()
		if err != nil {
			return false, errors.New("cannot limit wechat session reset")
		}
		if !allowed {
			return false, ErrResetTooFrequent
		}
		path = "/wxa/resetusersessionkey"
	}
	query := url.Values{"access_token": {token}, "openid": {openID}, "signature": {signature}, "sig_method": {"hmac_sha256"}}
	var result struct {
		ErrCode    *int   `json:"errcode"`
		OpenID     string `json:"openid"`
		SessionKey string `json:"session_key"`
	}
	err = p.requestJSON(ctx, http.MethodGet, "https://api.weixin.qq.com"+path+"?"+query.Encode(), nil, &result)
	if err != nil || result.ErrCode == nil {
		// A reset may already have happened. Do not replay it or keep its old signature.
		if reset {
			_ = p.discard(context.WithoutCancel(ctx), key, signature)
		}
		if err != nil {
			return false, err
		}
		return false, errors.New("wechat response missing errcode")
	}
	switch *result.ErrCode {
	case 0:
		if reset {
			if result.OpenID != openID || result.SessionKey == "" {
				_ = p.discard(context.WithoutCancel(ctx), key, signature)
				return false, errors.New("invalid wechat reset response")
			}
			if err := p.replaceSignature(ctx, key, signature, sessionSignature(result.SessionKey)); err != nil {
				return false, err
			}
		}
		return true, nil
	case 87007, 87009:
		if err := p.discard(ctx, key, signature); err != nil {
			return false, errors.New("cannot invalidate wechat session")
		}
		if reset {
			return false, ErrSessionUnavailable
		}
		return false, nil
	case 40001, 40014, 42001:
		_ = p.discard(ctx, p.tokenKey(), token)
	}
	return false, fmt.Errorf("wechat session error %d", *result.ErrCode)
}
