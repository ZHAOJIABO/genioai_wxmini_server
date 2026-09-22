package utils

import (
	"testing"
	"time"
)

func TestParseTimeString(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSuccess bool
		wantTime    time.Time
	}{
		{
			name:        "有效的DateOnly格式",
			input:       "2023-01-15",
			wantSuccess: true,
			wantTime:    time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "有效的DateTime格式",
			input:       "2023-01-15 14:30:45",
			wantSuccess: true,
			wantTime:    time.Date(2023, 1, 15, 14, 30, 45, 0, time.UTC),
		},
		{
			name:        "无效的时间格式",
			input:       "2023/01/15",
			wantSuccess: false,
		},
		{
			name:        "空字符串",
			input:       "",
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimeString(tt.input)
			if tt.wantSuccess {
				if err != nil {
					t.Errorf("期望成功但得到错误: %v", err)
					return
				}
				if !got.Equal(tt.wantTime) {
					t.Errorf("期望时间 %v，得到 %v", tt.wantTime, got)
				}
			} else {
				if err == nil {
					t.Errorf("期望错误但解析成功: %v", got)
				}
			}
		})
	}
}
