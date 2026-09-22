# Spec: Cursor-based Inspiration Pagination

## ADDED Requirements

### Requirement: Cursor-based Pagination Mechanism
**ID**: INS-CURSOR-001
**Priority**: High
**Category**: Feature Enhancement

系统SHALL实现基于游标(cursor)的灵感推荐分页机制。当客户端传递`prompt_id_cursor`参数时,服务端MUST返回该cursor之后的灵感列表。当cursor指向最后一条数据时,服务端SHALL循环返回从第一条开始的数据,实现无限循环浏览体验。

#### Scenario: First request without cursor
**Given** 客户端首次请求灵感推荐
**And** `prompt_id_cursor` 字段为空
**When** 调用 `GetInspirationPrompts` RPC
**Then** 服务端应当返回排序后的前N条灵感(N由limit决定,默认5)
**And** 响应中 `next_cursor` 应当为返回列表中最后一条的 `prompt_id`
**And** 响应中 `has_more` 应当为 `true` (如果还有更多数据)

**Example**:
```
Request: {tool_id: "tool_123", prompt_id_cursor: ""}
Response: {
  prompts: [p1, p2, p3, p4, p5],
  next_cursor: "p5",
  has_more: true
}
```

#### Scenario: Paginate with valid cursor
**Given** 客户端已获取第一页数据
**And** `prompt_id_cursor` 为上次响应的 `next_cursor` 值(如"p5")
**When** 调用 `GetInspirationPrompts` RPC
**Then** 服务端应当查找cursor在排序列表中的位置
**And** 应当返回cursor之后的N条灵感
**And** 响应中 `next_cursor` 应当为本次返回列表中最后一条的 `prompt_id`
**And** 响应中 `has_more` 应当反映是否还有更多数据

**Example**:
```
Request: {tool_id: "tool_123", prompt_id_cursor: "p5"}
Response: {
  prompts: [p6, p7, p8, p9, p10],
  next_cursor: "p10",
  has_more: true
}
```

#### Scenario: Reach end of list
**Given** 客户端请求到达列表末尾
**And** `prompt_id_cursor` 指向倒数第N条数据
**When** 调用 `GetInspirationPrompts` RPC
**And** 剩余数据不足limit条
**Then** 服务端应当返回所有剩余的灵感
**And** 响应中 `next_cursor` 应当为最后一条的 `prompt_id`
**And** 响应中 `has_more` 应当为 `false`

**Example**:
```
Request: {tool_id: "tool_123", prompt_id_cursor: "p18"}
Total: 20条数据
Response: {
  prompts: [p19, p20],
  next_cursor: "p20",
  has_more: false
}
```

#### Scenario: Circular pagination at end
**Given** 客户端已到达列表末尾
**And** `prompt_id_cursor` 为最后一条数据的 `prompt_id`
**When** 客户端再次调用 `GetInspirationPrompts` RPC
**Then** 服务端应当循环回到列表开头
**And** 应当返回从第一条开始的N条灵感
**And** 响应中 `next_cursor` 应当为本次返回列表中最后一条的 `prompt_id`
**And** 响应中 `has_more` 应当为 `true`

**Example**:
```
Request: {tool_id: "tool_123", prompt_id_cursor: "p20"}
Response: {
  prompts: [p1, p2, p3, p4, p5],  // 循环回到开头
  next_cursor: "p5",
  has_more: true
}
```

#### Scenario: Handle invalid cursor gracefully
**Given** 客户端传递的 `prompt_id_cursor` 在当前列表中不存在
**And** 可能是因为该灵感被删除、禁用或cursor值错误
**When** 调用 `GetInspirationPrompts` RPC
**Then** 服务端应当将cursor视为无效
**And** 应当从列表开头开始返回数据
**And** 应当记录Warn级别日志,包含无效的cursor值
**And** 返回成功响应(不报错)

**Example**:
```
Request: {tool_id: "tool_123", prompt_id_cursor: "p999"}
Response: {
  prompts: [p1, p2, p3, p4, p5],  // 从头开始
  next_cursor: "p5",
  has_more: true
}
Log: WARN "invalid cursor: p999, reset to beginning"
```

#### Scenario: Handle empty inspiration list
**Given** 指定tool_type没有启用的灵感
**When** 调用 `GetInspirationPrompts` RPC
**Then** 服务端应当返回空数组
**And** 响应中 `next_cursor` 应当为空字符串
**And** 响应中 `has_more` 应当为 `false`

**Example**:
```
Request: {tool_id: "tool_999", prompt_id_cursor: ""}
Response: {
  prompts: [],
  next_cursor: "",
  has_more: false
}
```

### Requirement: Stable Sorting for Pagination
**ID**: INS-CURSOR-002
**Priority**: High
**Category**: Feature Enhancement

系统MUST实现确定性的稳定排序算法,确保多次调用时排序结果一致。排序规则SHALL按以下优先级执行:第一优先级为应用数降序,第二优先级为`sort_weight`降序,第三优先级为`prompt_id`字典序升序。系统MUST移除当前的随机排序逻辑(`rand.Shuffle`)。

#### Scenario: Consistent ordering across requests
**Given** 系统中有多个灵感,应用数统计相同
**When** 客户端多次调用 `GetInspirationPrompts` RPC
**And** 传递相同的参数(tool_id和cursor)
**Then** 每次返回的灵感顺序MUST完全一致
**And** 不应出现随机排序导致的顺序变化

**Example**:
```
灵感数据: p1(count=10), p2(count=5, weight=100), p3(count=5, weight=50)
第一次调用: [p1, p2, p3]
第二次调用: [p1, p2, p3]  // 顺序一致
第三次调用: [p1, p2, p3]  // 顺序一致
```

#### Scenario: Multi-level sorting rules
**Given** 系统中有灵感数据如下:
- p1: count=10, weight=0
- p2: count=5, weight=100
- p3: count=5, weight=50
- p4: count=5, weight=50, prompt_id="p4"
- p5: count=5, weight=50, prompt_id="p5"

**When** 系统执行排序
**Then** 排序结果应当为: [p1, p2, p3, p4, p5]
**And** 排序依据:
  - p1 排第一(count=10最大)
  - p2 排第二(count=5,但weight=100最大)
  - p3 排第三(count=5, weight=50)
  - p4 排在p5前面(count和weight相同,按prompt_id字典序)

#### Scenario: Remove random shuffle logic
**Given** 当前代码中存在 `rand.Shuffle` 调用
**When** 实施本变更
**Then** 应当删除所有随机排序相关代码
**And** 应当使用 `sort.SliceStable` 实现确定性排序
**And** 单元测试应当验证排序结果的稳定性

### Requirement: Proto Schema Enhancement
**ID**: INS-CURSOR-003
**Priority**: High
**Category**: API Enhancement

`GetInspirationPromptsResponse` proto消息MUST增加两个新字段:`next_cursor`(string类型)和`has_more`(bool类型)。这些字段SHALL为客户端提供分页所需的元数据,使客户端能够判断是否还有更多数据以及下次请求应使用的cursor值。

#### Scenario: Response includes next_cursor field
**Given** 服务端成功处理分页请求
**When** 构造响应消息
**Then** `next_cursor` 字段MUST包含本次返回列表中最后一条灵感的 `prompt_id`
**And** 如果返回列表为空,`next_cursor` 应当为空字符串

#### Scenario: Response includes has_more field
**Given** 服务端完成分页处理
**When** 构造响应消息
**Then** `has_more` 字段MUST准确反映是否还有未返回的数据
**And** 当 `has_more=false` 时,表示已到达列表末尾
**And** 客户端可根据此字段决定是否显示"加载更多"按钮

#### Scenario: Proto backward compatibility
**Given** 旧客户端尚未更新proto定义
**When** 旧客户端调用新服务端接口
**Then** protobuf向后兼容机制应当自动忽略新增字段
**And** 旧客户端应当能够正常解析 `prompts` 字段
**And** 不应出现反序列化错误

### Requirement: Service Layer Method Signature Update
**ID**: INS-CURSOR-004
**Priority**: High
**Category**: Implementation

`InspirationPromptService.GetInspirationPrompts` 方法签名MUST更新以支持游标分页。方法MUST接受 `cursor string` 参数,并返回三个值:`prompts`, `nextCursor`, `hasMore`。方法MUST正确实现游标查找、分页切片和循环逻辑。

#### Scenario: Service method accepts cursor parameter
**Given** API层接收到客户端请求
**When** 调用Service层方法
**Then** 应当将 `prompt_id_cursor` 作为参数传递给Service层
**And** Service层应当正确处理空cursor和非空cursor的情况

#### Scenario: Service method returns pagination metadata
**Given** Service层完成分页处理
**When** 返回结果到API层
**Then** 应当返回三个值: `prompts`, `nextCursor`, `hasMore`
**And** API层应当将这些值映射到proto响应消息中

#### Scenario: Service layer implements cursor search
**Given** Service层接收到非空cursor参数
**When** 在排序后的列表中查找cursor
**Then** 应当遍历列表查找匹配的 `prompt_id`
**And** 返回找到的索引位置
**And** 如果未找到,返回-1

#### Scenario: Service layer implements pagination slice
**Given** 已找到cursor位置(或确定起始位置)
**When** 执行分页切片
**Then** 起始索引应当为 `cursorIndex + 1` (cursor的下一个位置)
**And** 结束索引应当为 `startIndex + limit`
**And** 应当处理索引越界的情况(不超过列表长度)

#### Scenario: Service layer implements circular logic
**Given** cursor指向最后一条数据
**When** 计算起始索引时发现已到末尾
**Then** 应当将起始索引重置为0
**And** 应当从列表开头返回数据
**And** 日志应当记录循环行为

### Requirement: API Layer Integration
**ID**: INS-CURSOR-005
**Priority**: High
**Category**: Implementation

`PromptServer.GetInspirationPrompts` API方法MUST从请求中提取 `prompt_id_cursor`,并将其传递给Service层。API层MUST将Service层返回的 `nextCursor` 和 `hasMore` 填充到响应消息中。错误处理逻辑MUST保持现有的容错性,cursor处理失败不应导致整个请求失败。

#### Scenario: Extract cursor from request
**Given** 客户端请求包含 `prompt_id_cursor` 字段
**When** API层处理请求
**Then** 应当调用 `req.GetPromptIdCursor()` 提取cursor值
**And** 应当将cursor值传递给Service层方法

#### Scenario: Fill response metadata
**Given** Service层返回分页结果和元数据
**When** API层构造响应消息
**Then** 应当将 `nextCursor` 填充到 `response.next_cursor` 字段
**And** 应当将 `hasMore` 填充到 `response.has_more` 字段
**And** 应当保持现有的 `prompts` 字段填充逻辑

#### Scenario: Error handling does not break pagination
**Given** 在处理cursor时发生错误(如cursor无效)
**When** Service层容错处理该错误
**Then** Service层应当从头开始返回数据
**And** API层应当返回成功响应
**And** 不应因cursor问题导致请求失败

### Requirement: Remove Random Sorting Logic
**ID**: INS-CURSOR-006
**Priority**: High
**Category**: Code Refactoring

当前 `GetInspirationPrompts` 方法中的随机排序逻辑(`rand.Shuffle`)MUST被移除。该随机性与游标分页机制冲突,导致cursor无法准确定位。移除后,系统SHALL完全依赖确定性的多级排序算法。

#### Scenario: Identify and remove rand.Shuffle calls
**Given** 当前代码中存在以下逻辑:
```go
rand.Shuffle(len(promptsWithCount), func(i, j int) {
    promptsWithCount[i], promptsWithCount[j] = promptsWithCount[j], promptsWithCount[i]
})
```
**When** 实施重构
**Then** 应当删除上述 `rand.Shuffle` 调用
**And** 应当直接使用 `sort.SliceStable` 进行稳定排序

#### Scenario: Update unit tests to remove randomness checks
**Given** 现有单元测试验证随机排序行为
**When** 移除随机逻辑后
**Then** 应当删除或修改相关测试用例
**And** 应当新增测试验证排序结果的确定性

### Requirement: Default Limit Value Handling
**ID**: INS-CURSOR-007
**Priority**: Medium
**Category**: Implementation

当 `limit` 参数为0或负数时,系统SHALL使用默认值5。该逻辑MUST在Service层实现,确保API层无需关心limit的合法性验证。

#### Scenario: Handle zero limit
**Given** Service层接收到 `limit=0`
**When** 进行参数校验
**Then** 应当将limit重置为默认值5
**And** 日志应当记录参数重置行为

#### Scenario: Handle negative limit
**Given** Service层接收到 `limit=-1`
**When** 进行参数校验
**Then** 应当将limit重置为默认值5
**And** 日志应当记录参数重置行为

## Non-Functional Requirements

### Requirement: Performance
**ID**: INS-CURSOR-NFR-001
**Priority**: High

游标分页逻辑MUST NOT显著增加接口响应时间。系统SHALL满足以下性能指标:

- 游标查找耗时: < 5ms (线性查找,n<1000)
- 排序耗时: < 10ms (O(n log n), n<1000)
- 总体额外耗时: < 20ms
- 分页逻辑MUST NOT成为性能瓶颈

#### Scenario: Performance within acceptable limits
**Given** 系统有1000条灵感数据
**When** 执行游标分页操作
**Then** 总耗时MUST < 20ms
**And** 不应成为接口响应的瓶颈

### Requirement: Reliability
**ID**: INS-CURSOR-NFR-002
**Priority**: High

Cursor处理失败MUST NOT影响接口可用性。系统SHALL实现以下容错机制:

- 无效cursor时MUST自动从头开始,不报错
- 排序结果变化导致cursor失效时MUST容错处理
- 所有边界情况MUST有明确的降级策略
- 错误MUST记录日志便于排查

#### Scenario: Graceful degradation on cursor errors
**Given** 游标处理出现错误
**When** Service层检测到错误
**Then** MUST从头开始返回数据
**And** MUST NOT返回错误响应

### Requirement: Consistency
**ID**: INS-CURSOR-NFR-003
**Priority**: High

排序结果SHALL在短期内保持一致。系统MUST满足以下一致性要求:

- 相同参数的多次调用MUST返回相同顺序
- 统计数据变化时允许排序结果变化(非强一致性)
- 客户端SHALL能容忍cursor偶尔失效的情况

#### Scenario: Consistent ordering for same parameters
**Given** 客户端使用相同参数调用多次
**When** 统计数据未变化
**Then** 返回结果的顺序MUST一致

### Requirement: Backward Compatibility
**ID**: INS-CURSOR-NFR-004
**Priority**: High

新版本MUST与旧客户端兼容。系统SHALL满足以下兼容性要求:

- 旧客户端不传 `prompt_id_cursor` 时,行为MUST与旧版本一致
- Protobuf新增字段MUST NOT影响旧客户端反序列化
- 旧客户端MUST能正常解析 `prompts` 字段
- 变更MUST NOT引入Breaking Change

#### Scenario: Old client compatibility
**Given** 旧客户端未更新proto定义
**When** 调用新版本服务端接口
**Then** 旧客户端MUST能正常工作
**And** MUST能正确解析响应数据

## Testing Requirements

### Requirement: Unit Test Coverage
**ID**: INS-CURSOR-TEST-001
**Priority**: High

`GetInspirationPrompts` 方法MUST达到 ≥90% 的测试覆盖率。测试SHALL覆盖以下场景:

- ✅ 空cursor首次请求
- ✅ 有效cursor分页
- ✅ 无效cursor容错
- ✅ 到达末尾循环
- ✅ 空列表边界情况
- ✅ 单条数据循环
- ✅ Limit边界值(0, 负数, 超大值)
- ✅ 排序稳定性验证
- ✅ Cursor查找性能

#### Scenario: Comprehensive test coverage
**Given** 实现了游标分页功能
**When** 运行单元测试套件
**Then** 覆盖率MUST ≥ 90%
**And** 所有核心场景MUST被测试

### Requirement: Integration Test Coverage
**ID**: INS-CURSOR-TEST-002
**Priority**: High

系统MUST提供端到端的集成测试,验证完整的分页流程。测试SHALL覆盖:

- 端到端分页流程测试
- API层与Service层集成验证
- Proto字段正确填充验证
- 完整的用户浏览流程模拟

#### Scenario: End-to-end pagination flow
**Given** 完整的系统环境
**When** 执行集成测试
**Then** MUST验证从API到Service的完整流程
**And** MUST验证proto响应字段正确

### Requirement: Performance Test
**ID**: INS-CURSOR-TEST-003
**Priority**: Medium

系统SHALL通过性能测试,验证在大数据量下的响应时间。测试MUST验证:

- 1000条数据下的响应时间 < 100ms
- 游标查找不成为瓶颈
- 高并发场景的压测

#### Scenario: Performance under load
**Given** 系统有1000条测试数据
**When** 执行性能测试
**Then** 响应时间MUST < 100ms
**And** 游标查找MUST NOT成为瓶颈

## Documentation Requirements

### Requirement: Code Documentation
**ID**: INS-CURSOR-DOC-001
**Priority**: High

`GetInspirationPrompts` 方法MUST包含详细的代码注释。注释SHALL说明:

- 游标机制的工作原理
- 排序规则和优先级
- 循环逻辑和边界情况

#### Scenario: Comprehensive code comments
**Given** 代码实现完成
**When** Review代码
**Then** MUST包含详细的函数和逻辑注释
**And** 注释MUST清晰说明关键逻辑

### Requirement: API Documentation
**ID**: INS-CURSOR-DOC-002
**Priority**: High

系统MUST更新RPC接口文档。文档SHALL包含:

- `prompt_id_cursor` 参数的用法说明
- `next_cursor` 和 `has_more` 字段的含义
- 客户端使用示例代码

#### Scenario: Updated API documentation
**Given** API变更已完成
**When** 用户查阅文档
**Then** MUST能找到新字段的完整说明
**And** MUST有清晰的使用示例

### Requirement: Migration Guide
**ID**: INS-CURSOR-DOC-003
**Priority**: Medium

系统SHALL提供客户端迁移指南。指南MUST包含:

- 如何利用新字段实现"加载更多"功能
- 向后兼容性保证说明
- 常见问题解答

#### Scenario: Client migration guidance
**Given** 客户端开发者需要适配新接口
**When** 查阅迁移指南
**Then** MUST有清晰的迁移步骤
**And** MUST有向后兼容性说明

## Acceptance Criteria

本需求被认为完成,当且仅当满足以下所有条件:

1. ✅ Service层实现游标分页逻辑,单元测试覆盖率≥90%
2. ✅ API层正确集成Service层,传递cursor参数并填充响应字段
3. ✅ Proto定义更新,增加 `next_cursor` 和 `has_more` 字段
4. ✅ 排序逻辑移除随机性,实现稳定排序
5. ✅ 循环逻辑正确实现,到达末尾后自动回到开头
6. ✅ 所有边界情况都有正确的容错处理
7. ✅ 集成测试验证完整的分页流程
8. ✅ 性能测试通过,响应时间增加<20ms
9. ✅ 向后兼容性验证,旧客户端正常工作
10. ✅ 代码Review通过,文档齐全
11. ✅ `make proto` 成功执行,生成的代码无编译错误
12. ✅ `make fmt` 和所有测试通过

## Dependencies

- **Proto更新**: 需要在VA Interface proto仓库中添加新字段
- **客户端适配**: 客户端需要更新proto定义并维护cursor状态
- **代码Review**: 需要团队Review排序逻辑的变更
- **灰度发布**: 建议分阶段发布,观察线上表现

## Rollback Plan

如果线上出现问题,回滚策略:

1. **保留向后兼容**: 旧客户端不受影响,只需回滚服务端代码
2. **快速回滚**: Git revert相关提交,重新部署
3. **数据无影响**: 不涉及数据库schema变更,无需数据迁移
4. **监控指标**: 监控错误率、响应时间、cursor失效率

## Related Requirements

- 关联历史提案: `2025-11-18-migrate-inspiration-tracking`
- 未来增强: 个性化推荐排序、A/B测试支持
