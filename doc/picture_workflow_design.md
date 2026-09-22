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

1.  **`TaskExecutionService` (位于 `internal/task/picture_workflow/picture_task_executor.go`)**:
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

## 8. 失败处理与退款重构方案

### 8.1. 现状与问题

经过代码审查，发现当前系统的失败处理和积分退款逻辑存在以下问题：

1.  **逻辑分散**: 退款逻辑主要实现在 `KlingImage2VideoExecutor` 的 `SyncProviderStatus` 方法中，通过一个私有函数 `updateTaskFieldsAndSendEvent` 调用。这使得退款与特定的执行器紧密耦合。
2.  **实现不一致 (Bugs)**: `Gpt4oImage2ImageExecutor` 在其所有失败路径（包括任务提交失败和状态同步失败）中，均**没有实现积分退款逻辑**，这会导致用户在任务失败时无法拿回点数，是严重的业务缺陷。
3.  **可维护性差**: 每当系统中增加一个新的执行器时，开发者必须手动、重复地实现一套相同的"检查失败 -> 更新状态 -> 退款"的逻辑，极易因遗漏而产生新的Bug。

### 8.2. 重构目标

将失败处理和积分退款的逻辑从各个执行器中剥离出来，集中到统一的服务中进行管理，确保所有类型的任务在任何失败场景下都遵循一致、可靠的处理流程。

### 8.3. 核心方案

在 `PictureTaskService` (`internal/service/picture_task.go`) 中创建一个统一的失败处理核心方法，作为全系统处理任务失败的唯一入口。

#### 8.3.1. 新增核心方法 `PictureTaskService.FailTask`

```go
// FailTask 是一个原子操作，用于处理任何任务的失败流程。
// 它是系统中唯一应该将任务状态置为 FAILED 的地方。
func (s *PictureTaskService) FailTask(ctx context.Context, task *model.PictureTask, reason string) error {
    // 1. 获取最新任务状态，防止重复处理
    // 2. 检查是否已是终态，若是则直接返回
    // 3. 在数据库事务中执行以下操作：
    //    a. 更新任务状态为 FAILED，并记录 reason
    //    b. 调用 creditService.RefundCredits 返还积分
    // 4. 释放用户的处理锁 (user processing lock)
    // 5. 发送任务失败的进度事件
}
```

#### 8.3.2. 代码改造路径

1.  **`TaskExecutionService` (`picture_task_executor.go`)**:
    *   在 `executeTaskWorker` 方法中，所有因前置检查（如解析参数、获取工作流）失败而需要终止任务的地方，将不再手动更新任务状态，而是改为调用 `pictureTaskService.FailTask`。

2.  **所有执行器 (`Executor`) 的 `Process` 方法**:
    *   在与外部服务（如可灵、GPT-4O）交互并发生**不可重试的失败**时，调用 `pictureTaskService.FailTask`。

3.  **所有执行器 (`Executor`) 的 `SyncProviderStatus` 方法**:
    *   当从外部服务同步到任务**最终失败状态**时，调用 `pictureTaskService.FailTask`。
    *   移除执行器内部所有独立的退款和状态更新逻辑（例如，删除 `kling_..._executor.go` 中的 `updateTaskFieldsAndSendEvent` 方法，或移除其退款部分）。

### 8.4. 解决循环依赖的技术方案

在实施中，一个核心挑战是 `PictureTaskService` 和 `ExecutorService` 之间的循环依赖：
*   `PictureTaskService` 需要调用 `ExecutorService` 来处理任务。
*   `ExecutorService` 需要将 `PictureTaskService` （作为失败处理器）注入到其管理的每个执行器中，以便它们可以调用 `FailTask`。

我们采用**依赖倒置原则**和**两阶段初始化**来优雅地解决此问题。

1.  **定义接口 (依赖倒置)**:
    *   在 `picture_generate` 包中定义一个 `TaskFailureHandler` 接口，该接口仅暴露 `FailTask` 方法。
    *   所有的执行器（`TaskExecutor`）将依赖这个抽象的 `TaskFailureHandler` 接口，而不是具体的 `PictureTaskService` 实现。这打破了 `picture_generate` 包对 `service` 包的直接依赖。

2.  **实现接口与设置器 (Setter Injection)**:
    *   `PictureTaskService` 实现了 `TaskFailureHandler` 接口。
    *   `ExecutorService` 提供一个 `SetTaskFailureHandler(handler TaskFailureHandler)` 方法。
    *   `PictureTaskService` 提供一个 `SetExecutorService(service *ExecutorService)` 方法。

3.  **执行器接口职责明确化**:
    *   `SyncProviderStatus`: 此方法的核心职责是**与外部服务同步，并返回一个明确的任务状态**。它不应该包含结果处理（如文件下载、上传）等可能失败的I/O操作。当它返回 `COMPLETED` 时，应代表"供应商侧已确认完成"，后续步骤由调用方（`ProviderStatusSyncProcessor`）驱动。
    *   `ProcessSuccessfulResult`: 此方法在 `SyncProviderStatus` 返回 `COMPLETED` 后被调用。它的职责是**处理成功之后的所有后续工作**，例如下载生成的结果、上传到OSS、更新数据库中的结果字段等。如果此步骤失败，它应该返回 `error`，由调用方负责将任务标记为最终失败。

4.  **两阶段初始化 (在 `bootstrap/service_provider.go` 中装配)**:
    *   **阶段一 (创建实例)**: 分别创建 `PictureTaskService` 和 `ExecutorService` 的实例。在创建时，将它们相互依赖的字段暂时留空 (`nil`)。
    *   **阶段二 (注入依赖)**: 在实例创建完毕后，调用各自的 `Set...` 方法，将对方的实例注入进去，从而"连接"好依赖关系，完成整个依赖图的闭环。

这个方案是 Go 服务化架构中解决循环依赖的标准最佳实践，保证了模块间的清晰界限和高内聚性。

### 8.5. 方案优势

1.  **高内聚，低耦合**: 失败处理和退款的核心业务逻辑内聚于 `PictureTaskService`，与各个执行器的具体实现解耦。
2.  **保证一致性**: 确保所有任务的失败处理流程完全一致，从根本上杜绝"部分执行器未实现退款"这类Bug。
3.  **易于维护和扩展**: 新增执行器时，开发者只需关注其核心的 `Process` 和 `SyncProviderStatus` 逻辑，失败时调用统一的 `FailTask` 方法即可，无需再关心退款细节，极大简化了开发心智负担。

## 9. 实施规划

本章节将详细列出实现上述重构方案的具体步骤，并用于追踪完成状态。

### 第一阶段：奠定基础 - 统一失败处理服务

-   [x] **步骤 1.1: 实现 `PictureTaskService.FailTask` 方法**
    -   **任务**: 在 `internal/service/picture_task.go` 中创建核心的 `FailTask` 方法。
    -   **实现细节**: 该方法需整合**获取最新任务状态、检查终态以防重复处理、更新任务状态为 FAILED、调用 `creditService.RefundCredits`、释放用户锁**以及**发送失败事件**等所有相关逻辑。

-   [x] **步骤 1.2: 注入 `PictureTaskService` 依赖**
    -   **任务**: `TaskExecutionService` 和所有执行器（Executor）都需要调用 `FailTask`，因此必须将 `PictureTaskService` 作为依赖注入。
    -   **实现细节**:
        -   修改 `internal/task/picture_workflow/picture_task_executor.go` 中的 `TaskExecutionService` 结构体和构造函数 `NewTaskExecutionService`。
        -   修改 `internal/service/picture_generate/executor.go` 中的 `TaskExecutor` 接口，为其增加一个 `SetPictureTaskService` 之类的方法，或者在创建时直接传入。
        -   更新 `internal/bootstrap/service_provider.go` 中相关的服务初始化代码，正确注入依赖。

### 第二阶段：重构现有逻辑

-   [x] **步骤 2.1: 重构 `TaskExecutionService`**
    -   **任务**: 改造 `internal/task/picture_workflow/picture_task_executor.go`。
    -   **实现细节**: 在 `executeTaskWorker` 方法中，将所有前置检查失败（如参数错误）后对 `markTaskFailed` 的调用，替换为对 `pictureTaskService.FailTask` 的调用。

-   [x] **步骤 2.2: 重构 `Gpt4oImage2ImageExecutor` (修复Bug)**
    -   **任务**: 改造 `internal/service/picture_generate/gpt4o_image2image_executor.go`。
    -   **实现细节**:
        -   在 `Process` 方法的所有失败路径中，用对 `pictureTaskService.FailTask` 的调用替换掉现有的手动状态更新和锁释放逻辑，从而修复**不退款**的 Bug。
        -   为其 `SyncProviderStatus` 方法添加逻辑：当检测到任务在 GPT-4O 侧执行失败时，调用 `pictureTaskService.FailTask`。

-   [x] **步骤 2.3: 重构 `KlingImage2VideoExecutor`**
    -   **任务**: 改造 `internal/service/picture_generate/kling_image2video_executor.go`。
    -   **实现细节**:
        -   在 `handleSubmissionError` 方法中，对于不可重试的错误，用对 `pictureTaskService.FailTask` 的调用替换手动状态更新。
        -   在 `SyncProviderStatus` 方法中，当从可灵同步到失败状态时，用对 `pictureTaskService.FailTask` 的调用，替换掉对 `updateTaskFieldsAndSendEvent` 的调用。
        -   精简或移除 `updateTaskFieldsAndSendEvent` 方法中的退款逻辑，使其职责更纯粹。

### 第三阶段：清理与验证

-   [x] **步骤 3.1: 最终代码审查**
    -   **任务**: 在所有修改完成后，进行一次全面的代码审查。
    -   **实现细节**: 使用 `grep` 等工具，确保 `creditService.RefundCredits` 的调用只存在于 `PictureTaskService` 中（针对任务失败场景）。确认所有旧的、分散的失败处理逻辑均已被移除。

-   [x] **步骤 3.2: 更新规划文档**
    -   **任务**: 将本文档中所有步骤标记为已完成。
    -   **实现细节**: `[ ]` -> `[x]`

## 10. 任务重试机制

系统支持两种重试场景：自动重试和手动重试。

### 10.1. 自动重试 (任务提交阶段)

-   **触发条件**: 当任务首次提交给第三方服务（如 可灵 API）时，如果发生的是可重试的网络错误或对方服务临时不可用，任务状态会被置为 `PENDING_SUBMISSION_RETRY`。
-   **执行者**: `SubmissionRetryProcessor` 后台处理器会定期扫描此状态的任务。
-   **积分处理**: 此类重试是**系统层面的容错**，发生在首次尝试扣款**之后**、任务被真正执行**之前**。因此，它**不会也无需**进行额外的积分检查和扣除。它只是在网络状况恢复后，将已经"付费"的任务再次尝试提交给供应商。

### 10.2. 手动重试 (用户主动触发)

-   **触发条件**: 任务因各种原因（如生成内容不合规、执行超时等）最终进入 `FAILED` 状态后，用户可以通过界面或API调用 `RetryFailedTask` 接口来主动发起重试。
-   **核心原则**: **手动重试在业务上被视为一次独立的、全新的任务尝试，其成本与原始任务完全相同。** 整个积分生命周期遵循一个清晰的"尝试扣款，失败退款"模式。
-   **执行者**: `PictureTaskService.RetryFailedTask` 方法。

#### 10.2.1. 积分处理与业务逻辑规划

为了修复现有实现中"免费重试"的漏洞，`RetryFailedTask` 方法将按以下逻辑重构：

1.  **加载原始任务**: 根据 `taskID` 从数据库加载失败的任务记录。
2.  **状态校验**: 确保任务状态确实为 `FAILED`，否则拒绝重试。
3.  **获取消耗点数**: 直接从失败的 `task` 记录中读取 `CreditPoints` 字段。这确保了重试扣费与初次提交扣费的金额完全一致，作为一次公平的"快照"交易，避免了因工作流价格变动带来的不一致性。
4.  **检查并扣除积分**:
    -   如果从任务记录中读出的 `CreditPoints > 0`，则调用 `creditService.DeductCredits` 方法，传入 `TransactionTypeTaskRetryDeduction` 作为交易类型，对用户进行等额积分的扣除。
    -   如果积分不足，扣除操作将失败，`RetryFailedTask` 会立即向用户返回"积分不足"的错误。
5.  **重置并激活任务**:
    -   只有在积分扣除成功后（或任务本身是免费的），才继续执行以下操作。
    -   将任务状态从 `FAILED` 更新为 `PENDING`。
    -   **清空** `Error`、`ResultJSON` 等执行结果字段，重置进度。
    -   **保存**更新后的任务到数据库。
6.  **进入队列**: 任务进入 `PENDING` 状态后，`TaskExecutionService` 会在下一个调度周期中自动发现并执行它，后续流程与新任务完全相同。

通过此方案，可以确保手动重试的业务逻辑闭环，杜绝免费使用的漏洞，并为未来的审计提供清晰的消费记录。

#### 10.2.2. 代码实施规划

为了实现上述业务逻辑，需要对 `internal/service/picture_task.go` 文件中的 `RetryFailedTask` 方法进行重构。

**目标方法**: `RetryFailedTask(ctx context.Context, taskID string)`

**实施步骤**:

- [x] **步骤 1: 引入数据库事务**
    -   整个重试操作（从加载任务到更新状态）必须包裹在一个数据库事务中，确保原子性。

- [x] **步骤 2: 加载并锁定任务**
    -   在事务中，通过 `taskDao.GetTask` 加载任务，并使用 `gorm:"FOR UPDATE"` 对记录行加锁，防止并发操作导致数据不一致。
    -   校验任务状态是否为 `FAILED`，若不是则立即返回错误。

- [x] **步骤 3: 获取并检查积分成本**
    -   从加载的 `task` 实例中直接读取 `task.CreditPoints` 的值。

- [x] **步骤 4: 执行积分扣除**
    -   **最终方案**: 修改 `credit.Service` 的 `DeductCredits` 方法，为其增加 `transactionType` 参数。
    -   如果 `task.CreditPoints > 0`，则调用 `s.creditService.DeductCredits()`，并传入 `TransactionTypeTaskRetryDeduction` 作为交易类型。
    -   若该方法返回积分不足的错误，则回滚事务并向上层返回明确的错误信息。

- [x] **步骤 5: 重置任务状态**
    -   当积分扣除成功或任务免费时，在 `task` 对象上更新以下字段：
        -   `Status`: `pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING`
        -   `Error`: `""`
        -   `ResultJSON`: `""`
        -   `Progress`: `0`
        -   `UpdatedAt`: `time.Now()`
        -   `RetryCount`: `task.RetryCount + 1` (需确认 `model.PictureTask` 中存在此字段，若无则需添加)

- [x] **步骤 6: 持久化与提交**
    -   调用 `s.taskDao.UpdateTask()` 将更新后的 `task` 对象持久化到数据库。
    -   提交数据库事务。
    -   返回成功响应，表示任务已重新进入处理队列。

### 10.3. 并发控制方案选型：数据库行级锁

在多实例部署的环境下，必须对"任务重试"操作进行并发控制，以防止同一任务被重复提交和扣款。

我们选择使用 **MySQL 的事务性行级锁 (`SELECT ... FOR UPDATE`)** 作为并发控制方案，而非引入外部的分布式锁（如Redis）。

#### 10.3.1. 选型理由

1.  **强一致性保证**: 我们的核心操作是"读取-修改-写入"，必须保证原子性。将数据（任务状态）和锁（行锁）放在同一个存储引擎（MySQL）中，可以利用数据库事务的 ACID 特性，完美地保证了数据一致性。当事务提交时，数据的更新和锁的释放是原子性地同时完成的。

2.  **架构简洁性**: 该方案依赖于项目已有的 MySQL 服务，无需引入和维护额外的组件（如Redis或Zookeeper），降低了系统的复杂度和故障点。

3.  **性能匹配场景**: "任务重试"是一个低频的用户操作，其并发量远未达到数据库行级锁的性能瓶颈。对于此场景，行级锁的性能绰绰有余。

4.  **原生多实例支持**: 数据库行级锁天然地解决了多实例部署下的并发问题。当多个服务实例同时尝试锁定同一行数据时，数据库会确保只有一个事务能够成功，其他事务则必须等待或失败，从而保证了操作的串行化。

#### 10.3.2. 与Redis分布式锁的对比

| 特性 | MySQL行级锁 | Redis分布式锁 | 结论（针对本场景） |
| :--- | :--- | :--- | :--- |
| **一致性保障** | **极强** (ACID事务) | **弱** (跨系统原子性难保证) | **行级锁胜出** |
| **实现复杂度** | **极低** (一行SQL) | **高** (需处理网络、续期等) | **行级锁胜出** |
| **运维成本** | **零新增** | **高** (需维护额外组件) | **行级锁胜出** |

综上所述，对于"任务重试"这一要求强一致性的低并发场景，使用MySQL行级锁是最简单、最可靠、最优雅的架构选择。

## 11. 缺陷修复：ProviderStatusSyncProcessor 锁越权问题

本章节记录针对 `ProviderStatusSyncProcessor` 越权释放 `user_processing_lock` 导致的重复任务提交问题的修复过程。

### 11.1 修复方案

核心思想是**明确职责边界**：
1.  **失败处理中心化**: `ProviderStatusSyncProcessor` 中所有导致任务**最终失败**的路径，都应委托给 `PictureTaskService.FailTask` 方法处理。该方法是全系统处理任务失败的唯一入口，它原子化地完成了**退款、更新状态、释放锁**等一系列操作。
2.  **锁操作权限最小化**: `ProviderStatusSyncProcessor` 只应在任务**同步成功 (COMPLETED)** 后，才尝试释放锁，并且必须**校验锁的归属权**，确保不会误删其他任务的锁。

### 11.2 实施步骤

- [x] **步骤一：为 `ProviderStatusSyncProcessor` 注入失败处理服务**
    - [x] **目标**: 使 `ProviderStatusSyncProcessor` 能够调用统一的失败处理逻辑 `FailTask`。
    - [x] **操作**:
        1.  修改 `internal/task/picture_workflow/provider_status_sync_processor.go` 中的 `ProviderStatusSyncProcessor` 结构体，增加 `taskService *service.PictureTaskService` 字段。
        2.  修改构造函数 `NewProviderStatusSyncProcessor`，增加 `taskService *service.PictureTaskService` 参数并进行赋值。
        3.  更新依赖注入文件 (例如 `internal/bootstrap/service_provider.go`) 中 `ProviderStatusSyncProcessor` 的初始化代码，将 `PictureTaskService` 实例注入。

- [x] **步骤二：重构 `handleSingleTask` 中的失败处理路径**
    - [x] **目标**: 将分散、不正确的失败处理，替换为对中心化服务的统一调用。
    - [x] **操作**: 在 `internal/task/picture_workflow/provider_status_sync_processor.go` 的 `handleSingleTask` 方法中：
        1.  **任务超时分支**: 删除原有手动更新状态和释放锁的代码，替换为对 `p.taskService.FailTask(ctx, task, "Provider task processing timeout.")` 的调用，然后 `return`。
        2.  **执行器为nil分支**: 同上，删除原有代码，替换为对 `p.taskService.FailTask(ctx, task, "No suitable executor found for status sync.")` 的调用，然后 `return`。
        3.  **执行器同步失败分支**: 在 `executor.SyncProviderStatus` 返回 `syncErr` 且该错误为不可重试的场景下，增加对 `p.taskService.FailTask(ctx, task, fmt.Sprintf("Executor sync failed: %v", syncErr))` 的调用，然后 `return`。

- [x] **步骤三：修正 `releaseUserProcessingLock` 的权限**
    - [x] **目标**: 保留同步器在任务**正常完成**时释放锁的能力，但必须确保它不会"误删"其他任务的锁。
    - [x] **操作**: 在 `internal/task/picture_workflow/provider_status_sync_processor.go` 中，将 `releaseUserProcessingLock` 方法的实现，完全替换为 `internal/service/picture_task.go` 中 `releaseUserProcessingLock` 的实现。新实现必须包含 **先用 `GET` 命令查询锁的持有者，只有当持有者是当前任务时，才执行 `DEL` 命令** 的逻辑。

- [x] **步骤四：更新规划文档**
    - [x] **目标**: 记录本次修复已完成。
    - [x] **操作**: 将本章节所有步骤标记为已完成。
