package wechat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-redis/redis"
	"va_visionai_server/internal/service/auth/types"
)

func sessionProvider(t *testing.T) *Provider {
	t.Helper()
	p := newTestProvider(t)
	if err := p.Initialize(context.Background(), types.ProviderConfig{ProjectID: "project", ClientID: "wx-app", ClientSecret: "secret-app"}); err != nil {
		t.Fatal(err)
	}
	if err := p.redis.Set(p.signatureKey("user"), sessionSignature("old-session-key"), time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	return p
}

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestSessionCheckAndReset(t *testing.T) {
	p := sessionProvider(t)
	ctx := context.Background()
	tokenCalls, checkCalls, resetCalls := 0, 0, 0
	p.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/cgi-bin/stable_token" {
			tokenCalls++
			var body map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if req.Method != http.MethodPost || body["appid"] != "wx-app" || body["secret"] != "secret-app" || body["grant_type"] != "client_credential" || body["force_refresh"] != false {
				t.Fatal("wrong token request")
			}
			return jsonResponse(`{"access_token":"platform-token","expires_in":7200}`), nil
		}
		q := req.URL.Query()
		expected := sessionSignature("old-session-key")
		if resetCalls > 0 {
			expected = sessionSignature("new-session-key")
		}
		if req.Method != http.MethodGet || q.Get("openid") != "user" || q.Get("signature") != expected || q.Get("sig_method") != "hmac_sha256" || q.Get("access_token") != "platform-token" {
			t.Fatal("wrong session signature or parameters")
		}
		if req.URL.Path == "/wxa/checksession" {
			checkCalls++
			return jsonResponse(`{"errcode":0}`), nil
		}
		if req.URL.Path != "/wxa/resetusersessionkey" {
			t.Fatalf("unexpected path %s", req.URL.Path)
		}
		resetCalls++
		return jsonResponse(`{"errcode":0,"openid":"user","session_key":"new-session-key"}`), nil
	})
	identity := ProviderType + ":wx-app:user"
	valid, err := p.Session(ctx, "project", identity, false)
	if err != nil || !valid {
		t.Fatalf("check failed: %v", err)
	}
	if ttl := p.redis.TTL(p.tokenKey()).Val(); ttl < 7138*time.Second || ttl > 7140*time.Second {
		t.Fatalf("unexpected token TTL: %v", ttl)
	}
	before := p.redis.PTTL(p.signatureKey("user")).Val()
	valid, err = p.Session(ctx, "project", identity, true)
	if err != nil || !valid {
		t.Fatalf("reset failed: %v", err)
	}
	after := p.redis.PTTL(p.signatureKey("user")).Val()
	if after > before || after < before-time.Second {
		t.Fatalf("reset extended/lost TTL: %v -> %v", before, after)
	}
	if got := p.redis.Get(p.signatureKey("user")).Val(); got != sessionSignature("new-session-key") {
		t.Fatal("new signature not saved")
	}
	valid, err = p.Session(ctx, "project", identity, false)
	if err != nil || !valid {
		t.Fatalf("check after reset failed: %v", err)
	}
	if _, err := p.Session(ctx, "project", identity, true); !errors.Is(err, ErrResetTooFrequent) {
		t.Fatalf("missing reset cooldown: %v", err)
	}
	if tokenCalls != 1 || checkCalls != 2 || resetCalls != 1 {
		t.Fatal("unexpected retries or cache misses")
	}
}

func TestSessionErrors(t *testing.T) {
	for _, tc := range []struct {
		name, response                       string
		reset, wantError, removed, dropToken bool
	}{
		{"expired", `{"errcode":87007}`, false, false, true, false},
		{"invalid signature", `{"errcode":87009}`, false, false, true, false},
		{"reset expired", `{"errcode":87007}`, true, true, true, false},
		{"busy", `{"errcode":-1}`, false, true, false, false},
		{"rate limit", `{"errcode":45011}`, true, true, false, false},
		{"expired token", `{"errcode":42001}`, false, true, false, true},
		{"malformed reset", `{`, true, true, true, false},
		{"missing errcode", `{}`, false, true, false, false},
		{"wrong reset user", `{"errcode":0,"openid":"other","session_key":"secret"}`, true, true, true, false},
		{"missing reset key", `{"errcode":0,"openid":"user"}`, true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := sessionProvider(t)
			p.redis.Set(p.tokenKey(), "platform-token", time.Hour)
			calls := 0
			p.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return jsonResponse(tc.response), nil })
			valid, err := p.Session(context.Background(), "project", ProviderType+":wx-app:user", tc.reset)
			if valid || (err != nil) != tc.wantError {
				t.Fatalf("valid=%v err=%v", valid, err)
			}
			if calls != 1 {
				t.Fatal("operation retried")
			}
			if (p.redis.Exists(p.signatureKey("user")).Val() == 0) != tc.removed {
				t.Fatal("wrong signature invalidation")
			}
			if (p.redis.Exists(p.tokenKey()).Val() == 0) != tc.dropToken {
				t.Fatal("wrong token invalidation")
			}
		})
	}
}

func TestSessionRejectsOtherIdentityAndMissingSession(t *testing.T) {
	p := sessionProvider(t)
	p.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network call"); return nil, nil })
	for _, pair := range [][2]string{{"other", ProviderType + ":wx-app:user"}, {"project", ProviderType + ":other-app:user"}, {"project", "user"}} {
		if _, err := p.Session(context.Background(), pair[0], pair[1], true); !errors.Is(err, types.ErrInvalidLoginContext) {
			t.Fatal("identity mismatch accepted")
		}
	}
	valid, err := p.Session(context.Background(), "project", ProviderType+":wx-app:missing", false)
	if valid || err != nil {
		t.Fatal("missing cache should require re-login")
	}
	if _, err := p.Session(context.Background(), "project", ProviderType+":wx-app:missing", true); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatal("missing session reset accepted")
	}
}

func TestResetNeverOverwritesConcurrentLogin(t *testing.T) {
	for _, response := range []string{`{"errcode":0,"openid":"user","session_key":"reset-key"}`, `{"errcode":87009}`} {
		p := sessionProvider(t)
		p.redis.Set(p.tokenKey(), "platform-token", time.Hour)
		p.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			p.redis.Set(p.signatureKey("user"), "newer-login-signature", time.Hour)
			return jsonResponse(response), nil
		})
		_, _ = p.Session(context.Background(), "project", ProviderType+":wx-app:user", true)
		if p.redis.Get(p.signatureKey("user")).Val() != "newer-login-signature" {
			t.Fatal("new login was overwritten or deleted")
		}
	}
}

func TestResetUncertainFailureInvalidatesWithoutLeaking(t *testing.T) {
	p := sessionProvider(t)
	p.redis.Set(p.tokenKey(), "platform-secret", time.Hour)
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New(r.URL.String()) })
	_, err := p.Session(context.Background(), "project", ProviderType+":wx-app:user", true)
	if err == nil || strings.Contains(err.Error(), "platform-secret") || strings.Contains(err.Error(), "signature=") {
		t.Fatal("unsafe transport error")
	}
	if !errors.Is(p.redis.Get(p.signatureKey("user")).Err(), redis.Nil) {
		t.Fatal("uncertain reset retained old signature")
	}
}

func TestSessionSignatureVector(t *testing.T) {
	// HMAC-SHA256(key="key", message=""), independently fixed expected value.
	if got := sessionSignature("key"); got != "5d5d139563c95b5967b9bd9a8c9b233a9dedb45072794cd232dc1b74832607d0" {
		t.Fatalf("wrong signature: %s", got)
	}
}
