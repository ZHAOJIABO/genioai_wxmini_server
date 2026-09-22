package wechat

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis"

	"va_visionai_server/internal/service/auth/types"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLogin(t *testing.T) {
	for _, tc := range []struct {
		name, body       string
		status           int
		invalid, success bool
	}{
		{"success without unionid", `{"openid":"user-one","session_key":"secret-session"}`, 200, false, true},
		{"success with unionid", `{"openid":"user-one","session_key":"secret-session","unionid":"union","errcode":0}`, 200, false, true},
		{"invalid code", `{"errcode":40029,"errmsg":"secret-code"}`, 200, true, false},
		{"used code", `{"errcode":40163}`, 200, true, false},
		{"risk rejected", `{"errcode":40226}`, 200, true, false},
		{"rate limited", `{"errcode":45011}`, 200, false, false},
		{"upstream failure", `{"errcode":-1}`, 200, false, false},
		{"missing openid", `{"session_key":"secret-session"}`, 200, false, false},
		{"missing session key", `{"openid":"user-one"}`, 200, false, false},
		{"malformed", `{`, 200, false, false},
		{"http failure", `{"openid":"user-one"}`, 503, false, false},
		{"oversized", strings.Repeat(" ", 65537), 200, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestProvider(t)
			if err := p.Initialize(context.Background(), types.ProviderConfig{ProjectID: "project", ClientID: "wx-app", ClientSecret: "secret-app"}); err != nil {
				t.Fatal(err)
			}
			calls := 0
			p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				q := r.URL.Query()
				if r.Method != "GET" || r.URL.Host != "api.weixin.qq.com" || r.URL.Path != "/sns/jscode2session" || q.Get("appid") != "wx-app" || q.Get("secret") != "secret-app" || q.Get("js_code") != "code+&=" || q.Get("grant_type") != "authorization_code" {
					t.Fatalf("incorrect request")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})
			info, err := p.Login(context.Background(), &types.WeChatLoginParams{BaseLoginParams: &types.BaseLoginParams{ProjectID: "project"}, Code: "code+&=", AuthType: "miniprogram"})
			if calls != 1 {
				t.Fatalf("code exchange attempted %d times", calls)
			}
			if tc.success {
				if err != nil {
					t.Fatal(err)
				}
				if info.ProviderUserID != "wechat_miniprogram:wx-app:user-one" || info.ExtraData != nil {
					t.Fatalf("unexpected identity: %+v", info)
				}
				if p.redis.Get(p.signatureKey("user-one")).Val() != sessionSignature("secret-session") {
					t.Fatal("login did not retain the session signature")
				}
			} else {
				if err == nil || info != nil {
					t.Fatal("expected failure")
				}
				if errors.Is(err, types.ErrInvalidCredentials) != tc.invalid {
					t.Fatalf("wrong error classification: %v", err)
				}
				if strings.Contains(err.Error(), "secret-") {
					t.Fatal("secret leaked")
				}
			}
		})
	}
}

func TestLoginRejectsInvalidInputWithoutNetwork(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Initialize(context.Background(), types.ProviderConfig{ProjectID: "project", ClientID: "wx-app", ClientSecret: "secret-app"}); err != nil {
		t.Fatal(err)
	}
	p.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network call"); return nil, nil })
	for _, input := range []*types.WeChatLoginParams{
		nil,
		{Code: "code"},
		{BaseLoginParams: &types.BaseLoginParams{ProjectID: "project"}, Code: " "},
		{BaseLoginParams: &types.BaseLoginParams{ProjectID: "project"}, Code: "code", AuthType: "app"},
		{BaseLoginParams: &types.BaseLoginParams{ProjectID: "other"}, Code: "code"},
	} {
		if _, err := p.Login(context.Background(), input); err == nil {
			t.Fatal("expected invalid input")
		}
	}
}

func TestTransportErrorDoesNotLeakCredentials(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Initialize(context.Background(), types.ProviderConfig{ProjectID: "project", ClientID: "wx-app", ClientSecret: "secret-app"}); err != nil {
		t.Fatal(err)
	}
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New(r.URL.String()) })
	_, err := p.Login(context.Background(), &types.WeChatLoginParams{BaseLoginParams: &types.BaseLoginParams{ProjectID: "project"}, Code: "secret-code"})
	if err == nil || strings.Contains(err.Error(), "secret-") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestRedirectIsRejected(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Initialize(context.Background(), types.ProviderConfig{ProjectID: "project", ClientID: "wx-app", ClientSecret: "secret-app"}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://example.com/"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	_, err := p.Login(context.Background(), &types.WeChatLoginParams{BaseLoginParams: &types.BaseLoginParams{ProjectID: "project"}, Code: "code"})
	if err == nil || calls != 1 {
		t.Fatal("redirect was not rejected")
	}
}

func TestMissingCredentialsAreRejected(t *testing.T) {
	for _, config := range []types.ProviderConfig{
		{},
		{ProjectID: "default", ClientID: "wx-app", ClientSecret: "secret"},
		{ProjectID: "project", ClientID: "wx-app"},
		{ProjectID: "project", ClientSecret: "secret"},
	} {
		if err := newTestProvider(t).Initialize(context.Background(), config); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}

func TestContextCancellation(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Initialize(context.Background(), types.ProviderConfig{ProjectID: "project", ClientID: "wx-app", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.Login(ctx, &types.WeChatLoginParams{BaseLoginParams: &types.BaseLoginParams{ProjectID: "project"}, Code: "code"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func newTestProvider(t *testing.T) *Provider {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewProvider(client)
}
