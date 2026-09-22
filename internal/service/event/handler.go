package event

import (
	"context"

	"va_visionai_server/internal/model"
)

// Handler 事件处理器接口
type Handler interface {
	// HandleEvent 处理事件
	HandleEvent(ctx context.Context, event *model.UserEvent) error
	// EventType 返回处理器支持的事件类型
	EventType() []string
}

// Registry 事件处理器注册表
type Registry struct {
	handlers map[string]Handler
}

// NewRegistry 创建事件注册表
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]Handler),
	}
}

// Register 注册事件处理器
func (r *Registry) Register(h Handler) {
	for _, eventType := range h.EventType() {
		r.handlers[eventType] = h
	}
}

// GetHandler 获取事件处理器
func (r *Registry) GetHandler(eventType string) Handler {
	if handler, ok := r.handlers[eventType]; ok {
		return handler
	}
	return r.handlers["default"]
}
