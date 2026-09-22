package event_reporter

import (
	"context"
	"testing"

	"va_visionai_server/internal/constants"
)

// TestEventBuilder_FromContext_Integration 端到端集成测试：验证 FromContext 能正确提取业务层设置的 user_id 和 trace_id
func TestEventBuilder_FromContext_Integration(t *testing.T) {
	// 模拟 gRPC interceptor 设置的真实 context
	ctx := context.Background()
	ctx = context.WithValue(ctx, constants.CtxUserID, "test_user_123")
	ctx = context.WithValue(ctx, constants.CtxTraceID, "test_trace_456")

	// 创建 noop reporter（不需要真实连接 event_sink）
	reporter := &noopReporter{}

	// 使用 EventBuilder 构建事件
	event := reporter.NewEvent(EventTypeTaskSubmit).
		FromContext(ctx).
		Payload(map[string]any{
			"task_id": "task_123",
		}).
		Build()

	// 验证 user_id 和 trace_id 是否正确提取
	if event.GetUserId() != "test_user_123" {
		t.Errorf("Event user_id = %v, want %v", event.GetUserId(), "test_user_123")
	}

	if event.GetTraceId() != "test_trace_456" {
		t.Errorf("Event trace_id = %v, want %v", event.GetTraceId(), "test_trace_456")
	}

	// 验证其他字段
	if event.GetEventType() != EventTypeTaskSubmit {
		t.Errorf("Event type = %v, want %v", event.GetEventType(), EventTypeTaskSubmit)
	}

	if event.GetEventId() == "" {
		t.Error("Event ID should be auto-generated")
	}
}

// TestEventBuilder_FromContext_EmptyContext 测试空 context 的情况
func TestEventBuilder_FromContext_EmptyContext(t *testing.T) {
	ctx := context.Background()
	reporter := &noopReporter{}

	event := reporter.NewEvent(EventTypeTaskSubmit).
		FromContext(ctx).
		Payload(map[string]any{
			"task_id": "task_123",
		}).
		Build()

	// 空 context 应该不提取到 user_id 和 trace_id
	if event.GetUserId() != "" {
		t.Errorf("Event user_id should be empty, got %v", event.GetUserId())
	}

	if event.GetTraceId() != "" {
		t.Errorf("Event trace_id should be empty, got %v", event.GetTraceId())
	}
}

// TestEventBuilder_ManualOverride 测试手动设置可以覆盖 FromContext 提取的值
func TestEventBuilder_ManualOverride(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, constants.CtxUserID, "context_user")
	ctx = context.WithValue(ctx, constants.CtxTraceID, "context_trace")

	reporter := &noopReporter{}

	// FromContext 之后手动设置应该覆盖
	event := reporter.NewEvent(EventTypeTaskSubmit).
		FromContext(ctx).
		UserID("manual_user").
		TraceID("manual_trace").
		Payload(map[string]any{
			"task_id": "task_123",
		}).
		Build()

	if event.GetUserId() != "manual_user" {
		t.Errorf("Event user_id = %v, want %v", event.GetUserId(), "manual_user")
	}

	if event.GetTraceId() != "manual_trace" {
		t.Errorf("Event trace_id = %v, want %v", event.GetTraceId(), "manual_trace")
	}
}

// TestEventBuilder_FromContext_BeforeManual 测试先手动设置再 FromContext 的情况
func TestEventBuilder_FromContext_BeforeManual(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, constants.CtxUserID, "context_user")
	ctx = context.WithValue(ctx, constants.CtxTraceID, "context_trace")

	reporter := &noopReporter{}

	// 手动设置在前，FromContext 应该覆盖
	event := reporter.NewEvent(EventTypeTaskSubmit).
		UserID("manual_user").
		TraceID("manual_trace").
		FromContext(ctx).
		Payload(map[string]any{
			"task_id": "task_123",
		}).
		Build()

	if event.GetUserId() != "context_user" {
		t.Errorf("Event user_id = %v, want %v (FromContext should override manual)", event.GetUserId(), "context_user")
	}

	if event.GetTraceId() != "context_trace" {
		t.Errorf("Event trace_id = %v, want %v (FromContext should override manual)", event.GetTraceId(), "context_trace")
	}
}
