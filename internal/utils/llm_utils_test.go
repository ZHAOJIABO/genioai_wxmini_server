package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractJSONFromLLMResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "正常JSON响应",
			input:    `这是一些前缀文本 {"key": "value"} 这是一些后缀文本`,
			expected: `{"key": "value"}`,
			hasError: false,
		},
		{
			name:     "多行JSON响应",
			input:    "一些文本\n{\n\"key\": \"value\"\n}\n更多文本",
			expected: `{"key": "value"}`,
			hasError: false,
		},
		{
			name:     "嵌套JSON响应",
			input:    `前缀 {"outer": {"inner": "value"}} 后缀`,
			expected: `{"outer": {"inner": "value"}}`,
			hasError: false,
		},
		{
			name:     "无JSON内容",
			input:    "这是一个没有JSON的响应",
			expected: "",
			hasError: true,
		},
		{
			name:     "不完整的JSON",
			input:    "这是一个不完整的JSON {\"key\": \"value\"",
			expected: "",
			hasError: true,
		},
		{
			name:     "空响应",
			input:    "",
			expected: "",
			hasError: true,
		},
		{
			name:     "只有花括号",
			input:    "{}",
			expected: "{}",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractJSONFromLLMResponse(tt.input)
			if tt.hasError {
				require.Error(t, err)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestRemoveTrailingCommaInArray(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "简单数组末尾逗号",
			input:    `["1","2",]`,
			expected: `["1","2"]`,
		},
		{
			name:     "嵌套数组末尾逗号",
			input:    `[["3","4"],["5","6"],["1","2",]]`,
			expected: `[["3","4"],["5","6"],["1","2"]]`,
		},
		{
			name:     "带空格的数组",
			input:    `["1" , "2" , ]`,
			expected: `["1" , "2"]`,
		},
		{
			name:     "多行格式数组",
			input:    "[\n  \"1\",\n  \"2\",\n]",
			expected: "[\n  \"1\",\n  \"2\"\n]",
		},
		{
			name:     "字符串中包含方括号和逗号",
			input:    `["a[1,2]", "b[3,4]",]`,
			expected: `["a[1,2]", "b[3,4]"]`,
		},
		{
			name:     "没有末尾逗号的数组",
			input:    `["1","2"]`,
			expected: `["1","2"]`,
		},
		{
			name:     "空数组",
			input:    `[]`,
			expected: `[]`,
		},
		{
			name:     "复杂嵌套数组",
			input:    `[["a","b"],["c","d",],["e","f"],]`,
			expected: `[["a","b"],["c","d"],["e","f"]]`,
		},
		{
			name:     "转义引号的数组",
			input:    `["\"quoted\"","text",]`,
			expected: `["\"quoted\"","text"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveTrailingCommaInArray(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
