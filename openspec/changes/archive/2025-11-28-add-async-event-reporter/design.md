## Context

### 背景
va_visionai_server 需要向独立的 event_sink 服务上报各类业务埋点事件。event_sink 提供 HTTP 和 gRPC 两种接入方式，考虑到性能和类型安全，选择使用 gRPC。

### 约束
- 事件上报**不能阻塞**主业务流程
- 进程**正常关闭**时应尽可能刷新队列中的事件
- 内存占用需有上限（队列满时需要处理策略）
- 简单可维护，避免过度设计
- **必须完整支持 event_sink API 文档中定义的所有字段**

### 利益相关者
- 后端开发团队：使用此客户端进行埋点
- 数据团队：消费 event_sink 中的事件数据

---

## Goals / Non-Goals

### Goals
1. 提供简洁易用的事件上报 API
2. 异步非阻塞，不影响主流程延迟
3. 支持批量上报，减少网络开销
4. 优雅关闭，进程退出时刷新缓冲区
5. **提供完整的事件构建器，支持所有 event_sink 字段**
6. **提供 Context 辅助函数（ServerContext、ActionContext）**

### Non-Goals
1. 不实现持久化队列（磁盘/Redis）
2. 不实现复杂重试策略（指数退避、死信队列等）
3. 不保证 100% 不丢事件（进程 crash 时可能丢失）

---

## Decisions

### Decision 1: 使用内存 Channel 作为本地队列

**选择**: 使用带缓冲的 Go channel

**理由**:
- Go channel 原生支持，无需引入第三方依赖
- 性能优秀，适合高并发场景
- 可通过 `select` 实现超时和取消

**替代方案考虑**:
- Ring Buffer: 更复杂，收益不明显
- Redis Queue: 增加外部依赖，违背简单原则
- 磁盘持久化: 复杂度高，不符合当前需求

### Decision 2: 批量上报触发策略

**选择**: 双条件触发（数量阈值 OR 时间阈值，先到先触发）

**默认配置**:
- `BatchSize`: 100 条
- `FlushInterval`: 5 秒

**理由**:
- 高流量时按数量批量，减少请求次数
- 低流量时按时间触发，确保事件及时上报
- 两种策略互补，覆盖不同场景

### Decision 3: 队列满时的处理策略

**选择**: 非阻塞发送，队列满时丢弃并记录日志

**理由**:
- 避免因上报阻塞导致主流程延迟
- 埋点数据允许少量丢失
- 通过监控告警发现队列积压问题

### Decision 4: gRPC 连接管理

**选择**: 单例长连接，由 EventReporter 内部管理

**理由**:
- gRPC 支持连接复用和 HTTP/2 多路复用
- 避免频繁建立连接的开销
- 连接断开时自动重连（gRPC 内置）

### Decision 5: 优雅关闭机制

**选择**: Context 取消 + Shutdown 方法

**流程**:
1. 调用 `Shutdown(ctx)` 或 Context 被取消
2. 停止接收新事件
3. 设置 Flush 超时（默认 10 秒）
4. 将队列中剩余事件批量发送
5. 关闭 gRPC 连接

### Decision 6: 事件构建器模式（Builder Pattern）

**选择**: 提供链式调用的 EventBuilder

**理由**:
- 事件字段较多，Builder 模式提供更好的可读性
- 支持可选字段的灵活组合
- 编译时类型检查，减少运行时错误
- 便于后续扩展新字段

### Decision 7: Context 从 gRPC/HTTP 请求中自动提取

**选择**: 提供 `FromContext(ctx)` 系列函数，自动提取 trace_id、user_id 等

**理由**:
- 减少业务代码重复
- 确保链路追踪信息一致
- 与现有 context 传递机制集成

---

## Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                        va_visionai_server                       │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│   ┌──────────────┐                  ┌──────────────────────┐   │
│   │ Business     │  EventBuilder    │   EventReporter      │   │
│   │ Service      │─────────────────>│                      │   │
│   └──────────────┘  .Build().Track()│  ┌────────────────┐  │   │
│         │                           │  │ Event Channel  │  │   │
│         │ NewEventBuilder()         │  │ (buffered)     │  │   │
│         ▼                           │  └───────┬────────┘  │   │
│   ┌──────────────┐                  │          │           │   │
│   │ EventBuilder │                  │          ▼           │   │
│   │ .EventType() │                  │  ┌────────────────┐  │   │
│   │ .UserID()    │                  │  │ Flush Worker   │  │   │
│   │ .Action()    │                  │  │ (goroutine)    │  │   │
│   │ .Payload()   │                  │  └───────┬────────┘  │   │
│   │ .Build()     │                  └──────────┼───────────┘   │
│   └──────────────┘                             │               │
│                                                │               │
└────────────────────────────────────────────────┼───────────────┘
                                                 │ gRPC
                                                 ▼
┌────────────────────────────────────────────────────────────────┐
│                        event_sink                              │
│                                                                │
│   EventService.IngestBatch(events []*Event)                    │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

---

## API Design

### 核心接口

```go
// EventReporter 事件上报客户端
type EventReporter interface {
    // ============ 异步批量上报（默认模式）============

    // Track 异步上报单个事件（非阻塞，入队等待批量发送）
    // 适用于：普通埋点事件、非关键业务事件
    Track(event *eventsv1.Event)

    // ============ 实时同步上报 ============

    // TrackNow 实时上报单个事件（同步阻塞，立即发送）
    // 适用于：重要事件、关键业务节点、需要确认发送成功的场景
    // 返回 error 表示发送是否成功
    TrackNow(ctx context.Context, event *eventsv1.Event) error

    // ============ 构建器 ============

    // NewEvent 创建事件构建器
    NewEvent(eventType string) *EventBuilder

    // ============ 队列控制 ============

    // Flush 立即刷新队列中的事件（将队列中所有事件批量发送）
    // 适用于：主动触发批量上报、程序即将退出前
    Flush(ctx context.Context) error

    // Shutdown 优雅关闭（停止接收新事件，刷新队列，关闭连接）
    Shutdown(ctx context.Context) error

    // QueueLen 返回当前队列长度（用于监控）
    QueueLen() int
}
```

### 事件构建器（完整支持 event_sink 所有字段）

```go
// EventBuilder 链式事件构建器
type EventBuilder struct {
    reporter *reporter
    event    *eventsv1.Event
}

// ============ 核心标识字段 ============

// EventID 设置事件ID（可选，不设置则自动生成 UUID）
func (b *EventBuilder) EventID(id string) *EventBuilder

// EventType 设置事件类型（必填，已在 NewEvent 中设置）
// 无需再调用，除非要覆盖

// Source 设置事件来源（可选，默认使用配置的 source）
func (b *EventBuilder) Source(source string) *EventBuilder

// ============ 用户与会话 ============

// UserID 设置用户ID
func (b *EventBuilder) UserID(userID string) *EventBuilder

// SessionID 设置会话ID
func (b *EventBuilder) SessionID(sessionID string) *EventBuilder

// TraceID 设置链路追踪ID
func (b *EventBuilder) TraceID(traceID string) *EventBuilder

// Sequence 设置事件序号（会话内递增）
func (b *EventBuilder) Sequence(seq int64) *EventBuilder

// ============ 时间戳 ============

// OccurredAt 设置事件发生时间（可选，默认 time.Now()）
func (b *EventBuilder) OccurredAt(t time.Time) *EventBuilder

// OccurredMs 直接设置毫秒时间戳
func (b *EventBuilder) OccurredMs(ms int64) *EventBuilder

// ============ 上下文信息 ============

// ClientContext 设置客户端上下文
func (b *EventBuilder) ClientContext(ctx *eventsv1.ClientContext) *EventBuilder

// ServerContext 设置服务端上下文
func (b *EventBuilder) ServerContext(ctx *eventsv1.ServerContext) *EventBuilder

// ActionContext 设置动作上下文
func (b *EventBuilder) ActionContext(ctx *eventsv1.ActionContext) *EventBuilder

// Action 便捷方法：快速设置 ActionContext
func (b *EventBuilder) Action(domain, actionType, operation, target string) *EventBuilder

// ActionSuccess 设置动作成功
func (b *EventBuilder) ActionSuccess(durationMs int64) *EventBuilder

// ActionFailed 设置动作失败
func (b *EventBuilder) ActionFailed(durationMs int64, err error) *EventBuilder

// ActionMetadata 设置动作元数据
func (b *EventBuilder) ActionMetadata(metadata map[string]any) *EventBuilder

// ============ 扩展属性 ============

// Attributes 设置事件扩展属性
func (b *EventBuilder) Attributes(attrs map[string]any) *EventBuilder

// Attr 添加单个扩展属性
func (b *EventBuilder) Attr(key string, value any) *EventBuilder

// ============ 业务数据 ============

// Payload 设置事件负载（必填）
func (b *EventBuilder) Payload(payload map[string]any) *EventBuilder

// PayloadJSON 从 JSON bytes 设置 payload
func (b *EventBuilder) PayloadJSON(jsonBytes []byte) *EventBuilder

// ============ Context 自动提取 ============

// FromContext 从 context.Context 自动提取 trace_id、user_id 等
func (b *EventBuilder) FromContext(ctx context.Context) *EventBuilder

// FromGRPCContext 从 gRPC incoming context 提取信息
func (b *EventBuilder) FromGRPCContext(ctx context.Context) *EventBuilder

// ============ 构建与发送 ============

// Build 构建事件（返回 *eventsv1.Event，不发送）
func (b *EventBuilder) Build() *eventsv1.Event

// Track 构建并异步发送事件（入队，非阻塞）
// 适用于：普通事件
func (b *EventBuilder) Track()

// TrackNow 构建并实时发送事件（同步阻塞，立即发送）
// 适用于：重要事件，需要确认发送成功
func (b *EventBuilder) TrackNow(ctx context.Context) error
```

### Context 构建辅助函数

```go
// ============ ServerContext 构建 ============

// NewServerContext 创建服务端上下文
func NewServerContext() *ServerContextBuilder

type ServerContextBuilder struct {
    ctx *eventsv1.ServerContext
}

// IP 设置客户端IP
func (b *ServerContextBuilder) IP(ip string) *ServerContextBuilder

// UserAgent 设置 User-Agent
func (b *ServerContextBuilder) UserAgent(ua string) *ServerContextBuilder

// Geo 设置地理位置
func (b *ServerContextBuilder) Geo(country, region, city string) *ServerContextBuilder

// ServerNode 设置服务节点标识
func (b *ServerContextBuilder) ServerNode(node string) *ServerContextBuilder

// ProcessLatency 设置处理延迟
func (b *ServerContextBuilder) ProcessLatency(ms int64) *ServerContextBuilder

// FromHTTPRequest 从 HTTP 请求中提取（IP、User-Agent）
func (b *ServerContextBuilder) FromHTTPRequest(r *http.Request) *ServerContextBuilder

// FromGRPCPeer 从 gRPC peer 中提取 IP
func (b *ServerContextBuilder) FromGRPCPeer(ctx context.Context) *ServerContextBuilder

// Build 构建
func (b *ServerContextBuilder) Build() *eventsv1.ServerContext


// ============ ActionContext 构建 ============

// NewActionContext 创建动作上下文
func NewActionContext() *ActionContextBuilder

type ActionContextBuilder struct {
    ctx *eventsv1.ActionContext
}

// Domain 设置动作领域（internal/external）
func (b *ActionContextBuilder) Domain(domain string) *ActionContextBuilder

// Type 设置动作类型（function/api/database/cache/queue/rpc）
func (b *ActionContextBuilder) Type(actionType string) *ActionContextBuilder

// Operation 设置动作操作（execute/call/query/command/publish/consume）
func (b *ActionContextBuilder) Operation(op string) *ActionContextBuilder

// Target 设置动作目标
func (b *ActionContextBuilder) Target(target string) *ActionContextBuilder

// Success 设置成功状态
func (b *ActionContextBuilder) Success(duration time.Duration) *ActionContextBuilder

// Failed 设置失败状态
func (b *ActionContextBuilder) Failed(duration time.Duration, err error) *ActionContextBuilder

// Metadata 设置元数据
func (b *ActionContextBuilder) Metadata(m map[string]any) *ActionContextBuilder

// Build 构建
func (b *ActionContextBuilder) Build() *eventsv1.ActionContext


// ============ 预定义 ActionContext 工厂函数 ============

// APICallAction 创建 API 调用类型的 ActionContext
func APICallAction(method, endpoint string, statusCode int, duration time.Duration) *eventsv1.ActionContext

// DatabaseAction 创建数据库操作类型的 ActionContext
func DatabaseAction(queryType, table string, rowsAffected int, duration time.Duration) *eventsv1.ActionContext

// FunctionAction 创建函数执行类型的 ActionContext
func FunctionAction(funcName string, success bool, duration time.Duration) *eventsv1.ActionContext

// CacheAction 创建缓存操作类型的 ActionContext
func CacheAction(operation, key string, hit bool, duration time.Duration) *eventsv1.ActionContext

// RPCAction 创建 RPC 调用类型的 ActionContext
func RPCAction(service, method string, success bool, duration time.Duration) *eventsv1.ActionContext
```

### ClientContext 构建辅助函数

```go
// NewClientContext 创建客户端上下文构建器
func NewClientContext() *ClientContextBuilder

type ClientContextBuilder struct {
    ctx *eventsv1.ClientContext
}

// DeviceID 设置设备ID
func (b *ClientContextBuilder) DeviceID(id string) *ClientContextBuilder

// OS 设置操作系统信息
func (b *ClientContextBuilder) OS(name, version string) *ClientContextBuilder

// Device 设置设备信息
func (b *ClientContextBuilder) Device(brand, model string) *ClientContextBuilder

// App 设置应用信息
func (b *ClientContextBuilder) App(id, version string) *ClientContextBuilder

// Network 设置网络类型
func (b *ClientContextBuilder) Network(networkType string) *ClientContextBuilder

// Locale 设置语言区域
func (b *ClientContextBuilder) Locale(locale, timezone string) *ClientContextBuilder

// Screen 设置屏幕信息
func (b *ClientContextBuilder) Screen(resolution string, dpi int) *ClientContextBuilder

// Build 构建
func (b *ClientContextBuilder) Build() *eventsv1.ClientContext
```

### Payload 构建辅助函数

```go
// PayloadBuilder Payload 构建器
type PayloadBuilder struct {
    data map[string]any
}

// NewPayload 创建 Payload 构建器
func NewPayload() *PayloadBuilder

// Set 设置键值对
func (b *PayloadBuilder) Set(key string, value any) *PayloadBuilder

// Merge 合并 map
func (b *PayloadBuilder) Merge(m map[string]any) *PayloadBuilder

// Build 构建为 map
func (b *PayloadBuilder) Build() map[string]any

// JSON 构建为 JSON bytes
func (b *PayloadBuilder) JSON() []byte
```

---

## 使用示例

### 示例 1：普通事件（异步批量上报）

```go
// 用户登录事件 - 普通埋点，入队等待批量发送
reporter.NewEvent("user_login").
    UserID(userID).
    FromContext(ctx).  // 自动提取 trace_id 等
    Payload(map[string]any{
        "login_method": "password",
        "remember_me":  true,
    }).
    Track()  // 非阻塞，立即返回
```

### 示例 2：重要事件（实时同步上报）

```go
// 支付成功事件 - 重要！需要立即上报并确认成功
err := reporter.NewEvent("payment_success").
    UserID(userID).
    FromContext(ctx).
    Payload(map[string]any{
        "order_id":   orderID,
        "amount":     amount,
        "currency":   "CNY",
        "payment_method": "alipay",
    }).
    TrackNow(ctx)  // 同步阻塞，立即发送，返回 error

if err != nil {
    // 上报失败，可记录本地日志或重试
    log.Error("failed to track payment event", zap.Error(err))
}
```

### 示例 3：主动触发批量上报

```go
// 场景：在某个业务节点后，希望立即将队列中的事件发送出去
// 比如：用户完成一系列操作后

reporter.NewEvent("step_1").UserID(userID).Payload(...).Track()
reporter.NewEvent("step_2").UserID(userID).Payload(...).Track()
reporter.NewEvent("step_3").UserID(userID).Payload(...).Track()

// 主动触发批量上报，不等待阈值
if err := reporter.Flush(ctx); err != nil {
    log.Warn("flush events failed", zap.Error(err))
}
```

### 示例 4：带 ActionContext 的 API 调用事件

```go
// 记录外部 API 调用
start := time.Now()
resp, err := httpClient.Do(req)
duration := time.Since(start)

reporter.NewEvent("external_api_call").
    UserID(userID).
    FromContext(ctx).
    ActionContext(APICallAction("POST", "/api/v1/payment", resp.StatusCode, duration)).
    Payload(map[string]any{
        "api":     "payment_service",
        "action":  "create_order",
        "success": err == nil,
    }).
    Track()
```

### 示例 5：数据库操作事件

```go
start := time.Now()
result := db.Create(&user)
duration := time.Since(start)

reporter.NewEvent("database_operation").
    FromContext(ctx).
    ActionContext(DatabaseAction("INSERT", "users", 1, duration)).
    Payload(map[string]any{
        "table": "users",
        "operation": "create",
    }).
    Track()
```

### 示例 6：完整构建（所有字段）

```go
reporter.NewEvent("complex_event").
    EventID("evt-custom-id").
    UserID(userID).
    SessionID(sessionID).
    TraceID(traceID).
    Sequence(seq).
    OccurredAt(eventTime).
    Source("custom-source").
    ClientContext(NewClientContext().
        OS("ios", "17.0").
        Device("Apple", "iPhone 15 Pro").
        App("com.example.app", "2.0.0").
        Build()).
    ServerContext(NewServerContext().
        IP(clientIP).
        UserAgent(userAgent).
        ServerNode(hostname).
        Build()).
    ActionContext(NewActionContext().
        Domain("external").
        Type("api").
        Operation("call").
        Target("/api/v1/users").
        Success(235 * time.Millisecond).
        Metadata(map[string]any{
            "method": "POST",
            "status_code": 200,
        }).
        Build()).
    Attributes(map[string]any{
        "experiment_id": "exp-123",
        "ab_group": "treatment",
    }).
    Payload(map[string]any{
        "action": "create_user",
        "result": "success",
    }).
    Track()
```

---

## 配置项

```go
type Config struct {
    // gRPC 服务地址（必填）
    Addr string

    // 队列容量（默认 10000）
    QueueSize int

    // 批量大小阈值（默认 100）
    BatchSize int

    // 刷新间隔（默认 5s）
    FlushInterval time.Duration

    // 关闭超时（默认 10s）
    ShutdownTimeout time.Duration

    // 事件来源标识（默认 "va_visionai_server"）
    Source string

    // 服务节点标识（用于 ServerContext，默认取 hostname）
    ServerNode string

    // 是否启用（默认 true，设为 false 时 Track 为空操作）
    Enabled bool
}
```

---

## 扩展性设计

### 1. 插件式 Context 提取器

```go
// ContextExtractor 接口，支持自定义 context 提取逻辑
type ContextExtractor interface {
    ExtractUserID(ctx context.Context) string
    ExtractTraceID(ctx context.Context) string
    ExtractSessionID(ctx context.Context) string
}

// 默认实现从 context.Value 提取
// 可替换为从 metadata、header 等提取
```

### 2. 事件拦截器/中间件

```go
// EventInterceptor 事件拦截器，可用于：
// - 添加公共字段
// - 过滤敏感数据
// - 采样
type EventInterceptor func(event *eventsv1.Event) *eventsv1.Event

func (r *reporter) Use(interceptors ...EventInterceptor)
```

### 3. 预定义事件类型

```go
// 常用事件类型常量
const (
    EventTypeUserLogin    = "user_login"
    EventTypeUserLogout   = "user_logout"
    EventTypeAPICall      = "api_call"
    EventTypeDatabaseOp   = "database_operation"
    EventTypeCacheOp      = "cache_operation"
    EventTypeTaskCreated  = "task_created"
    EventTypeTaskComplete = "task_completed"
    // ...
)
```

---

## Risks / Trade-offs

### Risk 1: 进程 Crash 时事件丢失
- **影响**: 内存队列中的事件会丢失
- **缓解**: 这是可接受的，埋点数据允许少量丢失；如果需要更高可靠性，未来可扩展为持久化队列

### Risk 2: 队列积压导致内存增长
- **影响**: 如果 event_sink 不可用，队列可能积压
- **缓解**: 设置队列容量上限，满时丢弃新事件并记录日志；添加监控指标

### Risk 3: gRPC 连接不稳定
- **影响**: 上报失败率上升
- **缓解**: gRPC 内置重连机制；失败时记录日志，不阻塞主流程

### Risk 4: Builder 链式调用可能遗漏必填字段
- **影响**: 事件被 event_sink 拒绝
- **缓解**: `Build()` 方法进行校验，缺少必填字段时记录警告日志

---

## Migration Plan

1. **新增代码**: 添加 `internal/service/event_reporter/` 包
2. **配置扩展**: 在配置文件中添加 event_sink 相关配置
3. **DI 集成**: 在 `ServiceProvider` 中初始化 EventReporter
4. **业务集成**: 在需要埋点的位置调用 `reporter.NewEvent().Track()`
5. **监控**: 添加队列长度、发送成功率等 metrics

无需数据库迁移，向后兼容。

---

## Open Questions

~~1. **事件 ID 生成策略**: 由调用方生成还是 Reporter 自动生成？~~
   - **已解决**: EventBuilder 默认自动生成 UUID，也支持通过 `EventID()` 手动设置

~~2. **是否需要优先级队列**: 某些关键事件是否需要优先上报？~~
   - **已解决**: V1 不支持，保持简单；预留 `EventInterceptor` 扩展点

3. **监控指标**: 需要暴露哪些 metrics？
   - 建议：队列长度、发送成功/失败数、批量大小分布、丢弃事件数
