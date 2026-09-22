# Tasks: Migrate Inspiration Tracking

本任务列表按照执行顺序编排,每个任务都可独立验证。

## Phase 1: 实现新的事件统计逻辑 (保留旧接口)

### Task 1.1: 扩展 ReportServer 依赖
**Description**: 为 `ReportServer` 添加 `InspirationPromptService` 依赖

**Files**:
- `internal/api/report.go`

**Changes**:
```go
type ReportServer struct {
    reportService            *service.ReportService
    eventService             *event.EventService
    watchService             *event.WatchService
    inspirationPromptService *service.InspirationPromptService  // 新增

    vai.UnimplementedReportServiceServer
}

func NewReportServer(
    reportService *service.ReportService,
    eventService *event.EventService,
    watchService *event.WatchService,
    inspirationPromptService *service.InspirationPromptService,  // 新增
) *ReportServer {
    return &ReportServer{
        reportService:            reportService,
        eventService:             eventService,
        watchService:             watchService,
        inspirationPromptService: inspirationPromptService,  // 新增
    }
}
```

**Validation**:
- [ ] 编译通过 `go build ./internal/api`
- [ ] 运行相关测试通过 `go test ./internal/api`

---

### Task 1.2: 实现灵感统计处理方法
**Description**: 在 `ReportServer` 中实现 `handleInspirationTracking` 方法

**Files**:
- `internal/api/report.go`

**Implementation**:
```go
// handleInspirationTracking 处理灵感应用统计
func (s *ReportServer) handleInspirationTracking(ctx context.Context, userID string, trackingData *vai.TrackingEventData) {
    // 1. 验证 extra_data
    extraData := trackingData.GetExtraData()
    if extraData == "" {
        zlog.LogWithContext(ctx).Warn("inspiration tracking: empty extra_data")
        return
    }

    // 2. 解析 JSON
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

    // 3. 验证必填字段
    if data.InspirationPromptID == "" {
        zlog.LogWithContext(ctx).Warn("inspiration tracking: missing inspiration_prompt_id")
        return
    }

    // 4. 调用服务记录
    if err := s.inspirationPromptService.ApplyInspirationPrompt(ctx, data.InspirationPromptID, userID, data.ToolID, data.ToolType); err != nil {
        zlog.LogWithContext(ctx).Error("inspiration tracking: failed to record application",
            zap.Error(err),
            zap.String("prompt_id", data.InspirationPromptID),
            zap.String("user_id", userID))
        return
    }

    zlog.LogWithContext(ctx).Info("inspiration tracking: recorded application",
        zap.String("prompt_id", data.InspirationPromptID),
        zap.String("user_id", userID),
        zap.String("tool_id", data.ToolID))
}
```

**Validation**:
- [ ] 编译通过
- [ ] 添加单元测试覆盖所有分支
- [ ] 测试通过率 ≥90%

---

### Task 1.3: 集成灵感统计到 ReportUserEvent
**Description**: 在 `ReportUserEvent` 方法中调用灵感统计逻辑

**Files**:
- `internal/api/report.go`

**Changes**:
在 `ReportUserEvent` 方法的事件上报逻辑之后添加:
```go
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
```

**Validation**:
- [ ] 编译通过
- [ ] 添加集成测试验证完整流程
- [ ] 测试数据正确写入数据库

---

### Task 1.4: 更新 Bootstrap 依赖注入
**Description**: 在 `ServiceProvider` 中更新 `ReportServer` 初始化

**Files**:
- `internal/bootstrap/service_provider.go`

**Changes**:
```go
func (sp *ServiceProvider) InitReportServer() *api.ReportServer {
    return api.NewReportServer(
        sp.GetReportService(),
        sp.GetEventService(),
        sp.GetWatchService(),
        sp.GetInspirationPromptService(),  // 新增
    )
}
```

**Validation**:
- [ ] 编译通过
- [ ] 启动服务成功
- [ ] 依赖正确注入,服务可正常调用

---

### Task 1.5: 编写单元测试
**Description**: 为 `handleInspirationTracking` 编写完整的单元测试

**Files**:
- `internal/api/report_test.go` (新建或扩展)

**Test Cases**:
1. ✅ 正常场景: 有效的JSON数据,成功记录
2. ✅ extra_data为空字符串
3. ✅ JSON格式错误
4. ✅ 缺失 inspiration_prompt_id
5. ✅ 服务调用失败(数据库错误)
6. ✅ 只有 inspiration_prompt_id,其他字段为空(应该成功)

**Validation**:
- [ ] 测试覆盖率 ≥90%
- [ ] 所有测试通过
- [ ] 运行 `go test -race ./internal/api` 无竞态

---

### Task 1.6: 本地验证新逻辑
**Description**: 本地启动服务,手动测试新的统计逻辑

**Steps**:
1. 启动服务: `go run ./cmd -c conf/server.yaml`
2. 构造 `ReportUserEvent` 请求
3. 设置 `event_type = TRACKING_EVENT`
4. 设置 `TrackingEventData.event_type = TRACKING_EVENT_TYPE_TOOLS_DETAIL_CREATE_BTN`
5. 设置 `extra_data = {"inspiration_prompt_id":"test_prompt_1","tool_id":"test_tool_1"}`
6. 发送请求
7. 查询数据库验证记录生成

**Validation**:
- [ ] 请求返回成功
- [ ] 数据库中生成记录
- [ ] 日志输出正确
- [ ] 无错误日志

---

## Phase 2: 监控与数据一致性验证 (并行期)

### Task 2.1: 添加监控日志
**Description**: 增强日志记录,便于监控迁移进度

**Files**:
- `internal/api/report.go`
- `internal/api/prompt.go`

**Changes**:
- 在旧接口 `ApplyInspirationPrompt` 添加标识日志: "using deprecated API"
- 在新逻辑 `handleInspirationTracking` 添加标识日志: "using new event-based tracking"

**Validation**:
- [ ] 两种方式都能从日志中清晰区分
- [ ] 便于统计新旧接口的调用量

---

### Task 2.2: 部署到测试环境
**Description**: 将包含新逻辑的代码部署到测试环境

**Steps**:
1. 提交代码到feature分支
2. 触发CI/CD构建
3. 部署到测试环境
4. 烟雾测试

**Validation**:
- [ ] 服务正常启动
- [ ] 健康检查通过
- [ ] 旧接口仍可用
- [ ] 新逻辑能正常工作

---

### Task 2.3: 对比数据一致性
**Description**: 验证新旧方式记录的数据是否一致

**Steps**:
1. 使用相同参数分别调用旧接口和新方式
2. 查询数据库中的两条记录
3. 对比字段值是否一致

**SQL Example**:
```sql
SELECT * FROM inspiration_applications
WHERE user_id = 'test_user'
AND created_at > NOW() - INTERVAL 1 HOUR
ORDER BY created_at DESC
LIMIT 10;
```

**Validation**:
- [ ] 两条记录的 prompt_id 一致
- [ ] user_id 一致
- [ ] tool_id 一致
- [ ] tool_type 一致
- [ ] applied_at 时间接近

---

### Task 2.4: 客户端迁移准备
**Description**: 为客户端团队提供迁移文档和示例

**Deliverables**:
1. 迁移指南文档 (markdown)
2. 新旧方式对比示例代码
3. 常见问题FAQ

**Validation**:
- [ ] 文档清晰易懂
- [ ] 示例代码可直接运行
- [ ] 客户端团队确认理解

---

## Phase 3: 清理旧代码

### Task 3.1: 确认旧接口调用量为零
**Description**: 监控生产环境,确认客户端已完全迁移

**Monitoring**:
- 查询近7天的旧接口调用日志
- 统计调用次数
- 确认趋势降为0

**SQL Example**:
```sql
-- 假设有事件日志表
SELECT COUNT(*) FROM api_logs
WHERE endpoint = 'ApplyInspirationPrompt'
AND timestamp > NOW() - INTERVAL 7 DAY;
```

**Validation**:
- [ ] 旧接口调用量 = 0
- [ ] 新方式调用量稳定
- [ ] 统计数据连续性良好

---

### Task 3.3: 重新生成 proto 代码
**Description**: 执行 `make proto` 重新生成 Go 代码

**Command**:
```bash
make proto
```

**Expected**:
- 生成的代码中不再包含 `ApplyInspirationPrompt` 相关方法
- `internal/va_interface/service.pb.go` 更新
- `internal/va_interface/service_grpc.pb.go` 更新

**Validation**:
- [ ] `make proto` 执行成功
- [ ] 搜索生成代码,确认无 `ApplyInspirationPrompt`
- [ ] 编译失败(预期,因为实现代码还在)

---

### Task 3.4: 删除服务端实现代码
**Description**: 删除 `PromptServer.ApplyInspirationPrompt` 方法

**Files**:
- `internal/api/prompt.go`

**Changes**:
删除整个 `ApplyInspirationPrompt` 方法 (约233-316行)

**Validation**:
- [ ] 文件保存成功
- [ ] 编译通过 `go build ./...`
- [ ] 无未使用的import

---

### Task 3.5: 删除相关单元测试
**Description**: 删除旧接口的单元测试代码

**Files**:
- `internal/service/inspiration_prompt_test.go` (如果有针对旧API的测试)

**Changes**:
- 保留 `InspirationPromptService.ApplyInspirationPrompt` 的测试(核心逻辑)
- 删除 `PromptServer.ApplyInspirationPrompt` 的API层测试

**Validation**:
- [ ] 测试文件编译通过
- [ ] 运行 `go test ./...` 全部通过
- [ ] 覆盖率未降低

---

### Task 3.6: 运行完整测试套件
**Description**: 确保删除后系统功能正常

**Commands**:
```bash
# 格式化和检查
make fmt

# 单元测试
go test ./... -v

# 竞态检测
go test -race ./...

# 构建服务
go build -o build/server ./cmd

# 启动服务(本地验证)
./build/server -c conf/server.yaml
```

**Validation**:
- [ ] 所有测试通过
- [ ] 无竞态问题
- [ ] 服务正常启动
- [ ] 健康检查通过

---

### Task 3.7: 更新文档
**Description**: 更新相关文档,删除旧API的说明

**Files**:
- API文档
- 系统架构文档
- 客户端对接文档

**Changes**:
- 删除 `ApplyInspirationPrompt` API说明
- 更新为 `ReportUserEvent` + 事件类型的说明
- 添加迁移完成的标注

**Validation**:
- [ ] 文档更新完整
- [ ] 无遗漏的旧API引用
- [ ] 新方式说明清晰

---

### Task 3.8: 提交代码并创建PR
**Description**: 提交清理后的代码,创建Pull Request

**Commit Message**:
```
refactor: migrate inspiration tracking to event system

- Remove deprecated ApplyInspirationPrompt RPC
- Integrate inspiration tracking into ReportUserEvent
- Use TRACKING_EVENT_TYPE_TOOLS_DETAIL_CREATE_BTN event type
- Clean up unused API implementation and tests

BREAKING CHANGE: ApplyInspirationPrompt RPC removed, clients must use ReportUserEvent

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>
```

**PR Description**:
- 说明迁移背景和目的
- 列出主要变更
- 说明已完成的验证
- 附上数据一致性验证截图

**Validation**:
- [ ] PR创建成功
- [ ] CI检查全部通过
- [ ] Code Review无重大问题
- [ ] 合并到主分支

---

### Task 3.9: 生产环境部署和监控
**Description**: 部署到生产环境并密切监控

**Steps**:
1. 部署到生产环境
2. 监控服务启动状态
3. 监控错误日志
4. 监控灵感统计数据写入
5. 持续观察24小时

**Validation**:
- [ ] 服务正常启动
- [ ] 无报错日志
- [ ] 统计数据持续写入
- [ ] 无用户反馈问题

---

## Phase 4: 验证和归档

### Task 4.1: 数据完整性验证
**Description**: 验证迁移前后统计数据的连续性

**SQL Queries**:
```sql
-- 按天统计应用数
SELECT DATE(applied_at) as date, COUNT(*) as count
FROM inspiration_applications
WHERE applied_at > NOW() - INTERVAL 14 DAY
GROUP BY DATE(applied_at)
ORDER BY date;

-- 验证无数据断层
```

**Validation**:
- [ ] 迁移前后数据无断层
- [ ] 统计曲线平滑
- [ ] 无异常峰值或谷值

---

### Task 4.2: 性能验证
**Description**: 验证新方式的性能表现

**Metrics**:
- `ReportUserEvent` 接口 P99 延迟
- 灵感统计处理耗时
- 数据库写入性能

**Validation**:
- [ ] P99延迟 < 100ms
- [ ] 灵感统计耗时 < 5ms
- [ ] 无性能退化

---

### Task 4.3: 验证覆盖率
**Description**: 确认测试覆盖率达标

**Command**:
```bash
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

**Validation**:
- [ ] `internal/api/report.go` 覆盖率 ≥85%
- [ ] `internal/service/inspiration_prompt.go` 覆盖率 ≥85%
- [ ] 整体覆盖率未降低

---

### Task 4.4: 更新 OpenSpec 状态
**Description**: 将提案标记为已完成并归档

**Steps**:
1. 运行 `openspec archive migrate-inspiration-tracking`
2. 确认提案移动到 `openspec/changes/archive/` 目录
3. 更新项目文档链接

**Validation**:
- [ ] 提案成功归档
- [ ] 不再出现在活跃提案列表
- [ ] 归档记录包含完成日期

---

## Success Criteria

完成所有任务后,系统应满足:

✅ `ApplyInspirationPrompt` RPC已删除
✅ `ReportUserEvent` 正确处理灵感统计
✅ 统计数据准确无误
✅ 测试覆盖率 ≥85%
✅ 无性能退化
✅ 文档已更新
✅ 生产环境运行稳定

## Dependencies

**依赖关系图**:
```
Phase 1 (Task 1.1-1.6) → Phase 2 (Task 2.1-2.4) → Phase 3 (Task 3.1-3.9) → Phase 4 (Task 4.1-4.4)
                         ↑ 可并行                  ↑ 串行执行              ↑ 验证
```

**关键路径**:
- Phase 1完成后才能进入Phase 2
- Phase 3依赖客户端完全迁移(Task 3.1)
- Phase 4可与Phase 3部分并行

## Estimated Timeline

- Phase 1: 3-5天 (开发+测试)
- Phase 2: 1-3周 (客户端迁移周期)
- Phase 3: 2-3天 (清理+验证)
- Phase 4: 1天 (最终验证+归档)

**Total**: 约4-6周
