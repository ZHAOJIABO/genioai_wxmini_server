package bootstrap

import (
	"context"

	iq "va_visionai_server/internal/quota"
	qcfg "va_visionai_server/internal/service/quota"
	taskslot "va_visionai_server/internal/task/slot"
)

// UnifiedReleaserAdapter 适配 service.TaskQuotaChecker 为 task.UnifiedSlotReleaser
type UnifiedReleaserAdapter struct{ checker *qcfg.TaskQuotaChecker }

func NewUnifiedReleaserAdapter(checker *qcfg.TaskQuotaChecker) *UnifiedReleaserAdapter {
	return &UnifiedReleaserAdapter{checker: checker}
}

// ReleaseSlot 统一释放接口，桥接到 TaskQuotaChecker.ReleaseSlot
func (a *UnifiedReleaserAdapter) ReleaseSlot(ctx context.Context, quotaType iq.QuotaType, userID, taskID string) error {
	// service/quota.TaskQuotaChecker.ReleaseSlot 接受其自身的 QuotaType，
	// 该类型已在 service 层通过 type alias 收敛到 internal/quota.QuotaType。
	// 因此可直接透传。
	return a.checker.ReleaseSlot(ctx, quotaType, userID, taskID)
}

func (a *UnifiedReleaserAdapter) IsDegradeMode() bool {
	return a.checker.IsDegradeMode()
}

// ConcurrentReleaserCompat 并发对账器的兼容适配器（透传到统一释放接口）
type ConcurrentReleaserCompat struct{ unified taskslot.UnifiedSlotReleaser }

func NewConcurrentReleaserCompat(unified taskslot.UnifiedSlotReleaser) *ConcurrentReleaserCompat {
	return &ConcurrentReleaserCompat{unified: unified}
}

func (c *ConcurrentReleaserCompat) ReleaseConcurrentSlot(ctx context.Context, userID, taskID string) error {
	return c.unified.ReleaseSlot(ctx, iq.QuotaTypeConcurrent, userID, taskID)
}

func (c *ConcurrentReleaserCompat) IsDegradeMode() bool {
	return c.unified.IsDegradeMode()
}

// QueueReleaserCompat 队列对账器的兼容适配器（透传到统一释放接口）
type QueueReleaserCompat struct{ unified taskslot.UnifiedSlotReleaser }

func NewQueueReleaserCompat(unified taskslot.UnifiedSlotReleaser) *QueueReleaserCompat {
	return &QueueReleaserCompat{unified: unified}
}

func (q *QueueReleaserCompat) ReleaseQueueSlot(ctx context.Context, userID, taskID string) error {
	return q.unified.ReleaseSlot(ctx, iq.QuotaTypeQueue, userID, taskID)
}

func (q *QueueReleaserCompat) IsDegradeMode() bool { return q.unified.IsDegradeMode() }
