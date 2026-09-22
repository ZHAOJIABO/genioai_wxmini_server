# Proposal: implement-cursor-based-inspiration-pagination

## Overview

实现基于游标(cursor)的灵感推荐分页机制,使用户每次调用 `GetInspirationPrompts` RPC时能够获取不同的灵感内容,形成循环浏览体验。

## Why

当前 `GetInspirationPrompts` 接口存在以下问题:

1. **重复推荐**: 每次调用返回相同的结果(按应用数排序的前N条),用户无法查看更多灵感
2. **用户体验差**: 用户希望能够"刷新"看到不同的灵感推荐,而不是每次都看到相同内容
3. **内容利用率低**: 大量优质灵感被排在后面,用户难以发现

Proto定义中已添加 `prompt_id_cursor` 字段,但Service层和API层尚未实现相应逻辑。

## What Changes

### 核心变更

1. **游标机制实现**:
   - 客户端传递 `prompt_id_cursor` 表示上次推荐的最后一个prompt_id
   - 服务端返回该cursor之后的N条灵感
   - 如果cursor是最后一条,则从头开始循环返回

2. **排序稳定性保证**:
   - 移除当前的随机排序(`rand.Shuffle`)
   - 使用确定性多级排序:应用数降序 → sort_weight降序 → prompt_id字典序
   - 确保多次调用时排序结果一致

3. **循环逻辑**:
   - 当 `prompt_id_cursor` 为空时,返回前N条
   - 当cursor有值时,查找该ID在排序列表中的位置,返回后续N条
   - 当cursor指向最后一条或不存在时,循环返回前N条

4. **响应增强**:
   - 在Response中添加 `next_cursor` 字段(下次请求应传递的cursor值)
   - 添加 `has_more` 字段(是否还有更多数据,false表示已到末尾,下次将循环)

### Proto变更

**已完成**:
- ✅ `GetInspirationPromptsRequest.prompt_id_cursor` 已添加

**待添加** (需要重新生成proto):
```protobuf
message GetInspirationPromptsResponse {
  ResponseHeader response_header = 1;
  repeated InspirationPrompt prompts = 2;
  string next_cursor = 3;  // 新增: 下次请求应使用的cursor
  bool has_more = 4;       // 新增: 是否还有更多数据(false表示已到末尾)
}
```

### 代码变更范围

1. **Service层** (`internal/service/inspiration_prompt.go`):
   - 修改 `GetInspirationPrompts` 方法签名,增加 `cursor string` 参数
   - 实现稳定排序逻辑(移除rand.Shuffle)
   - 实现游标查找和循环逻辑
   - 返回值增加 `nextCursor` 和 `hasMore`

2. **API层** (`internal/api/prompt.go`):
   - 从请求中提取 `prompt_id_cursor`
   - 调用Service层方法时传递cursor参数
   - 将 `next_cursor` 和 `has_more` 填充到响应中

3. **测试** (`internal/service/inspiration_prompt_test.go`):
   - 新增游标分页测试用例
   - 新增循环逻辑测试用例
   - 新增边界情况测试(空列表、单条数据、cursor不存在等)

## Benefits

1. **提升用户体验**: 用户可以通过连续调用浏览所有灵感,不再重复看到相同内容
2. **内容曝光均衡**: 所有灵感都有机会被用户看到,不局限于热门Top N
3. **客户端实现简单**: 客户端只需维护 `next_cursor` 字段,无需复杂状态管理
4. **无需服务端状态**: 纯无状态设计,符合RESTful原则,无需Redis存储
5. **循环浏览**: 自动循环回到开头,提供无限浏览体验

## Implementation Strategy

### Phase 1: Service层实现 (Week 1)
1. 修改排序逻辑,移除随机性
2. 实现游标查找逻辑
3. 实现循环返回逻辑
4. 编写单元测试,覆盖率 ≥85%

### Phase 2: API层集成 (Week 1)
1. 修改 `GetInspirationPrompts` API处理逻辑
2. 传递cursor参数到Service层
3. 填充响应字段

### Phase 3: Proto更新与测试 (Week 1-2)
1. 更新proto定义(添加 `next_cursor` 和 `has_more` 字段)
2. 执行 `make proto` 重新生成代码
3. 集成测试验证完整流程
4. 更新API文档

### Phase 4: 客户端适配 (Week 2-3)
1. 客户端更新proto定义
2. 维护 `next_cursor` 状态
3. 实现"加载更多"或"刷新推荐"功能
4. 灰度发布验证

## Technical Details

### 稳定排序算法

```go
// 多级稳定排序,确保结果可重现
sort.SliceStable(promptsWithCount, func(i, j int) bool {
    // 第一优先级: 应用数降序
    if promptsWithCount[i].Count != promptsWithCount[j].Count {
        return promptsWithCount[i].Count > promptsWithCount[j].Count
    }
    // 第二优先级: sort_weight降序
    if promptsWithCount[i].Prompt.SortWeight != promptsWithCount[j].Prompt.SortWeight {
        return promptsWithCount[i].Prompt.SortWeight > promptsWithCount[j].Prompt.SortWeight
    }
    // 第三优先级: prompt_id字典序(确保完全稳定)
    return promptsWithCount[i].Prompt.PromptID < promptsWithCount[j].Prompt.PromptID
})
```

### 游标查找逻辑

```go
func findCursorPosition(prompts []*model.InspirationPrompt, cursor string) int {
    if cursor == "" {
        return -1  // 无cursor,从头开始
    }

    for i, p := range prompts {
        if p.PromptID == cursor {
            return i  // 找到cursor位置
        }
    }

    return -1  // cursor不存在或已过期,从头开始
}
```

### 分页与循环逻辑

```go
func paginateWithCursor(prompts []*model.InspirationPrompt, cursor string, limit int) (
    result []*model.InspirationPrompt,
    nextCursor string,
    hasMore bool,
) {
    total := len(prompts)
    if total == 0 {
        return []*model.InspirationPrompt{}, "", false
    }

    // 查找cursor位置
    startIdx := findCursorPosition(prompts, cursor)
    if startIdx == -1 {
        // 无cursor或cursor无效,从头开始
        startIdx = 0
    } else {
        // 从cursor的下一个位置开始
        startIdx = startIdx + 1
    }

    // 如果已经到末尾,循环回到开头
    if startIdx >= total {
        startIdx = 0
    }

    // 计算结束位置
    endIdx := startIdx + limit
    if endIdx > total {
        endIdx = total
    }

    // 切片获取结果
    result = prompts[startIdx:endIdx]

    // 计算next_cursor和has_more
    if len(result) > 0 {
        nextCursor = result[len(result)-1].PromptID
        hasMore = endIdx < total  // 还有更多数据
    } else {
        nextCursor = ""
        hasMore = false
    }

    return result, nextCursor, hasMore
}
```

## Risks & Mitigations

### Risk 1: 排序结果变化导致cursor失效
**场景**: 在用户浏览期间,如果应用数统计发生变化(有新用户应用灵感),排序结果可能变化

**Mitigation**:
- 短期内(如1小时内)应用数统计变化不会太大,影响有限
- 即使cursor失效,最差情况是从头开始,不会报错
- 后续可考虑缓存用户的排序快照(可选优化)

### Risk 2: Proto字段添加导致向后兼容性问题
**场景**: 旧客户端不识别新字段

**Mitigation**:
- Protobuf天然支持向后兼容,旧客户端会忽略新字段
- 新字段设置为可选(optional),不影响旧客户端
- `prompt_id_cursor` 为空时,行为与旧版本一致

### Risk 3: 性能问题
**场景**: 每次需要获取所有数据并排序

**Mitigation**:
- 当前灵感数量预计不会太大(<1000条),排序开销可接受
- 后续可增加缓存优化(按tool_type缓存排序结果)
- 游标查找使用线性扫描,复杂度O(n),可接受

### Risk 4: Cursor被篡改或伪造
**场景**: 客户端可能传递错误的cursor值

**Mitigation**:
- Cursor不存在时自动从头开始,不报错
- 不依赖cursor的合法性,容错性强
- 后续可考虑签名验证(可选安全增强)

## Success Criteria

1. ✅ 用户连续调用接口能够获取不同的灵感推荐
2. ✅ 到达末尾后自动循环回到开头
3. ✅ 排序结果稳定,多次调用顺序一致
4. ✅ `next_cursor` 和 `has_more` 字段正确返回
5. ✅ 单元测试覆盖率 ≥85%
6. ✅ 集成测试验证完整流程
7. ✅ 向后兼容旧客户端(cursor为空时行为一致)
8. ✅ 性能无明显下降(<50ms额外耗时)

## Timeline

- **Week 1 Day 1-2**: Service层实现与单元测试
- **Week 1 Day 3**: API层集成
- **Week 1 Day 4-5**: Proto更新与集成测试
- **Week 2**: 文档编写与代码Review
- **Week 2-3**: 客户端适配与灰度发布

## Related Changes

- 影响文件:
  - `internal/service/inspiration_prompt.go` (核心逻辑)
  - `internal/api/prompt.go` (API集成)
  - `internal/service/inspiration_prompt_test.go` (测试)
  - `internal/va_interface/prompt.proto` (Proto定义)

- 相关历史提案:
  - `2025-11-18-migrate-inspiration-tracking`: 灵感统计迁移到事件上报

## Open Questions

### Q1: Proto Response字段是否需要添加?
**当前状态**: Request中已有 `prompt_id_cursor`,但Response中缺少 `next_cursor` 和 `has_more`

**选项**:
- A: 添加新字段,提供完整的游标信息(推荐)
- B: 客户端根据返回数量推断是否有更多(不够准确)

**决策**: 建议选择A,提供明确的分页信息

### Q2: Limit值是否需要可配置?
**当前状态**: API层硬编码 `limit=5`

**选项**:
- A: 保持固定值,简化逻辑
- B: 在Request中增加 `limit` 字段,支持可配置

**决策**: 建议先保持固定值,后续根据需求调整

### Q3: 是否需要缓存排序结果?
**当前状态**: 每次请求都重新排序

**选项**:
- A: 先不优化,验证功能后再考虑
- B: 立即实现缓存(按tool_type + date缓存)

**决策**: 建议选择A,遵循奥卡姆剃刀原则,先实现最简单的方案

### Q4: 是否需要支持按语言筛选?
**当前状态**: 返回所有语言版本

**选项**:
- A: 保持现状,不筛选语言
- B: 根据用户语言偏好筛选

**决策**: 建议保持现状,语言筛选可作为后续独立需求处理

## Notes

- 本提案采用纯无状态设计,无需引入Redis或其他存储
- 游标机制简单可靠,易于理解和维护
- 符合RESTful设计原则和奥卡姆剃刀原则
- Proto变更需要协调客户端团队同步更新
