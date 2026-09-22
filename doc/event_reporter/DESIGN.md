# EventReporter 技术设计

## 1. 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                        业务代码                                   │
│   reporter.NewEvent("USER_LOGIN").UserID("123").Track()         │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      EventBuilder                                │
│   链式构建 → 设置字段 → Build() 生成 Event                        │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      EventReporter                               │
│   ┌─────────────────┐    ┌─────────────────┐                    │
│   │   Interceptor   │───▶│   Event Queue   │                    │
│   │  (填充公共字段)  │    │   (chan, 10000) │                    │
│   └─────────────────┘    └─────────────────┘                    │
│                                   │                              │
│                                   ▼                              │
│                          ┌─────────────────┐                    │
│                          │  Flush Worker   │                    │
│                          │  (goroutine)    │                    │
│                          └─────────────────┘                    │
│                                   │                              │
│           ┌───────────────────────┼───────────────────────┐     │
│           │                       │                       │     │
│           ▼                       ▼                       ▼     │
│   ┌───────────────┐      ┌───────────────┐      ┌───────────┐  │
│   │ 批量 >= 100   │      │ 时间 >= 5s    │      │  Shutdown │  │
│   └───────────────┘      └───────────────┘      └───────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      gRPC Client                                 │
│   IngestEvent (单个) / IngestBatch (批量)                        │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      event_sink 服务                             │
└─────────────────────────────────────────────────────────────────┘
```

## 2. 核心组件

### 2.1 EventReporter 接口

```go
type EventReporter interface {
    // Track 异步上报（非阻塞，入队等待批量发送）
    Track(event *eventsv1.Event)

    // TrackNow 实时上报（同步阻塞，立即发送）
    TrackNow(ctx context.Context, event *eventsv1.Event) error

    // NewEvent 创建事件构建器
    NewEvent(eventType string) *EventBuilder

    // Flush 手动触发批量发送
    Flush(ctx context.Context) error

    // Shutdown 优雅关闭（发送剩余事件）
    Shutdown(ctx context.Context) error

    // QueueLen 返回队列长度（监控用）
    QueueLen() int
}
```

### 2.2 批量发送机制

**双触发条件**：
1. **数量阈值**：队列积累 >= BatchSize（默认 100）个事件
2. **时间阈值**：距离上次发送 >= FlushInterval（默认 5s）

**实现**：单个 goroutine（flushWorker）通过 select 监听：
- `eventCh`：事件入队
- `ticker.C`：定时触发
- `flushCh`：手动 Flush
- `doneCh`：关闭信号

### 2.3 Interceptor 拦截器

**用途**：在事件发送前统一处理，避免每次手动设置重复字段。

**DefaultInterceptor 自动填充**：
- `source`：事件来源（如 "va_visionai_server"）
- `occurred_ms`：事件发生时间
- `server_context.server_node`：服务节点标识

**扩展拦截器**：
- `SamplingInterceptor`：采样控制，降低高频事件上报量
- `FilterInterceptor`：条件过滤，丢弃不需要的事件
- `InterceptorChain`：组合多个拦截器

## 3. 数据流

### 3.1 异步上报流程（Track）

```
业务代码调用 Track()
    │
    ▼
applyInterceptor() ─── 填充 event_id、source、occurred_ms、server_node
    │
    ▼
eventCh <- event ─── 非阻塞入队（队列满则丢弃并记录日志）
    │
    ▼
flushWorker 消费 ─── 积累到 batch 数组
    │
    ▼
触发条件满足 ─── 数量阈值 OR 时间阈值
    │
    ▼
sendBatch() ─── 设置 sent_ms，调用 gRPC IngestBatch
```

### 3.2 同步上报流程（TrackNow）

```
业务代码调用 TrackNow(ctx)
    │
    ▼
applyInterceptor() ─── 填充公共字段
    │
    ▼
设置 sent_ms
    │
    ▼
gRPC IngestEvent() ─── 同步阻塞，立即发送
    │
    ▼
返回结果/错误
```

### 3.3 优雅关闭流程（Shutdown）

```
调用 Shutdown(ctx)
    │
    ▼
设置 shutdown 标志 ─── 拒绝新事件
    │
    ▼
close(doneCh) ─── 通知 flushWorker 停止
    │
    ▼
flushWorker 收集 channel 剩余事件
    │
    ▼
sendBatch() ─── 发送最后一批
    │
    ▼
关闭 gRPC 连接
```

## 4. 配置项

| 配置项 | 默认值 | 说明 |
|-------|-------|------|
| `enabled` | true | 是否启用（false 使用 NoopReporter）|
| `addr` | "" | event_sink gRPC 地址（空则禁用）|
| `queue_size` | 10000 | 事件队列大小 |
| `batch_size` | 100 | 批量发送数量阈值 |
| `flush_interval` | 5s | 批量发送时间阈值 |
| `shutdown_timeout` | 10s | 关闭超时时间 |
| `source` | "va_visionai_server" | 事件来源标识 |
| `server_node` | hostname | 服务节点标识 |

## 5. 错误处理

| 场景 | 处理方式 |
|-----|---------|
| 队列满 | 丢弃事件，记录 WARN 日志 |
| 批量发送失败 | 记录 ERROR 日志，不重试（避免积压）|
| TrackNow 失败 | 返回错误给调用方 |
| 关闭超时 | 返回 context.DeadlineExceeded |

## 6. 监控指标

当前通过日志记录关键事件：
- 启动/关闭日志
- 队列满丢弃事件
- 批量发送失败

预留扩展点：可添加 Prometheus 指标（队列长度、发送计数、延迟等）。

## 7. 线程安全

- `eventCh`：Go channel 天然线程安全
- `shutdown`：atomic.Bool 原子操作
- 单个 flushWorker goroutine 处理所有发送逻辑

## 8. 依赖关系

```
event_reporter
    ├── pkg/event_sink/v1 (proto 生成的 gRPC client)
    ├── go.uber.org/zap (日志)
    ├── google.golang.org/grpc (gRPC)
    └── github.com/google/uuid (事件ID生成)
```

## 9. 集成点

- **初始化**：`bootstrap/service_provider.go` 中创建 EventReporter
- **关闭**：`cmd/main.go` 的 `App.Shutdown()` 中调用 `EventReporter.Shutdown()`
- **使用**：通过 `ServiceProvider.EventReporter` 或 `ServerDeps.EventReporter` 获取
