package utils

import "testing"

func TestGetStandardAspectRatio(t *testing.T) {
	tests := []struct {
		width    int
		height   int
		expected string
		desc     string
	}{
		{512, 512, "1:1", "正方形"},
		{460, 460, "1:1", "WANX SD 1:1"},
		{345, 460, "3:4", "WANX SD 3:4"},
		{460, 345, "4:3", "WANX SD 4:3"},
		{405, 720, "9:16", "WANX SD 9:16"},
		{720, 405, "16:9", "WANX SD 16:9"},
		{348, 512, "3:4", "WANX HD 3:4"},
		{512, 348, "4:3", "WANX HD 4:3"},
		{486, 864, "9:16", "WANX HD 9:16"},
		{864, 486, "16:9", "WANX HD 16:9"},
		{1024, 768, "4:3", "标准4:3"},
		{768, 1024, "3:4", "标准3:4"},
		{1920, 1080, "16:9", "标准16:9"},
		{1080, 1920, "9:16", "标准9:16"},
		{100, 150, "3:4", "小尺寸3:4"},
		{200, 100, "4:3", "小尺寸4:3"},
		{0, 100, "1:1", "无效宽度"},
		{100, 0, "1:1", "无效高度"},
		{300, 400, "3:4", "近似3:4"},
		{400, 300, "4:3", "近似4:3"},
	}

	for _, test := range tests {
		result := GetStandardAspectRatio(test.width, test.height)
		if result != test.expected {
			t.Errorf("%s: GetStandardAspectRatio(%d, %d) = %s, 期望 %s",
				test.desc, test.width, test.height, result, test.expected)
		}
	}
}
