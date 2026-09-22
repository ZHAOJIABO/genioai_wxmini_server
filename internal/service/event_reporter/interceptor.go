package event_reporter

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	eventsv1 "va_visionai_server/pkg/event_sink/v1"
)

// EventInterceptor 事件拦截器接口
// 在事件发送前对事件进行处理，可用于：
// - 自动填充公共字段（source、server_node、时间戳等）
// - 敏感数据脱敏
// - 采样控制
// - 日志审计
type EventInterceptor interface {
	// Intercept 在事件发送前被调用
	// 返回处理后的事件，返回 nil 表示丢弃该事件
	Intercept(event *eventsv1.Event) *eventsv1.Event
}

// InterceptorFunc 函数类型拦截器，方便使用匿名函数
type InterceptorFunc func(event *eventsv1.Event) *eventsv1.Event

// Intercept 实现 EventInterceptor 接口
func (f InterceptorFunc) Intercept(event *eventsv1.Event) *eventsv1.Event {
	return f(event)
}

// DefaultInterceptor 默认拦截器，自动填充公共字段
type DefaultInterceptor struct {
	source     string
	serverNode string
	sessionID  string        // 会话ID，格式: sess_{hostname}_{启动时间戳}
	sequence   *atomic.Int64 // 序号计数器，单调递增
}

// NewDefaultInterceptor 创建默认拦截器
func NewDefaultInterceptor(source, serverNode string) *DefaultInterceptor {
	// 生成 session_id: sess_{hostname}_{启动时间戳}
	hostname, _ := os.Hostname()
	sessionID := fmt.Sprintf("sess_%s_%d", hostname, time.Now().UnixMilli())

	return &DefaultInterceptor{
		source:     source,
		serverNode: serverNode,
		sessionID:  sessionID,
		sequence:   &atomic.Int64{},
	}
}

// Intercept 自动填充公共字段
func (i *DefaultInterceptor) Intercept(event *eventsv1.Event) *eventsv1.Event {
	if event == nil {
		return nil
	}

	// 填充 source（如果未设置）
	if event.GetSource() == "" {
		event.Source = i.source
	}

	// 填充 occurred_ms（如果未设置）
	if event.GetOccurredMs() == 0 {
		event.OccurredMs = time.Now().UnixMilli()
	}

	// 填充 server_context.server_node（如果未设置）
	if event.GetServerContext() == nil {
		event.ServerContext = &eventsv1.ServerContext{}
	}
	if event.GetServerContext().GetServerNode() == "" {
		event.ServerContext.ServerNode = i.serverNode
	}

	// 填充 session_id（如果未设置）
	if event.GetSessionId() == "" {
		event.SessionId = i.sessionID
	}

	// 填充 sequence（如果未设置，始终递增确保唯一）
	if event.GetSequence() == 0 {
		event.Sequence = i.sequence.Add(1)
	}

	return event
}

// InterceptorChain 拦截器链，按顺序执行多个拦截器
type InterceptorChain struct {
	interceptors []EventInterceptor
}

// NewInterceptorChain 创建拦截器链
func NewInterceptorChain(interceptors ...EventInterceptor) *InterceptorChain {
	return &InterceptorChain{
		interceptors: interceptors,
	}
}

// Add 添加拦截器到链中
func (c *InterceptorChain) Add(interceptor EventInterceptor) *InterceptorChain {
	c.interceptors = append(c.interceptors, interceptor)
	return c
}

// Intercept 按顺序执行所有拦截器
func (c *InterceptorChain) Intercept(event *eventsv1.Event) *eventsv1.Event {
	for _, interceptor := range c.interceptors {
		event = interceptor.Intercept(event)
		if event == nil {
			return nil // 事件被丢弃
		}
	}
	return event
}

// Len 返回拦截器数量
func (c *InterceptorChain) Len() int {
	return len(c.interceptors)
}

// ============ 预定义拦截器 ============

// SamplingInterceptor 采样拦截器，按比例丢弃事件
type SamplingInterceptor struct {
	rate    float64 // 采样率 0.0-1.0
	counter int64
}

// NewSamplingInterceptor 创建采样拦截器
// rate: 采样率，0.1 表示只保留 10% 的事件
func NewSamplingInterceptor(rate float64) *SamplingInterceptor {
	if rate < 0 {
		rate = 0
	}
	if rate > 1 {
		rate = 1
	}
	return &SamplingInterceptor{rate: rate}
}

// Intercept 根据采样率决定是否保留事件
func (i *SamplingInterceptor) Intercept(event *eventsv1.Event) *eventsv1.Event {
	if i.rate >= 1.0 {
		return event // 100% 保留
	}
	if i.rate <= 0 {
		return nil // 0% 保留
	}

	i.counter++
	// 简单的计数器采样，每 N 个事件保留一个
	threshold := int64(1.0 / i.rate)
	if i.counter%threshold == 0 {
		return event
	}
	return nil
}

// FilterInterceptor 过滤拦截器，根据条件过滤事件
type FilterInterceptor struct {
	filter func(event *eventsv1.Event) bool
}

// NewFilterInterceptor 创建过滤拦截器
// filter: 返回 true 保留事件，返回 false 丢弃事件
func NewFilterInterceptor(filter func(event *eventsv1.Event) bool) *FilterInterceptor {
	return &FilterInterceptor{filter: filter}
}

// Intercept 根据过滤条件决定是否保留事件
func (i *FilterInterceptor) Intercept(event *eventsv1.Event) *eventsv1.Event {
	if i.filter(event) {
		return event
	}
	return nil
}
