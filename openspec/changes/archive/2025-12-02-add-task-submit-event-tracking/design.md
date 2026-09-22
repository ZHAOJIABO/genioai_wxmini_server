# Design: 任务提交事件上报

## Context

PictureForge 模块包含多种任务提交入口：
- `SubmitPictureForgeTask` - 普通任务提交
- `SubmitPictureToolsTask` - 工具任务提交（内部调用 SubmitPictureForgeTask）
- `SubmitRandomWorkflowTask` - 随机工作流提交（内部调用 SubmitPictureForgeTask）
- `PictureToVideoGuide` - 图转视频引导（内部调用 SubmitPictureForgeTask）
- `SubmitPictureTaskChain` - 任务链提交（独立路径，不经过 SubmitPictureForgeTask）

挑战：工具任务、随机工作流、图转视频引导最终都调用 `SubmitPictureForgeTask`，如果在各入口都上报，会导致一个任务上报多次事件。

## Goals / Non-Goals

**Goals:**
- 每个任务提交只上报一次事件
- 准确记录任务提交来源（normal/tool/random/guide/chain）
- 记录任务关键信息：task_id、workflow_id、积分消耗、会员状态等
- 任务链作为整体上报，而非两个独立任务

**Non-Goals:**
- 任务执行过程和结果的事件上报（已有 TASK_COMPLETED/TASK_FAILED）
- 历史数据回填

## Decisions

### Decision 1: 混合上报策略

**选择：** Service 层统一上报普通任务 + API 层上报任务链

**理由：**
- 路径 A/B/C/D 最终都经过 `PictureTaskService.SubmitTaskWithTx()`，在此处统一上报可避免重复
- 路径 E（任务链）不经过 `SubmitTaskWithTx()`，且需要聚合上报，必须在 API 层处理

```
┌───────────────────────────────────────────────────────────────────────────┐
│                              调用关系图                                    │
├───────────────────────────────────────────────────────────────────────────┤
│  路径 A: SubmitPictureForgeTask ──▶ SubmitTaskWithTx ──▶ [上报 Service层] │
│  路径 B: SubmitPictureToolsTask ──▶ A ──▶ SubmitTaskWithTx ──▶ [上报]     │
│  路径 C: SubmitRandomWorkflowTask ──▶ A ──▶ SubmitTaskWithTx ──▶ [上报]   │
│  路径 D: PictureToVideoGuide ──▶ A ──▶ SubmitTaskWithTx ──▶ [上报]        │
│  路径 E: SubmitPictureTaskChain ──▶ createFirstTask/preCreateSecond       │
│                                   ──▶ [上报 API层，独立事件类型]           │
└───────────────────────────────────────────────────────────────────────────┘
```

### Decision 2: 通过 Context 传递提交来源

**选择：** 使用 `context.WithValue` 传递 `submit_source`

**理由：**
- 现有代码已使用 context 传递 `PictureToolIDKey`，保持风格一致
- 避免修改 `SubmitTaskWithTx` 方法签名
- Service 层可透明获取来源信息

**实现：**
```go
// API 层注入
ctx = context.WithValue(ctx, constants.CtxTaskSubmitSource, "tool")

// Service 层读取
submitSource := "normal"
if src := ctx.Value(constants.CtxTaskSubmitSource); src != nil {
    submitSource = src.(string)
}
```

### Decision 3: 事件 Payload 设计

**TASK_SUBMIT Payload:**
| 字段 | 类型 | 说明 |
|-----|------|------|
| task_id | string | 任务ID |
| workflow_id | string | 工作流ID |
| workflow_type | string | 工作流类型 (IMAGE/VIDEO) |
| workflow_theme | string | 工作流主题 |
| credit_points | int | 消耗积分 |
| is_member | bool | 是否会员 |
| is_free | bool | 是否免费（积分为0） |
| submit_source | string | 提交来源 |
| tool_id | string | 工具ID（仅工具任务有值） |
| user_prompt | string | 用户输入的 prompt（截断至 500 字符） |
| input_images | []string | 用户上传的图片 URL 列表 |

**TASK_CHAIN_SUBMIT Payload:**
| 字段 | 类型 | 说明 |
|-----|------|------|
| image_task_id | string | 图像任务ID |
| video_task_id | string | 视频任务ID |
| first_workflow_id | string | 第一步工作流ID |
| total_cost | int | 总消耗积分 |
| is_member | bool | 是否会员 |
| user_prompt | string | 用户输入的 prompt（截断至 500 字符） |
| input_images | []string | 用户上传的图片 URL 列表 |

### Decision 4: 用户输入数据处理

**选择：** 记录用户输入的 prompt 和图片 URL，但进行适当截断

**理由：**
- prompt 和图片是任务的核心输入，对分析用户行为有价值
- prompt 可能很长，需要截断避免事件过大
- 图片 URL 直接记录，不做处理

**实现：**
- `user_prompt`：从 WorkflowInput 中提取 MT_TEXT 类型的输入，截断至 500 字符
- `input_images`：从 WorkflowInput 中提取 MT_IMAGE 类型的 URL 列表

## Risks / Trade-offs

| 风险 | 影响 | 缓解措施 |
|-----|------|---------|
| Context 传递 submit_source 可能被遗忘 | 部分任务 submit_source 为空 | 默认值为 "normal"；代码审查 |
| EventReporter 为 nil 时 panic | 服务崩溃 | 上报前检查 nil |
| 上报逻辑影响主流程性能 | 响应变慢 | 使用异步 Track() |

## Migration Plan

1. 添加常量和依赖注入（无影响）
2. 添加上报逻辑，默认启用
3. 发布后通过 event_sink 验证数据准确性

## Open Questions

无
