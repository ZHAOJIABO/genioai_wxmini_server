package credit

import (
	"testing"

	"va_visionai_server/conf"
)

func TestImageGenerationCost(t *testing.T) {
	original := conf.GlobalConfig.AmountConfig.FreeImageGeneration
	t.Cleanup(func() { conf.GlobalConfig.AmountConfig.FreeImageGeneration = original })
	for _, tc := range []struct {
		name                string
		free, image, member bool
		amount, want        int
	}{
		{"ordinary image with surcharges", true, true, false, 47, 0},
		{"member image", true, true, true, 47, 0},
		{"ordinary video remains paid", true, false, false, 90, 90},
		{"member video remains paid", true, false, true, 90, 90},
		{"disabled ordinary image", false, true, false, 47, 47},
		{"disabled preserves member waiver", false, true, true, 47, 0},
		{"historical paid image retry", true, true, false, 12, 0},
		{"disabled retry retains stored price", false, true, false, 12, 12},
		{"disabled retry retains previously free price", false, true, false, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conf.GlobalConfig.AmountConfig.FreeImageGeneration = tc.free
			if got := ImageGenerationCost(tc.amount, tc.image, tc.member); got != tc.want {
				t.Fatalf("cost = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestFreeImageChainRetainsVideoCost(t *testing.T) {
	original := conf.GlobalConfig.AmountConfig.FreeImageGeneration
	t.Cleanup(func() { conf.GlobalConfig.AmountConfig.FreeImageGeneration = original })
	conf.GlobalConfig.AmountConfig.FreeImageGeneration = true
	imageCost := ImageGenerationCost(15, true, false)
	videoCost := ImageGenerationCost(40, false, false)
	if imageCost != 0 || imageCost+videoCost != 40 {
		t.Fatalf("unexpected chain pricing: image=%d video=%d", imageCost, videoCost)
	}
}
