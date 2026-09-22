# ComfyUI 生成逻辑重构为 Executor 模式方案

## 1. 初始目标与背景

项目的核心目标是重构现有的图像生成逻辑。目前系统中有两种不同的生成方式：

1.  **传统方式**: 针对 ComfyUI 的生成逻辑（`ImageGenerator.Generate`），它在任务提交后启动一个独立的 goroutine，通过 WebSocket 长连接来监听和处理任务进度与结果。
2.  **执行器模式**: 针对 Kling、GPT-4O 等外部 API 的生成逻辑，它们遵循 `TaskExecutor` 接口，将任务的提交、状态同步、结果处理分解到不同的阶段，由统一的后台服务 (`TaskExecutionService`, `ProviderStatusSyncProcessor`) 进行调度和管理。

本次重构的目标是将 ComfyUI 的生成逻辑也改造为标准的 `TaskExecutor` 模式，从而使整个系统的任务处理架构更加统一、模块化、健壮且易于扩展。

## 2. 核心设计方案

我们将创建一个新的 `ComfyUIExecutor`，它会实现 `TaskExecutor` 接口。这使得 ComfyUI 的任务可以被现有的后台任务处理框架统一管理，替代原有的 `go func()` 和 WebSocket 监听循环。

-   **新文件**: `internal/service/picture_generate/comfyui_executor.go`
-   **新结构体**: `ComfyUIExecutor`

## 3. `TaskExecutor` 接口实现详解

`ComfyUIExecutor` 需要实现 `executor.go` 中定义的 `TaskExecutor` 接口的几个核心方法：

### 3.1 `GetName() string`

-   **职责**: 返回一个在 `ExecutorService` 中唯一的执行器名称。
-   **实现**: `return "comfyui_executor"`

### 3.2 `Match(task *model.PictureTask) bool`

-   **职责**: 判断此执行器是否能处理给定的任务。
-   **实现**: `return task.Executer == g.GetName()`
-   **注**: `ExecutorService` 会遍历所有已注册的执行器，并使用此方法来找到正确的执行器。任务的 `Executer` 字段将在提交时（`PictureTaskService` 中）根据工作流的 `UseApi` 属性进行设置。

### 3.3 `Process(ctx context.Context, task *model.PictureTask, workflow *model.Workflow, params map[string]string) error`

-   **职责**: 处理向 ComfyUI **初始提交任务**的逻辑，它将替代旧 `Generate` 函数的前半部分。
-   **实现步骤**:
    1.  从工作流 (`workflow.WorkflowJson`) 定义中准备 ComfyUI 的计算图 (`graph`)。
    2.  处理输入图片：如果任务参数 `params` 中包含图片 URL，则下载图片并调用 `client.UploadImage` 将其上传到 ComfyUI。
    3.  调用 `client.QueuePrompt(graph)` 将任务提交到 ComfyUI 的队列中。
    4.  **关键步骤**: 从 `QueuePrompt` 的返回值中获取 `PromptID`。
    5.  将获取到的 `PromptID` 保存到数据库 `va_picture_task` 表的 `executer_task_id` 字段中。
    6.  将任务状态更新为 `pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION`。
    7.  方法正常返回 `nil`，将后续的状态跟踪交给后台的 `ProviderStatusSyncProcessor`。

### 3.4 `SyncProviderStatus(ctx context.Context, task *model.PictureTask) (pb.WorkflowTaskStatus, error)`

-   **职责**: 同步并返回任务在 ComfyUI 侧的最新状态。它将替代旧 `Generate` 函数中基于 WebSocket 的消息监听循环，由 `ProviderStatusSyncProcessor` 服务周期性地调用。
-   **实现步骤**:
    1.  从 `task.ExecuterTaskID` 字段获取 `promptID`。
    2.  调用 ComfyUI 客户端，轮询 ComfyUI 的 `GET /history/{prompt_id}` API 接口。
    3.  分析API返回的历史记录：
        -   如果历史记录**为空**或**最新一项仍在执行**，则任务仍未完成，返回 `pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION`。
        -   如果历史记录中包含**产物输出 (outputs)**，则任务已成功，返回 `pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED`。
        -   如果历史记录中包含**错误或异常信息 (exception)**，则任务已失败，返回 `pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED`。

### 3.5 `ProcessSuccessfulResult(ctx context.Context, task *model.PictureTask) error`

-   **职责**: 当 `SyncProviderStatus` 方法确认任务状态为 `COMPLETED` 后，框架会自动调用此方法。它负责处理旧 `Generate` 函数的后半部分逻辑。
-   **实现步骤**:
    1.  **逻辑复用**: 此方法的逻辑与 `generator.go` 中的 `handleImageOutputs` 和 `handleGifOutputs` 高度相似。应将原有逻辑提取或重构后在此处调用。
    2.  再次调用 `GET /history/{prompt_id}` 获取最终的、完整的历史记录。
    3.  从历史记录的 `outputs` 字段解析出生成结果的文件名和类型（图片/GIF）。
    4.  调用 `client.GetImage` 或 `client.GetVideo` 从 ComfyUI 下载生成的产物。
    5.  将产物上传到我们自己的对象存储（OSS），并生成缩略图。
    6.  将最终的产物 URL、尺寸等信息存入 `PictureTask` 的 `ResultJSON` 字段。
    7.  （由框架处理）将任务最终状态更新为 `COMPLETED`。
    8.  （由框架处理）触发任务成功的用户事件。

### 3.6 `RetrySubmission(...)` & `IsSubmissionErrorRetryable(...)`

-   **职责**: 处理在初始 `Process` 方法中调用 `QueuePrompt` 时可能发生的、可重试的瞬时错误（例如网络波动、ComfyUI 服务临时不可用）。
-   **实现**: 根据需要实现具体的重试逻辑。

## 4. 与框架的集成

### 4.1 任务提交流程修改

-   **文件**: `internal/service/picture_task.go`
-   **方法**: `SubmitTask` 或 `SubmitTaskWithTx`
-   **修改**: 在创建 `PictureTask` 记录时，增加一个逻辑判断：
    ```go
    if !workflow.UseApi { // 假设 ComfyUI 任务的 UseApi 为 false
        task.Executer = "comfyui_executor"
    } else {
        // ... 其他执行器分配逻辑
    }
    ```

### 4.2 失败处理

-   `ComfyUIExecutor` 的所有失败路径（例如，在 `Process` 或 `SyncProviderStatus` 中遇到的不可重试错误）都应返回 `error`。
-   框架将捕获此错误，并调用统一的 `PictureTaskService.FailTask` 方法。该方法负责将任务状态置为 `FAILED`、调用积分服务进行退款、释放用户处理锁等一系列原子操作。执行器本身不再需要关心这些细节。

### 4.3 依赖注入

-   **文件**: `internal/bootstrap/service_provider.go`
-   **修改**:
    1.  在 `InitPictureGenerate` 或类似方法中，创建 `ComfyUIExecutor` 的实例。
    2.  将其注册到 `ExecutorService` 中：`executorService.Register(comfyUIExecutor)`。

## 5. 代码清理

-   **`internal/service/picture_generate/generator.go`**: `ImageGenerator.Generate` 方法及其内部的 WebSocket 循环逻辑将被**完全移除**。
-   **`internal/api/picture_forge.go`**: 调用 `ImageGenerator.Generate` 的 `go func() {}` 将被**完全移除**。

通过此计划，无论是 ComfyUI 任务还是其他 API 任务，都将被一个统一、健壮且可扩展的异步处理流水线来管理和执行，显著提升系统的稳定性和可维护性。 