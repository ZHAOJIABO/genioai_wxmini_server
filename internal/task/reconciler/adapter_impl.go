package reconciler

import (
	"context"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/quota"
	pb "va_visionai_server/internal/va_interface"
)

// ConcurrentReleaser 并发释放能力接口（由外部适配提供，避免循环依赖）
type ConcurrentReleaser interface {
	ReleaseConcurrentSlot(ctx context.Context, userID, taskID string) error
	IsDegradeMode() bool
}

// QueueReleaser 队列释放能力接口（由外部适配提供，避免循环依赖）
type QueueReleaser interface {
	ReleaseQueueSlot(ctx context.Context, userID, taskID string) error
	IsDegradeMode() bool
}

// ConcurrentAdapter 将并发对账器适配为通用骨架的 SlotAdapter
type ConcurrentAdapter struct {
	TaskDao  *dao.PictureTaskDao
	Releaser ConcurrentReleaser
}

func (a *ConcurrentAdapter) Type() string      { return "concurrent" }
func (a *ConcurrentAdapter) KeyPrefix() string { return quota.ConcurrentTasksKeyPrefix }
func (a *ConcurrentAdapter) KeySuffix() string { return quota.TasksKeySuffix }
func (a *ConcurrentAdapter) IsActive(status int32) bool {
	// 并发占用从 PROCESSING 贯穿至 AWAITING_PROVIDER_COMPLETION
	return status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING) ||
		status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION)
}
func (a *ConcurrentAdapter) Release(ctx context.Context, userID, taskID string) error {
	if a.Releaser == nil {
		return nil
	}
	return a.Releaser.ReleaseConcurrentSlot(ctx, userID, taskID)
}
func (a *ConcurrentAdapter) IsDegradeMode() bool {
	if a.Releaser == nil {
		return false
	}
	return a.Releaser.IsDegradeMode()
}
func (a *ConcurrentAdapter) GetTask(ctx context.Context, taskID string) (*model.PictureTask, error) {
	if a.TaskDao == nil {
		return nil, nil
	}
	return a.TaskDao.GetTask(ctx, taskID)
}

// QueueAdapter 将队列对账器适配为通用骨架的 SlotAdapter
type QueueAdapter struct {
	TaskGetter func(ctx context.Context, taskID string) (*model.PictureTask, error)
	Releaser   QueueReleaser
}

func (a *QueueAdapter) Type() string      { return "queue" }
func (a *QueueAdapter) KeyPrefix() string { return quota.QueueTasksKeyPrefix }
func (a *QueueAdapter) KeySuffix() string { return quota.TasksKeySuffix }
func (a *QueueAdapter) IsActive(status int32) bool {
	return status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING) ||
		status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING) ||
		status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING_SUBMISSION_RETRY) ||
		status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION)
}
func (a *QueueAdapter) Release(ctx context.Context, userID, taskID string) error {
	if a.Releaser == nil {
		return nil
	}
	return a.Releaser.ReleaseQueueSlot(ctx, userID, taskID)
}
func (a *QueueAdapter) IsDegradeMode() bool {
	if a.Releaser == nil {
		return false
	}
	return a.Releaser.IsDegradeMode()
}
func (a *QueueAdapter) GetTask(ctx context.Context, taskID string) (*model.PictureTask, error) {
	if a.TaskGetter == nil {
		return nil, nil
	}
	return a.TaskGetter(ctx, taskID)
}
