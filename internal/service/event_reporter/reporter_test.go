package event_reporter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	eventsv1 "va_visionai_server/pkg/event_sink/v1"
)

// ============ Config 测试 ============

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 10000, cfg.QueueSize)
	assert.Equal(t, 100, cfg.BatchSize)
	assert.Equal(t, 5*time.Second, cfg.FlushInterval)
	assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, "va_visionai_server", cfg.Source)
	assert.NotEmpty(t, cfg.ServerNode)
	assert.True(t, cfg.Enabled)
}

func TestConfigWithDefaults(t *testing.T) {
	cfg := Config{
		QueueSize: 500,
	}
	merged := cfg.WithDefaults()

	assert.Equal(t, 500, merged.QueueSize)
	assert.Equal(t, 100, merged.BatchSize)
	assert.Equal(t, 5*time.Second, merged.FlushInterval)
}

// ============ NoopReporter 测试 ============

func TestNoopReporter(t *testing.T) {
	logger := zap.NewNop()
	reporter, err := New(context.Background(), Config{Enabled: false}, logger)

	require.NoError(t, err)
	assert.NotNil(t, reporter)

	// Track 应该是空操作
	reporter.Track(&eventsv1.Event{EventType: "test"})

	// TrackNow 应该返回 nil
	err = reporter.TrackNow(context.Background(), &eventsv1.Event{EventType: "test"})
	require.NoError(t, err)

	// QueueLen 应该返回 0
	assert.Equal(t, 0, reporter.QueueLen())

	// Shutdown 应该成功
	err = reporter.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestNoopReporterWithEmptyAddr(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		Enabled: true,
		Addr:    "", // 空地址
	}
	reporter, err := New(context.Background(), cfg, logger)

	require.NoError(t, err)
	assert.NotNil(t, reporter)
	assert.Equal(t, 0, reporter.QueueLen()) // 应该是 noop reporter
}

// ============ EventBuilder 测试 ============

func TestEventBuilder_BasicFields(t *testing.T) {
	logger := zap.NewNop()
	reporter, _ := New(context.Background(), Config{Enabled: false}, logger)

	event := reporter.NewEvent("user_login").
		UserID("user123").
		SessionID("session456").
		TraceID("trace789").
		Sequence(1).
		Build()

	assert.NotEmpty(t, event.GetEventId())
	assert.Equal(t, "user_login", event.GetEventType())
	assert.Equal(t, "user123", event.GetUserId())
	assert.Equal(t, "session456", event.GetSessionId())
	assert.Equal(t, "trace789", event.GetTraceId())
	assert.Equal(t, int64(1), event.GetSequence())
}

func TestEventBuilder_CustomEventID(t *testing.T) {
	logger := zap.NewNop()
	reporter, _ := New(context.Background(), Config{Enabled: false}, logger)

	event := reporter.NewEvent("test").
		EventID("custom-id").
		Build()

	assert.Equal(t, "custom-id", event.GetEventId())
}

func TestEventBuilder_Timestamps(t *testing.T) {
	logger := zap.NewNop()
	reporter, _ := New(context.Background(), Config{Enabled: false}, logger)

	now := time.Now()
	event := reporter.NewEvent("test").
		OccurredAt(now).
		Build()

	assert.Equal(t, now.UnixMilli(), event.GetOccurredMs())

	// 测试 OccurredMs
	event2 := reporter.NewEvent("test").
		OccurredMs(12345678).
		Build()
	assert.Equal(t, int64(12345678), event2.GetOccurredMs())
}

func TestEventBuilder_Payload(t *testing.T) {
	logger := zap.NewNop()
	reporter, _ := New(context.Background(), Config{Enabled: false}, logger)

	payload := map[string]any{
		"key1": "value1",
		"key2": 123,
	}
	event := reporter.NewEvent("test").
		Payload(payload).
		Build()

	var decoded map[string]any
	err := json.Unmarshal(event.GetPayload(), &decoded)
	require.NoError(t, err)
	assert.Equal(t, "value1", decoded["key1"])
	assert.InEpsilon(t, float64(123), decoded["key2"], 0.001)
}

func TestEventBuilder_ActionContext(t *testing.T) {
	logger := zap.NewNop()
	reporter, _ := New(context.Background(), Config{Enabled: false}, logger)

	event := reporter.NewEvent("api_call").
		Action(DomainExternal, TypeAPI, OpCall, "/api/users").
		ActionSuccess(100).
		ActionMetadata(map[string]any{"method": "POST"}).
		Build()

	ctx := event.GetActionContext()
	require.NotNil(t, ctx)
	assert.Equal(t, DomainExternal, ctx.GetDomain())
	assert.Equal(t, TypeAPI, ctx.GetType())
	assert.Equal(t, OpCall, ctx.GetOperation())
	assert.Equal(t, "/api/users", ctx.GetTarget())
	assert.True(t, ctx.GetSuccess())
	assert.Equal(t, int64(100), ctx.GetDurationMs())
}

func TestEventBuilder_ActionFailed(t *testing.T) {
	logger := zap.NewNop()
	reporter, _ := New(context.Background(), Config{Enabled: false}, logger)

	testErr := errors.New("connection timeout")
	event := reporter.NewEvent("api_call").
		Action(DomainExternal, TypeAPI, OpCall, "/api/users").
		ActionFailed(200, testErr).
		Build()

	ctx := event.GetActionContext()
	require.NotNil(t, ctx)
	assert.False(t, ctx.GetSuccess())
	assert.Equal(t, int64(200), ctx.GetDurationMs())
	assert.Equal(t, "connection timeout", ctx.GetError())
}

func TestEventBuilder_Attributes(t *testing.T) {
	logger := zap.NewNop()
	reporter, _ := New(context.Background(), Config{Enabled: false}, logger)

	event := reporter.NewEvent("test").
		Attributes(map[string]any{"key1": "value1"}).
		Attr("key2", 123).
		Build()

	attrs := event.GetEventAttributes()
	require.NotNil(t, attrs)
	assert.Equal(t, "value1", attrs.GetFields()["key1"].GetStringValue())
	assert.InEpsilon(t, float64(123), attrs.GetFields()["key2"].GetNumberValue(), 0.001)
}

func TestEventBuilder_FromContext(t *testing.T) {
	logger := zap.NewNop()
	reporter, _ := New(context.Background(), Config{Enabled: false}, logger)

	ctx := context.Background()
	ctx = WithTraceID(ctx, "trace-123")
	ctx = WithUserID(ctx, "user-456")

	event := reporter.NewEvent("test").
		FromContext(ctx).
		Build()

	assert.Equal(t, "trace-123", event.GetTraceId())
	assert.Equal(t, "user-456", event.GetUserId())
}

// ============ ServerContext 构建器测试 ============

func TestServerContextBuilder(t *testing.T) {
	ctx := NewServerContext().
		IP("192.168.1.1").
		UserAgent("Mozilla/5.0").
		Geo("CN", "Beijing", "Beijing").
		ServerNode("node-1").
		ProcessLatency(50).
		Build()

	assert.Equal(t, "192.168.1.1", ctx.GetIp())
	assert.Equal(t, "Mozilla/5.0", ctx.GetUserAgent())
	assert.Equal(t, "CN", ctx.GetGeoCountry())
	assert.Equal(t, "Beijing", ctx.GetGeoRegion())
	assert.Equal(t, "Beijing", ctx.GetGeoCity())
	assert.Equal(t, "node-1", ctx.GetServerNode())
	assert.Equal(t, int64(50), ctx.GetProcessLatencyMs())
}

func TestServerContextBuilder_FromHTTPRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 192.168.1.1")
	req.Header.Set("User-Agent", "TestClient/1.0")

	ctx := NewServerContext().
		FromHTTPRequest(req).
		Build()

	assert.Equal(t, "10.0.0.1", ctx.GetIp())
	assert.Equal(t, "TestClient/1.0", ctx.GetUserAgent())
}

func TestServerContextBuilder_FromHTTPRequest_XRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Real-IP", "192.168.1.100")
	req.Header.Set("User-Agent", "TestClient/1.0")

	ctx := NewServerContext().
		FromHTTPRequest(req).
		Build()

	assert.Equal(t, "192.168.1.100", ctx.GetIp())
}

// ============ ActionContext 构建器测试 ============

func TestActionContextBuilder(t *testing.T) {
	ctx := NewActionContext().
		Domain(DomainExternal).
		Type(TypeAPI).
		Operation(OpCall).
		Target("/api/users").
		Success(100 * time.Millisecond).
		Metadata(map[string]any{"method": "GET"}).
		Build()

	assert.Equal(t, DomainExternal, ctx.GetDomain())
	assert.Equal(t, TypeAPI, ctx.GetType())
	assert.Equal(t, OpCall, ctx.GetOperation())
	assert.Equal(t, "/api/users", ctx.GetTarget())
	assert.True(t, ctx.GetSuccess())
	assert.Equal(t, int64(100), ctx.GetDurationMs())
}

func TestActionContextBuilder_Failed(t *testing.T) {
	testErr := errors.New("database error")
	ctx := NewActionContext().
		Domain(DomainExternal).
		Type(TypeDatabase).
		Failed(50*time.Millisecond, testErr).
		Build()

	assert.False(t, ctx.GetSuccess())
	assert.Equal(t, int64(50), ctx.GetDurationMs())
	assert.Equal(t, "database error", ctx.GetError())
}

// ============ 预定义 ActionContext 工厂函数测试 ============

func TestAPICallAction(t *testing.T) {
	ctx := APICallAction("POST", "/api/users", 201, 150*time.Millisecond)

	assert.Equal(t, DomainExternal, ctx.GetDomain())
	assert.Equal(t, TypeAPI, ctx.GetType())
	assert.Equal(t, OpCall, ctx.GetOperation())
	assert.Equal(t, "/api/users", ctx.GetTarget())
	assert.True(t, ctx.GetSuccess())
	assert.Equal(t, int64(150), ctx.GetDurationMs())
}

func TestAPICallAction_Failure(t *testing.T) {
	ctx := APICallAction("GET", "/api/error", 500, 200*time.Millisecond)

	assert.False(t, ctx.GetSuccess())
}

func TestDatabaseAction(t *testing.T) {
	ctx := DatabaseAction("INSERT", "users", 1, 10*time.Millisecond)

	assert.Equal(t, DomainExternal, ctx.GetDomain())
	assert.Equal(t, TypeDatabase, ctx.GetType())
	assert.Equal(t, OpQuery, ctx.GetOperation())
	assert.Equal(t, "users", ctx.GetTarget())
	assert.True(t, ctx.GetSuccess())
}

func TestFunctionAction(t *testing.T) {
	ctx := FunctionAction("processOrder", true, 50*time.Millisecond)

	assert.Equal(t, DomainInternal, ctx.GetDomain())
	assert.Equal(t, TypeFunction, ctx.GetType())
	assert.Equal(t, OpExecute, ctx.GetOperation())
	assert.Equal(t, "processOrder", ctx.GetTarget())
	assert.True(t, ctx.GetSuccess())
}

func TestCacheAction(t *testing.T) {
	ctx := CacheAction(OpGet, "user:123", true, 1*time.Millisecond)

	assert.Equal(t, DomainExternal, ctx.GetDomain())
	assert.Equal(t, TypeCache, ctx.GetType())
	assert.Equal(t, OpGet, ctx.GetOperation())
}

func TestRPCAction(t *testing.T) {
	ctx := RPCAction("UserService", "GetUser", true, 30*time.Millisecond)

	assert.Equal(t, DomainExternal, ctx.GetDomain())
	assert.Equal(t, TypeRPC, ctx.GetType())
	assert.Equal(t, OpCall, ctx.GetOperation())
	assert.Equal(t, "UserService/GetUser", ctx.GetTarget())
}

// ============ ClientContext 构建器测试 ============

func TestClientContextBuilder(t *testing.T) {
	ctx := NewClientContext().
		DeviceID("device-123").
		OS("ios", "17.0").
		Device("Apple", "iPhone 15 Pro").
		App("com.example.app", "2.0.0").
		Network("WiFi").
		Carrier("China Mobile").
		Locale("zh_CN", "Asia/Shanghai").
		CountryCode("CN").
		Screen("1920x1080", 326).
		Build()

	assert.Equal(t, "device-123", ctx.GetDeviceId())
	assert.Equal(t, "ios", ctx.GetOsName())
	assert.Equal(t, "17.0", ctx.GetOsVersion())
	assert.Equal(t, "Apple", ctx.GetDeviceBrand())
	assert.Equal(t, "iPhone 15 Pro", ctx.GetDeviceModel())
	assert.Equal(t, "com.example.app", ctx.GetAppId())
	assert.Equal(t, "2.0.0", ctx.GetAppVersion())
	assert.Equal(t, "WiFi", ctx.GetNetworkType())
	assert.Equal(t, "China Mobile", ctx.GetCarrier())
	assert.Equal(t, "zh_CN", ctx.GetLocale())
	assert.Equal(t, "Asia/Shanghai", ctx.GetTimeZone())
	assert.Equal(t, "CN", ctx.GetCountryCode())
	assert.Equal(t, "1920x1080", ctx.GetScreenResolution())
	assert.Equal(t, int32(326), ctx.GetScreenDpi())
}

// ============ PayloadBuilder 测试 ============

func TestPayloadBuilder(t *testing.T) {
	payload := NewPayload().
		Set("key1", "value1").
		Set("key2", 123).
		Merge(map[string]any{"key3": true}).
		Build()

	assert.Equal(t, "value1", payload["key1"])
	assert.Equal(t, 123, payload["key2"])
	assert.Equal(t, true, payload["key3"])
}

// ============ Context 提取器测试 ============

func TestContextExtractor(t *testing.T) {
	ctx := context.Background()
	ctx = WithTraceID(ctx, "trace-abc")
	ctx = WithUserID(ctx, "user-xyz")
	ctx = WithSessionID(ctx, "session-123")

	assert.Equal(t, "trace-abc", extractTraceID(ctx))
	assert.Equal(t, "user-xyz", extractUserID(ctx))
	assert.Equal(t, "session-123", extractSessionID(ctx))
}

func TestContextExtractor_EmptyContext(t *testing.T) {
	ctx := context.Background()

	assert.Empty(t, extractTraceID(ctx))
	assert.Empty(t, extractUserID(ctx))
	assert.Empty(t, extractSessionID(ctx))
}

// ============ 常量测试 ============

func TestConstants(t *testing.T) {
	// 验证常量定义正确（大写格式）
	assert.Equal(t, "USER_LOGIN", EventTypeUserLogin)
	assert.Equal(t, "INTERNAL", DomainInternal)
	assert.Equal(t, "EXTERNAL", DomainExternal)
	assert.Equal(t, "API", TypeAPI)
	assert.Equal(t, "DATABASE", TypeDatabase)
	assert.Equal(t, "CALL", OpCall)
}
