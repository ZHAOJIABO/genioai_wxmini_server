# Design: Migrate Inspiration Tracking to Event System

## Architecture Overview

当前架构采用独立RPC接口处理灵感应用统计,新架构将统计逻辑整合到统一的事件上报系统中。

### Current Architecture

```
Client
  └─> ApplyInspirationPrompt RPC
        └─> PromptServer.ApplyInspirationPrompt()
              └─> InspirationPromptService.ApplyInspirationPrompt()
                    └─> InspirationRepository.RecordApplication()
                          └─> MySQL (inspiration_applications表)
```

### Target Architecture

```
Client
  └─> ReportUserEvent RPC
        └─> ReportServer.ReportUserEvent()
              ├─> EventService.ReportEvent()  (通用事件处理)
              └─> [新增] 灵感应用统计分支
                    └─> InspirationPromptService.ApplyInspirationPrompt()
                          └─> InspirationRepository.RecordApplication()
                                └─> MySQL (inspiration_applications表)
```

## Component Design

### 1. Event Flow

```
ReportUserEvent RPC
  ↓
[1] 参数校验
  ↓
[2] 通用事件上报 (EventService.ReportEvent)
  ↓
[3] 事件类型判断
  ↓
[4] TRACKING_EVENT + TRACKING_EVENT_TYPE_TOOLS_DETAIL_CREATE_BTN?
  ↓ Yes
[5] 解析 extra_data JSON
  ↓
[6] 提取 inspiration_prompt_id
  ↓
[7] 调用 InspirationPromptService.ApplyInspirationPrompt
  ↓
[8] 记录到数据库
```

### 2. Data Flow

#### Input: TrackingEventData
```protobuf
message TrackingEventData {
  TrackingEventType event_type = 1;  // TRACKING_EVENT_TYPE_TOOLS_DETAIL_CREATE_BTN
  string page = 2;
  string extra_data = 3;  // JSON: {"inspiration_prompt_id":"prompt_1","tool_id":"tool_123"}
}
```

#### Processing: JSON Parsing
```json
{
  "inspiration_prompt_id": "prompt_1",  // 必填
  "tool_id": "tool_123",                // 可选,用于关联工具
  "tool_type": "image_generation"       // 可选,用于分类统计
}
```

#### Output: Database Record
```sql
INSERT INTO inspiration_applications (
  prompt_id,
  user_id,
  tool_id,
  tool_type,
  applied_at
) VALUES (?, ?, ?, ?, NOW());
```

### 3. Implementation Details

#### 3.1 ReportServer 增强

在 `internal/api/report.go` 的 `ReportUserEvent` 方法中增加灵感统计逻辑:

```go
func (s *ReportServer) ReportUserEvent(ctx context.Context, req *vai.ReportUserEventRequest) (*vai.ReportUserEventResponse, error) {
    // ... 现有的校验逻辑 ...

    // 现有的事件上报逻辑
    if err := s.eventService.ReportEvent(ctx, req.GetRequestHeader(), req.GetRequestHeader().GetUserId(), req.GetEventType().String(), req.GetEventData()); err != nil {
        // ... 错误处理 ...
    }

    // [新增] 灵感应用统计逻辑
    if req.GetEventType() == vai.UserEventType_TRACKING_EVENT {
        if trackingData, ok := req.GetEventData().(*vai.ReportUserEventRequest_TrackingData); ok {
            if trackingData.TrackingData.GetEventType() == vai.TrackingEventType_TRACKING_EVENT_TYPE_TOOLS_DETAIL_CREATE_BTN {
                s.handleInspirationTracking(ctx, req.GetRequestHeader().GetUserId(), trackingData.TrackingData)
            }
        }
    }

    return &vai.ReportUserEventResponse{
        ResponseHeader: common.BuildHeader(vai.StatusCode_SUCCESS),
    }, nil
}

func (s *ReportServer) handleInspirationTracking(ctx context.Context, userID string, trackingData *vai.TrackingEventData) {
    // 解析 extra_data
    extraData := trackingData.GetExtraData()
    if extraData == "" {
        zlog.LogWithContext(ctx).Warn("inspiration tracking: empty extra_data")
        return
    }

    var data struct {
        InspirationPromptID string `json:"inspiration_prompt_id"`
        ToolID             string `json:"tool_id"`
        ToolType           string `json:"tool_type"`
    }

    if err := json.Unmarshal([]byte(extraData), &data); err != nil {
        zlog.LogWithContext(ctx).Error("inspiration tracking: failed to parse extra_data",
            zap.Error(err),
            zap.String("extra_data", extraData))
        return
    }

    if data.InspirationPromptID == "" {
        zlog.LogWithContext(ctx).Warn("inspiration tracking: missing inspiration_prompt_id")
        return
    }

    // 调用服务记录应用
    if err := s.inspirationPromptService.ApplyInspirationPrompt(ctx, data.InspirationPromptID, userID, data.ToolID, data.ToolType); err != nil {
        zlog.LogWithContext(ctx).Error("inspiration tracking: failed to record application",
            zap.Error(err),
            zap.String("prompt_id", data.InspirationPromptID))
        // 注意: 这里不返回错误,避免影响主事件上报流程
    }
}
```

#### 3.2 依赖注入

在 `ReportServer` 中添加 `InspirationPromptService` 依赖:

```go
type ReportServer struct {
    reportService            *service.ReportService
    eventService             *event.EventService
    watchService             *event.WatchService
    inspirationPromptService *service.InspirationPromptService  // [新增]

    vai.UnimplementedReportServiceServer
}

func NewReportServer(
    reportService *service.ReportService,
    eventService *event.EventService,
    watchService *event.WatchService,
    inspirationPromptService *service.InspirationPromptService,  // [新增]
) *ReportServer {
    return &ReportServer{
        reportService:            reportService,
        eventService:             eventService,
        watchService:             watchService,
        inspirationPromptService: inspirationPromptService,  // [新增]
    }
}
```

#### 3.3 Bootstrap 配置

在 `internal/bootstrap/service_provider.go` 中更新 ReportServer 的初始化:

```go
func (sp *ServiceProvider) InitReportServer() *api.ReportServer {
    return api.NewReportServer(
        sp.GetReportService(),
        sp.GetEventService(),
        sp.GetWatchService(),
        sp.GetInspirationPromptService(),  // [新增]
    )
}
```

### 4. Error Handling Strategy

#### 4.1 容错原则
灵感统计失败不应影响主事件上报流程,采用"尽力而为"策略:
- 解析失败时记录日志但不中断
- 统计失败时记录日志但不返回错误
- 所有错误都要有详细的日志记录

#### 4.2 错误场景处理

| 错误场景 | 处理策略 |
|---------|---------|
| extra_data为空 | 记录Warn日志,返回 |
| JSON解析失败 | 记录Error日志(含原始数据),返回 |
| prompt_id缺失 | 记录Warn日志,返回 |
| 数据库写入失败 | 记录Error日志,返回 (不影响事件上报成功) |

### 5. Monitoring & Observability

#### 5.1 日志埋点

```go
// 成功场景
zlog.LogWithContext(ctx).Info("inspiration tracking: recorded application",
    zap.String("prompt_id", data.InspirationPromptID),
    zap.String("user_id", userID),
    zap.String("tool_id", data.ToolID))

// 失败场景
zlog.LogWithContext(ctx).Error("inspiration tracking: failed",
    zap.String("reason", "json_parse_error"),
    zap.String("extra_data", extraData),
    zap.Error(err))
```

#### 5.2 指标监控

建议添加的监控指标:
- `inspiration_tracking_total`: 总调用次数
- `inspiration_tracking_success`: 成功记录次数
- `inspiration_tracking_failed`: 失败次数
- `inspiration_tracking_parse_error`: JSON解析失败次数

### 6. Migration Path

#### Phase 1: 双写期 (Week 1)
- 部署新逻辑,通过 `ReportUserEvent` 记录统计
- 保留 `ApplyInspirationPrompt` 接口
- 对比两种方式的数据一致性

#### Phase 2: 灰度期 (Week 2-4)
- 客户端逐步切换到 `ReportUserEvent`
- 监控旧接口调用量下降趋势
- 确保新方式数据正常

#### Phase 3: 清理期 (Week 5)
- 确认旧接口调用量为0
- 执行 `make proto` 删除旧RPC定义
- 删除 `PromptServer.ApplyInspirationPrompt` 实现
- 清理相关测试代码

### 7. Testing Strategy

#### 7.1 单元测试

```go
func TestReportServer_HandleInspirationTracking(t *testing.T) {
    tests := []struct {
        name       string
        extraData  string
        wantErr    bool
        shouldCall bool
    }{
        {
            name:       "valid data",
            extraData:  `{"inspiration_prompt_id":"prompt_1","tool_id":"tool_123"}`,
            shouldCall: true,
        },
        {
            name:       "missing prompt_id",
            extraData:  `{"tool_id":"tool_123"}`,
            shouldCall: false,
        },
        {
            name:       "invalid json",
            extraData:  `{invalid}`,
            shouldCall: false,
        },
        {
            name:       "empty extra_data",
            extraData:  "",
            shouldCall: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ... 测试实现 ...
        })
    }
}
```

#### 7.2 集成测试

验证完整的事件上报 -> 统计记录流程:
1. 构造完整的 `ReportUserEventRequest`
2. 调用 `ReportUserEvent` RPC
3. 验证数据库中生成了对应的记录

### 8. Backward Compatibility

#### 8.1 Proto兼容性
- 删除 `ApplyInspirationPrompt` RPC不影响已部署的客户端
- 新客户端调用旧服务会收到"unimplemented"错误
- 需要先部署服务端,再更新客户端

#### 8.2 数据兼容性
- `inspiration_applications` 表结构不变
- 新旧方式写入的数据格式一致
- 历史数据可以正常查询

### 9. Performance Considerations

#### 9.1 性能影响评估
- 增加的处理逻辑: JSON解析 + 条件判断 + 服务调用
- 预计增加耗时: < 5ms
- 不会成为性能瓶颈

#### 9.2 优化建议
- JSON解析使用标准库,性能已优化
- 数据库写入已在事务外,不阻塞主流程
- 如需优化可考虑异步写入队列

### 10. Security Considerations

#### 10.1 输入验证
- 验证 `inspiration_prompt_id` 格式
- 限制 `extra_data` 大小(< 1KB)
- 防止SQL注入(使用参数化查询)

#### 10.2 数据隐私
- 不记录敏感的用户输入内容
- 只记录必要的统计标识
- 遵循GDPR数据保留策略

## Trade-offs

### Pros
✅ 统一事件上报架构,易于维护
✅ 减少RPC接口数量
✅ 提升系统扩展性
✅ 业务逻辑集中管理

### Cons
⚠️ 增加了 `ReportUserEvent` 的复杂度
⚠️ 需要客户端配合迁移
⚠️ 迁移期间需要维护两套代码

### Decision
权衡后认为长期收益大于短期成本,建议执行迁移。

## Alternatives Considered

### Alternative 1: 保持现状
**Pros**: 无需改动,风险最低
**Cons**: 接口冗余,不利于长期维护
**Decision**: 拒绝,不符合系统演进方向

### Alternative 2: 新增专门的proto字段
在 `TrackingEventData` 中新增 `string inspiration_prompt_id` 字段
**Pros**: 类型安全,不需要JSON解析
**Cons**: proto定义膨胀,每次新需求都要修改proto
**Decision**: 拒绝,`extra_data` 已足够灵活

### Alternative 3: 使用消息队列异步处理
**Pros**: 进一步解耦,提升性能
**Cons**: 增加系统复杂度,引入新依赖
**Decision**: 暂不采用,当前同步方案已满足需求

## References

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [gRPC Error Handling Best Practices](https://grpc.io/docs/guides/error/)
- [Go JSON Performance Tips](https://go.dev/blog/json)
