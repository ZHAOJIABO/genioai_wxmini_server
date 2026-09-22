package conf

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestWeChatConfigPreservesProjectKeys(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(`WeChatMiniPrograms:
  - ProjectID: "com.example.mini"
    AppID: "wxExample"
    AppSecret: "test-secret"
`)); err != nil {
		t.Fatal(err)
	}
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		t.Fatal(err)
	}
	if len(config.WeChatMiniPrograms) != 1 {
		t.Fatal("missing WeChat configuration")
	}
	credentials := config.WeChatMiniPrograms[0]
	if credentials.ProjectID != "com.example.mini" || credentials.AppID != "wxExample" || credentials.AppSecret != "test-secret" {
		t.Fatal("WeChat credentials were not decoded under the exact project key")
	}
}
