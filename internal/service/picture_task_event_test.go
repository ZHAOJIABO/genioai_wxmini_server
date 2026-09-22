package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"va_visionai_server/internal/constants"
	vai "va_visionai_server/internal/va_interface"
)

// TestExtractUserInputFromWorkflowInputs 测试 extractUserInputFromWorkflowInputs 辅助函数
func TestExtractUserInputFromWorkflowInputs(t *testing.T) {
	tests := []struct {
		name             string
		inputs           []*vai.WorkflowInput
		expectedPrompt   string
		expectedImgCount int
		expectedFirstImg string
	}{
		{
			name:             "空输入",
			inputs:           nil,
			expectedPrompt:   "",
			expectedImgCount: 0,
		},
		{
			name:             "空数组",
			inputs:           []*vai.WorkflowInput{},
			expectedPrompt:   "",
			expectedImgCount: 0,
		},
		{
			name: "只有文本输入",
			inputs: []*vai.WorkflowInput{
				{
					InputType:    vai.MessageType_MT_TEXT,
					InputContent: "这是一个测试提示词",
				},
			},
			expectedPrompt:   "这是一个测试提示词",
			expectedImgCount: 0,
		},
		{
			name: "只有图片输入",
			inputs: []*vai.WorkflowInput{
				{
					InputType:    vai.MessageType_MT_IMAGE,
					InputContent: "https://example.com/image1.jpg",
				},
			},
			expectedPrompt:   "",
			expectedImgCount: 1,
			expectedFirstImg: "https://example.com/image1.jpg",
		},
		{
			name: "混合输入_文本在前",
			inputs: []*vai.WorkflowInput{
				{
					InputType:    vai.MessageType_MT_TEXT,
					InputContent: "生成一张可爱的猫咪图片",
				},
				{
					InputType:    vai.MessageType_MT_IMAGE,
					InputContent: "https://example.com/ref_image.jpg",
				},
			},
			expectedPrompt:   "生成一张可爱的猫咪图片",
			expectedImgCount: 1,
			expectedFirstImg: "https://example.com/ref_image.jpg",
		},
		{
			name: "混合输入_图片在前",
			inputs: []*vai.WorkflowInput{
				{
					InputType:    vai.MessageType_MT_IMAGE,
					InputContent: "https://example.com/first_image.jpg",
				},
				{
					InputType:    vai.MessageType_MT_TEXT,
					InputContent: "参考这张图片生成",
				},
				{
					InputType:    vai.MessageType_MT_IMAGE,
					InputContent: "https://example.com/second_image.jpg",
				},
			},
			expectedPrompt:   "参考这张图片生成",
			expectedImgCount: 2,
			expectedFirstImg: "https://example.com/first_image.jpg",
		},
		{
			name: "多个文本输入_只取第一个",
			inputs: []*vai.WorkflowInput{
				{
					InputType:    vai.MessageType_MT_TEXT,
					InputContent: "第一个提示词",
				},
				{
					InputType:    vai.MessageType_MT_TEXT,
					InputContent: "第二个提示词",
				},
			},
			expectedPrompt:   "第一个提示词",
			expectedImgCount: 0,
		},
		{
			name: "空内容被忽略",
			inputs: []*vai.WorkflowInput{
				{
					InputType:    vai.MessageType_MT_TEXT,
					InputContent: "",
				},
				{
					InputType:    vai.MessageType_MT_TEXT,
					InputContent: "非空提示词",
				},
				{
					InputType:    vai.MessageType_MT_IMAGE,
					InputContent: "",
				},
				{
					InputType:    vai.MessageType_MT_IMAGE,
					InputContent: "https://example.com/valid.jpg",
				},
			},
			expectedPrompt:   "非空提示词",
			expectedImgCount: 1,
			expectedFirstImg: "https://example.com/valid.jpg",
		},
		{
			name: "长文本被截断_500字符",
			inputs: []*vai.WorkflowInput{
				{
					InputType: vai.MessageType_MT_TEXT,
					// 生成一个超过500字符的字符串
					InputContent: string(make([]byte, 600)),
				},
			},
			expectedPrompt:   string(make([]byte, 500)),
			expectedImgCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userPrompt, inputImages := extractUserInputFromWorkflowInputs(tt.inputs)

			assert.Equal(t, tt.expectedPrompt, userPrompt, "userPrompt mismatch")
			assert.Len(t, inputImages, tt.expectedImgCount, "inputImages count mismatch")

			if tt.expectedImgCount > 0 && tt.expectedFirstImg != "" {
				require.NotEmpty(t, inputImages, "inputImages should not be empty")
				assert.Equal(t, tt.expectedFirstImg, inputImages[0], "first image URL mismatch")
			}
		})
	}
}

// TestExtractUserInputFromWorkflowInputs_LongTextTruncation 专门测试长文本截断
func TestExtractUserInputFromWorkflowInputs_LongTextTruncation(t *testing.T) {
	// 创建一个600字符长度的字符串
	longText := make([]byte, 600)
	for i := range longText {
		longText[i] = 'a'
	}

	inputs := []*vai.WorkflowInput{
		{
			InputType:    vai.MessageType_MT_TEXT,
			InputContent: string(longText),
		},
	}

	userPrompt, _ := extractUserInputFromWorkflowInputs(inputs)

	assert.Len(t, userPrompt, 500, "prompt should be truncated to 500 characters")
	assert.Equal(t, string(longText[:500]), userPrompt, "truncated content should match first 500 chars")
}

// TestContextSubmitSource 测试 Context 中 submit_source 的读取
func TestContextSubmitSource(t *testing.T) {
	tests := []struct {
		name           string
		ctxValue       any
		expectedSource string
	}{
		{
			name:           "无值时默认为 normal",
			ctxValue:       nil,
			expectedSource: constants.SubmitSourceNormal,
		},
		{
			name:           "设置为 tool",
			ctxValue:       constants.SubmitSourceTool,
			expectedSource: constants.SubmitSourceTool,
		},
		{
			name:           "设置为 random",
			ctxValue:       constants.SubmitSourceRandom,
			expectedSource: constants.SubmitSourceRandom,
		},
		{
			name:           "设置为 guide",
			ctxValue:       constants.SubmitSourceGuide,
			expectedSource: constants.SubmitSourceGuide,
		},
		{
			name:           "空字符串回退为 normal",
			ctxValue:       "",
			expectedSource: constants.SubmitSourceNormal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.ctxValue != nil {
				ctx = context.WithValue(ctx, constants.CtxTaskSubmitSource, tt.ctxValue)
			}

			// 模拟 trackTaskSubmitEvent 中的逻辑
			submitSource := constants.SubmitSourceNormal
			if src := ctx.Value(constants.CtxTaskSubmitSource); src != nil {
				if srcStr, ok := src.(string); ok && srcStr != "" {
					submitSource = srcStr
				}
			}

			assert.Equal(t, tt.expectedSource, submitSource)
		})
	}
}
