package conf

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestFreeImageGenerationConfig(t *testing.T) {
	for _, tc := range []struct {
		yaml string
		want bool
	}{
		{"AmountConfig:\n  FreeImageGeneration: true\n", true},
		{"AmountConfig:\n  FreeImageGeneration: false\n", false},
		{"AmountConfig: {}\n", false},
	} {
		v := viper.New()
		v.SetConfigType("yaml")
		if err := v.ReadConfig(strings.NewReader(tc.yaml)); err != nil {
			t.Fatal(err)
		}
		var config Config
		if err := v.Unmarshal(&config); err != nil {
			t.Fatal(err)
		}
		if config.AmountConfig.FreeImageGeneration != tc.want {
			t.Fatalf("unexpected free-image setting for %q", tc.yaml)
		}
	}
}
