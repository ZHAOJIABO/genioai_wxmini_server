# Proposal: Add Version Filtering to Workflow Recommendation

**change-id**: `add-workflow-recommend-version-filter`
**capability**: `workflow-recommendation`
**status**: `draft`
**created**: 2025-01-18

## Problem Statement

当前 `GetWorkflowRecommend` 函数存在严重缺陷:未对客户端版本进行兼容性检查,可能推荐客户端无法使用的workflow,导致:

1. **用户体验问题**: 用户看到推荐但无法使用
2. **版本兼容性破坏**: 低版本客户端可能收到高版本功能的推荐
3. **架构不一致**: 同类功能 `GetToolRecommend` 已正确实现版本过滤,但workflow推荐缺失

### 现状分析

**当前实现问题 (service.go:567-631)**:
```go
func (s *PictureForgeService) GetWorkflowRecommend(ctx context.Context, limit int32) ([]*vai.Workflow, error) {
    // ❌ 缺少 appVersion, platform 参数
    // ❌ 无版本兼容性检查
    workflows, err := s.picForgeDao.ListWorkflowsByIDs(ctx, selectedIDs)
    // 仅过滤了禁用状态,未过滤版本
}
```

**对比正确实现 (service.go:157-228)**:
```go
func (s *PictureForgeService) GetToolRecommend(ctx context.Context, appVersion, platform string) {
    // ✅ 接收版本参数
    toolsInfo, err := s.ListPictureTools(ctx, appVersion, platform)
    // ✅ 内部已进行版本过滤
}
```

### 技术依据

1. **模型支持**: `WorkflowKind` 已有版本字段 (model/picture_forge.go:28-29):
   ```go
   IOSupportVersions      string  // iOS版本约束,如 ">=1.0.0,<2.0.0"
   AndroidSupportVersions string  // Android版本约束
   ```

2. **成熟逻辑**: 已有可复用的版本过滤实现:
   - `theme_management_service.go:175-195` - WorkflowKind级别的版本过滤
   - `picture_tools_service.go:310-343` - Tool级别的版本过滤
   - `utils.CheckVersionRange()` - 版本范围校验工具函数

3. **API层支持**: 请求头已包含必要信息 (api/picture_forge.go:1196-1203):
   ```go
   req.GetRequestHeader().GetApp().GetAppVersion()
   req.GetRequestHeader().GetDevice().GetOs()
   ```

## Proposed Solution

### 核心改动

1. **签名修改**: 为 `GetWorkflowRecommend` 添加版本参数
2. **过滤逻辑**: 在workflow加载后、转换为protobuf前进行版本过滤
3. **架构对齐**: 与 `GetToolRecommend` 保持一致的实现模式

### 实现策略

**分层设计**:
```
API Layer (picture_forge.go:1196)
  ↓ 提取 appVersion, platform
Service Layer (service.go:567)
  ↓ 传递版本参数
  ↓ 调用 ListWorkflowsByIDs (已过滤status)
  ↓ 批量加载 WorkflowKind
  ↓ 【新增】版本过滤 filterWorkflowsByKindVersion
  ↓ 转换为 protobuf
```

**版本过滤点**:
- 位置: 在 `GetKindInfoMap` 之后,`convertWorkflowToProto` 之前
- 依据: WorkflowKind 的版本约束字段
- 逻辑: 复用 `utils.CheckVersionRange` + 平台匹配

### 降级策略

1. **appVersion为空**: 跳过版本过滤,返回所有候选
2. **版本格式错误**: 记录警告日志,包含该workflow(宽松策略)
3. **Kind缺失**: 保留workflow(避免误过滤)

## Scope & Impact

### 变更范围

**修改文件**:
1. `internal/service/picture_forge/service.go` - 核心逻辑
2. `internal/api/picture_forge.go` - API层参数传递

**影响接口**:
- gRPC: `PictureForgeService.GetWorkflowRecommend` (仅内部实现,无破坏性)

### 非功能影响

- **性能**: 批量查询WorkflowKind,单次额外1个DB查询 (已在convertWorkflowToProto中执行,可复用)
- **兼容性**: 向后兼容,旧客户端传空版本时无影响
- **可测性**: 可单元测试 (Fake DAO + 版本过滤逻辑)

## Success Criteria

1. **功能正确性**:
   - iOS客户端仅收到满足 `IOSupportVersions` 的workflow
   - Android客户端仅收到满足 `AndroidSupportVersions` 的workflow
   - 版本范围格式: 支持 `>=1.0.0`, `<2.0.0`, `>=1.0.0,<2.0.0` 等

2. **边界处理**:
   - WorkflowKind无版本限制时,workflow正常返回
   - 版本字段为空时,视为无限制
   - 版本解析失败时,记录日志并保留workflow

3. **架构一致性**:
   - 与 `GetToolRecommend` 实现模式保持一致
   - 复用现有 `filterKindsByVersion` 相关逻辑

4. **测试覆盖**:
   - 单元测试覆盖率 ≥85%
   - 场景覆盖: 成功过滤、无限制、解析失败、kind缺失

## Questions & Clarifications

1. **降级策略确认**: 版本解析失败时,是保留workflow(宽松)还是过滤掉(严格)?
   - **建议**: 宽松策略,避免误伤,记录warn日志供排查

2. **平台过滤**: WorkflowKind模型只有iOS/Android版本字段,无平台显示字段,是否需要?
   - **当前**: 依据OS类型选择对应版本字段即可

3. **缓存策略**: WorkflowKind查询是否需要缓存?
   - **当前**: 推荐接口调用频率不高,先不缓存,后续监控后决策

## Related Work

- **参考实现**: `GetToolRecommend` (service.go:157-228)
- **依赖工具**: `utils.CheckVersionRange` - 版本范围校验
- **关联模型**: `WorkflowKind.IOSupportVersions/AndroidSupportVersions`

## Out of Scope

- Workflow级别的版本字段 (当前仅Kind级别控制,符合设计)
- 推荐算法调整 (仅添加过滤,不改排序/选择逻辑)
- 性能优化 (如缓存WorkflowKind,留待后续)
