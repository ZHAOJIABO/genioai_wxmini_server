# Design: Cursor-based Inspiration Pagination

## Architecture Overview

实现基于游标的分页机制,使灵感推荐接口支持循环浏览功能。采用纯无状态设计,所有状态由客户端维护。

### Current Architecture

```
Client
  └─> GetInspirationPrompts(tool_id) RPC
        └─> PromptServer.GetInspirationPrompts()
              └─> InspirationPromptService.GetInspirationPrompts(toolType, limit)
                    ├─> 获取所有启用的灵感
                    ├─> 统计应用数
                    ├─> 随机排序 + 应用数排序
                    └─> 返回前N条 (固定结果)
```

**问题**: 每次调用返回相同的前N条结果,无法浏览更多内容

### Target Architecture

```
Client (维护cursor状态)
  └─> GetInspirationPrompts(tool_id, cursor) RPC
        └─> PromptServer.GetInspirationPrompts()
              └─> InspirationPromptService.GetInspirationPrompts(toolType, cursor, limit)
                    ├─> 获取所有启用的灵感
                    ├─> 统计应用数
                    ├─> 稳定排序 (应用数 → sort_weight → prompt_id)
                    ├─> 查找cursor位置
                    ├─> 切片返回后续N条
                    └─> 返回 (prompts, next_cursor, has_more)
```

**改进**: 支持游标分页,循环浏览所有灵感

## Component Design

### 1. Data Flow

```
┌─────────────────────────────────────────────────────────────┐
│ Client Request                                               │
│ {tool_id: "tool_123", prompt_id_cursor: "p5"}               │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ API Layer (prompt.go)                                        │
│ 1. 验证请求参数                                              │
│ 2. 通过tool_id查询tool_type                                 │
│ 3. 调用Service层: GetInspirationPrompts(tool_type, cursor)  │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ Service Layer (inspiration_prompt.go)                        │
│                                                              │
│ Step 1: 查询所有启用灵感                                     │
│   ├─> DAO.GetPromptsByToolType(tool_type)                   │
│   └─> 结果: [p1, p2, p3, p4, p5, p6, p7, p8]                │
│                                                              │
│ Step 2: 统计近3天应用数                                      │
│   ├─> DAO.CountApplicationsBatch(promptIDs, threeDaysAgo)   │
│   └─> 结果: {p1:10, p2:5, p3:5, p4:3, p5:3, p6:1, p7:0,...} │
│                                                              │
│ Step 3: 稳定排序                                             │
│   ├─> 按应用数降序                                           │
│   ├─> 应用数相同时按sort_weight降序                         │
│   ├─> 再相同时按prompt_id字典序                             │
│   └─> 排序后: [p1, p2, p3, p4, p5, p6, p7, p8]              │
│                                                              │
│ Step 4: 查找cursor位置                                       │
│   ├─> 输入cursor="p5"                                        │
│   ├─> 在排序列表中查找p5的索引                               │
│   └─> 找到索引=4                                             │
│                                                              │
│ Step 5: 分页切片                                             │
│   ├─> startIdx = 4 + 1 = 5 (从cursor的下一个位置开始)       │
│   ├─> endIdx = 5 + 5 = 10 (但实际只有8条)                    │
│   ├─> 实际endIdx = 8                                         │
│   └─> 切片结果: [p6, p7, p8]                                 │
│                                                              │
│ Step 6: 计算next_cursor和has_more                           │
│   ├─> next_cursor = "p8" (最后一条的ID)                     │
│   └─> has_more = false (已到末尾)                            │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ Response                                                     │
│ {                                                            │
│   prompts: [p6, p7, p8],                                     │
│   next_cursor: "p8",                                         │
│   has_more: false                                            │
│ }                                                            │
└─────────────────────────────────────────────────────────────┘
```

### 2. Stable Sorting Algorithm

排序稳定性是游标机制的关键前提条件。

#### 2.1 问题分析

当前代码使用随机排序:
```go
// 问题: 每次调用结果不同,cursor无法定位
rand.Shuffle(len(promptsWithCount), func(i, j int) {
    promptsWithCount[i], promptsWithCount[j] = promptsWithCount[j], promptsWithCount[i]
})

sort.SliceStable(promptsWithCount, func(i, j int) bool {
    return promptsWithCount[i].Count > promptsWithCount[j].Count
})
```

#### 2.2 解决方案: 多级确定性排序

```go
// 完全确定的排序规则,保证结果可重现
sort.SliceStable(promptsWithCount, func(i, j int) bool {
    // 第一优先级: 应用数降序 (热门优先)
    if promptsWithCount[i].Count != promptsWithCount[j].Count {
        return promptsWithCount[i].Count > promptsWithCount[j].Count
    }

    // 第二优先级: sort_weight降序 (管理员配置的权重)
    if promptsWithCount[i].Prompt.SortWeight != promptsWithCount[j].Prompt.SortWeight {
        return promptsWithCount[i].Prompt.SortWeight > promptsWithCount[j].Prompt.SortWeight
    }

    // 第三优先级: prompt_id字典序 (保证完全确定性)
    return promptsWithCount[i].Prompt.PromptID < promptsWithCount[j].Prompt.PromptID
})
```

**优点**:
- ✅ 完全确定性,相同输入产生相同输出
- ✅ 兼顾热门推荐(应用数)和运营配置(sort_weight)
- ✅ prompt_id作为最终排序依据,保证稳定性

**权衡**:
- ⚠️ 失去了随机性,用户每次看到的初始顺序相同
- ✅ 但通过游标机制可以浏览所有内容,整体体验更好

### 3. Cursor Positioning Logic

#### 3.1 游标查找函数

```go
// findCursorPosition 在排序后的列表中查找cursor位置
// 返回值: cursor所在的索引,如果未找到返回-1
func findCursorPosition(prompts []*model.InspirationPrompt, cursor string) int {
    if cursor == "" {
        return -1  // 空cursor表示首次请求
    }

    for i, p := range prompts {
        if p.PromptID == cursor {
            return i
        }
    }

    return -1  // cursor不存在(可能已被删除或禁用)
}
```

**时间复杂度**: O(n), 可接受(灵感数量预计<1000)

**后续优化** (可选):
- 使用 map 存储 prompt_id -> index 映射,降低到 O(1)
- 当前场景下优化收益不大,保持简单即可

#### 3.2 分页与循环逻辑

```go
// paginateWithCursor 基于游标进行分页,支持循环
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

    // 边界情况: 空列表
    if total == 0 {
        return []*model.InspirationPrompt{}, "", false
    }

    // 边界情况: limit非法
    if limit <= 0 {
        limit = 5  // 默认值
    }

    // 步骤1: 查找起始位置
    startIdx := findCursorPosition(prompts, cursor)
    if startIdx == -1 {
        // 情况A: 首次请求(cursor为空)
        // 情况B: cursor无效(已被删除或禁用)
        // 处理: 从头开始
        startIdx = 0
    } else {
        // 情况C: cursor有效
        // 处理: 从cursor的下一个位置开始
        startIdx = startIdx + 1
    }

    // 步骤2: 处理循环逻辑
    if startIdx >= total {
        // 已经到达或超过末尾,循环回到开头
        startIdx = 0
    }

    // 步骤3: 计算结束位置
    endIdx := startIdx + limit
    if endIdx > total {
        endIdx = total
    }

    // 步骤4: 切片获取结果
    result = prompts[startIdx:endIdx]

    // 步骤5: 计算响应元数据
    if len(result) > 0 {
        // next_cursor = 本次返回的最后一条的ID
        nextCursor = result[len(result)-1].PromptID

        // has_more = 是否还有更多未读数据
        hasMore = endIdx < total
    } else {
        // 理论上不应该发生(除非total=0,但前面已处理)
        nextCursor = ""
        hasMore = false
    }

    return result, nextCursor, hasMore
}
```

### 4. State Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    客户端状态机                              │
└─────────────────────────────────────────────────────────────┘

     [初始状态]
         │
         │ 用户进入灵感推荐页
         ▼
   ┌──────────┐
   │ cursor="" │ ──────────────┐
   └──────────┘               │
         │                    │
         │ 请求: cursor=""    │
         ▼                    │
   ┌──────────────────┐       │
   │ 收到: [p1...p5]   │       │
   │ next_cursor="p5" │       │
   │ has_more=true    │       │
   └──────────────────┘       │
         │                    │
         │ 用户点击"查看更多" │
         ▼                    │
   ┌──────────┐               │
   │ cursor="p5"│ ─────────┐  │
   └──────────┘            │  │
         │                 │  │
         │ 请求: cursor="p5"  │
         ▼                 │  │
   ┌──────────────────┐    │  │
   │ 收到: [p6...p10]  │    │  │
   │ next_cursor="p10"│    │  │
   │ has_more=true    │    │  │
   └──────────────────┘    │  │
         │                 │  │
         │ 用户继续点击    │  │
         ▼                 │  │
   ┌──────────┐            │  │
   │cursor="p10"│ ──────┐  │  │
   └──────────┘         │  │  │
         │              │  │  │
         │ 请求: cursor="p10" │
         ▼              │  │  │
   ┌──────────────────┐ │  │  │
   │ 收到: [p11...p12]│ │  │  │
   │ next_cursor="p12"│ │  │  │
   │ has_more=false   │ │  │  │ ← 已到末尾
   └──────────────────┘ │  │  │
         │              │  │  │
         │ 用户再次点击 │  │  │
         ▼              │  │  │
   ┌──────────┐         │  │  │
   │cursor="p12"│        │  │  │
   └──────────┘         │  │  │
         │              │  │  │
         │ 请求: cursor="p12" │
         ▼              │  │  │
   ┌──────────────────┐ │  │  │
   │ 收到: [p1...p5]   │ │  │  │ ← 循环回到开头
   │ next_cursor="p5" │ │  │  │
   │ has_more=true    │ │  │  │
   └──────────────────┘ │  │  │
         │              │  │  │
         └──────────────┴──┴──┘
            (循环往复)
```

### 5. Service Layer Implementation

#### 5.1 方法签名变更

**Before**:
```go
func (s *InspirationPromptService) GetInspirationPrompts(
    ctx context.Context,
    toolType string,
    limit int32,
) ([]*model.InspirationPrompt, error)
```

**After**:
```go
func (s *InspirationPromptService) GetInspirationPrompts(
    ctx context.Context,
    toolType string,
    cursor string,     // 新增: 游标参数
    limit int32,
) (
    prompts []*model.InspirationPrompt,
    nextCursor string,  // 新增: 下次请求的cursor
    hasMore bool,       // 新增: 是否还有更多数据
    err error,
)
```

#### 5.2 完整实现伪代码

```go
func (s *InspirationPromptService) GetInspirationPrompts(
    ctx context.Context,
    toolType string,
    cursor string,
    limit int32,
) ([]*model.InspirationPrompt, string, bool, error) {
    // 1. 参数校验与默认值
    if limit <= 0 {
        limit = 5
    }

    // 2. 获取所有启用的灵感
    allPrompts, err := s.repo.GetPromptsByToolType(toolType)
    if err != nil {
        zlog.LogWithContext(ctx).Error("获取灵感列表失败", zap.Error(err))
        return nil, "", false, fmt.Errorf("获取灵感列表失败: %w", err)
    }

    if len(allPrompts) == 0 {
        return []*model.InspirationPrompt{}, "", false, nil
    }

    // 3. 统计应用数
    threeDaysAgo := time.Now().Add(-72 * time.Hour)
    promptIDs := extractPromptIDs(allPrompts)
    countMap, err := s.repo.CountApplicationsBatch(promptIDs, threeDaysAgo)
    if err != nil {
        zlog.LogWithContext(ctx).Error("统计应用数失败", zap.Error(err))
        return nil, "", false, fmt.Errorf("统计应用数失败: %w", err)
    }

    // 4. 组装PromptWithCount
    promptsWithCount := buildPromptsWithCount(allPrompts, countMap)

    // 5. 稳定排序 (移除随机性)
    sort.SliceStable(promptsWithCount, func(i, j int) bool {
        if promptsWithCount[i].Count != promptsWithCount[j].Count {
            return promptsWithCount[i].Count > promptsWithCount[j].Count
        }
        if promptsWithCount[i].Prompt.SortWeight != promptsWithCount[j].Prompt.SortWeight {
            return promptsWithCount[i].Prompt.SortWeight > promptsWithCount[j].Prompt.SortWeight
        }
        return promptsWithCount[i].Prompt.PromptID < promptsWithCount[j].Prompt.PromptID
    })

    // 6. 提取排序后的Prompt列表
    sortedPrompts := extractPrompts(promptsWithCount)

    // 7. 基于游标分页
    result, nextCursor, hasMore := paginateWithCursor(sortedPrompts, cursor, int(limit))

    // 8. 日志记录
    zlog.LogWithContext(ctx).Info("灵感推荐成功",
        zap.String("tool_type", toolType),
        zap.String("cursor", cursor),
        zap.Int("total", len(sortedPrompts)),
        zap.Int("returned", len(result)),
        zap.String("next_cursor", nextCursor),
        zap.Bool("has_more", hasMore))

    return result, nextCursor, hasMore, nil
}
```

### 6. API Layer Integration

#### 6.1 请求处理

```go
func (s *PromptServer) GetInspirationPrompts(
    ctx context.Context,
    req *vai.GetInspirationPromptsRequest,
) (*vai.GetInspirationPromptsResponse, error) {
    // ... 现有的参数校验逻辑 ...

    // 获取tool_type
    toolType := getToolType(ctx, req.GetToolId())

    // 提取cursor参数
    cursor := req.GetPromptIdCursor()  // 新增

    // 调用Service层
    prompts, nextCursor, hasMore, err := s.InspirationPromptService.GetInspirationPrompts(
        ctx,
        toolType,
        cursor,  // 新增参数
        5,       // 固定limit
    )

    if err != nil {
        // ... 错误处理 ...
    }

    // 转换为协议格式
    respPrompts := convertToProtoPrompts(prompts)

    // 构造响应 (包含新字段)
    rsp := &vai.GetInspirationPromptsResponse{
        ResponseHeader: &vai.ResponseHeader{Code: vai.StatusCode_SUCCESS},
        Prompts:        respPrompts,
        NextCursor:     nextCursor,  // 新增
        HasMore:        hasMore,     // 新增
    }

    return rsp, nil
}
```

### 7. Edge Cases & Error Handling

| 场景 | 输入 | 预期行为 | 实现策略 |
|------|------|----------|----------|
| 首次请求 | cursor="" | 返回前N条 | startIdx=0 |
| 正常分页 | cursor="p5" | 返回p5之后的N条 | 查找索引+1 |
| 到达末尾 | cursor="p_last" | 循环返回前N条 | startIdx=0, hasMore=true |
| Cursor无效 | cursor="p999" | 从头开始 | 查找失败视为cursor="" |
| 空列表 | 无数据 | 返回空数组 | 提前返回 |
| Limit=0 | limit=0 | 使用默认值5 | 参数校验 |
| 单条数据 | total=1 | 循环返回该条 | 正常处理 |
| 数据变化 | 排序结果改变 | Cursor失效,从头开始 | 容错处理 |

### 8. Performance Considerations

#### 8.1 时间复杂度分析

| 操作 | 复杂度 | 数据量假设 | 预计耗时 |
|------|--------|-----------|---------|
| 查询所有灵感 | O(n) | n=500 | <10ms |
| 批量统计应用数 | O(n) | n=500 | <20ms |
| 排序 | O(n log n) | n=500 | <5ms |
| 游标查找 | O(n) | n=500 | <1ms |
| 切片 | O(1) | - | <1ms |
| **总计** | **O(n log n)** | **n=500** | **<40ms** |

**结论**: 性能开销在可接受范围内,无需特殊优化

#### 8.2 优化方向 (可选)

1. **缓存排序结果**:
   - Key: `inspiration:sorted:{tool_type}:{date}`
   - TTL: 1小时
   - 收益: 避免重复排序,降低到O(n)

2. **游标查找优化**:
   - 使用 map 存储 `{prompt_id: index}` 映射
   - 降低查找复杂度到O(1)
   - 收益: 可忽略(当前耗时<1ms)

3. **预加载统计数据**:
   - 后台定时任务更新应用数缓存
   - 收益: 降低实时查询压力

**决策**: 当前不实施优化,待性能监控数据支撑后再决策

### 9. Testing Strategy

#### 9.1 单元测试用例

```go
// 测试分组
func TestGetInspirationPrompts_CursorPagination(t *testing.T) {
    tests := []struct {
        name           string
        cursor         string
        limit          int
        expectPrompts  []string  // 预期返回的prompt_id列表
        expectNext     string
        expectHasMore  bool
    }{
        {
            name:          "首次请求_cursor为空",
            cursor:        "",
            limit:         3,
            expectPrompts: []string{"p1", "p2", "p3"},
            expectNext:    "p3",
            expectHasMore: true,
        },
        {
            name:          "第二页_cursor有效",
            cursor:        "p3",
            limit:         3,
            expectPrompts: []string{"p4", "p5", "p6"},
            expectNext:    "p6",
            expectHasMore: true,
        },
        {
            name:          "最后一页_未满limit",
            cursor:        "p6",
            limit:         3,
            expectPrompts: []string{"p7", "p8"},
            expectNext:    "p8",
            expectHasMore: false,
        },
        {
            name:          "循环回到开头",
            cursor:        "p8",
            limit:         3,
            expectPrompts: []string{"p1", "p2", "p3"},
            expectNext:    "p3",
            expectHasMore: true,
        },
        {
            name:          "cursor无效_从头开始",
            cursor:        "p999",
            limit:         3,
            expectPrompts: []string{"p1", "p2", "p3"},
            expectNext:    "p3",
            expectHasMore: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // 测试实现...
        })
    }
}

func TestGetInspirationPrompts_StableSorting(t *testing.T) {
    // 测试排序稳定性: 多次调用结果一致
}

func TestGetInspirationPrompts_EmptyList(t *testing.T) {
    // 测试空列表场景
}

func TestGetInspirationPrompts_SingleItem(t *testing.T) {
    // 测试单条数据循环
}
```

#### 9.2 集成测试

```go
func TestGetInspirationPromptsAPI_FullCycle(t *testing.T) {
    // 端到端测试: 模拟用户完整浏览流程
    // 1. 首次请求
    // 2. 连续翻页
    // 3. 到达末尾
    // 4. 循环回到开头
}
```

### 10. Monitoring & Observability

#### 10.1 关键指标

```go
// Prometheus指标定义
var (
    inspirationRequestTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "inspiration_request_total",
            Help: "Total number of inspiration requests",
        },
        []string{"tool_type", "has_cursor"},
    )

    inspirationCursorInvalid = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "inspiration_cursor_invalid_total",
            Help: "Total number of invalid cursor requests",
        },
        []string{"tool_type"},
    )

    inspirationPaginationPosition = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "inspiration_pagination_position",
            Help:    "Distribution of pagination positions",
            Buckets: []float64{0, 0.25, 0.5, 0.75, 1.0},
        },
        []string{"tool_type"},
    )
)
```

#### 10.2 日志埋点

```go
zlog.LogWithContext(ctx).Info("灵感推荐",
    zap.String("tool_type", toolType),
    zap.String("cursor", cursor),
    zap.Int("total_count", totalCount),
    zap.Int("returned_count", len(result)),
    zap.String("next_cursor", nextCursor),
    zap.Bool("has_more", hasMore),
    zap.Bool("cursor_valid", cursorValid),
    zap.Int64("latency_ms", latency),
)
```

## Trade-offs

### Pros ✅
- **简单实现**: 无需引入新依赖(Redis等)
- **无状态设计**: 符合RESTful原则,易于扩展
- **用户体验好**: 支持循环浏览所有内容
- **性能可接受**: 开销<50ms,无需优化
- **向后兼容**: cursor为空时行为与旧版本一致

### Cons ⚠️
- **排序变化影响**: 统计数据变化时cursor可能失效
- **无随机性**: 失去了原有的随机排序特性
- **需要proto变更**: 需要协调客户端同步更新

### Decision ✅
综合考虑,该方案的优点远大于缺点,建议实施。

## Alternatives Considered

### Alternative 1: Offset-based分页
**方案**: 使用 `offset` + `limit` 参数

**Pros**: 更通用,易于理解

**Cons**:
- 需要客户端维护offset数值
- offset语义不如cursor直观
- 性能与cursor方案相同

**Decision**: 拒绝。Cursor方案语义更清晰,客户端维护更简单

### Alternative 2: 服务端状态管理
**方案**: Redis存储用户浏览进度

**Pros**: 多端同步状态

**Cons**:
- 增加系统复杂度
- 引入Redis依赖
- 需要处理过期和并发

**Decision**: 拒绝。违反奥卡姆剃刀原则,收益不明显

### Alternative 3: 时间窗口随机
**方案**: 按小时/天固定随机种子

**Pros**: 保留随机性

**Cons**:
- 无法实现真正的分页
- 用户体验差(无法浏览全部)

**Decision**: 拒绝。不满足核心需求

## Security Considerations

### 1. Cursor伪造
**风险**: 客户端可能伪造cursor值

**Mitigation**:
- Cursor不存在时自动回退到首页,不报错
- 不依赖cursor的合法性,容错性强
- 后续可考虑签名验证(HMAC)

### 2. 性能攻击
**风险**: 恶意客户端高频请求

**Mitigation**:
- API层限流(Rate Limiting)
- 监控异常请求模式
- 缓存排序结果

## Future Enhancements

1. **个性化推荐**: 基于用户历史行为调整排序
2. **A/B测试**: 支持多种排序策略对比
3. **缓存优化**: 按tool_type缓存排序结果
4. **签名验证**: 增强cursor安全性
5. **统计优化**: 预计算应用数,减少实时查询

## References

- [Cursor Pagination vs Offset Pagination](https://slack.engineering/evolving-api-pagination-at-slack/)
- [gRPC Pagination Best Practices](https://cloud.google.com/apis/design/design_patterns#list_pagination)
- [Go Sorting Stability](https://go.dev/blog/slices-intro)
