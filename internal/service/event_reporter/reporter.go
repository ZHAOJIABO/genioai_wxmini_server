package event_reporter

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	eventsv1 "va_visionai_server/pkg/event_sink/v1"
)

// EventReporter 事件上报客户端接口
type EventReporter interface {
	// Track 异步上报单个事件（非阻塞，入队等待批量发送）
	Track(event *eventsv1.Event)

	// TrackNow 实时上报单个事件（同步阻塞，立即发送）
	TrackNow(ctx context.Context, event *eventsv1.Event) error

	// NewEvent 创建事件构建器
	NewEvent(eventType string) *EventBuilder

	// Flush 立即刷新队列中的事件
	Flush(ctx context.Context) error

	// Shutdown 优雅关闭
	Shutdown(ctx context.Context) error

	// QueueLen 返回当前队列长度
	QueueLen() int
}

// reporter EventReporter 的实现
type reporter struct {
	config      Config
	client      eventsv1.EventServiceClient
	conn        *grpc.ClientConn
	interceptor EventInterceptor // 拦截器链

	eventCh  chan *eventsv1.Event
	flushCh  chan chan error
	doneCh   chan struct{}
	shutdown atomic.Bool

	wg     sync.WaitGroup
	logger *zap.Logger
}

// New 创建新的 EventReporter
func New(ctx context.Context, cfg Config, logger *zap.Logger) (EventReporter, error) {
	cfg = cfg.WithDefaults()

	if !cfg.Enabled {
		return &noopReporter{config: cfg, logger: logger}, nil
	}

	if cfg.Addr == "" {
		logger.Warn("event_sink addr is empty, using noop reporter")
		return &noopReporter{config: cfg, logger: logger}, nil
	}

	conn, err := grpc.NewClient(cfg.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.Warn("failed to create grpc client for event_sink, using noop reporter",
			zap.String("addr", cfg.Addr),
			zap.Error(err),
		)
		return &noopReporter{config: cfg, logger: logger}, nil
	}

	// 主动触发连接（grpc.NewClient 默认懒连接）
	conn.Connect()

	// 验证连接可用性（带超时）
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := checkConnection(dialCtx, conn); err != nil {
		logger.Warn("failed to connect to event_sink, using noop reporter",
			zap.String("addr", cfg.Addr),
			zap.Error(err),
		)
		conn.Close()
		return &noopReporter{config: cfg, logger: logger}, nil
	}

	r := &reporter{
		config:      cfg,
		client:      eventsv1.NewEventServiceClient(conn),
		conn:        conn,
		interceptor: NewDefaultInterceptor(cfg.Source, cfg.ServerNode), // 默认拦截器
		eventCh:     make(chan *eventsv1.Event, cfg.QueueSize),
		flushCh:     make(chan chan error),
		doneCh:      make(chan struct{}),
		logger:      logger.Named("event_reporter"),
	}

	// 启动 flush worker
	r.wg.Add(1)
	go r.flushWorker()

	r.logger.Info("event reporter started",
		zap.String("addr", cfg.Addr),
		zap.Int("queue_size", cfg.QueueSize),
		zap.Int("batch_size", cfg.BatchSize),
		zap.Duration("flush_interval", cfg.FlushInterval),
	)

	return r, nil
}

// Track 异步上报单个事件
func (r *reporter) Track(event *eventsv1.Event) {
	if r.shutdown.Load() {
		r.logger.Warn("event reporter is shutting down, event dropped",
			zap.String("event_id", event.GetEventId()),
			zap.String("event_type", event.GetEventType()),
		)
		return
	}

	// 通过拦截器处理事件
	event = r.applyInterceptor(event)
	if event == nil {
		return // 事件被拦截器丢弃
	}

	select {
	case r.eventCh <- event:
		// 成功入队
	default:
		// 队列已满，丢弃事件
		r.logger.Warn("event queue is full, event dropped",
			zap.String("event_id", event.GetEventId()),
			zap.String("event_type", event.GetEventType()),
			zap.Int("queue_len", len(r.eventCh)),
		)
	}
}

// TrackNow 实时上报单个事件
func (r *reporter) TrackNow(ctx context.Context, event *eventsv1.Event) error {
	// 通过拦截器处理事件
	event = r.applyInterceptor(event)
	if event == nil {
		return nil // 事件被拦截器丢弃
	}

	event.SentMs = time.Now().UnixMilli()

	_, err := r.client.IngestEvent(ctx, &eventsv1.IngestEventRequest{
		Event: event,
	})
	if err != nil {
		r.logger.Error("failed to send event",
			zap.String("event_id", event.GetEventId()),
			zap.String("event_type", event.GetEventType()),
			zap.Error(err),
		)
	}
	return err
}

// NewEvent 创建事件构建器
func (r *reporter) NewEvent(eventType string) *EventBuilder {
	return newEventBuilder(r, eventType)
}

// Flush 立即刷新队列中的事件
func (r *reporter) Flush(ctx context.Context) error {
	resultCh := make(chan error, 1)

	select {
	case r.flushCh <- resultCh:
		select {
		case err := <-resultCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Shutdown 优雅关闭
func (r *reporter) Shutdown(ctx context.Context) error {
	if r.shutdown.Swap(true) {
		return nil // 已经在关闭
	}

	r.logger.Info("shutting down event reporter")

	// 通知 worker 停止
	close(r.doneCh)

	// 等待 worker 完成
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		r.logger.Info("event reporter shutdown completed")
	case <-ctx.Done():
		r.logger.Warn("event reporter shutdown timeout, some events may be lost")
		return ctx.Err()
	}

	// 关闭 gRPC 连接
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

// QueueLen 返回当前队列长度
func (r *reporter) QueueLen() int {
	return len(r.eventCh)
}

// flushWorker 后台批量发送 worker
func (r *reporter) flushWorker() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]*eventsv1.Event, 0, r.config.BatchSize)

	for {
		select {
		case event := <-r.eventCh:
			batch = append(batch, event)
			if len(batch) >= r.config.BatchSize {
				r.sendBatch(batch)
				batch = make([]*eventsv1.Event, 0, r.config.BatchSize)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				r.sendBatch(batch)
				batch = make([]*eventsv1.Event, 0, r.config.BatchSize)
			}

		case resultCh := <-r.flushCh:
			// 手动触发 flush
			if len(batch) > 0 {
				err := r.sendBatch(batch)
				batch = make([]*eventsv1.Event, 0, r.config.BatchSize)
				resultCh <- err
			} else {
				resultCh <- nil
			}

		case <-r.doneCh:
			// 关闭时发送剩余事件
			// 先把 channel 中的事件全部收集
			for {
				select {
				case event := <-r.eventCh:
					batch = append(batch, event)
				default:
					goto drain
				}
			}
		drain:
			if len(batch) > 0 {
				r.sendBatch(batch)
			}
			return
		}
	}
}

// sendBatch 批量发送事件
func (r *reporter) sendBatch(events []*eventsv1.Event) error {
	if len(events) == 0 {
		return nil
	}

	// 设置发送时间
	now := time.Now().UnixMilli()
	for _, e := range events {
		if e.GetSentMs() == 0 {
			e.SentMs = now
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := r.client.IngestBatch(ctx, &eventsv1.IngestBatchRequest{
		Events: events,
	})
	if err != nil {
		r.logger.Error("failed to send batch",
			zap.Int("batch_size", len(events)),
			zap.Error(err),
		)
		return err
	}

	r.logger.Debug("batch sent successfully",
		zap.Int("batch_size", len(events)),
		zap.Int32("accepted_count", resp.GetAcceptedCount()),
	)
	return nil
}

// applyInterceptor 应用拦截器处理事件
func (r *reporter) applyInterceptor(event *eventsv1.Event) *eventsv1.Event {
	// 先填充 event_id（如果未设置）
	if event.GetEventId() == "" {
		event.EventId = uuid.New().String()
	}
	// 应用拦截器
	if r.interceptor != nil {
		return r.interceptor.Intercept(event)
	}
	return event
}

// SetInterceptor 设置自定义拦截器（替换默认拦截器）
func (r *reporter) SetInterceptor(interceptor EventInterceptor) {
	r.interceptor = interceptor
}

// noopReporter 空操作实现，用于禁用模式
type noopReporter struct {
	config Config
	logger *zap.Logger
}

func (n *noopReporter) Track(event *eventsv1.Event) {}

func (n *noopReporter) TrackNow(ctx context.Context, event *eventsv1.Event) error {
	return nil
}

func (n *noopReporter) NewEvent(eventType string) *EventBuilder {
	return newEventBuilder(n, eventType)
}

func (n *noopReporter) Flush(ctx context.Context) error {
	return nil
}

func (n *noopReporter) Shutdown(ctx context.Context) error {
	n.logger.Info("noop event reporter shutdown")
	return nil
}

func (n *noopReporter) QueueLen() int {
	return 0
}

// checkConnection 验证 gRPC 连接是否可用
func checkConnection(ctx context.Context, conn *grpc.ClientConn) error {
	for {
		state := conn.GetState()
		if state == 2 { // connectivity.Ready
			return nil
		}
		if !conn.WaitForStateChange(ctx, state) {
			return ctx.Err()
		}
	}
}
