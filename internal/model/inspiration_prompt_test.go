package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestInspirationPrompt_IsEnabled 测试 IsEnabled 方法
func TestInspirationPrompt_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		status   int8
		expected bool
	}{
		{
			name:     "状态为1时返回true",
			status:   1,
			expected: true,
		},
		{
			name:     "状态为0时返回false",
			status:   0,
			expected: false,
		},
		{
			name:     "状态为负数时返回false",
			status:   -1,
			expected: false,
		},
		{
			name:     "状态为大于1时返回false",
			status:   2,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := &InspirationPrompt{
				Status: tt.status,
			}
			result := prompt.IsEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}
