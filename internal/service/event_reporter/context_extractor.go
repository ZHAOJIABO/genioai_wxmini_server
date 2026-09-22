package event_reporter

import (
	"context"

	"va_visionai_server/internal/constants"
)

// ContextExtractor 接口，支持自定义 context 提取逻辑
type ContextExtractor interface {
	ExtractUserID(ctx context.Context) string
	ExtractTraceID(ctx context.Context) string
	ExtractSessionID(ctx context.Context) string
}

// DefaultContextExtractor 默认 context 提取器
type DefaultContextExtractor struct{}

// ExtractUserID 从 context 中提取 user_id
func (e *DefaultContextExtractor) ExtractUserID(ctx context.Context) string {
	if v := ctx.Value(constants.CtxUserID); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ExtractTraceID 从 context 中提取 trace_id
func (e *DefaultContextExtractor) ExtractTraceID(ctx context.Context) string {
	if v := ctx.Value(constants.CtxTraceID); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ExtractSessionID 从 context 中提取 session_id
func (e *DefaultContextExtractor) ExtractSessionID(ctx context.Context) string {
	if v := ctx.Value(constants.CtxSessionID); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// 默认提取器实例
var defaultExtractor = &DefaultContextExtractor{}

// extractTraceID 从 context 中提取 trace_id
func extractTraceID(ctx context.Context) string {
	return defaultExtractor.ExtractTraceID(ctx)
}

// extractUserID 从 context 中提取 user_id
func extractUserID(ctx context.Context) string {
	return defaultExtractor.ExtractUserID(ctx)
}

// extractSessionID 从 context 中提取 session_id
func extractSessionID(ctx context.Context) string {
	return defaultExtractor.ExtractSessionID(ctx)
}

// WithTraceID 将 trace_id 注入 context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, constants.CtxTraceID, traceID)
}

// WithUserID 将 user_id 注入 context
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, constants.CtxUserID, userID)
}

// WithSessionID 将 session_id 注入 context
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, constants.CtxSessionID, sessionID)
}
