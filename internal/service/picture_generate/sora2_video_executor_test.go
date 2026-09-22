package picture_generate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"va_visionai_server/internal/constants"
)

func TestSora2Executor_Match(t *testing.T) {
	executor := &Sora2Executor{}

	tests := []struct {
		name   string
		params map[string]string
		want   bool
	}{
		{
			name:   "匹配sora2",
			params: map[string]string{"provider": "sora2"},
			want:   true,
		},
		{
			name:   "不匹配其他provider",
			params: map[string]string{"provider": "gemini"},
			want:   false,
		},
		{
			name:   "没有provider参数",
			params: map[string]string{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := executor.Match(tt.params)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSora2Executor_GetName(t *testing.T) {
	executor := &Sora2Executor{}
	assert.Equal(t, constants.SoraExecutorName, executor.GetName())
}

func TestSora2Executor_determineTaskType(t *testing.T) {
	executor := &Sora2Executor{}

	tests := []struct {
		name   string
		params map[string]string
		want   string
	}{
		{
			name:   "文生视频 - 只有prompt",
			params: map[string]string{"prompt": "a beautiful landscape"},
			want:   "text_to_video",
		},
		{
			name:   "图生视频 - 有image参数",
			params: map[string]string{"prompt": "make it move", "image": "http://example.com/image.jpg"},
			want:   "image_to_video",
		},
		{
			name:   "图生视频 - 有LoadImage参数",
			params: map[string]string{"prompt": "animate this", "LoadImage1": "http://example.com/image.jpg"},
			want:   "image_to_video",
		},
		{
			name:   "文生视频 - 空image参数",
			params: map[string]string{"prompt": "create video", "image": ""},
			want:   "text_to_video",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := executor.determineTaskType(tt.params)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSora2Executor_buildSoraParameters(t *testing.T) {
	executor := &Sora2Executor{}
	ctx := context.Background()

	tests := []struct {
		name    string
		params  map[string]string
		wantErr bool
		check   func(t *testing.T, result map[string]interface{})
	}{
		{
			name: "基本文生视频参数",
			params: map[string]string{
				"model":      "sora2",
				"prompt":     "test prompt",
				"duration":   "5",
				"resolution": "1920x1080",
			},
			wantErr: false,
			check: func(t *testing.T, result map[string]interface{}) {
				assert.Equal(t, "sora2", result["model"])
				assert.Equal(t, "test prompt", result["prompt"])
				assert.Equal(t, "5", result["duration"])
				assert.Equal(t, "1920x1080", result["resolution"])
			},
		},
		{
			name: "图生视频参数",
			params: map[string]string{
				"model": "sora2",
				"image": "http://example.com/image.jpg",
			},
			wantErr: false,
			check: func(t *testing.T, result map[string]interface{}) {
				assert.Equal(t, "sora2", result["model"])
				assert.Equal(t, "http://example.com/image.jpg", result["input_image_url"])
			},
		},
		{
			name: "多图输入参数",
			params: map[string]string{
				"model":      "sora2",
				"LoadImage1": "http://example.com/1.jpg",
				"LoadImage2": "http://example.com/2.jpg",
			},
			wantErr: false,
			check: func(t *testing.T, result map[string]interface{}) {
				images := result["image_urls"].([]string)
				assert.Len(t, images, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := executor.buildSoraParameters(ctx, nil, tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.check != nil {
					tt.check(t, got)
				}
			}
		})
	}
}

func TestSora2Executor_IsSubmissionErrorRetryable(t *testing.T) {
	executor := &Sora2Executor{}
	// Sora2不支持重试
	assert.False(t, executor.IsSubmissionErrorRetryable(nil))
	assert.False(t, executor.IsSubmissionErrorRetryable(assert.AnError))
}
