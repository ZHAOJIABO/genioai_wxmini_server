# event-reporter Specification

## Purpose
TBD - created by archiving change add-async-event-reporter. Update Purpose after archive.
## Requirements
### Requirement: 异步事件上报客户端

系统 SHALL 提供一个异步、非阻塞的事件上报客户端，用于向 event_sink 服务发送埋点事件。

#### Scenario: 基本事件上报（异步批量）
- **GIVEN** EventReporter 已初始化且与 event_sink 服务连接正常
- **WHEN** 业务代码调用 `Track(event)` 方法
- **THEN** 事件 SHALL 被放入本地队列，调用立即返回（非阻塞）
- **AND** 事件 SHALL 在满足触发条件后被批量发送到 event_sink

#### Scenario: 使用 EventBuilder 异步上报事件
- **GIVEN** EventReporter 已初始化
- **WHEN** 业务代码调用 `NewEvent(eventType).UserID(id).Payload(data).Track()`
- **THEN** 系统 SHALL 自动生成唯一的 event_id
- **AND** 系统 SHALL 自动填充 occurred_ms 和 sent_ms 时间戳
- **AND** 系统 SHALL 使用配置的 source 标识
- **AND** 事件 SHALL 被放入本地队列

---

### Requirement: 实时同步上报

系统 SHALL 提供实时同步上报能力，用于重要事件的立即发送。

#### Scenario: 使用 TrackNow 实时上报
- **GIVEN** EventReporter 已初始化
- **AND** event_sink 服务可用
- **WHEN** 业务代码调用 `TrackNow(ctx, event)` 方法
- **THEN** 系统 SHALL 立即通过 gRPC 发送单条事件（IngestEvent）
- **AND** 方法 SHALL 阻塞直到发送完成或超时
- **AND** 返回 error 表示发送结果

#### Scenario: 使用 EventBuilder 实时上报
- **GIVEN** EventReporter 已初始化
- **WHEN** 业务代码调用 `NewEvent(eventType).Payload(data).TrackNow(ctx)`
- **THEN** 系统 SHALL 构建事件并立即同步发送
- **AND** 返回 error 表示发送结果

#### Scenario: 实时上报失败
- **GIVEN** event_sink 服务不可用或网络异常
- **WHEN** 调用 `TrackNow(ctx, event)`
- **THEN** 方法 SHALL 返回错误
- **AND** 调用方可根据 error 决定是否重试或降级处理

#### Scenario: 实时上报超时
- **GIVEN** ctx 设置了超时时间
- **WHEN** gRPC 调用耗时超过 ctx 超时
- **THEN** 方法 SHALL 返回 context.DeadlineExceeded 错误

---

### Requirement: 事件构建器（EventBuilder）

系统 SHALL 提供链式调用的 EventBuilder，支持 event_sink API 文档中定义的所有字段。

#### Scenario: 构建基本事件
- **GIVEN** 调用 `reporter.NewEvent("user_login")`
- **WHEN** 链式调用 `.UserID(id).Payload(data).Track()`
- **THEN** 系统 SHALL 构建包含 event_type、user_id、payload 的完整事件
- **AND** 系统 SHALL 自动生成 event_id（UUID 格式）
- **AND** 系统 SHALL 自动设置 occurred_ms 为当前时间戳

#### Scenario: 构建完整事件（所有字段）
- **GIVEN** 调用 `reporter.NewEvent(eventType)`
- **WHEN** 链式调用所有可选方法设置字段
- **THEN** EventBuilder SHALL 支持以下字段设置：
  | 方法 | 对应字段 | 说明 |
  |------|----------|------|
  | EventID(id) | event_id | 自定义事件ID |
  | Source(src) | source | 事件来源 |
  | UserID(id) | user_id | 用户ID |
  | SessionID(id) | session_id | 会话ID |
  | TraceID(id) | trace_id | 链路追踪ID |
  | Sequence(n) | sequence | 事件序号 |
  | OccurredAt(t) | occurred_ms | 事件发生时间 |
  | ClientContext(ctx) | client_context | 客户端上下文 |
  | ServerContext(ctx) | server_context | 服务端上下文 |
  | ActionContext(ctx) | action_context | 动作上下文 |
  | Attributes(map) | event_attributes | 扩展属性 |
  | Payload(map) | payload | 业务数据 |

#### Scenario: 从 Context 自动提取信息
- **GIVEN** 调用 `builder.FromContext(ctx)`
- **WHEN** context 中包含 trace_id、user_id 等信息
- **THEN** EventBuilder SHALL 自动提取并设置对应字段

---

### Requirement: ServerContext 构建器

系统 SHALL 提供 ServerContextBuilder，用于构建服务端上下文信息。

#### Scenario: 手动构建 ServerContext
- **GIVEN** 调用 `NewServerContext()`
- **WHEN** 链式调用 `.IP(ip).UserAgent(ua).ServerNode(node).Build()`
- **THEN** 系统 SHALL 返回完整的 ServerContext 对象

#### Scenario: 从 HTTP 请求提取 ServerContext
- **GIVEN** 调用 `NewServerContext().FromHTTPRequest(r)`
- **WHEN** HTTP 请求包含 X-Forwarded-For 和 User-Agent 头
- **THEN** 系统 SHALL 自动提取 IP 地址和 User-Agent
- **AND** 正确处理代理转发的 IP 地址

#### Scenario: 从 gRPC Peer 提取 ServerContext
- **GIVEN** 调用 `NewServerContext().FromGRPCPeer(ctx)`
- **WHEN** gRPC context 包含 peer 信息
- **THEN** 系统 SHALL 自动提取客户端 IP 地址

---

### Requirement: ActionContext 构建器

系统 SHALL 提供 ActionContextBuilder，用于记录系统动作（API调用、数据库操作等）。

#### Scenario: 手动构建 ActionContext
- **GIVEN** 调用 `NewActionContext()`
- **WHEN** 链式调用设置 domain、type、operation、target 等字段
- **THEN** 系统 SHALL 返回完整的 ActionContext 对象

#### Scenario: 使用预定义工厂函数
- **GIVEN** 需要记录 API 调用
- **WHEN** 调用 `APICallAction("POST", "/api/users", 200, duration)`
- **THEN** 系统 SHALL 返回预填充的 ActionContext
- **AND** domain 设置为 "external"
- **AND** type 设置为 "api"
- **AND** operation 设置为 "call"

#### Scenario: ActionContext 工厂函数列表
系统 SHALL 提供以下预定义工厂函数：
| 函数 | 用途 | 预设值 |
|------|------|--------|
| APICallAction | HTTP API 调用 | domain=external, type=api |
| DatabaseAction | 数据库操作 | domain=external, type=database |
| FunctionAction | 函数执行 | domain=internal, type=function |
| CacheAction | 缓存操作 | domain=external, type=cache |
| RPCAction | RPC 调用 | domain=external, type=rpc |

---

### Requirement: ClientContext 构建器

系统 SHALL 提供 ClientContextBuilder，用于构建客户端上下文信息。

#### Scenario: 构建 ClientContext
- **GIVEN** 调用 `NewClientContext()`
- **WHEN** 链式调用 `.OS(name, ver).Device(brand, model).App(id, ver).Build()`
- **THEN** 系统 SHALL 返回完整的 ClientContext 对象
- **AND** 所有字段 SHALL 正确映射到 proto 定义

---

### Requirement: 批量上报机制

系统 SHALL 支持基于数量阈值和时间阈值的批量上报策略。

#### Scenario: 数量阈值触发
- **GIVEN** BatchSize 配置为 N（默认 100）
- **WHEN** 本地队列中累积的事件数量达到 N
- **THEN** 系统 SHALL 立即触发批量发送
- **AND** 调用 event_sink 的 `IngestBatch` gRPC 接口

#### Scenario: 时间阈值触发
- **GIVEN** FlushInterval 配置为 T（默认 5 秒）
- **AND** 队列中有未发送的事件
- **WHEN** 距离上次发送已超过 T 时间
- **THEN** 系统 SHALL 触发批量发送，即使未达到数量阈值

#### Scenario: 先到先触发
- **GIVEN** 队列中有事件
- **WHEN** 数量阈值或时间阈值任一条件满足
- **THEN** 系统 SHALL 立即触发批量发送

---

### Requirement: 本地事件队列

系统 SHALL 维护一个带容量限制的本地事件队列。

#### Scenario: 队列正常接收
- **GIVEN** 队列未满
- **WHEN** 新事件到达
- **THEN** 事件 SHALL 被成功加入队列

#### Scenario: 队列满时丢弃
- **GIVEN** 队列已满（达到 QueueSize 上限，默认 10000）
- **WHEN** 新事件到达
- **THEN** 新事件 SHALL 被丢弃（不阻塞调用方）
- **AND** 系统 SHALL 记录警告日志

#### Scenario: 队列长度监控
- **GIVEN** EventReporter 正在运行
- **WHEN** 调用 `QueueLen()` 方法
- **THEN** 系统 SHALL 返回当前队列中的事件数量

---

### Requirement: 优雅关闭

系统 SHALL 支持优雅关闭，确保在进程退出前尽可能发送队列中的事件。

#### Scenario: 正常关闭
- **GIVEN** EventReporter 正在运行
- **WHEN** 调用 `Shutdown(ctx)` 方法
- **THEN** 系统 SHALL 停止接收新事件
- **AND** 系统 SHALL 将队列中剩余事件批量发送
- **AND** 系统 SHALL 关闭 gRPC 连接
- **AND** 方法 SHALL 在 ctx 超时前返回

#### Scenario: 关闭超时
- **GIVEN** EventReporter 正在关闭
- **AND** 队列中有大量未发送事件
- **WHEN** ctx 超时或被取消
- **THEN** 系统 SHALL 立即停止发送
- **AND** 未发送的事件 MAY 被丢弃
- **AND** 系统 SHALL 记录警告日志

---

### Requirement: 错误处理

系统 SHALL 在上报失败时记录日志，不影响主业务流程。

#### Scenario: gRPC 调用失败
- **GIVEN** event_sink 服务不可用或网络异常
- **WHEN** 批量发送失败
- **THEN** 系统 SHALL 记录错误日志
- **AND** 本批次事件 MAY 被丢弃（不重试）
- **AND** 系统 SHALL 继续处理后续事件

#### Scenario: 事件校验失败
- **GIVEN** 调用 `Build()` 构建事件
- **WHEN** 缺少必填字段（event_type 或 payload）
- **THEN** 系统 SHALL 记录警告日志
- **AND** 尽可能填充默认值后继续发送

---

### Requirement: 配置管理

系统 SHALL 支持通过配置文件配置 EventReporter 的行为。

#### Scenario: 配置项加载
- **GIVEN** 配置文件中定义了 event_sink 相关配置
- **WHEN** EventReporter 初始化
- **THEN** 系统 SHALL 使用配置的值
- **AND** 未配置的项 SHALL 使用默认值

#### Scenario: 配置项列表
系统 SHALL 支持以下配置项：
| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| addr | string | - | event_sink gRPC 地址（必填） |
| queue_size | int | 10000 | 本地队列容量 |
| batch_size | int | 100 | 批量发送数量阈值 |
| flush_interval | duration | 5s | 刷新时间间隔 |
| shutdown_timeout | duration | 10s | 关闭超时时间 |
| source | string | "va_visionai_server" | 事件来源标识 |
| server_node | string | hostname | 服务节点标识 |
| enabled | bool | true | 是否启用 |

---

### Requirement: 扩展性支持

系统 SHALL 提供扩展点，支持未来功能增强。

#### Scenario: Context 提取器扩展
- **GIVEN** 需要自定义 context 信息提取逻辑
- **WHEN** 实现 `ContextExtractor` 接口并注册
- **THEN** EventBuilder 的 `FromContext` 方法 SHALL 使用自定义提取器

#### Scenario: 事件拦截器
- **GIVEN** 需要对所有事件添加公共字段或过滤敏感数据
- **WHEN** 调用 `reporter.Use(interceptor)`
- **THEN** 所有事件在发送前 SHALL 经过拦截器处理

#### Scenario: 禁用模式
- **GIVEN** 配置 `enabled: false`
- **WHEN** 调用 `Track()` 或 `NewEvent().Track()`
- **THEN** 系统 SHALL 直接返回，不执行任何操作（空操作模式）

### Requirement: 任务提交事件上报

系统 SHALL 在任务提交成功后上报 `TASK_SUBMIT` 事件，用于追踪任务提交行为。

#### Scenario: 普通任务提交上报
- **GIVEN** EventReporter 已初始化且可用
- **AND** 用户通过 `SubmitPictureForgeTask` 提交任务
- **WHEN** `PictureTaskService.SubmitTaskWithTx()` 执行成功
- **THEN** 系统 SHALL 异步上报 `TASK_SUBMIT` 事件
- **AND** Payload SHALL 包含以下字段：
  | 字段 | 说明 |
  |------|------|
  | task_id | 任务ID |
  | workflow_id | 工作流ID |
  | workflow_type | 工作流类型 (IMAGE/VIDEO) |
  | workflow_theme | 工作流主题 |
  | credit_points | 消耗积分 |
  | is_member | 是否会员 |
  | is_free | 是否免费 |
  | submit_source | 提交来源 (normal) |
  | tool_id | 工具ID（空） |
  | user_prompt | 用户输入的 prompt（截断至 500 字符） |
  | input_images | 用户上传的图片 URL 列表 |

#### Scenario: 工具任务提交上报
- **GIVEN** EventReporter 已初始化且可用
- **AND** 用户通过 `SubmitPictureToolsTask` 提交工具任务
- **WHEN** 内部调用 `SubmitPictureForgeTask` 并最终执行 `SubmitTaskWithTx()` 成功
- **THEN** 系统 SHALL 上报一次 `TASK_SUBMIT` 事件（避免重复）
- **AND** `submit_source` SHALL 为 "tool"
- **AND** `tool_id` SHALL 包含工具ID

#### Scenario: 随机工作流任务提交上报
- **GIVEN** EventReporter 已初始化且可用
- **AND** 用户通过 `SubmitRandomWorkflowTask` 提交任务
- **WHEN** 内部调用 `SubmitPictureForgeTask` 并最终执行 `SubmitTaskWithTx()` 成功
- **THEN** 系统 SHALL 上报一次 `TASK_SUBMIT` 事件
- **AND** `submit_source` SHALL 为 "random"

#### Scenario: 图转视频引导任务提交上报
- **GIVEN** EventReporter 已初始化且可用
- **AND** 用户通过 `PictureToVideoGuide` 提交任务
- **WHEN** 内部调用 `SubmitPictureForgeTask` 并最终执行 `SubmitTaskWithTx()` 成功
- **THEN** 系统 SHALL 上报一次 `TASK_SUBMIT` 事件
- **AND** `submit_source` SHALL 为 "guide"

#### Scenario: EventReporter 不可用时静默跳过
- **GIVEN** EventReporter 为 nil 或未启用
- **WHEN** 任务提交成功
- **THEN** 系统 SHALL 静默跳过上报，不影响主流程
- **AND** 不 SHALL 抛出异常或记录错误日志

---

### Requirement: 任务链提交事件上报

系统 SHALL 在任务链提交成功后上报 `TASK_CHAIN_SUBMIT` 事件，将任务链作为整体记录。

#### Scenario: 任务链提交上报
- **GIVEN** EventReporter 已初始化且可用
- **AND** 用户通过 `SubmitPictureTaskChain` 提交任务链
- **WHEN** 事务执行成功，图像任务和视频任务均创建完成
- **THEN** 系统 SHALL 异步上报 `TASK_CHAIN_SUBMIT` 事件
- **AND** Payload SHALL 包含以下字段：
  | 字段 | 说明 |
  |------|------|
  | image_task_id | 图像任务ID |
  | video_task_id | 视频任务ID |
  | first_workflow_id | 第一步工作流ID |
  | total_cost | 总消耗积分 |
  | is_member | 是否会员 |
  | user_prompt | 用户输入的 prompt（截断至 500 字符） |
  | input_images | 用户上传的图片 URL 列表 |

#### Scenario: 任务链不触发单独任务上报
- **GIVEN** 用户通过 `SubmitPictureTaskChain` 提交任务链
- **WHEN** 任务链创建成功
- **THEN** 系统 SHALL 仅上报 `TASK_CHAIN_SUBMIT` 事件
- **AND** 不 SHALL 额外上报 `TASK_SUBMIT` 事件
- **AND** 任务链中的图像任务和视频任务通过 `TASK_CHAIN_SUBMIT` 统一追踪

---

### Requirement: 提交来源标识传递

系统 SHALL 通过 Context 传递任务提交来源标识，供 Service 层读取。

#### Scenario: API 层注入提交来源
- **GIVEN** 用户调用任务提交 API
- **WHEN** API 层处理请求时
- **THEN** 系统 SHALL 将提交来源注入 Context
- **AND** 来源标识 SHALL 遵循以下映射：
  | API | submit_source |
  |-----|---------------|
  | SubmitPictureForgeTask (直接调用) | normal |
  | SubmitPictureToolsTask | tool |
  | SubmitRandomWorkflowTask | random |
  | PictureToVideoGuide | guide |

#### Scenario: Service 层读取提交来源
- **GIVEN** Context 中包含提交来源标识
- **WHEN** `PictureTaskService.SubmitTaskWithTx()` 上报事件
- **THEN** 系统 SHALL 从 Context 读取 `submit_source`
- **AND** 若 Context 中无此字段，默认值 SHALL 为 "normal"

---

### Requirement: 事件类型常量定义

系统 SHALL 在 `event_reporter/constants.go` 中定义任务提交相关事件类型常量。

#### Scenario: 常量定义
- **GIVEN** EventReporter 模块
- **WHEN** 需要上报任务提交事件
- **THEN** 系统 SHALL 使用以下预定义常量：
  | 常量名 | 值 | 用途 |
  |--------|-----|------|
  | EventTypeTaskSubmit | "TASK_SUBMIT" | 单个任务提交 |
  | EventTypeTaskChainSubmit | "TASK_CHAIN_SUBMIT" | 任务链提交 |

