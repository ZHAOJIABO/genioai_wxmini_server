# Slot流速控制设计

> 目标：以统一、健壮、可维护的方式管理“用户任务流速”，覆盖提交队列长度与执行并发两条维度；通过原子占位、终态释放、周期性对账与降级策略，确保 Redis 与数据库状态一致，避免误拒与过载；本文档为完整设计说明，不再体现重构阶段信息。

## 一、术语与口径
- 队列长度（MaxQueueSize）：某用户处于非终态的任务数量（`PENDING + PROCESSING + PENDING_SUBMISSION_RETRY + AWAITING_PROVIDER_COMPLETION`）。用于限制“提交速率”，在提交时检查。
- 并发执行数（MaxConcurrent）：某用户正在执行的任务数量（仅 `PROCESSING`）。用于限制“执行速率”，在任务从 `PENDING` 进入 `PROCESSING` 时检查与占位。
- Redis键：
  - 并发计数键：`visionai:concurrent:<userID>`（数值），集合键：`visionai:concurrent:<userID>:tasks`
  - 队列计数键：`visionai:queue:<userID>`（数值），集合键：`visionai:queue:<userID>:tasks`
  - TTL：使用统一配置 `key_ttl_seconds` 控制（例如 3600 秒）。

## 二、配置与加载
- 任务限制配置（JSON，经 `TaskConfigManager` 加载，带缓存与校验）：
  - 新格式：`{"member": {"max_queue_size": 6, "max_concurrent": 3}, "non_member": {"max_queue_size": 2, "max_concurrent": 1}}`
  - 旧格式兼容：`{"member": 6, "non_member": 2}` 自动转换为新格式（并发值按约定或默认推导）。
  - 预留字段：`priority`、`channel`（fast/normal/slow）、`rate_limit`（每分钟）方便未来扩展“优先级/快速通道/动态限流”。
- 常量键：`ConfigKeyTaskConcurrencyLimits`（唯一使用）。

## 三、核心流程
1) 提交阶段（控制队列长度）
- 入口：`PictureTaskService.SubmitTaskWithTx`
- 步骤：
  - 加载配置（会员/非会员差异）；
  - 统计 active 任务数（DB 真实口径）；若 `>= MaxQueueSize` → 拒绝提交（`ERR_TASK_QUEUE_LIMIT_EXCEEDED`）；
  - Redis原子占位队列槽位：`CheckAndReserveQueueSlot(userID, taskID, MaxQueueSize)`（INCR + SADD）；DB 持久化任务为 `PENDING`；
  - 失败路径：释放队列槽位（幂等）。

2) 调度执行阶段（控制并发执行）
- 入口：`TaskStateManager.TransitionToProcessing` 或执行器调度器（`picture_task_executor`）
- 步骤：
  - 分布式锁（防并发争抢）；验证任务状态为 `PENDING`；
  - Redis原子占位并发槽位：`CheckAndReserveConcurrentSlot(userID, taskID, MaxConcurrent)`；
  - 成功后将任务更新为 `PROCESSING`；更新失败或提交执行失败时回滚并发占位；
  - 在 `PROCESSING` 中由具体执行器完成后续处理。

3) 终态释放（生命周期闭环）
- 入口：任务完成/失败（`COMPLETED/FAILED`）的统一 Hook：`RegisterOnTaskTerminatedHook`
- 行为：
  - 释放并发槽位：`ReleaseConcurrentSlot(userID, taskID)`；
  - 释放队列槽位：`ReleaseQueueSlot(userID, taskID)`；
- 幂等：释放脚本检查集合成员存在性；重复释放不会产生负效应；降级模式下跳过写操作（见第五章）。

## 四、Redis原子脚本（占位/释放）
- 占位：
  - 读取当前计数 → 若 `>= max_limit` 返回失败；
  - 成功时 `INCR` 计数并 `SADD <key>:tasks taskID`；设置统一 TTL；
- 释放：
  - `SISMEMBER` 验证任务是否存在于集合；存在则 `SREM` 并且在计数 `> 0` 时 `DECR`；否则返回未释放（日志提示）。
- 键构建统一：`buildKey(quotaType, userID)`，集合键以 `:tasks` 后缀拼接；TTL 来自配置。

## 五、降级与恢复（高可用）
- Redis健康监测：`RedisHealthChecker` 周期检测；发现连接异常切入降级模式（`degradeMode=true`）。
- 降级策略：
  - 占位检查：退化到数据库统计（仅并发维度使用 DB `PROCESSING` 统计）；队列维度仍以 DB 统计为准；
  - 释放：降级模式下跳过 Redis 写操作（避免异常期间状态进一步紊乱），依赖后续对账器纠偏；
  - 恢复：健康检查自动恢复后退出降级模式。

## 六、对账器（一致性纠偏）
1) 并发槽位对账器（`ConcurrentSlotReconciler`）
- 作用：周期性扫描 `visionai:concurrent:*:tasks` 集合，逐个任务比对 DB 状态；若任务不存在或非 active → 释放并发槽位。
- 机制：集群锁（互斥）、WorkerPool、每轮超时控制、按批 SCAN/SSCAN、降级模式跳过。

2) 队列槽位对账器（`QueueSlotReconciler`）
- 作用：周期性扫描 `visionai:queue:*:tasks` 集合，基于 DB 真实状态释放异常队列占位。
- 机制同并发对账器；Active 状态口径与提交时一致。

3) 统一适配与工具
- 适配器：`UnifiedReleaserAdapter` 将 `TaskQuotaChecker` 的统一释放能力桥接为对账器可用接口；并为并发/队列提供兼容适配器。
- Key解析：`ParseUserIDFromKey(prefix, suffix)` 统一实现；避免重复代码与解析错误。
- 指标：Prometheus 指标按统一命名与标签（type=concurrent|queue）采集运行次数、耗时、扫描键/任务量、释放成功/错误、降级跳过等；可按需启用。

## 七、与业务代码的集成
- `PictureTaskService`：
  - `checkActiveTasks` 拆分口径，先队列长度检查，再并发提示；并发满但队列未满时允许排队（返回提示不拒绝）。
  - 对外行为保持稳定：提交拒绝仅因队列超限。
- `service_provider.go`：
  - 注册终态 Hook，确保并发/队列双释放；
  - 按配置启动并发与队列对账器（可独立启停与调参）。
- 执行器（`picture_task_executor`）：
  - 通过 `TaskStateManager` 原子转换 `PENDING → PROCESSING` 并预占并发槽位；
  - 失败时回滚占位；成功完成后由 Hook 统一释放。

## 八、可观测与运维
- 日志：占位/释放成功与失败、降级切换、对账轮次统计、锁获取失败等；单任务错误不阻断轮次。
- 指标：可在 `internal/prometheus` 下注册统一指标；默认谨慎启用，避免噪声。
- 运维工具建议：提供查看与清理键的脚本（只在紧急场景使用）。

## 九、边界与可靠性
- 幂等释放：避免重复释放/竞态导致计数错减；结合集合校验保证一致性。
- 短超时 DB 查询：对账器在高负载下也不阻塞业务；误释放由下一次提交重新占位自愈，并受日志/指标监控。
- 扫描性能：`scan_count`、`worker_pool_size`、`max_tasks_per_round` 组合控制每轮负载；可按用户分片扩展。

## 十、测试要点
- 单元测试：Key解析边界、Active 判定、释放幂等；降级模式下行为；
- 集成测试：构造“终态任务仍在集合”场景，对账后集合与计数同步下降；
- 并发提交与调度：验证队列/并发双限生效；
- 兼容性：旧配置格式加载与默认值降级。

## 十一、配置示例（片段）
```yaml
reconciler:
  enabled: true
  concurrent:
    enabled: true
    interval: 60s
    worker_pool_size: 8
    redis_scan_count: 100
    max_tasks_per_round: 2000
  queue:
    enabled: true
    interval: 60s
    worker_pool_size: 8
    redis_scan_count: 100
    max_tasks_per_round: 2000

key_ttl_seconds: 3600
```

## 十二、总体收益
- 提交速率与执行速率分维度精确控制；
- 全链路原子化与幂等释放，减少竞态与漏计；
- 周期对账兜底，配合降级策略确保在 Redis 异常时仍能稳定运行；
- 配置化与可观测性完善，支持未来的优先级与动态限流扩展。

## 十三、实现现状与代码收敛说明（工程化）
- 统一骨架：并发/队列对账均通过 `internal/task/reconciler.ReconcilerBase` 承载扫描、锁、派发与释放判定逻辑；差异通过 `ConcurrentAdapter` 与 `QueueAdapter` 注入。
- 壳层职责：`internal/task/slot/*_slot_reconciler.go` 仅负责依赖拼装与生命周期 `Start/Stop`，不再包含对账业务逻辑。
- 配额类型枚举：配额类型统一收敛到 `internal/quota/types.go` 的 `QuotaType`、`QuotaTypeQueue`、`QuotaTypeConcurrent`，避免在 `task`、`service`、`bootstrap` 多处重复定义；`service/quota.TaskQuotaChecker` 以类型别名的方式对齐统一枚举。
- 释放接口：`bootstrap.UnifiedReleaserAdapter` 将 `TaskQuotaChecker` 的统一释放能力适配为对账器可用接口，并直接透传统一的 `QuotaType`，减少中间映射。
- 键前后缀与解析：键前缀/后缀统一来自 `internal/quota` 包；对账器使用统一的 `ParseUserIDFromKey(prefix, suffix)` 进行解析，确保一致性与可测试性。
