package auth

import (
	"testing"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/auth/providers/wechat"
)

func TestValidWeChatCaller(t *testing.T) {
	base := model.UserRecord{ProjectID: "project", UserId: "user", LoginType: wechat.ProviderType, AccessToken: "token", AccessTokenExpired: 200}
	if !validWeChatCaller(&base, "project", "user", "token", 100) {
		t.Fatal("valid caller rejected")
	}
	for _, tc := range []struct {
		project, user, token string
		now                  int64
	}{
		{"other", "user", "token", 100}, {"project", "other", "token", 100},
		{"project", "user", "wrong", 100}, {"project", "user", "", 100}, {"project", "user", "token", 200},
	} {
		if validWeChatCaller(&base, tc.project, tc.user, tc.token, tc.now) {
			t.Fatal("invalid caller accepted")
		}
	}
	for _, mutate := range []func(*model.UserRecord){
		func(u *model.UserRecord) { u.Status = 1 },
		func(u *model.UserRecord) { u.LoginType = "temp" },
		func(u *model.UserRecord) { u.LoginType = "google" },
		func(u *model.UserRecord) { u.AccessToken = "" },
	} {
		user := base
		mutate(&user)
		if validWeChatCaller(&user, "project", "user", "token", 100) {
			t.Fatal("unauthorized caller accepted")
		}
	}
}
