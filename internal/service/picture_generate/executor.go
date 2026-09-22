package picture_generate

import (
	"context"

	"va_visionai_server/internal/model"
	pb "va_visionai_server/internal/va_interface"
)

// TaskFailureHandler 定义了一个处理任务失败的接口，以打破包之间的循环依赖。
// PictureTaskService 将会实现这个接口。
type TaskFailureHandler interface {
	FailTask(ctx context.Context, task *model.PictureTask, errMsg string) error
	FailTaskWithReason(ctx context.Context, task *model.PictureTask, errMsg string, reason string) error
}

// TaskExecutor 定义了所有任务执行器必须实现的完整接口。
// 它通过接口嵌入组合了其他子接口。
type TaskExecutor interface {
	Match(params map[string]string) bool
	Process(ctx context.Context, taskID string, params map[string]string) error
	GetName() string
	SetFailureHandler(handler TaskFailureHandler)
	TaskExecutorForRetry
	TaskExecutorForSync
}

// TaskExecutorForSync 定义了执行器需要为状态同步处理器提供的能力。
type TaskExecutorForSync interface {
	GetName() string
	SyncProviderStatus(ctx context.Context, task *model.PictureTask) (pb.WorkflowTaskStatus, error)
	ProcessSuccessfulResult(ctx context.Context, task *model.PictureTask) error
}

// TaskExecutorForRetry 定义了执行器需要为重试处理器提供的能力。
type TaskExecutorForRetry interface {
	RetrySubmission(ctx context.Context, task *model.PictureTask) error
	IsSubmissionErrorRetryable(err error) bool
}
