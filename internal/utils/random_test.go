package utils

import (
	"fmt"
	"strconv"
	"testing"
)

func TestGenerateRandomNumber(t *testing.T) {
	tests := []struct {
		name   string
		length int
		want   int // 期望的位数
	}{
		{
			name:   "generate 4 digits number",
			length: 4,
			want:   4,
		},
		{
			name:   "generate 16 digits number",
			length: 16,
			want:   16,
		},
		{
			name:   "generate 1 digit number",
			length: 1,
			want:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateRandomNumber(tt.length)

			// 打印生成的随机数
			fmt.Printf("Generated %d-digit random number: %d\n", tt.length, got)

			// 转换为字符串检查长度
			gotStr := strconv.FormatInt(got, 10)
			if len(gotStr) != tt.want {
				t.Errorf("GenerateRandomNumber() got length = %v, want length %v", len(gotStr), tt.want)
			}

			// 检查生成的数字是否在合理范围内
			min := int64(1)
			max := int64(9)
			for i := 1; i < tt.length; i++ {
				min *= 10
				max = max*10 + 9
			}

			if got < min || got > max {
				t.Errorf("GenerateRandomNumber() = %v, want between %v and %v", got, min, max)
			}
		})
	}
}
