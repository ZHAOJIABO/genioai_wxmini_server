# Change: 添加异步事件上报客户端

## Why

当前 va_visionai_server 需要向 event_sink 服务上报埋点事件。为了避免事件上报阻塞主业务流程，需要设计一个**异步、高效、健壮**的事件上报客户端。该客户端需要支持：
- 异步非阻塞上报（不影响主流程延迟）
- 本地事件队列缓冲（支持批量上报）
- 智能刷新策略（满N条或超时X秒触发上报）
- 优雅关闭（确保进程退出时不丢失事件）

## What Changes

### 新增能力
- **Event Reporter 客户端**: 封装 gRPC 客户端，提供简洁的事件上报 API
- **本地事件队列**: 基于 channel 的内存队列，缓冲待上报事件
- **批量上报机制**: 支持按数量阈值（默认 100 条）或时间阈值（默认 5 秒）触发批量发送
- **异步发送 goroutine**: 后台消费队列，批量调用 gRPC IngestBatch 接口
- **优雅关闭**: 支持 context 取消和信号通知，确保队列中事件被刷新后再退出
- **错误处理与重试**: 上报失败时的重试策略和降级处理

### 代码位置
- 新增包: `internal/service/event_reporter/` 或 `pkg/event_reporter/`
- 配置项: 新增 event_sink gRPC 地址、批量阈值、超时时间等配置

## Impact

- **Affected specs**: 新增 `event-reporter` 能力规范
- **Affected code**:
  - 新增 `internal/service/event_reporter/` 包
  - 修改 `internal/bootstrap/service_provider.go` 注册 EventReporter
  - 修改 `conf/` 配置结构，新增 event_sink 相关配置
- **Dependencies**: 复用已生成的 `pkg/event_sink/v1` gRPC 客户端

## Non-Goals

- 不实现持久化队列（磁盘或 Redis）—— 进程重启允许丢失未发送事件
- 不实现复杂的重试策略（如指数退避）—— 保持简单，失败记录日志即可
- 不修改 event_sink 服务端实现
