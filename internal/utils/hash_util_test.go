package utils

import (
	"testing"
)

func TestCalculateSHA256(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "空字节数组",
			input:    []byte{},
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:     "简单字符串",
			input:    []byte("hello world"),
			expected: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name:     "中文字符",
			input:    []byte("你好世界"),
			expected: "beca6335b20ff57ccc47403ef4d9e0b8fccb4442b3151c2e7d50050673d43172",
		},
		{
			name:     "数字",
			input:    []byte("12345"),
			expected: "5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateSHA256(tt.input)
			if result != tt.expected {
				t.Errorf("CalculateSHA256() = %v, 期望 %v", result, tt.expected)
			}
		})
	}
}
