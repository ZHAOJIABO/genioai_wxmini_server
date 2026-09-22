## 1. 基础设施

- [x] 1.1 创建 `internal/service/event_reporter/` 包目录结构
- [x] 1.2 定义 `Config` 配置结构体（含所有配置项）
- [x] 1.3 在 `conf/` 中添加 event_sink 相关配置项
- [x] 1.4 编写配置加载和默认值逻辑

## 2. 核心实现

- [x] 2.1 定义 `EventReporter` 接口
- [x] 2.2 实现 `reporter` 结构体（包含 channel、gRPC client、状态管理）
- [x] 2.3 实现 `New()` 构造函数（初始化 gRPC 连接、启动 worker）
- [x] 2.4 实现 `Track()` 方法（非阻塞发送到 channel，异步批量上报）
- [x] 2.5 实现 `TrackNow()` 方法（同步阻塞，调用 IngestEvent 立即发送）
- [x] 2.6 实现 flush worker goroutine（批量消费和发送）
- [x] 2.7 实现 `Flush()` 方法（手动触发批量上报）
- [x] 2.8 实现 `Shutdown()` 方法（优雅关闭）
- [x] 2.9 实现 `QueueLen()` 方法（队列长度监控）

## 3. 事件构建器（EventBuilder）

- [x] 3.1 实现 `EventBuilder` 结构体
- [x] 3.2 实现 `NewEvent(eventType)` 工厂方法
- [x] 3.3 实现核心字段方法：`EventID()`, `Source()`, `UserID()`, `SessionID()`, `TraceID()`, `Sequence()`
- [x] 3.4 实现时间戳方法：`OccurredAt()`, `OccurredMs()`
- [x] 3.5 实现上下文方法：`ClientContext()`, `ServerContext()`, `ActionContext()`
- [x] 3.6 实现 Action 便捷方法：`Action()`, `ActionSuccess()`, `ActionFailed()`, `ActionMetadata()`
- [x] 3.7 实现扩展属性方法：`Attributes()`, `Attr()`
- [x] 3.8 实现 Payload 方法：`Payload()`, `PayloadJSON()`
- [x] 3.9 实现 Context 提取：`FromContext()`, `FromGRPCContext()`
- [x] 3.10 实现终结方法：`Build()`, `Track()`, `TrackNow(ctx)`

## 4. Context 构建器

### 4.1 ServerContext 构建器
- [x] 4.1.1 实现 `ServerContextBuilder` 结构体
- [x] 4.1.2 实现基础方法：`IP()`, `UserAgent()`, `Geo()`, `ServerNode()`, `ProcessLatency()`
- [x] 4.1.3 实现 `FromHTTPRequest()` 方法（从 HTTP 请求提取）
- [x] 4.1.4 实现 `FromGRPCPeer()` 方法（从 gRPC peer 提取）
- [x] 4.1.5 实现 `Build()` 方法

### 4.2 ActionContext 构建器
- [x] 4.2.1 实现 `ActionContextBuilder` 结构体
- [x] 4.2.2 实现基础方法：`Domain()`, `Type()`, `Operation()`, `Target()`
- [x] 4.2.3 实现结果方法：`Success()`, `Failed()`, `Metadata()`
- [x] 4.2.4 实现 `Build()` 方法
- [x] 4.2.5 实现预定义工厂函数：`APICallAction()`, `DatabaseAction()`, `FunctionAction()`, `CacheAction()`, `RPCAction()`

### 4.3 ClientContext 构建器
- [x] 4.3.1 实现 `ClientContextBuilder` 结构体
- [x] 4.3.2 实现方法：`DeviceID()`, `OS()`, `Device()`, `App()`, `Network()`, `Locale()`, `Screen()`
- [x] 4.3.3 实现 `Build()` 方法

### 4.4 Payload 构建器（可选）
- [x] 4.4.1 实现 `PayloadBuilder` 结构体
- [x] 4.4.2 实现方法：`Set()`, `Merge()`, `Build()`, `JSON()`

## 5. 辅助功能

- [x] 5.1 实现事件 ID 自动生成（UUID）
- [x] 5.2 实现时间戳自动填充（occurred_ms, sent_ms）
- [x] 5.3 实现 map 到 structpb.Struct 转换
- [x] 5.4 实现 ContextExtractor 接口（可扩展）
- [x] 5.5 实现 EventInterceptor 机制 ✅ 已实现（DefaultInterceptor、InterceptorChain、SamplingInterceptor、FilterInterceptor）
- [x] 5.6 添加日志记录（启动、关闭、错误、丢弃事件）
- [ ] 5.7 添加基础 metrics（可选，如队列长度、发送计数）- 预留扩展点

## 6. 预定义常量

- [x] 6.1 定义事件类型常量（EventTypeUserLogin 等）
- [x] 6.2 定义 ActionContext Domain 常量（DomainInternal, DomainExternal）
- [x] 6.3 定义 ActionContext Type 常量（TypeAPI, TypeDatabase 等）
- [x] 6.4 定义 ActionContext Operation 常量（OpCall, OpQuery 等）

## 7. 集成

- [x] 7.1 在 `bootstrap/service_provider.go` 中初始化 EventReporter
- [x] 7.2 将 EventReporter 注入到需要使用的服务中（通过 ServiceProvider 和 ServerDeps）
- [x] 7.3 在程序退出流程中调用 `Shutdown()` ✅ 已在 cmd/main.go 中添加
- [x] 7.4 添加 NoopReporter（用于测试或禁用模式）

## 8. 测试

- [x] 8.1 编写 EventBuilder 单元测试（字段设置正确性）
- [x] 8.2 编写 Context Builder 单元测试
- [x] 8.3 编写 EventReporter 单元测试（NoopReporter）
- [x] 8.4 测试批量触发条件（数量阈值、时间阈值）✅ FakeEventServiceServer 实现
- [x] 8.5 测试 TrackNow 实时上报（成功、失败、超时）✅
- [x] 8.6 测试 Flush 手动触发批量上报 ✅
- [x] 8.7 测试队列满时的丢弃行为 ✅
- [x] 8.8 测试优雅关闭流程 ✅
- [x] 8.9 测试 FromContext/FromGRPCContext 提取逻辑

## 9. 文档与示例

- [x] 9.1 编写使用示例代码 ✅ doc/event_reporter/EXAMPLES.md
- [x] 9.2 更新 AGENTS.md 或相关文档 ✅ 已添加 EventReporter 文档索引

---

## 实现说明

### 已完成的功能

1. **基础设施**: 完整的包结构、配置管理、默认值支持
2. **核心功能**: EventReporter 接口及实现，支持异步批量上报和同步实时上报
3. **EventBuilder**: 完整的链式构建器，支持所有 event_sink 字段
4. **Context 构建器**: ServerContext、ActionContext、ClientContext、PayloadBuilder
5. **预定义常量**: 事件类型、Domain、Type、Operation 常量
6. **集成**: ServiceProvider 初始化和依赖注入

### 预留扩展点

1. **EventInterceptor**: 接口已设计，可在后续需要时实现
2. **Metrics**: 可在后续添加 Prometheus 指标
3. **Shutdown 调用**: 需要在 main.go 的优雅关闭流程中添加

### 测试覆盖

- 基础测试覆盖率: 57.1%
- 覆盖了 Builder、Context、常量等核心功能
- gRPC 相关测试需要 mock server，暂未实现

---

## 依赖关系

```
1.x (基础设施)
  └─> 2.x (核心实现)
        ├─> 3.x (EventBuilder) ─┬─> 4.x (Context Builders)
        │                       └─> 5.x (辅助功能)
        └─> 6.x (预定义常量)

7.x (集成) 依赖 2.x, 3.x, 4.x 完成
8.x (测试) 依赖对应模块完成
```

## 可并行任务

- 3.x 和 4.x 可在 2.2 完成后并行开发
- 4.1.x、4.2.x、4.3.x 可并行开发
- 5.x 可与 3.x、4.x 并行开发
- 6.x 可随时开发
- 8.x 各测试可在对应模块完成后并行执行

## 实现优先级

**Phase 1（核心功能）**：1.x → 2.x → 3.1-3.4, 3.8, 3.10 ✅

**Phase 2（上下文支持）**：3.5-3.7 → 4.1.x → 4.2.x → 4.3.x ✅

**Phase 3（增强功能）**：3.9 → 5.x → 6.x ✅

**Phase 4（集成测试）**：7.x → 8.x → 9.x ✅（部分完成）
