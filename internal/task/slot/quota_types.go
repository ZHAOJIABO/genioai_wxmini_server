package slot

import (
	"context"

	iq "va_visionai_server/internal/quota"
)

// UnifiedSlotReleaser 统一释放接口，供并发/队列对账器共同使用
// 由 bootstrap 层用 TaskQuotaChecker 适配实现，避免 task <-> service 循环依赖
type UnifiedSlotReleaser interface {
	ReleaseSlot(ctx context.Context, quotaType iq.QuotaType, userID, taskID string) error
	IsDegradeMode() bool
}
