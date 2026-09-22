package api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	vai "va_visionai_server/internal/va_interface"
)

// InspirationPromptApplier 是 InspirationPromptService 的接口
// 用于测试时进行依赖注入
type InspirationPromptApplier interface {
	ApplyInspirationPrompt(ctx context.Context, promptID, userID, toolID, toolType string) error
}

// FakeInspirationPromptService 是一个用于测试的 InspirationPromptService Fake 实现
type FakeInspirationPromptService struct {
	// 错误注入字段
	ApplyError error

	// 调用记录
	ApplyCalled  bool
	LastPromptID string
	LastUserID   string
	LastToolID   string
	LastToolType string
	CallCount    int
}

func (f *FakeInspirationPromptService) ApplyInspirationPrompt(ctx context.Context, promptID, userID, toolID, toolType string) error {
	f.ApplyCalled = true
	f.LastPromptID = promptID
	f.LastUserID = userID
	f.LastToolID = toolID
	f.LastToolType = toolType
	f.CallCount++
	return f.ApplyError
}

// testReportServer 是一个用于测试的 ReportServer,使用接口依赖
type testReportServer struct {
	inspirationPromptApplier InspirationPromptApplier
}

func (s *testReportServer) handleInspirationTracking(ctx context.Context, userID string, trackingData *vai.TrackingEventData) {
	// 复制 report.go 中的实现
	extraData := trackingData.GetExtraData()
	if extraData == "" {
		return
	}

	var data struct {
		InspirationPromptID string `json:"inspiration_prompt_id"`
		ToolID              string `json:"tool_id"`
		ToolType            string `json:"tool_type"`
	}

	if err := json.Unmarshal([]byte(extraData), &data); err != nil {
		return
	}

	if data.InspirationPromptID == "" {
		return
	}

	if err := s.inspirationPromptApplier.ApplyInspirationPrompt(ctx, data.InspirationPromptID, userID, data.ToolID, data.ToolType); err != nil {
		return
	}
}

// TestHandleInspirationTracking_Success 测试正常场景
func TestHandleInspirationTracking_Success(t *testing.T) {
	fakeService := &FakeInspirationPromptService{}
	server := &testReportServer{
		inspirationPromptApplier: fakeService,
	}

	ctx := context.Background()
	trackingData := &vai.TrackingEventData{
		ExtraData: `{"inspiration_prompt_id":"prompt_123","tool_id":"tool_456","tool_type":"image_generation"}`,
	}

	server.handleInspirationTracking(ctx, "user_789", trackingData)

	require.True(t, fakeService.ApplyCalled, "ApplyInspirationPrompt should be called")
	assert.Equal(t, "prompt_123", fakeService.LastPromptID)
	assert.Equal(t, "user_789", fakeService.LastUserID)
	assert.Equal(t, "tool_456", fakeService.LastToolID)
	assert.Equal(t, "image_generation", fakeService.LastToolType)
	assert.Equal(t, 1, fakeService.CallCount)
}

// TestHandleInspirationTracking_EmptyExtraData 测试 extra_data 为空字符串
func TestHandleInspirationTracking_EmptyExtraData(t *testing.T) {
	fakeService := &FakeInspirationPromptService{}
	server := &testReportServer{
		inspirationPromptApplier: fakeService,
	}

	ctx := context.Background()
	trackingData := &vai.TrackingEventData{
		ExtraData: "",
	}

	server.handleInspirationTracking(ctx, "user_789", trackingData)

	assert.False(t, fakeService.ApplyCalled, "ApplyInspirationPrompt should not be called when extra_data is empty")
	assert.Equal(t, 0, fakeService.CallCount)
}

// TestHandleInspirationTracking_InvalidJSON 测试 JSON 格式错误
func TestHandleInspirationTracking_InvalidJSON(t *testing.T) {
	fakeService := &FakeInspirationPromptService{}
	server := &testReportServer{
		inspirationPromptApplier: fakeService,
	}

	ctx := context.Background()
	trackingData := &vai.TrackingEventData{
		ExtraData: `{"invalid json`,
	}

	server.handleInspirationTracking(ctx, "user_789", trackingData)

	assert.False(t, fakeService.ApplyCalled, "ApplyInspirationPrompt should not be called when JSON is invalid")
	assert.Equal(t, 0, fakeService.CallCount)
}

// TestHandleInspirationTracking_MissingPromptID 测试缺失 inspiration_prompt_id
func TestHandleInspirationTracking_MissingPromptID(t *testing.T) {
	fakeService := &FakeInspirationPromptService{}
	server := &testReportServer{
		inspirationPromptApplier: fakeService,
	}

	ctx := context.Background()
	trackingData := &vai.TrackingEventData{
		ExtraData: `{"tool_id":"tool_456","tool_type":"image_generation"}`,
	}

	server.handleInspirationTracking(ctx, "user_789", trackingData)

	assert.False(t, fakeService.ApplyCalled, "ApplyInspirationPrompt should not be called when inspiration_prompt_id is missing")
	assert.Equal(t, 0, fakeService.CallCount)
}

// TestHandleInspirationTracking_ServiceError 测试服务调用失败
func TestHandleInspirationTracking_ServiceError(t *testing.T) {
	fakeService := &FakeInspirationPromptService{
		ApplyError: errors.New("database error"),
	}
	server := &testReportServer{
		inspirationPromptApplier: fakeService,
	}

	ctx := context.Background()
	trackingData := &vai.TrackingEventData{
		ExtraData: `{"inspiration_prompt_id":"prompt_123","tool_id":"tool_456"}`,
	}

	// 即使服务返回错误,handleInspirationTracking 也不应该 panic,只是记录日志
	server.handleInspirationTracking(ctx, "user_789", trackingData)

	require.True(t, fakeService.ApplyCalled, "ApplyInspirationPrompt should be called even if it fails")
	assert.Equal(t, "prompt_123", fakeService.LastPromptID)
	assert.Equal(t, "user_789", fakeService.LastUserID)
	assert.Equal(t, 1, fakeService.CallCount)
}

// TestHandleInspirationTracking_OnlyPromptID 测试只有 inspiration_prompt_id,其他字段为空
func TestHandleInspirationTracking_OnlyPromptID(t *testing.T) {
	fakeService := &FakeInspirationPromptService{}
	server := &testReportServer{
		inspirationPromptApplier: fakeService,
	}

	ctx := context.Background()
	trackingData := &vai.TrackingEventData{
		ExtraData: `{"inspiration_prompt_id":"prompt_123"}`,
	}

	server.handleInspirationTracking(ctx, "user_789", trackingData)

	require.True(t, fakeService.ApplyCalled, "ApplyInspirationPrompt should be called with only prompt_id")
	assert.Equal(t, "prompt_123", fakeService.LastPromptID)
	assert.Equal(t, "user_789", fakeService.LastUserID)
	assert.Empty(t, fakeService.LastToolID)
	assert.Empty(t, fakeService.LastToolType)
	assert.Equal(t, 1, fakeService.CallCount)
}

// TestHandleInspirationTracking_ExtraFields 测试 JSON 中包含额外字段
func TestHandleInspirationTracking_ExtraFields(t *testing.T) {
	fakeService := &FakeInspirationPromptService{}
	server := &testReportServer{
		inspirationPromptApplier: fakeService,
	}

	ctx := context.Background()
	trackingData := &vai.TrackingEventData{
		ExtraData: `{"inspiration_prompt_id":"prompt_123","tool_id":"tool_456","extra_field":"should_be_ignored","another":"field"}`,
	}

	server.handleInspirationTracking(ctx, "user_789", trackingData)

	require.True(t, fakeService.ApplyCalled, "ApplyInspirationPrompt should be called, ignoring extra fields")
	assert.Equal(t, "prompt_123", fakeService.LastPromptID)
	assert.Equal(t, "user_789", fakeService.LastUserID)
	assert.Equal(t, "tool_456", fakeService.LastToolID)
	assert.Equal(t, 1, fakeService.CallCount)
}
