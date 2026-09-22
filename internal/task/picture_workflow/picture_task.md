# 图片任务处理方案

## 1. 引言

本文档旨在提出对现有图片任务处理系统（尤其是 `PictureTaskService` 中的 `TaskProcessor` 相关逻辑）的重构方案。目标是将其迁移到更合适的 `internal/task/` 目录结构下，并引入更优雅的任务排队机制（确保单个用户在单位时间内只处理一个任务）以及一个持久化的任务检查与恢复执行器。

## 2. 当前系统分析

### 2.1. 任务提交流程 (`PictureTaskService.SubmitTask`)
- 用户提交任务后，系统会进行一系列校验（包括 `maxActiveTasksPerUser`，限制用户在系统中的总活跃任务数）。
- 任务元数据被持久化到数据库，初始状态为 `WORKFLOW_TASK_STATUS_PENDING`。
- 任务信息被封装成 `TaskInfo` 对象，并发送到一个名为 `taskChan` 的带缓冲的 Go channel 中。

### 2.2. 任务处理流程 (`PictureTaskService.TaskProcessor`)
- `TaskProcessor` 启动一个 goroutine，该 goroutine 从 `taskChan` 中消费 `TaskInfo`。
- 对于每个 `TaskInfo`，调用 `PictureTaskService.GenerateImage` 方法来实际执行图片/视频生成。
- `GenerateImage` 及其调用的下游服务 (`executorService`, `imageGenerator`) 负责更新数据库中任务的状态（例如，`COMPLETED`, `FAILED`）和结果。

### 2.3. 现有机制的局限性
1.  **内存队列的脆弱性**: `taskChan` 是一个内存中的队列。如果应用重启，`taskChan` 中的所有未处理任务都会丢失，除非它们已经被持久化到数据库且有其他机制来恢复处理。
2.  **用户任务并发控制不足**:
    *   `maxActiveTasksPerUser` 限制的是用户在数据库中"活跃"（非终态）任务的总数，在提交时检查。
    *   `TaskProcessor` 从 `taskChan` 中取出任务后，如果多个任务来自同一用户且 `taskChan` 中连续排列，它们可能会被 `GenerateImage` 几乎同时处理（如果 `GenerateImage` 内部是异步的或者 `TaskProcessor` 未来扩展为多worker模式）。当前 `TaskProcessor` 是单 goroutine 消费 `taskChan`，但并不能严格保证一个用户的一个任务执行完毕后再执行该用户的下一个任务。
3.  **模块职责耦合**: 任务调度和执行逻辑与 `PictureTaskService` 紧密耦合，不利于独立扩展和维护。
4.  **恢复机制缺失**: 没有明确的机制在程序重启后自动检查数据库中处于 `PENDING` 状态的任务并重新触发处理。

## 3. 提议的架构方案

我们将引入一个新的服务 `TaskExecutionService`，专门负责任务的调度和执行。它将取代当前的 `TaskProcessor` 和 `taskChan`。

### 3.1. 核心组件

1.  **`TaskExecutionService` (位于 `internal/task/picture_task_executor.go`)**:
    *   **职责**:
        *   定期轮询数据库，查找处于 `PENDING` 状态的任务。
        *   管理用户级别的处理锁，确保每个用户同时只有一个任务在处理。
        *   将符合条件的任务分发给内部的 worker 执行。
        *   提供 `Start()` 和 `Stop()` 方法来控制其生命周期。
    *   **依赖**: `PictureTaskDao`, `PictureForgeDao`, `redis.Client`, `picture_generate.ExecutorService`, `picture_generate.ImageGenerator`。

2.  **数据库 (PostgreSQL/MySQL)**:
    *   作为任务队列的持久化存储。任务状态将包括 `PENDING`, `PROCESSING`, `COMPLETED`, `FAILED`。

3.  **Redis**:
    *   用于实现分布式用户处理锁。例如，键 `visionai:user_processing_lock:<userID>`，值为当前处理的 `taskID`，并设置TTL。

### 3.2. 工作流程

#### 3.2.1. 任务提交 (`PictureTaskService.SubmitTask` - 修改)
1.  请求校验、`maxActiveTasksPerUser` 检查等现有逻辑保持不变。
2.  任务信息（包括项目ID、用户ID、工作流ID、参数等）被持久化到数据库，状态设置为 `WORKFLOW_TASK_STATUS_PENDING`。
3.  **移除**: 不再将任务发送到 `taskChan`。`TaskExecutionService` 会从数据库中拉取。

#### 3.2.2. `TaskExecutionService` - 调度逻辑 (`schedulerLoop`)
1.  服务启动后，`schedulerLoop` 按固定间隔（例如，每5秒）运行。
2.  **查询待处理任务**: 从数据库查询一批状态为 `PENDING` 的任务，按创建时间 (`created_at ASC`) 排序。
    *   需要 `PictureTaskDao` 提供新方法: `GetPendingTasks(ctx context.Context, limit int) ([]*model.PictureTask, error)`。
3.  **用户锁检查与任务派发**:
    *   对每个查询到的 `PENDING` 任务：
        *   获取任务的 `UserID`。
        *   尝试使用 Redis `SETNX` 命令为该用户获取一个处理锁 (e.g., `visionai:user_processing_lock:<userID>`)。
        *   **如果成功获取锁**:
            *   启动一个新的 goroutine (`executeTaskWorker`) 来处理该任务。
            *   （可选）可以先在数据库中将任务状态更新为 `PROCESSING`，或者由 `executeTaskWorker` 负责更新。为减少调度器负担和确保状态准确性，建议由 `executeTaskWorker` 在实际开始处理前更新。
        *   **如果获取锁失败** (表示该用户已有任务在处理中):
            *   跳过此任务。该任务将在后续的调度周期中被再次尝试。

#### 3.2.3. `TaskExecutionService` - 任务执行逻辑 (`executeTaskWorker`)
1.  此函数在单独的 goroutine 中为每个任务执行。
2.  **获取上下文**: 创建一个新的 `context.Context` 用于此任务的执行。
3.  **释放锁**:  "用户处理锁 (visionai:user_processing_lock:<userID>) 的释放时机取决于任务的处理路径：
如果任务在 executeTaskWorker 内部直接达到终态（例如，前置校验失败、获取工作流定义失败等），则锁在此 worker 完成时释放。
如果任务被成功提交给外部处理单元（例如，ComfyUI 或第三方 API），并且任务状态变为 AWAITING_PROVIDER_COMPLETION，则此锁将继续保持，直到后续的状态同步机制确认任务达到最终状态 (COMPLETED 或 FAILED)。"
4.  **更新任务状态为 `PROCESSING`**: 在数据库中将任务状态更新为 `WORKFLOW_TASK_STATUS_PROCESSING`，并清空可能存在的旧错误信息。
5.  **获取任务详情**:
    *   根据 `task.WorkflowID` 从 `PictureForgeDao` 获取 `model.Workflow` 详细信息（包括 `WorkflowJson`, `UseApi` 等）。
    *   从 `task.ParamJSON` 解析任务参数 `map[string]string`。
6.  **执行图片/视频生成**:
    *   调用与原 `PictureTaskService.GenerateImage` 类似的逻辑，使用 `executorService` 和 `imageGenerator`。
    *   这些底层服务负责与 ComfyUI/API 交互，并在成功或失败时更新数据库中的任务状态（`COMPLETED`/`FAILED`）、结果 (`ResultJSON`) 和错误信息 (`Error`)。
7.  **日志记录**: 详细记录任务执行的各个阶段和结果。

### 3.3. 数据结构和状态

*   **数据库任务状态 (`model.PictureTask.Status`)**:
    *   `pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING`
    *   `pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING`
    *   `pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED`
    *   `pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED`
*   **Redis 用户处理锁**:
    *   **Key**: `visionai:user_processing_lock:<userID>` (e.g., `visionai:user_processing_lock:user_abc123`)
    *  **TTL**: "应大于单个任务可能的最长执行时间，包括等待外部API同步的整个周期 (e.g., 几十分钟到几小时，视具体业务而定)。锁的及时释放由任务达到终态的处理逻辑保证，TTL 作为兜底机制。"
    *   **Value**: `taskID` (用于调试和追踪)


### 3.4. 目录和文件结构变更

*   **移除**: `PictureTaskService.TaskProcessor()` 方法和 `PictureTaskService.taskChan` 字段。
*   **新增文件**: `internal/task/picture_workflow/picture_task_executor.go`，包含 `TaskExecutionService` 的定义、调度和 worker 逻辑。
*   **修改文件**:
    *   `internal/service/picture_task.go`: 修改 `SubmitTask` 方法，移除对 `taskChan` 的写入。
    *   `internal/dao/picture_task.go`: 新增 `GetPendingTasks` 方法。
    *   服务初始化代码 (e.g., `cmd/server/main.go` 或类似地方): 初始化并启动 `TaskExecutionService`。

## 4. 具体需求满足情况

1.  **移动 `TaskProcessor`**:
    *   所有任务处理和调度逻辑将集中到 `internal/task/picture_task_executor.go`。

2.  **优雅的排队机制 (同一用户单位时间内只处理1个任务)**:
    *   通过 `TaskExecutionService` 在调度时为每个用户获取 Redis 锁实现。只有获得锁的用户才能开始新任务的处理。

3.  **任务检查执行器 (数据库恢复与顺序执行)**:
    *   `TaskExecutionService` 的调度器定期从数据库拉取 `PENDING` 任务，天然支持了程序重启后的任务恢复。
    *   结合用户锁，确保了即使用户提交了多个任务，这些任务也会因为锁机制而被该用户的 worker 顺序处理（一个完成后释放锁，下一个才能获取锁并开始）。

## 5. 方案优势

1.  **持久化与可靠性**: 任务队列基于数据库，应用重启不会丢失待处理任务。
2.  **精确的并发控制**: Redis 锁确保了每个用户同时只有一个任务在执行，满足业务需求。
3.  **解耦与可维护性**: 任务调度执行逻辑从核心服务中分离，职责更清晰，易于维护和独立扩展。
4.  **可观测性**: Redis 锁和任务状态的清晰化，有助于监控系统负载和用户任务处理情况。

## 6. 初始化和集成

1.  在应用启动时，创建 `picture_generate.ExecutorService` 和 `picture_generate.ImageGenerator` 的实例。
2.  创建 `dao.PictureTaskDao`, `dao.PictureForgeDao`, 和 `redis.Client` 的实例。
3.  使用这些依赖项实例化 `TaskExecutionService`。
4.  调用 `taskExecutionService.Start()` 启动任务调度器。
5.  确保在应用关闭时调用 `taskExecutionService.Stop()` 以实现优雅停机。

## 7. 注意事项与未来考虑

*   **错误处理与重试**: 当前方案依赖底层生成服务处理单个任务的失败和状态更新。可以考虑在 `TaskExecutionService`层面增加更复杂的重试策略（例如，对特定类型的 Redis 错误或临时性 DB 错误进行重试）。
*   **死信队列**: 对于持续失败的任务，可以考虑将其移至特定的"死信"状态或表，以便人工干预。
*   **配置**: 调度间隔、Redis锁TTL等应可配置。
*   **Context传播**: 确保在所有异步操作和外部调用中正确传播 `context.Context`，以便实现超时控制和优雅关闭。
*   **Worker池**: 如果单个任务执行时间较短且任务量巨大，`executeTaskWorker` 可以派发给一个固定大小的 goroutine 池，而不是无限创建 goroutine，以控制资源消耗。当前方案为每个任务启动一个 goroutine，对于大部分图片生成场景（耗时较长）是可接受的。
