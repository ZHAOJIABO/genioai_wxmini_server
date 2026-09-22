## ADDED Requirements

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
