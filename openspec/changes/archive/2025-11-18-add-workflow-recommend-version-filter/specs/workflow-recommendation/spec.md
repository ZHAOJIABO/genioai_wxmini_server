# Specification: Workflow Recommendation

**capability-id**: `workflow-recommendation`
**status**: `draft`
**last-updated**: 2025-01-18

---

## ADDED Requirements

### Requirement: Version-Aware Recommendation MUST Filter By Client Version

**REQ-WFR-001**: Workflow推荐必须根据客户端版本和平台进行过滤

推荐系统 **MUST** 根据客户端的应用版本(appVersion)和操作系统平台(platform)过滤workflow列表,确保仅推荐客户端能够正常使用的workflow。

系统 **SHALL**:
- 根据 WorkflowKind 的版本约束字段过滤workflow
- 为 iOS 客户端使用 `IOSupportVersions` 字段
- 为 Android 客户端使用 `AndroidSupportVersions` 字段
- 使用 `utils.CheckVersionRange` 进行版本范围校验

**Rationale**:
- 避免推荐客户端不支持的功能,提升用户体验
- 保持与工具推荐(Tool Recommendation)的架构一致性
- 支持渐进式功能发布和版本控制

#### Scenario: iOS客户端版本过滤

**Given**:
- iOS客户端调用 `GetWorkflowRecommend`,appVersion="1.5.0", platform="ios"
- Workflow A 所属的 WorkflowKind 设置 `ios_support_versions=">=1.0.0,<2.0.0"`
- Workflow B 所属的 WorkflowKind 设置 `ios_support_versions=">=2.0.0"`

**When**:
- 服务端处理推荐请求

**Then**:
- 推荐列表包含 Workflow A (满足版本约束 1.5.0 ∈ [1.0.0, 2.0.0))
- 推荐列表不包含 Workflow B (不满足版本约束 1.5.0 < 2.0.0)

**Acceptance Criteria**:
- [x] 版本范围校验使用 `utils.CheckVersionRange` 工具函数
- [x] 版本约束格式支持 `>=x.y.z`, `<x.y.z`, `>=a,<b` 等语义化版本范围

#### Scenario: Android客户端版本过滤

**Given**:
- Android客户端调用 `GetWorkflowRecommend`,appVersion="3.0.0", platform="android"
- Workflow C 所属的 WorkflowKind 设置 `android_support_versions="<3.0.0"`
- Workflow D 所属的 WorkflowKind 设置 `android_support_versions=">=3.0.0"`

**When**:
- 服务端处理推荐请求

**Then**:
- 推荐列表不包含 Workflow C (不满足版本约束)
- 推荐列表包含 Workflow D (满足版本约束)

**Acceptance Criteria**:
- [x] 根据平台(ios/android)选择对应的版本约束字段
- [x] 平台映射使用 `constants.MappingOS` 标准化处理

#### Scenario: 无版本限制的Workflow

**Given**:
- 客户端调用 `GetWorkflowRecommend`,appVersion="1.0.0", platform="ios"
- Workflow E 所属的 WorkflowKind,`ios_support_versions=""` (空字符串)

**When**:
- 服务端处理推荐请求

**Then**:
- 推荐列表包含 Workflow E (无版本限制视为兼容所有版本)

**Acceptance Criteria**:
- [x] 版本字段为空时,跳过版本校验
- [x] 无版本限制的workflow对所有客户端可见

#### Scenario: WorkflowKind缺失时的降级

**Given**:
- Workflow F 的 `kind_id="kind_x"`,但数据库中不存在 `kind_x` 对应的 WorkflowKind 记录

**When**:
- 服务端处理推荐请求

**Then**:
- 推荐列表仍包含 Workflow F (宽松策略,避免误过滤)
- 日志记录 WARN 级别消息,包含 workflow_id 和缺失的 kind_id

**Acceptance Criteria**:
- [x] Kind缺失不阻断workflow展示
- [x] 异常情况有明确的日志记录

#### Scenario: 版本格式解析失败时的降级

**Given**:
- Workflow G 所属的 WorkflowKind,`ios_support_versions="invalid-format"`
- 客户端 appVersion="1.0.0"

**When**:
- `utils.CheckVersionRange` 返回解析错误

**Then**:
- 推荐列表包含 Workflow G (宽松策略)
- 日志记录 WARN 级别消息,包含错误详情和 workflow_id

**Acceptance Criteria**:
- [x] 版本解析失败时采用宽松策略(保留workflow)
- [x] 错误日志包含足够的排查信息(workflow_id, kind_id, version_string, error)

#### Scenario: 空版本参数时的行为

**Given**:
- 客户端未提供 appVersion 参数 (appVersion="")

**When**:
- 服务端处理推荐请求

**Then**:
- 跳过所有版本过滤逻辑
- 返回所有符合其他条件(如status=1)的workflow

**Acceptance Criteria**:
- [x] appVersion为空时,版本过滤函数直接返回原始列表
- [x] 向后兼容旧版本客户端

---

### Requirement: API Signature MUST Accept Version Parameters

**REQ-WFR-002**: `GetWorkflowRecommend` Service层函数签名必须接收版本参数

服务层函数 **MUST** 接收 `appVersion` 和 `platform` 参数,支持版本过滤逻辑。

API层 **SHALL**:
- 从请求头提取 `app.app_version` 作为 appVersion
- 使用 `constants.MappingOS` 标准化 platform 值
- 将版本参数传递给 Service层

**Rationale**:
- 版本过滤需要客户端版本信息
- 与 `GetToolRecommend` 保持API设计一致性

#### Scenario: API层参数传递

**Given**:
- 客户端调用 gRPC 接口 `GetWorkflowRecommend`,请求头包含:
  - `request_header.app.app_version = "1.5.0"`
  - `request_header.device.os = OS_IOS`

**When**:
- API层 `picture_forge.go:GetWorkflowRecommend` 处理请求

**Then**:
- 提取 `appVersion = "1.5.0"`
- 映射 `platform = constants.IOS`
- 调用 `pictureForgeService.GetWorkflowRecommend(ctx, limit, appVersion, platform)`

**Acceptance Criteria**:
- [x] API层正确提取版本和平台信息
- [x] 使用 `constants.MappingOS` 标准化平台值
- [x] 参数传递链路完整: API → Service → Filter

---

### Requirement: Version Filtering MUST Record Key Metrics

**REQ-WFR-003**: 版本过滤必须记录关键指标日志

系统 **MUST** 记录版本过滤的关键指标,便于监控和问题排查。

日志记录 **SHALL**:
- 使用 INFO 级别记录过滤前后的workflow数量变化
- 使用 WARN 级别记录异常情况 (Kind缺失、版本解析失败)
- 包含结构化字段: workflow_id, kind_id, app_version, platform
- 使用 zap 结构化日志框架

**Rationale**:
- 了解版本过滤的实际影响
- 快速定位版本配置错误

#### Scenario: 过滤指标日志

**Given**:
- 系统从Top10中随机选择10个workflow候选
- 版本过滤后剩余7个workflow

**When**:
- 完成推荐处理

**Then**:
- 日志记录以下信息 (INFO级别):
  ```
  workflow recommend completed
  - top10_count: 10
  - selected_count: 10
  - after_version_filter: 7
  - valid_count: 7
  - app_version: "1.5.0"
  - platform: "ios"
  ```

**Acceptance Criteria**:
- [x] 过滤前后的workflow数量变化可见
- [x] 包含客户端版本和平台信息
- [x] 使用结构化日志 (zap fields)

#### Scenario: 异常情况日志

**Given**:
- Workflow的Kind缺失或版本解析失败

**When**:
- 过滤逻辑遇到异常

**Then**:
- 记录 WARN 级别日志,包含:
  - workflow_id
  - kind_id (如适用)
  - 错误类型 (kind_missing / version_parse_error)
  - 错误详情

**Acceptance Criteria**:
- [x] 异常有明确的日志记录
- [x] 日志包含足够的排查上下文

---

## Implementation Notes

### 架构对齐

参考 `GetToolRecommend` 的实现模式 (service.go:157-228):
- API层提取版本参数
- Service层调用带版本过滤的列表函数
- 返回过滤后的结果

### 复用现有逻辑

- 版本范围校验: `utils.CheckVersionRange(appVersion, versionRange)`
- 平台映射: `constants.MappingOS(osType)`
- 版本过滤模式: 参考 `theme_management_service.go:175-195`

### 数据模型依赖

```go
// WorkflowKind (model/picture_forge.go:12-30)
type WorkflowKind struct {
    IOSupportVersions      string  // 如 ">=1.0.0,<2.0.0"
    AndroidSupportVersions string  // 如 ">=1.0.0"
    // ...
}
```

### 过滤函数签名

```go
func (s *PictureForgeService) filterWorkflowsByKindVersion(
    ctx context.Context,
    workflows []*model.Workflow,
    kindInfoMap map[string]*model.WorkflowKind,
    appVersion string,
    platform string,
) []*model.Workflow
```

---

## Testing Requirements

### Unit Test Coverage

- 目标覆盖率: ≥85%
- 核心测试场景:
  1. 版本匹配成功
  2. 版本不匹配过滤
  3. 无版本限制
  4. Kind缺失降级
  5. 版本解析失败降级
  6. 空版本参数跳过过滤

### Integration Test

- 使用不同版本客户端调用 `GetWorkflowRecommend`
- 验证返回的workflow均满足版本约束
- 检查日志输出符合预期

---

## Open Questions

1. **版本范围格式标准化**:
   - 当前依赖 `utils.CheckVersionRange`,支持哪些格式?
   - 建议: 文档化支持的版本范围语法,确保配置规范

2. **缓存策略**:
   - WorkflowKind查询是否需要缓存?
   - 当前: 不缓存,待监控推荐接口调用频率后决策

---

## References

- 参考实现: `GetToolRecommend` (service.go:157-228)
- 版本过滤: `ThemeManagementService.filterKindsByVersion` (theme_management_service.go:175-195)
- 工具过滤: `PictureToolsService.filterToolsByVersion` (picture_tools_service.go:310-343)
- 数据模型: `WorkflowKind` (model/picture_forge.go:12-30)
