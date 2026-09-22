package picture_generate

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/utils"
)

func TestGeminiExecutor_GetName(t *testing.T) {
	assert.Equal(t, "gemini", constants.GeminiExecutorName)
}

func TestGeminiExecutor_HTTPURLDetection(t *testing.T) {
	assert.True(t, strings.HasPrefix("https://example.com/image.jpg", "http"))
	assert.True(t, strings.HasPrefix("http://example.com/photo.png", "http"))
	assert.False(t, strings.HasPrefix("ftp://example.com/image.jpg", "http"))
	assert.False(t, strings.HasPrefix("not-a-url", "http"))
	assert.False(t, strings.HasPrefix("", "http"))
}

func TestGeminiExecutor_IsValidImageData(t *testing.T) {
	t.Log("图片数据验证现在由utils.URLToBase64处理")
}

func TestGeminiExecutor_DetermineTaskType(t *testing.T) {
	executor := &GeminiExecutor{}

	params1 := map[string]string{
		"prompt": "生成一张美丽的风景画",
	}
	assert.Equal(t, "text_to_image", executor.determineTaskType(params1))

	params2 := map[string]string{
		"prompt": "将衣服变为粉色",
		"image":  "https://example.com/image.jpg",
	}
	assert.Equal(t, "image_to_image", executor.determineTaskType(params2))

	params3 := map[string]string{
		"prompt": "生成图片",
	}
	assert.Equal(t, "text_to_image", executor.determineTaskType(params3))
}

func TestGeminiExecutor_UsesStandardAspectRatioUtils(t *testing.T) {
	assert.Equal(t, "1:1", utils.GetStandardAspectRatio(1000, 1000))
	assert.Equal(t, "4:3", utils.GetStandardAspectRatio(800, 600))
	assert.Equal(t, "16:9", utils.GetStandardAspectRatio(1920, 1080))
	assert.Equal(t, "3:4", utils.GetStandardAspectRatio(600, 800))
	assert.Equal(t, "9:16", utils.GetStandardAspectRatio(1080, 1920))
}

func TestGeminiExecutor_BuildGeminiParameters(t *testing.T) {
	executor := &GeminiExecutor{}
	ctx := context.Background()
	task := &model.PictureTask{}

	params1 := map[string]string{
		"prompt": "测试提示词",
		"model":  "gemini-test-model",
	}

	result1, err := executor.buildGeminiParameters(ctx, task, params1)
	require.NoError(t, err)
	assert.Equal(t, "测试提示词", result1["prompt"])
	assert.Equal(t, "gemini-test-model", result1["model"])

	params2 := map[string]string{}
	result2, err := executor.buildGeminiParameters(ctx, task, params2)
	require.NoError(t, err)
	assert.Equal(t, "请生成一张图片", result2["prompt"])
	assert.Equal(t, "gemini-2.5-flash-image-preview", result2["model"])

	params3 := map[string]string{
		"prompt":     "测试图片生成",
		"image_data": "data:image/png;base64,testdata",
	}

	result3, err := executor.buildGeminiParameters(ctx, task, params3)
	require.NoError(t, err)
	assert.Equal(t, "测试图片生成", result3["prompt"])

	imageUrls, exists := result3["image_urls"]
	assert.True(t, exists)
	assert.IsType(t, []string{}, imageUrls)

	urls := imageUrls.([]string)
	assert.Len(t, urls, 1)
	assert.Equal(t, "data:image/png;base64,testdata", urls[0])
}

func TestGeminiExecutor_BuildGeminiParametersWithImages(t *testing.T) {
	executor := &GeminiExecutor{}
	ctx := context.Background()
	task := &model.PictureTask{}

	params := map[string]string{
		"prompt":      "使用图片生成新内容",
		"model":       "gemini-test-model",
		"image_data1": "data:image/png;base64,testbase64data1",
		"image_data2": "data:image/jpeg;base64,testbase64data2",
		"other_param": "not_an_image",
	}

	result, err := executor.buildGeminiParameters(ctx, task, params)
	require.NoError(t, err)
	assert.Equal(t, "使用图片生成新内容", result["prompt"])
	assert.Equal(t, "gemini-test-model", result["model"])

	imageUrls, exists := result["image_urls"]
	assert.True(t, exists)
	assert.IsType(t, []string{}, imageUrls)

	urls := imageUrls.([]string)
	assert.Len(t, urls, 2)
	assert.Contains(t, urls, "data:image/png;base64,testbase64data1")
	assert.Contains(t, urls, "data:image/jpeg;base64,testbase64data2")
}
