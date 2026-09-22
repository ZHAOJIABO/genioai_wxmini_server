# Spec: Inspiration Event Tracking

## ADDED Requirements

### Requirement: Event-based Inspiration Tracking
**ID**: INS-001
**Priority**: High
**Category**: Feature Enhancement

系统SHALL支持通过统一的事件上报机制记录用户应用灵感prompt的行为。当用户在工具详情页应用灵感时,客户端MUST通过`ReportUserEvent` RPC上报`TRACKING_EVENT_TYPE_TOOLS_DETAIL_CREATE_BTN`事件,服务端SHALL解析`extra_data`中的JSON数据并记录到数据库。

#### Scenario: User applies inspiration via tools detail page
**Given** 用户在工具详情页浏览灵感推荐列表
**When** 用户点击"创建"按钮应用某个灵感prompt
**Then** 客户端应当通过 `ReportUserEvent` RPC上报事件
**And** `UserEventType` 为 `TRACKING_EVENT`
**And** `TrackingEventData.event_type` 为 `TRACKING_EVENT_TYPE_TOOLS_DETAIL_CREATE_BTN`
**And** `TrackingEventData.extra_data` 包含JSON格式的灵感数据:
```json
{
  "inspiration_prompt_id": "prompt_123",
  "tool_id": "tool_456",
  "tool_type": "image_generation"
}
```

#### Scenario: Server processes inspiration tracking event
**Given** 服务端收到 `ReportUserEvent` 请求
**And** `UserEventType` 为 `TRACKING_EVENT`
**And** `TrackingEventData.event_type` 为 `TRACKING_EVENT_TYPE_TOOLS_DETAIL_CREATE_BTN`
**And** `extra_data` 包含有效的 `inspiration_prompt_id`
**When** 服务端处理该事件
**Then** 应当成功上报通用事件
**And** 应当解析 `extra_data` JSON数据
**And** 应当提取 `inspiration_prompt_id`, `tool_id`, `tool_type`
**And** 应当调用 `InspirationPromptService.ApplyInspirationPrompt` 记录应用
**And** 应当在数据库 `inspiration_applications` 表中插入一条记录
**And** 返回成功响应 `StatusCode_SUCCESS`

#### Scenario: Handle missing inspiration_prompt_id gracefully
**Given** 服务端收到灵感统计事件
**And** `extra_data` 中缺失 `inspiration_prompt_id` 字段
**When** 服务端处理该事件
**Then** 应当记录Warn级别日志
**And** 应当跳过灵感统计逻辑
**And** 应当继续完成通用事件上报
**And** 返回成功响应 `StatusCode_SUCCESS` (不阻断主流程)

#### Scenario: Handle JSON parse error gracefully
**Given** 服务端收到灵感统计事件
**And** `extra_data` 包含非法的JSON格式
**When** 服务端解析JSON失败
**Then** 应当记录Error级别日志,包含原始 `extra_data` 内容
**And** 应当跳过灵感统计逻辑
**And** 应当继续完成通用事件上报
**And** 返回成功响应 `StatusCode_SUCCESS` (不阻断主流程)

#### Scenario: Handle database write failure gracefully
**Given** 服务端成功解析灵感统计数据
**And** 数据库写入失败(如连接断开)
**When** `InspirationPromptService.ApplyInspirationPrompt` 返回错误
**Then** 应当记录Error级别日志,包含错误详情
**And** 应当继续完成通用事件上报
**And** 返回成功响应 `StatusCode_SUCCESS` (不阻断主流程)

### Requirement: Backward Compatibility during Migration
**ID**: INS-002
**Priority**: High
**Category**: Migration

在迁移期间,系统SHALL支持新旧两种方式同时工作,确保平滑过渡。服务端MUST保留`ApplyInspirationPrompt` RPC的实现,直到确认所有客户端已完成迁移。新旧两种方式MUST产生一致的数据结构和统计结果。

#### Scenario: Old API still works during migration
**Given** proto定义中 `ApplyInspirationPrompt` RPC尚未删除
**And** 客户端尚未完全迁移到新方式
**When** 客户端调用 `ApplyInspirationPrompt` RPC
**Then** 服务端应当正常处理该请求
**And** 应当记录灵感应用数据到数据库
**And** 返回成功响应

#### Scenario: Both old and new methods produce consistent data
**Given** 同一用户在短时间内应用相同的灵感
**When** 一次通过 `ApplyInspirationPrompt` RPC
**And** 一次通过 `ReportUserEvent` + `TRACKING_EVENT`
**Then** 两次记录的数据结构应当一致
**And** 都应当包含: `prompt_id`, `user_id`, `tool_id`, `tool_type`, `applied_at`
**And** 统计查询结果应当包含两次记录

### Requirement: Remove deprecated API after migration
**ID**: INS-003
**Priority**: Medium
**Category**: Cleanup

迁移完成后,系统SHALL删除旧的`ApplyInspirationPrompt` RPC及其实现。在删除前,MUST确认旧接口调用量已降为零至少7天。执行`make proto`后,生成的代码MUST不包含`ApplyInspirationPrompt`相关方法,且系统MUST能够正常编译和运行。

#### Scenario: Execute make proto after proto update
**Given** proto定义中已删除 `ApplyInspirationPrompt` RPC
**When** 执行 `make proto` 命令
**Then** 应当成功重新生成proto Go代码
**And** 生成的代码中不应包含 `ApplyInspirationPrompt` 相关方法
**And** 不应产生编译错误

#### Scenario: Remove server-side implementation
**Given** proto代码已重新生成
**When** 删除 `PromptServer.ApplyInspirationPrompt` 方法实现
**And** 删除相关的单元测试
**Then** 项目应当能够成功编译 (`go build ./...`)
**And** 所有测试应当通过 (`go test ./...`)
**And** 不应存在对已删除方法的引用

#### Scenario: Verify no client still using old API
**Given** 旧API已从服务端删除
**When** 监控系统日志和指标
**Then** 不应出现 `ApplyInspirationPrompt` 相关的调用错误
**And** 灵感统计数据应当持续正常写入
**And** 通过 `ReportUserEvent` 的统计量应当稳定

## MODIFIED Requirements

### Requirement: Event Reporting Service Integration
**ID**: INS-004
**Priority**: High
**Category**: Integration

`ReportServer` SHALL集成`InspirationPromptService`以处理灵感统计逻辑。在系统启动时,`ServiceProvider` MUST正确注入`InspirationPromptService`依赖到`ReportServer`,且该依赖MUST不为nil。

#### Scenario: ReportServer has InspirationPromptService dependency
**Given** 系统启动时初始化 `ReportServer`
**When** 调用 `NewReportServer` 构造函数
**Then** 应当接收 `InspirationPromptService` 作为依赖参数
**And** 应当将其保存为 `ReportServer` 的字段
**And** 字段不应为nil

#### Scenario: Bootstrap correctly wires dependencies
**Given** 应用程序启动
**When** `ServiceProvider.InitReportServer()` 执行
**Then** 应当正确创建 `InspirationPromptService` 实例
**And** 应当将其注入到 `ReportServer` 中
**And** `ReportServer` 应当可以正常调用 `InspirationPromptService` 的方法

### Requirement: Enhanced Logging and Observability
**ID**: INS-005
**Priority**: Medium
**Category**: Observability

系统SHALL提供完善的日志记录,便于问题排查和数据分析。每个灵感统计操作MUST记录对应级别的日志(Info/Warn/Error),日志MUST包含关键字段如`prompt_id`、`user_id`、错误详情等。

#### Scenario: Log successful inspiration tracking
**Given** 灵感统计成功记录到数据库
**When** 完成处理
**Then** 应当记录Info级别日志
**And** 日志应当包含字段: `prompt_id`, `user_id`, `tool_id`, `tool_type`
**And** 日志消息应当为: "inspiration tracking: recorded application"

#### Scenario: Log JSON parse error with context
**Given** `extra_data` JSON解析失败
**When** 捕获错误
**Then** 应当记录Error级别日志
**And** 日志应当包含: 原始 `extra_data` 字符串, 错误详情
**And** 日志消息应当为: "inspiration tracking: failed to parse extra_data"

#### Scenario: Log missing required field
**Given** `extra_data` 缺失 `inspiration_prompt_id`
**When** 校验失败
**Then** 应当记录Warn级别日志
**And** 日志消息应当为: "inspiration tracking: missing inspiration_prompt_id"

#### Scenario: Log database write failure
**Given** 数据库写入失败
**When** `ApplyInspirationPrompt` 返回错误
**Then** 应当记录Error级别日志
**And** 日志应当包含: `prompt_id`, `user_id`, 错误详情
**And** 日志消息应当为: "inspiration tracking: failed to record application"

## REMOVED Requirements

### Requirement: Dedicated Inspiration Application RPC
**ID**: INS-006-REMOVED
**Priority**: N/A
**Category**: Deprecated

~~系统应当提供独立的 `ApplyInspirationPrompt` RPC接口用于记录灵感应用。~~

**Reason for Removal**:
功能已迁移到统一的 `ReportUserEvent` 事件上报机制中,通过 `TRACKING_EVENT_TYPE_TOOLS_DETAIL_CREATE_BTN` 事件类型实现。独立的RPC接口造成接口冗余,不利于系统架构统一管理。

**Impact**:
- 客户端需要修改调用方式,从调用 `ApplyInspirationPrompt` 改为调用 `ReportUserEvent`
- 服务端在迁移期间需要同时支持两种方式
- proto定义需要删除 `ApplyInspirationPrompt` RPC声明

**Migration Path**:
1. Phase 1: 服务端实现新方式,保留旧接口
2. Phase 2: 客户端逐步迁移,监控调用量
3. Phase 3: 确认迁移完成后删除旧代码

## Non-Functional Requirements

### Requirement: Performance
**ID**: INS-NFR-001
**Priority**: Medium

灵感统计逻辑不应显著影响事件上报性能。

- JSON解析耗时: < 1ms (典型场景)
- 额外处理耗时: < 5ms (包含数据库写入)
- 不应阻塞主事件上报流程

### Requirement: Reliability
**ID**: INS-NFR-002
**Priority**: High

灵感统计失败不应影响通用事件上报成功。

- 任何灵感统计错误都不应返回失败响应
- 错误应当记录日志便于排查
- 主事件上报应当始终成功

### Requirement: Data Integrity
**ID**: INS-NFR-003
**Priority**: High

统计数据应当准确可靠。

- 同一用户多次应用同一灵感应当生成多条记录
- 时间戳应当准确记录用户行为发生时间
- 不应丢失有效的统计数据

### Requirement: Security
**ID**: INS-NFR-004
**Priority**: High

输入数据应当经过验证,防止注入攻击。

- 验证 `inspiration_prompt_id` 格式
- 限制 `extra_data` 大小 (< 1KB)
- 使用参数化SQL查询防止注入
- 不记录敏感用户数据

## Testing Requirements

### Unit Test Coverage
- `ReportServer.handleInspirationTracking`: ≥90%
- 覆盖所有错误场景: 空数据、解析失败、字段缺失、数据库错误
- 使用Fake Repository,不依赖真实数据库

### Integration Test Coverage
- 完整的 `ReportUserEvent` -> 统计记录流程
- 验证数据库中记录生成
- 验证日志输出正确

### Migration Test
- 验证旧接口在迁移期间仍可用
- 验证新旧方式数据一致性
- 验证旧代码删除后系统正常运行

## Documentation Requirements

### Code Documentation
- `handleInspirationTracking` 方法添加详细注释
- 说明 `extra_data` JSON格式
- 说明错误处理策略

### API Documentation
- 更新 `ReportUserEvent` 接口文档
- 说明灵感统计的使用方式
- 提供示例代码

### Migration Guide
- 编写客户端迁移指南
- 说明迁移步骤和注意事项
- 提供新旧方式对比示例
