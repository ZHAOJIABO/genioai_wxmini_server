# Tasks: implement-cursor-based-inspiration-pagination

## Overview
实现基于游标的灵感推荐分页功能,分为Proto更新、Service层实现、API层集成、测试验证四个阶段。

---

## Phase 1: Proto Schema Update (依赖外部)

### Task 1.1: Update proto definition
**Description**: 在VA Interface proto仓库中更新 `GetInspirationPromptsResponse` 消息定义

**Changes**:
```protobuf
message GetInspirationPromptsResponse {
  ResponseHeader response_header = 1;
  repeated InspirationPrompt prompts = 2;
  string next_cursor = 3;  // 新增: 下次请求使用的cursor
  bool has_more = 4;       // 新增: 是否还有更多数据
}
```

**Validation**:
- [ ] Proto定义编译通过
- [ ] 字段编号无冲突
- [ ] 向后兼容性检查通过

**Dependencies**: Proto仓库访问权限

**Estimated Time**: 30分钟

---

### Task 1.2: Generate Go code
**Description**: 执行 `make proto` 重新生成Go代码

**Commands**:
```bash
make proto
```

**Validation**:
- [ ] 生成的Go代码无编译错误
- [ ] `GetInspirationPromptsResponse` 包含新字段
- [ ] IDE能正确识别新字段

**Dependencies**: Task 1.1完成

**Estimated Time**: 10分钟

---

## Phase 2: Service Layer Implementation (核心逻辑)

### Task 2.1: Remove random sorting logic
**Description**: 移除 `GetInspirationPrompts` 方法中的随机排序代码

**File**: `internal/service/inspiration_prompt.go`

**Changes**:
```go
// 删除以下代码:
// rand.Shuffle(len(promptsWithCount), func(i, j int) {
//     promptsWithCount[i], promptsWithCount[j] = promptsWithCount[j], promptsWithCount[i]
// })
```

**Validation**:
- [ ] `rand.Shuffle` 调用已完全移除
- [ ] 代码编译通过
- [ ] 静态分析无警告

**Dependencies**: 无

**Estimated Time**: 10分钟

---

### Task 2.2: Implement stable sorting algorithm
**Description**: 实现确定性多级排序逻辑

**File**: `internal/service/inspiration_prompt.go`

**Changes**:
```go
// 实现三级排序
sort.SliceStable(promptsWithCount, func(i, j int) bool {
    // 第一优先级: 应用数降序
    if promptsWithCount[i].Count != promptsWithCount[j].Count {
        return promptsWithCount[i].Count > promptsWithCount[j].Count
    }
    // 第二优先级: sort_weight降序
    if promptsWithCount[i].Prompt.SortWeight != promptsWithCount[j].Prompt.SortWeight {
        return promptsWithCount[i].Prompt.SortWeight > promptsWithCount[j].Prompt.SortWeight
    }
    // 第三优先级: prompt_id字典序
    return promptsWithCount[i].Prompt.PromptID < promptsWithCount[j].Prompt.PromptID
})
```

**Validation**:
- [ ] 排序逻辑正确实现
- [ ] 相同输入产生相同输出
- [ ] 单元测试验证排序稳定性

**Dependencies**: Task 2.1完成

**Estimated Time**: 20分钟

---

### Task 2.3: Implement cursor positioning helper
**Description**: 实现游标查找辅助函数

**File**: `internal/service/inspiration_prompt.go`

**Changes**:
```go
// findCursorPosition 查找cursor在列表中的位置
// 返回索引,如果未找到返回-1
func findCursorPosition(prompts []*model.InspirationPrompt, cursor string) int {
    if cursor == "" {
        return -1
    }

    for i, p := range prompts {
        if p.PromptID == cursor {
            return i
        }
    }

    return -1
}
```

**Validation**:
- [ ] 函数逻辑正确
- [ ] 空cursor返回-1
- [ ] 无效cursor返回-1
- [ ] 有效cursor返回正确索引

**Dependencies**: 无

**Estimated Time**: 15分钟

---

### Task 2.4: Implement pagination with cursor
**Description**: 实现基于游标的分页切片逻辑

**File**: `internal/service/inspiration_prompt.go`

**Changes**:
```go
// paginateWithCursor 基于游标分页,支持循环
func paginateWithCursor(
    prompts []*model.InspirationPrompt,
    cursor string,
    limit int,
) (
    result []*model.InspirationPrompt,
    nextCursor string,
    hasMore bool,
) {
    total := len(prompts)
    if total == 0 {
        return []*model.InspirationPrompt{}, "", false
    }

    if limit <= 0 {
        limit = 5
    }

    // 查找起始位置
    startIdx := findCursorPosition(prompts, cursor)
    if startIdx == -1 {
        startIdx = 0
    } else {
        startIdx = startIdx + 1
    }

    // 循环逻辑
    if startIdx >= total {
        startIdx = 0
    }

    // 计算结束位置
    endIdx := startIdx + limit
    if endIdx > total {
        endIdx = total
    }

    // 切片
    result = prompts[startIdx:endIdx]

    // 计算元数据
    if len(result) > 0 {
        nextCursor = result[len(result)-1].PromptID
        hasMore = endIdx < total
    } else {
        nextCursor = ""
        hasMore = false
    }

    return result, nextCursor, hasMore
}
```

**Validation**:
- [ ] 空列表返回空数组
- [ ] 首次请求(cursor="")从头开始
- [ ] 有效cursor返回后续数据
- [ ] 到达末尾正确循环
- [ ] nextCursor和hasMore正确计算

**Dependencies**: Task 2.3完成

**Estimated Time**: 30分钟

---

### Task 2.5: Update GetInspirationPrompts method signature
**Description**: 修改Service层方法签名和实现

**File**: `internal/service/inspiration_prompt.go`

**Changes**:
```go
// 修改方法签名
func (s *InspirationPromptService) GetInspirationPrompts(
    ctx context.Context,
    toolType string,
    cursor string,      // 新增参数
    limit int32,
) (
    prompts []*model.InspirationPrompt,
    nextCursor string,  // 新增返回值
    hasMore bool,       // 新增返回值
    err error,
)

// 方法实现中调用paginateWithCursor
result, nextCursor, hasMore := paginateWithCursor(sortedPrompts, cursor, int(limit))
return result, nextCursor, hasMore, nil
```

**Validation**:
- [ ] 方法签名更新正确
- [ ] 调用paginateWithCursor
- [ ] 返回值正确映射
- [ ] 编译通过

**Dependencies**: Task 2.4完成

**Estimated Time**: 15分钟

---

### Task 2.6: Add logging for cursor operations
**Description**: 增加日志记录游标处理过程

**File**: `internal/service/inspiration_prompt.go`

**Changes**:
```go
// 记录关键日志
zlog.LogWithContext(ctx).Info("灵感推荐分页",
    zap.String("tool_type", toolType),
    zap.String("cursor", cursor),
    zap.Int("total_count", totalCount),
    zap.Int("returned_count", len(result)),
    zap.String("next_cursor", nextCursor),
    zap.Bool("has_more", hasMore),
)

// cursor无效时记录警告
if cursor != "" && cursorIdx == -1 {
    zlog.LogWithContext(ctx).Warn("无效的cursor,从头开始",
        zap.String("cursor", cursor),
        zap.String("tool_type", toolType),
    )
}
```

**Validation**:
- [ ] 关键操作有日志记录
- [ ] 日志包含必要的上下文信息
- [ ] 日志级别合理(Info/Warn)

**Dependencies**: Task 2.5完成

**Estimated Time**: 15分钟

---

## Phase 3: API Layer Integration

### Task 3.1: Update API layer to extract cursor
**Description**: 修改API层从请求中提取cursor参数

**File**: `internal/api/prompt.go`

**Changes**:
```go
func (s *PromptServer) GetInspirationPrompts(
    ctx context.Context,
    req *vai.GetInspirationPromptsRequest,
) (*vai.GetInspirationPromptsResponse, error) {
    // ... 现有逻辑 ...

    // 提取cursor
    cursor := req.GetPromptIdCursor()

    // 调用Service层
    prompts, nextCursor, hasMore, err := s.InspirationPromptService.GetInspirationPrompts(
        ctx,
        toolType,
        cursor,  // 传递cursor
        5,
    )

    // ... 错误处理 ...
}
```

**Validation**:
- [ ] 正确提取cursor参数
- [ ] 传递给Service层
- [ ] 编译通过

**Dependencies**: Task 2.5完成, Task 1.2完成

**Estimated Time**: 15分钟

---

### Task 3.2: Fill response with pagination metadata
**Description**: 将分页元数据填充到响应中

**File**: `internal/api/prompt.go`

**Changes**:
```go
// 构造响应
rsp := &vai.GetInspirationPromptsResponse{
    ResponseHeader: &vai.ResponseHeader{Code: vai.StatusCode_SUCCESS},
    Prompts:        respPrompts,
    NextCursor:     nextCursor,  // 新增
    HasMore:        hasMore,     // 新增
}
```

**Validation**:
- [ ] next_cursor字段正确填充
- [ ] has_more字段正确填充
- [ ] 响应消息完整

**Dependencies**: Task 3.1完成

**Estimated Time**: 10分钟

---

### Task 3.3: Update API logging
**Description**: 更新API层日志,记录分页信息

**File**: `internal/api/prompt.go`

**Changes**:
```go
zlog.LogWithContext(ctx).Info("GetInspirationPrompts",
    zap.String("tool_id", req.GetToolId()),
    zap.String("cursor", cursor),              // 新增
    zap.String("next_cursor", nextCursor),     // 新增
    zap.Bool("has_more", hasMore),             // 新增
    zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
)
```

**Validation**:
- [ ] 日志包含分页信息
- [ ] 日志格式统一

**Dependencies**: Task 3.2完成

**Estimated Time**: 10分钟

---

## Phase 4: Testing

### Task 4.1: Write unit tests for cursor positioning
**Description**: 编写游标查找函数的单元测试

**File**: `internal/service/inspiration_prompt_test.go`

**Test Cases**:
```go
func TestFindCursorPosition(t *testing.T) {
    tests := []struct{
        name     string
        cursor   string
        expected int
    }{
        {"空cursor", "", -1},
        {"有效cursor", "p3", 2},
        {"无效cursor", "p999", -1},
        {"首个cursor", "p1", 0},
        {"最后cursor", "p8", 7},
    }
}
```

**Validation**:
- [ ] 所有测试用例通过
- [ ] 覆盖边界情况
- [ ] 断言清晰准确

**Dependencies**: Task 2.3完成

**Estimated Time**: 30分钟

---

### Task 4.2: Write unit tests for pagination logic
**Description**: 编写分页逻辑的单元测试

**File**: `internal/service/inspiration_prompt_test.go`

**Test Cases**:
```go
func TestPaginateWithCursor(t *testing.T) {
    tests := []struct{
        name          string
        cursor        string
        limit         int
        expectPrompts []string
        expectNext    string
        expectHasMore bool
    }{
        {"首次请求", "", 3, []string{"p1","p2","p3"}, "p3", true},
        {"第二页", "p3", 3, []string{"p4","p5","p6"}, "p6", true},
        {"最后一页", "p6", 3, []string{"p7","p8"}, "p8", false},
        {"循环回头", "p8", 3, []string{"p1","p2","p3"}, "p3", true},
        {"无效cursor", "p999", 3, []string{"p1","p2","p3"}, "p3", true},
        {"空列表", "", 3, []string{}, "", false},
        {"单条数据", "", 1, []string{"p1"}, "p1", false},
    }
}
```

**Validation**:
- [ ] 所有测试用例通过
- [ ] 覆盖循环逻辑
- [ ] 覆盖边界情况

**Dependencies**: Task 2.4完成

**Estimated Time**: 45分钟

---

### Task 4.3: Write unit tests for stable sorting
**Description**: 编写稳定排序的单元测试

**File**: `internal/service/inspiration_prompt_test.go`

**Test Cases**:
```go
func TestGetInspirationPrompts_StableSorting(t *testing.T) {
    // 测试多次调用结果一致
    // 测试多级排序规则
    // 测试相同count时按weight排序
    // 测试相同weight时按prompt_id排序
}
```

**Validation**:
- [ ] 验证排序确定性
- [ ] 验证多级排序规则
- [ ] 多次运行结果一致

**Dependencies**: Task 2.2完成

**Estimated Time**: 30分钟

---

### Task 4.4: Write unit tests for GetInspirationPrompts
**Description**: 编写Service层主方法的完整测试

**File**: `internal/service/inspiration_prompt_test.go`

**Test Cases**:
```go
func TestGetInspirationPrompts_WithCursor(t *testing.T) {
    // 测试完整的分页流程
    // 测试cursor传递
    // 测试返回值正确性
    // 测试错误场景
}
```

**Validation**:
- [ ] 测试覆盖率 ≥90%
- [ ] 所有场景测试通过
- [ ] 使用fake repository

**Dependencies**: Task 2.6完成

**Estimated Time**: 60分钟

---

### Task 4.5: Update existing random sorting tests
**Description**: 更新或删除现有的随机排序测试

**File**: `internal/service/inspiration_prompt_test.go`

**Changes**:
- 删除 `TestGetInspirationPrompts_SameCountRandomOrder`
- 修改相关断言,改为验证确定性排序

**Validation**:
- [ ] 旧测试已删除或更新
- [ ] 新测试覆盖相同场景
- [ ] 所有测试通过

**Dependencies**: Task 2.2完成

**Estimated Time**: 20分钟

---

### Task 4.6: Write integration test
**Description**: 编写端到端集成测试

**File**: `internal/api/prompt_test.go` (如不存在则创建)

**Test Cases**:
```go
func TestGetInspirationPromptsAPI_CursorPagination(t *testing.T) {
    // 模拟完整的RPC调用
    // 验证响应字段正确
    // 验证分页流程完整
}
```

**Validation**:
- [ ] 端到端流程验证通过
- [ ] 响应字段正确填充
- [ ] 模拟真实使用场景

**Dependencies**: Task 3.3完成

**Estimated Time**: 45分钟

---

### Task 4.7: Run all tests and verify coverage
**Description**: 运行所有测试并验证覆盖率

**Commands**:
```bash
go test ./internal/service/... -v
go test ./internal/api/... -v
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep inspiration
```

**Validation**:
- [ ] 所有测试通过
- [ ] Service层覆盖率 ≥90%
- [ ] API层覆盖率 ≥85%
- [ ] 无测试警告或跳过

**Dependencies**: Task 4.1-4.6完成

**Estimated Time**: 20分钟

---

## Phase 5: Code Quality & Documentation

### Task 5.1: Run make fmt
**Description**: 执行代码格式化和lint检查

**Commands**:
```bash
make fmt
```

**Validation**:
- [ ] 代码格式化通过
- [ ] Lint检查无错误
- [ ] 无需手动调整

**Dependencies**: 所有代码完成

**Estimated Time**: 10分钟

---

### Task 5.2: Add method documentation
**Description**: 为新方法和辅助函数添加注释

**Files**:
- `internal/service/inspiration_prompt.go`
- `internal/api/prompt.go`

**Requirements**:
- godoc格式注释
- 说明参数和返回值
- 说明核心逻辑和边界情况

**Validation**:
- [ ] 所有公开方法有注释
- [ ] 注释清晰准确
- [ ] godoc渲染正确

**Dependencies**: 所有代码完成

**Estimated Time**: 30分钟

---

### Task 5.3: Update API documentation
**Description**: 更新RPC接口文档(如果存在)

**File**: `docs/api/prompt.md` (或相应文档位置)

**Content**:
- 说明cursor参数的使用方法
- 说明next_cursor和has_more的含义
- 提供客户端使用示例

**Validation**:
- [ ] 文档完整准确
- [ ] 示例代码可运行
- [ ] 包含常见问题解答

**Dependencies**: 所有代码完成

**Estimated Time**: 30分钟

---

## Phase 6: Verification & Deployment

### Task 6.1: Manual testing
**Description**: 手动测试完整流程

**Test Steps**:
1. 启动本地服务
2. 使用grpcurl测试首次请求
3. 使用返回的next_cursor请求第二页
4. 连续请求直到循环回到开头
5. 测试无效cursor的容错性

**Validation**:
- [ ] 首次请求返回正确
- [ ] 分页逻辑工作正常
- [ ] 循环逻辑正确
- [ ] 容错机制有效

**Dependencies**: 所有测试通过

**Estimated Time**: 30分钟

---

### Task 6.2: Code review preparation
**Description**: 准备代码Review材料

**Deliverables**:
- 代码变更清单
- 测试覆盖率报告
- 性能测试结果
- 向后兼容性说明

**Validation**:
- [ ] 材料完整
- [ ] 变更说明清晰
- [ ] 风险评估完整

**Dependencies**: Task 6.1完成

**Estimated Time**: 20分钟

---

### Task 6.3: Performance testing
**Description**: 验证性能指标

**Test Cases**:
- 测试500条数据的响应时间
- 测试1000条数据的响应时间
- 测试游标查找耗时

**Validation**:
- [ ] 响应时间 < 100ms
- [ ] 游标查找 < 5ms
- [ ] 无性能回退

**Dependencies**: Task 6.1完成

**Estimated Time**: 30分钟

---

### Task 6.4: Create deployment checklist
**Description**: 创建部署检查清单

**Checklist**:
- [ ] Proto已更新并重新生成
- [ ] 所有测试通过
- [ ] make fmt通过
- [ ] 代码Review通过
- [ ] 向后兼容性验证
- [ ] 监控指标配置
- [ ] 回滚方案准备

**Dependencies**: 所有任务完成

**Estimated Time**: 15分钟

---

## Summary

### Total Estimated Time
- Phase 1: 40分钟
- Phase 2: 115分钟
- Phase 3: 35分钟
- Phase 4: 250分钟
- Phase 5: 70分钟
- Phase 6: 95分钟
- **Total: ~10小时**

### Critical Path
1. Task 1.1-1.2 (Proto更新)
2. Task 2.1-2.6 (Service层实现)
3. Task 3.1-3.3 (API层集成)
4. Task 4.1-4.7 (测试验证)
5. Task 5.1-5.3 (代码质量)
6. Task 6.1-6.4 (验证部署)

### Parallelizable Tasks
- Task 4.1, 4.2, 4.3 可并行执行
- Task 5.2, 5.3 可并行执行

### Key Milestones
1. ✅ Proto更新完成 (40分钟)
2. ✅ Service层实现完成 (2.5小时)
3. ✅ API层集成完成 (3小时)
4. ✅ 测试覆盖率达标 (7小时)
5. ✅ 代码Review通过 (9小时)
6. ✅ 准备就绪部署 (10小时)

### Risk Items
⚠️ Task 1.1: 依赖外部proto仓库,可能需要协调
⚠️ Task 4.4: 测试复杂度高,可能需要更多时间
⚠️ Task 6.1: 手动测试可能发现意外问题

### Dependencies外部依赖
- Proto仓库更新权限
- 客户端团队协调(用于最终验证)
