# Change: 添加任务提交事件上报

## Why

当前任务提交流程缺乏埋点事件上报，无法追踪和统计任务提交的来源、类型、积分消耗等关键业务指标。需要为 PictureForge 任务提交流程添加事件上报能力，以支持后续的数据分析和运营决策。

## What Changes

- 新增 `TASK_SUBMIT` 事件类型，用于记录单个任务提交
- 新增 `TASK_CHAIN_SUBMIT` 事件类型，用于记录任务链提交
- 在 `PictureTaskService.SubmitTaskWithTx()` 成功后上报 `TASK_SUBMIT` 事件
- 在 `PictureForgeServer.SubmitPictureTaskChain()` 成功后上报 `TASK_CHAIN_SUBMIT` 事件
- 通过 Context 传递提交来源标识（normal/tool/random/guide），避免重复上报
- 为 `PictureTaskService` 注入 `EventReporter` 依赖

## Impact

- Affected specs: `event-reporter`
- Affected code:
  - `internal/service/event_reporter/constants.go` - 新增事件类型常量
  - `internal/constants/context.go` - 新增 Context Key 常量
  - `internal/service/picture_task.go` - 添加 EventReporter 依赖和上报逻辑
  - `internal/api/picture_forge.go` - 注入提交来源标识、任务链上报
  - `internal/bootstrap/service_provider.go` - 注入 EventReporter
