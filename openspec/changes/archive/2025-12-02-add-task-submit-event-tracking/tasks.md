# Tasks: 任务提交事件上报

## 1. 常量定义

- [x] 1.1 在 `internal/service/event_reporter/constants.go` 添加事件类型常量
  - `EventTypeTaskSubmit = "TASK_SUBMIT"`
  - `EventTypeTaskChainSubmit = "TASK_CHAIN_SUBMIT"`

- [x] 1.2 在 `internal/constants/` 添加 Context Key 常量
  - `CtxTaskSubmitSource` - 提交来源 Key
  - 提交来源值常量：`SubmitSourceNormal`, `SubmitSourceTool`, `SubmitSourceRandom`, `SubmitSourceGuide`

## 2. Service 层实现

- [x] 2.1 修改 `PictureTaskService` 结构体，添加 `eventReporter` 字段
  - 文件：`internal/service/picture_task.go`

- [x] 2.2 修改 `NewPictureTaskService` 构造函数，接收 `EventReporter` 参数

- [x] 2.3 添加 `extractUserInputFromWorkflowInputs` 辅助方法
  - 从 WorkflowInput 列表中提取 MT_TEXT 类型内容作为 user_prompt
  - 从 WorkflowInput 列表中提取 MT_IMAGE 类型 URL 作为 input_images
  - user_prompt 截断至 500 字符

- [x] 2.4 添加 `trackTaskSubmitEvent` 私有方法
  - 从 Context 读取 `submit_source`（默认 "normal"）
  - 从 Context 读取 `tool_id`
  - 调用 `extractUserInputFromWorkflowInputs` 提取用户输入
  - 构建并上报 `TASK_SUBMIT` 事件（包含 user_prompt 和 input_images）

- [x] 2.5 在 `SubmitTaskWithTx` 成功返回前调用 `trackTaskSubmitEvent`

## 3. API 层实现

- [x] 3.1 修改 `PictureForgeServer` 结构体，添加 `eventReporter` 字段
  - 文件：`internal/api/picture_forge.go`

- [x] 3.2 修改 `NewPictureForgeServer` 构造函数

- [x] 3.3 在 `SubmitPictureToolsTask` 中注入 `submit_source = "tool"`
  - 使用 `context.WithValue(ctx, constants.CtxTaskSubmitSource, "tool")`

- [x] 3.4 在 `SubmitRandomWorkflowTask` 中注入 `submit_source = "random"`

- [x] 3.5 在 `PictureToVideoGuide` 中注入 `submit_source = "guide"`

- [x] 3.6 在 `SubmitPictureTaskChain` 事务成功后上报 `TASK_CHAIN_SUBMIT` 事件
  - Payload 包含：image_task_id, video_task_id, first_workflow_id, total_cost, is_member
  - 提取 user_prompt 和 input_images 并包含在 Payload 中

## 4. 依赖注入

- [x] 4.1 修改 `internal/bootstrap/service_provider.go`
  - 将 `EventReporter` 注入到 `PictureTaskService`
  - 将 `EventReporter` 注入到 `PictureForgeServer`（通过 ServerDeps）

## 5. 测试验证

- [x] 5.1 编写 `PictureTaskService.trackTaskSubmitEvent` 单元测试
  - 测试 submit_source 默认值
  - 测试 Context 传递的 submit_source
  - 测试 EventReporter 为 nil 时不 panic

- [ ] 5.2 手动验证
  - 提交普通任务，验证 event_sink 收到 TASK_SUBMIT 事件
  - 提交工具任务，验证 submit_source = "tool"
  - 提交任务链，验证 event_sink 收到 TASK_CHAIN_SUBMIT 事件
  - 验证无重复上报
  - 验证 user_prompt 和 input_images 正确记录
