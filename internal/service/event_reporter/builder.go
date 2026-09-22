package event_reporter

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"

	eventsv1 "va_visionai_server/pkg/event_sink/v1"
)

// EventBuilder 链式事件构建器
type EventBuilder struct {
	reporter EventReporter
	event    *eventsv1.Event
}

// newEventBuilder 创建新的事件构建器
func newEventBuilder(reporter EventReporter, eventType string) *EventBuilder {
	return &EventBuilder{
		reporter: reporter,
		event: &eventsv1.Event{
			EventId:   uuid.New().String(),
			EventType: eventType,
		},
	}
}

// ============ 核心标识字段 ============

// EventID 设置事件ID（可选，不设置则自动生成 UUID）
func (b *EventBuilder) EventID(id string) *EventBuilder {
	b.event.EventId = id
	return b
}

// Source 设置事件来源（可选，默认使用配置的 source）
func (b *EventBuilder) Source(source string) *EventBuilder {
	b.event.Source = source
	return b
}

// ============ 用户与会话 ============

// UserID 设置用户ID
func (b *EventBuilder) UserID(userID string) *EventBuilder {
	b.event.UserId = userID
	return b
}

// SessionID 设置会话ID
func (b *EventBuilder) SessionID(sessionID string) *EventBuilder {
	b.event.SessionId = sessionID
	return b
}

// TraceID 设置链路追踪ID
func (b *EventBuilder) TraceID(traceID string) *EventBuilder {
	b.event.TraceId = traceID
	return b
}

// Sequence 设置事件序号（会话内递增）
func (b *EventBuilder) Sequence(seq int64) *EventBuilder {
	b.event.Sequence = seq
	return b
}

// BatchSequence 设置批次序号
func (b *EventBuilder) BatchSequence(seq int64) *EventBuilder {
	b.event.BatchSequence = seq
	return b
}

// ============ 时间戳 ============

// OccurredAt 设置事件发生时间（可选，默认 time.Now()）
func (b *EventBuilder) OccurredAt(t time.Time) *EventBuilder {
	b.event.OccurredMs = t.UnixMilli()
	return b
}

// OccurredMs 直接设置毫秒时间戳
func (b *EventBuilder) OccurredMs(ms int64) *EventBuilder {
	b.event.OccurredMs = ms
	return b
}

// ============ 上下文信息 ============

// ClientContext 设置客户端上下文
func (b *EventBuilder) ClientContext(ctx *eventsv1.ClientContext) *EventBuilder {
	b.event.ClientContext = ctx
	return b
}

// ServerContext 设置服务端上下文
func (b *EventBuilder) ServerContext(ctx *eventsv1.ServerContext) *EventBuilder {
	b.event.ServerContext = ctx
	return b
}

// ActionContext 设置动作上下文
func (b *EventBuilder) ActionContext(ctx *eventsv1.ActionContext) *EventBuilder {
	b.event.ActionContext = ctx
	return b
}

// Action 便捷方法：快速设置 ActionContext
func (b *EventBuilder) Action(domain, actionType, operation, target string) *EventBuilder {
	if b.event.GetActionContext() == nil {
		b.event.ActionContext = &eventsv1.ActionContext{}
	}
	b.event.ActionContext.Domain = domain
	b.event.ActionContext.Type = actionType
	b.event.ActionContext.Operation = operation
	b.event.ActionContext.Target = target
	return b
}

// ActionSuccess 设置动作成功
func (b *EventBuilder) ActionSuccess(durationMs int64) *EventBuilder {
	if b.event.GetActionContext() == nil {
		b.event.ActionContext = &eventsv1.ActionContext{}
	}
	b.event.ActionContext.Success = true
	b.event.ActionContext.DurationMs = durationMs
	return b
}

// ActionFailed 设置动作失败
func (b *EventBuilder) ActionFailed(durationMs int64, err error) *EventBuilder {
	if b.event.GetActionContext() == nil {
		b.event.ActionContext = &eventsv1.ActionContext{}
	}
	b.event.ActionContext.Success = false
	b.event.ActionContext.DurationMs = durationMs
	if err != nil {
		b.event.ActionContext.Error = err.Error()
	}
	return b
}

// ActionMetadata 设置动作元数据
func (b *EventBuilder) ActionMetadata(metadata map[string]any) *EventBuilder {
	if b.event.GetActionContext() == nil {
		b.event.ActionContext = &eventsv1.ActionContext{}
	}
	if s, err := structpb.NewStruct(metadata); err == nil {
		b.event.ActionContext.Metadata = s
	}
	return b
}

// ============ 扩展属性 ============

// Attributes 设置事件扩展属性
func (b *EventBuilder) Attributes(attrs map[string]any) *EventBuilder {
	if s, err := structpb.NewStruct(attrs); err == nil {
		b.event.EventAttributes = s
	}
	return b
}

// Attr 添加单个扩展属性
func (b *EventBuilder) Attr(key string, value any) *EventBuilder {
	if b.event.GetEventAttributes() == nil {
		b.event.EventAttributes = &structpb.Struct{
			Fields: make(map[string]*structpb.Value),
		}
	}
	if v, err := structpb.NewValue(value); err == nil {
		b.event.EventAttributes.Fields[key] = v
	}
	return b
}

// ============ 业务数据 ============

// Payload 设置事件负载（必填）
func (b *EventBuilder) Payload(payload map[string]any) *EventBuilder {
	if data, err := json.Marshal(payload); err == nil {
		b.event.Payload = data
	}
	return b
}

// PayloadJSON 从 JSON bytes 设置 payload
func (b *EventBuilder) PayloadJSON(jsonBytes []byte) *EventBuilder {
	b.event.Payload = jsonBytes
	return b
}

// PayloadStruct 从任意结构体设置 payload
func (b *EventBuilder) PayloadStruct(v any) *EventBuilder {
	if data, err := json.Marshal(v); err == nil {
		b.event.Payload = data
	}
	return b
}

// ============ Context 自动提取 ============

// FromContext 从 context.Context 自动提取 trace_id、user_id 等
func (b *EventBuilder) FromContext(ctx context.Context) *EventBuilder {
	// 从 context 中提取 trace_id
	if traceID := extractTraceID(ctx); traceID != "" {
		b.event.TraceId = traceID
	}
	// 从 context 中提取 user_id
	if userID := extractUserID(ctx); userID != "" {
		b.event.UserId = userID
	}
	return b
}

// FromGRPCContext 从 gRPC incoming context 提取信息
func (b *EventBuilder) FromGRPCContext(ctx context.Context) *EventBuilder {
	return b.FromContext(ctx)
}

// ============ 构建与发送 ============

// Build 构建事件（返回 *eventsv1.Event，不发送）
func (b *EventBuilder) Build() *eventsv1.Event {
	return b.event
}

// Track 构建并异步发送事件（入队，非阻塞）
func (b *EventBuilder) Track() {
	b.reporter.Track(b.event)
}

// TrackNow 构建并实时发送事件（同步阻塞，立即发送）
func (b *EventBuilder) TrackNow(ctx context.Context) error {
	return b.reporter.TrackNow(ctx, b.event)
}
