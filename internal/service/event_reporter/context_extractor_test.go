package event_reporter

import (
	"context"
	"testing"

	"va_visionai_server/internal/constants"
)

func TestDefaultContextExtractor_ExtractUserID(t *testing.T) {
	extractor := &DefaultContextExtractor{}

	tests := []struct {
		name     string
		setupCtx func() context.Context
		want     string
	}{
		{
			name: "使用业务层常量 CtxUserID",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), constants.CtxUserID, "user123")
			},
			want: "user123",
		},
		{
			name: "context 中没有 user_id",
			setupCtx: func() context.Context {
				return context.Background()
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			got := extractor.ExtractUserID(ctx)
			if got != tt.want {
				t.Errorf("ExtractUserID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultContextExtractor_ExtractTraceID(t *testing.T) {
	extractor := &DefaultContextExtractor{}

	tests := []struct {
		name     string
		setupCtx func() context.Context
		want     string
	}{
		{
			name: "使用业务层常量 CtxTraceID",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), constants.CtxTraceID, "trace123")
			},
			want: "trace123",
		},
		{
			name: "context 中没有 trace_id",
			setupCtx: func() context.Context {
				return context.Background()
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			got := extractor.ExtractTraceID(ctx)
			if got != tt.want {
				t.Errorf("ExtractTraceID() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExtractUserID_Integration 集成测试：模拟真实业务场景
func TestExtractUserID_Integration(t *testing.T) {
	// 模拟 gRPC interceptor 设置的 context（使用 constants.CtxUserID）
	ctx := context.Background()
	ctx = context.WithValue(ctx, constants.CtxUserID, "real_user_123")
	ctx = context.WithValue(ctx, constants.CtxTraceID, "real_trace_456")

	// 提取 user_id
	userID := extractUserID(ctx)
	if userID != "real_user_123" {
		t.Errorf("extractUserID() = %v, want %v", userID, "real_user_123")
	}

	// 提取 trace_id
	traceID := extractTraceID(ctx)
	if traceID != "real_trace_456" {
		t.Errorf("extractTraceID() = %v, want %v", traceID, "real_trace_456")
	}
}
