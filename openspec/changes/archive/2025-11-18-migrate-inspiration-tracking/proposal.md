# Proposal: migrate-inspiration-tracking

## Overview

将灵感prompt应用统计逻辑从独立的 `ApplyInspirationPrompt` RPC接口迁移到 `ReportUserEvent` 统一事件上报接口,实现业务逻辑统一管理。

## Why

当前灵感prompt功能使用独立的 `ApplyInspirationPrompt` RPC接口来记录用户应用灵感的行为。这种设计存在以下问题:

1. **接口冗余**: 为单一业务行为维护独立的RPC接口,增加了proto定义和API维护成本
2. **统计分散**: 用户行为统计分散在不同的接口中,不利于统一的数据分析和监控
3. **扩展性差**: 每次新增类似的统计需求都需要创建新的RPC接口

## What Changes

### 核心变更

1. **删除独立RPC接口**: 从proto定义中删除 `ApplyInspirationPrompt` RPC方法
2. **复用事件上报接口**: 通过 `ReportUserEvent` RPC的 `TRACKING_EVENT` 类型上报灵感应用行为
3. **事件类型标识**: 使用已存在的 `TRACKING_EVENT_TYPE_TOOLS_DETAIL_CREATE_BTN` (值=61) 来标识用户点击工具详情页创建按钮的行为
4. **数据传递格式**: 在 `TrackingEventData.extra_data` 字段中传递JSON格式的灵感相关数据

### 数据结构设计

**extra_data JSON格式**:
```json
{
  "inspiration_prompt_id": "prompt_1",
  "tool_id": "tool_123",
  "tool_type": "image_generation"
}
```

### 业务逻辑变更

在 `ReportServer.ReportUserEvent` 中增加灵感应用统计逻辑:
- 当 `UserEventType == TRACKING_EVENT` 时
- 且 `TrackingEventData.TrackingEventType == TRACKING_EVENT_TYPE_TOOLS_DETAIL_CREATE_BTN` 时
- 解析 `extra_data` JSON数据
- 调用 `InspirationPromptService.ApplyInspirationPrompt` 记录应用行为

## Benefits

1. **统一管理**: 所有用户行为统计集中在 `ReportUserEvent` 接口,便于监控和分析
2. **减少接口数量**: 删除冗余的RPC接口,降低维护成本
3. **提升扩展性**: 未来类似的统计需求可直接复用事件上报机制
4. **保持向后兼容**: 服务端灵感统计逻辑保持不变,只是触发方式改变

## Migration Strategy

### Phase 1: 服务端支持新方式
- 在 `ReportUserEvent` 中实现灵感应用统计逻辑
- 保留旧的 `ApplyInspirationPrompt` 接口(标记为deprecated)
- 两种方式同时支持

### Phase 2: 客户端迁移
- 客户端逐步迁移到使用 `ReportUserEvent` 上报
- 监控新旧接口调用量

### Phase 3: 清理旧代码
- 确认客户端完全迁移后
- 执行 `make proto` 重新生成proto代码(proto定义中已删除 `ApplyInspirationPrompt`)
- 删除服务端 `ApplyInspirationPrompt` 相关实现代码
- 清理相关单元测试

## Risks & Mitigations

### Risk 1: 客户端未及时更新
**Mitigation**:
- Phase 1先保留旧接口,给客户端足够的迁移时间
- 通过监控确认旧接口调用量降为0后再删除

### Risk 2: extra_data JSON解析失败
**Mitigation**:
- 实现健壮的JSON解析逻辑
- 解析失败时记录错误日志但不中断主流程
- 设置合理的默认值

### Risk 3: 数据丢失
**Mitigation**:
- 在Phase 1保持两种方式同时可用
- 对比两种方式的统计数据,确保一致性

## Success Criteria

1. `ReportUserEvent` 能正确处理灵感应用统计
2. 统计数据与旧接口保持一致
3. 删除旧代码后系统正常运行
4. 单元测试覆盖率 ≥85%
5. 所有相关文档已更新

## Timeline

- Week 1: 实现服务端新逻辑,保留旧接口
- Week 2-4: 客户端迁移周期
- Week 5: 清理旧代码,完成迁移

## Related Changes

- 影响文件:
  - `internal/api/report.go`
  - `internal/api/prompt.go` (删除 `ApplyInspirationPrompt`)
  - `internal/service/inspiration_prompt.go` (保留核心逻辑)
  - proto定义 (删除 `ApplyInspirationPrompt` RPC)

## Open Questions

1. 是否需要在 `TrackingEventData` 中新增专门的字段,而不是使用通用的 `extra_data`?
   - **决策**: 使用 `extra_data`,保持proto定义的灵活性

2. 是否需要支持批量上报多个灵感应用?
   - **决策**: 当前保持单次上报,未来可扩展
