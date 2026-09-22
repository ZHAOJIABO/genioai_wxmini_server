package event_reporter

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eventsv1 "va_visionai_server/pkg/event_sink/v1"
)

// ============ Fake gRPC Server ============

// FakeEventServiceServer 用于测试的 Fake gRPC Server
type FakeEventServiceServer struct {
	eventsv1.UnimplementedEventServiceServer

	mu            sync.Mutex
	singleEvents  []*eventsv1.Event // IngestEvent 收到的事件
	batchEvents   []*eventsv1.Event // IngestBatch 收到的事件
	batchCalls    int               // IngestBatch 调用次数
	singleCalls   int               // IngestEvent 调用次数
	injectErr     error             // 注入错误
	injectDelay   time.Duration     // 注入延迟
	rejectBatches int32             // 拒绝的批次数（用于测试失败场景）
}

// NewFakeEventServiceServer 创建 Fake Server
func NewFakeEventServiceServer() *FakeEventServiceServer {
	return &FakeEventServiceServer{
		singleEvents: make([]*eventsv1.Event, 0),
		batchEvents:  make([]*eventsv1.Event, 0),
	}
}

// IngestEvent 实现单事件接收
func (s *FakeEventServiceServer) IngestEvent(ctx context.Context, req *eventsv1.IngestEventRequest) (*eventsv1.IngestEventResponse, error) {
	if s.injectDelay > 0 {
		select {
		case <-time.After(s.injectDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if s.injectErr != nil {
		return nil, s.injectErr
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.singleCalls++
	if req.GetEvent() != nil {
		s.singleEvents = append(s.singleEvents, req.GetEvent())
	}

	return &eventsv1.IngestEventResponse{
		EventId: req.GetEvent().GetEventId(),
		Status:  "accepted",
	}, nil
}

// IngestBatch 实现批量事件接收
func (s *FakeEventServiceServer) IngestBatch(ctx context.Context, req *eventsv1.IngestBatchRequest) (*eventsv1.IngestBatchResponse, error) {
	if s.injectDelay > 0 {
		select {
		case <-time.After(s.injectDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if s.injectErr != nil {
		return nil, s.injectErr
	}

	// 检查是否需要拒绝
	if atomic.LoadInt32(&s.rejectBatches) > 0 {
		atomic.AddInt32(&s.rejectBatches, -1)
		return nil, status.Error(codes.Unavailable, "server unavailable")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.batchCalls++
	s.batchEvents = append(s.batchEvents, req.GetEvents()...)

	return &eventsv1.IngestBatchResponse{
		AcceptedCount: int32(len(req.GetEvents())),
	}, nil
}

// GetSingleEvents 获取收到的单事件
func (s *FakeEventServiceServer) GetSingleEvents() []*eventsv1.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*eventsv1.Event, len(s.singleEvents))
	copy(result, s.singleEvents)
	return result
}

// GetBatchEvents 获取收到的批量事件
func (s *FakeEventServiceServer) GetBatchEvents() []*eventsv1.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*eventsv1.Event, len(s.batchEvents))
	copy(result, s.batchEvents)
	return result
}

// GetAllEvents 获取所有收到的事件
func (s *FakeEventServiceServer) GetAllEvents() []*eventsv1.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*eventsv1.Event, 0, len(s.singleEvents)+len(s.batchEvents))
	result = append(result, s.singleEvents...)
	result = append(result, s.batchEvents...)
	return result
}

// GetBatchCallCount 获取批量调用次数
func (s *FakeEventServiceServer) GetBatchCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.batchCalls
}

// GetSingleCallCount 获取单事件调用次数
func (s *FakeEventServiceServer) GetSingleCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.singleCalls
}

// Reset 重置所有状态
func (s *FakeEventServiceServer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.singleEvents = make([]*eventsv1.Event, 0)
	s.batchEvents = make([]*eventsv1.Event, 0)
	s.batchCalls = 0
	s.singleCalls = 0
	s.injectErr = nil
	s.injectDelay = 0
	atomic.StoreInt32(&s.rejectBatches, 0)
}

// SetError 设置注入错误
func (s *FakeEventServiceServer) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.injectErr = err
}

// SetDelay 设置注入延迟
func (s *FakeEventServiceServer) SetDelay(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.injectDelay = d
}

// SetRejectBatches 设置拒绝的批次数
func (s *FakeEventServiceServer) SetRejectBatches(n int) {
	atomic.StoreInt32(&s.rejectBatches, int32(n))
}

// ============ 测试辅助函数 ============

// testServer 封装测试所需的 gRPC server 和 client
type testServer struct {
	fakeServer *FakeEventServiceServer
	grpcServer *grpc.Server
	listener   net.Listener
	addr       string
}

// startTestServer 启动测试用 gRPC server
func startTestServer(t *testing.T) *testServer {
	t.Helper()

	// 创建 listener
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	// 创建 fake server
	fakeServer := NewFakeEventServiceServer()

	// 创建 gRPC server
	grpcServer := grpc.NewServer()
	eventsv1.RegisterEventServiceServer(grpcServer, fakeServer)

	// 启动 server
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			// server 关闭时会返回错误，忽略
		}
	}()

	return &testServer{
		fakeServer: fakeServer,
		grpcServer: grpcServer,
		listener:   lis,
		addr:       lis.Addr().String(),
	}
}

// stop 停止测试 server
func (ts *testServer) stop() {
	ts.grpcServer.GracefulStop()
}

// createTestReporter 创建连接到测试 server 的 reporter
func createTestReporter(t *testing.T, ts *testServer, cfg Config) EventReporter {
	t.Helper()

	cfg.Addr = ts.addr
	cfg.Enabled = true

	logger := zap.NewNop()
	reporter, err := New(context.Background(), cfg, logger)
	require.NoError(t, err)

	return reporter
}

// ============ 集成测试 ============

// TestReporter_BatchBySize 测试按数量阈值批量发送
func TestReporter_BatchBySize(t *testing.T) {
	ts := startTestServer(t)
	defer ts.stop()

	cfg := Config{
		QueueSize:     1000,
		BatchSize:     5, // 每 5 个事件触发一次批量发送
		FlushInterval: 10 * time.Second,
		Source:        "test-service",
		ServerNode:    "test-node",
	}

	reporter := createTestReporter(t, ts, cfg)
	defer func() {
		_ = reporter.Shutdown(context.Background())
	}()

	// 发送 5 个事件，应该触发一次批量发送
	for i := 0; i < 5; i++ {
		reporter.NewEvent("TEST_EVENT").
			UserID(fmt.Sprintf("user-%d", i)).
			Track()
	}

	// 等待批量发送完成
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, ts.fakeServer.GetBatchCallCount())
	assert.Len(t, ts.fakeServer.GetBatchEvents(), 5)

	// 再发送 3 个事件，不应该触发新的批量发送
	for i := 0; i < 3; i++ {
		reporter.NewEvent("TEST_EVENT").Track()
	}

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 1, ts.fakeServer.GetBatchCallCount()) // 仍然是 1

	// 再发送 2 个事件，凑够 5 个，触发第二次批量发送
	for i := 0; i < 2; i++ {
		reporter.NewEvent("TEST_EVENT").Track()
	}

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 2, ts.fakeServer.GetBatchCallCount())
	assert.Len(t, ts.fakeServer.GetBatchEvents(), 10)
}

// TestReporter_BatchByInterval 测试按时间阈值批量发送
func TestReporter_BatchByInterval(t *testing.T) {
	ts := startTestServer(t)
	defer ts.stop()

	cfg := Config{
		QueueSize:     1000,
		BatchSize:     100,                    // 较大的批量大小
		FlushInterval: 200 * time.Millisecond, // 较短的时间间隔
		Source:        "test-service",
		ServerNode:    "test-node",
	}

	reporter := createTestReporter(t, ts, cfg)
	defer func() {
		_ = reporter.Shutdown(context.Background())
	}()

	// 发送 3 个事件（未达到批量大小）
	for i := 0; i < 3; i++ {
		reporter.NewEvent("TEST_EVENT").Track()
	}

	// 立即检查，不应该有批量发送
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 0, ts.fakeServer.GetBatchCallCount())

	// 等待时间间隔过后，应该触发批量发送
	time.Sleep(250 * time.Millisecond)
	assert.Equal(t, 1, ts.fakeServer.GetBatchCallCount())
	assert.Len(t, ts.fakeServer.GetBatchEvents(), 3)
}

// TestReporter_TrackNow_Success 测试 TrackNow 成功场景
func TestReporter_TrackNow_Success(t *testing.T) {
	ts := startTestServer(t)
	defer ts.stop()

	cfg := Config{
		Source:     "test-service",
		ServerNode: "test-node",
	}

	reporter := createTestReporter(t, ts, cfg)
	defer func() {
		_ = reporter.Shutdown(context.Background())
	}()

	// 同步发送事件
	err := reporter.NewEvent("CRITICAL_EVENT").
		UserID("user-123").
		TrackNow(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, ts.fakeServer.GetSingleCallCount())
	assert.Len(t, ts.fakeServer.GetSingleEvents(), 1)
	assert.Equal(t, "CRITICAL_EVENT", ts.fakeServer.GetSingleEvents()[0].GetEventType())
}

// TestReporter_TrackNow_Error 测试 TrackNow 失败场景
func TestReporter_TrackNow_Error(t *testing.T) {
	ts := startTestServer(t)
	defer ts.stop()

	cfg := Config{
		Source:     "test-service",
		ServerNode: "test-node",
	}

	reporter := createTestReporter(t, ts, cfg)
	defer func() {
		_ = reporter.Shutdown(context.Background())
	}()

	// 注入错误
	ts.fakeServer.SetError(status.Error(codes.Internal, "internal error"))

	// 同步发送事件，应该返回错误
	err := reporter.NewEvent("TEST_EVENT").TrackNow(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal error")
}

// TestReporter_TrackNow_Timeout 测试 TrackNow 超时场景
func TestReporter_TrackNow_Timeout(t *testing.T) {
	ts := startTestServer(t)
	defer ts.stop()

	cfg := Config{
		Source:     "test-service",
		ServerNode: "test-node",
	}

	reporter := createTestReporter(t, ts, cfg)
	defer func() {
		_ = reporter.Shutdown(context.Background())
	}()

	// 注入延迟
	ts.fakeServer.SetDelay(500 * time.Millisecond)

	// 使用短超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := reporter.NewEvent("TEST_EVENT").TrackNow(ctx)

	require.Error(t, err)
	// gRPC 会包装 context 错误，检查错误消息包含 DeadlineExceeded
	assert.Contains(t, err.Error(), "DeadlineExceeded")
}

// TestReporter_Flush 测试手动 Flush
func TestReporter_Flush(t *testing.T) {
	ts := startTestServer(t)
	defer ts.stop()

	cfg := Config{
		QueueSize:     1000,
		BatchSize:     100,              // 较大的批量大小
		FlushInterval: 10 * time.Second, // 较长的时间间隔
		Source:        "test-service",
		ServerNode:    "test-node",
	}

	reporter := createTestReporter(t, ts, cfg)
	defer func() {
		_ = reporter.Shutdown(context.Background())
	}()

	// 发送 3 个事件
	for i := 0; i < 3; i++ {
		reporter.NewEvent("TEST_EVENT").Track()
	}

	// 立即检查，不应该有批量发送
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 0, ts.fakeServer.GetBatchCallCount())

	// 手动 Flush
	err := reporter.Flush(context.Background())
	require.NoError(t, err)

	// Flush 后应该立即发送
	assert.Equal(t, 1, ts.fakeServer.GetBatchCallCount())
	assert.Len(t, ts.fakeServer.GetBatchEvents(), 3)
}

// TestReporter_QueueFull 测试队列满时的丢弃行为
func TestReporter_QueueFull(t *testing.T) {
	ts := startTestServer(t)
	defer ts.stop()

	cfg := Config{
		QueueSize:     5,                // 非常小的队列
		BatchSize:     100,              // 较大的批量大小，不会触发发送
		FlushInterval: 10 * time.Second, // 较长的时间间隔
		Source:        "test-service",
		ServerNode:    "test-node",
	}

	reporter := createTestReporter(t, ts, cfg)
	defer func() {
		_ = reporter.Shutdown(context.Background())
	}()

	// 发送超过队列大小的事件
	for i := 0; i < 10; i++ {
		reporter.NewEvent("TEST_EVENT").Track()
	}

	// 等待一下让事件入队
	time.Sleep(50 * time.Millisecond)

	// 队列长度应该等于或小于队列大小
	assert.LessOrEqual(t, reporter.QueueLen(), 5)
}

// TestReporter_GracefulShutdown 测试优雅关闭
func TestReporter_GracefulShutdown(t *testing.T) {
	ts := startTestServer(t)
	defer ts.stop()

	cfg := Config{
		QueueSize:       1000,
		BatchSize:       100,              // 较大的批量大小
		FlushInterval:   10 * time.Second, // 较长的时间间隔
		ShutdownTimeout: 5 * time.Second,
		Source:          "test-service",
		ServerNode:      "test-node",
	}

	reporter := createTestReporter(t, ts, cfg)

	// 发送一些事件（不会自动触发批量发送）
	for i := 0; i < 7; i++ {
		reporter.NewEvent("TEST_EVENT").
			UserID(fmt.Sprintf("user-%d", i)).
			Track()
	}

	// 等待事件入队（worker 会消费 channel 中的事件到 batch 中）
	time.Sleep(200 * time.Millisecond)

	// 验证还没有批量发送
	assert.Equal(t, 0, ts.fakeServer.GetBatchCallCount())

	// 优雅关闭，应该发送剩余事件
	err := reporter.Shutdown(context.Background())
	require.NoError(t, err)

	// 验证所有事件都被发送
	assert.Equal(t, 1, ts.fakeServer.GetBatchCallCount())
	assert.Len(t, ts.fakeServer.GetBatchEvents(), 7)
}

// TestReporter_ShutdownTimeout 测试关闭超时
func TestReporter_ShutdownTimeout(t *testing.T) {
	ts := startTestServer(t)
	defer ts.stop()

	// 注入延迟，使发送变慢
	ts.fakeServer.SetDelay(500 * time.Millisecond)

	cfg := Config{
		QueueSize:       1000,
		BatchSize:       5,
		FlushInterval:   10 * time.Second,
		ShutdownTimeout: 100 * time.Millisecond, // 很短的超时
		Source:          "test-service",
		ServerNode:      "test-node",
	}

	reporter := createTestReporter(t, ts, cfg)

	// 发送事件
	for i := 0; i < 5; i++ {
		reporter.NewEvent("TEST_EVENT").Track()
	}

	// 使用短超时的 context 进行关闭
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := reporter.Shutdown(ctx)

	// 应该超时
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestReporter_EventFieldsFilled 测试事件字段正确填充
func TestReporter_EventFieldsFilled(t *testing.T) {
	ts := startTestServer(t)
	defer ts.stop()

	cfg := Config{
		BatchSize:     1, // 每个事件立即发送
		FlushInterval: 10 * time.Second,
		Source:        "my-service",
		ServerNode:    "node-001",
	}

	reporter := createTestReporter(t, ts, cfg)
	defer func() {
		_ = reporter.Shutdown(context.Background())
	}()

	// 发送事件（只设置部分字段）
	reporter.NewEvent("USER_LOGIN").
		UserID("user-123").
		Track()

	// 等待发送
	time.Sleep(100 * time.Millisecond)

	events := ts.fakeServer.GetBatchEvents()
	require.Len(t, events, 1)

	event := events[0]

	// 验证自动填充的字段
	assert.NotEmpty(t, event.GetEventId())                                // UUID 自动生成
	assert.Equal(t, "my-service", event.GetSource())                      // source 自动填充
	assert.NotZero(t, event.GetOccurredMs())                              // occurred_ms 自动填充
	assert.NotZero(t, event.GetSentMs())                                  // sent_ms 自动填充
	assert.Equal(t, "node-001", event.GetServerContext().GetServerNode()) // server_node 自动填充

	// 验证手动设置的字段
	assert.Equal(t, "USER_LOGIN", event.GetEventType())
	assert.Equal(t, "user-123", event.GetUserId())
}

// TestReporter_MultipleShutdownCalls 测试多次调用 Shutdown
func TestReporter_MultipleShutdownCalls(t *testing.T) {
	ts := startTestServer(t)
	defer ts.stop()

	cfg := Config{
		Source:     "test-service",
		ServerNode: "test-node",
	}

	reporter := createTestReporter(t, ts, cfg)

	// 第一次关闭
	err1 := reporter.Shutdown(context.Background())
	require.NoError(t, err1)

	// 第二次关闭，应该是幂等的
	err2 := reporter.Shutdown(context.Background())
	require.NoError(t, err2)
}

// TestReporter_TrackAfterShutdown 测试关闭后发送事件
func TestReporter_TrackAfterShutdown(t *testing.T) {
	ts := startTestServer(t)
	defer ts.stop()

	cfg := Config{
		BatchSize:  1,
		Source:     "test-service",
		ServerNode: "test-node",
	}

	reporter := createTestReporter(t, ts, cfg)

	// 关闭
	err := reporter.Shutdown(context.Background())
	require.NoError(t, err)

	// 关闭后发送事件，应该被丢弃（不会 panic）
	reporter.NewEvent("TEST_EVENT").Track()

	// 验证没有新事件被发送
	time.Sleep(100 * time.Millisecond)
	assert.Empty(t, ts.fakeServer.GetAllEvents())
}
