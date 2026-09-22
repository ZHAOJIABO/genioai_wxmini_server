package slot

import "time"

// 统一的 Reconciler 默认配置常量（slot 子模块）
const (
	defaultReconcileInterval       = time.Minute
	defaultRedisScanCount    int64 = 1000
	defaultMaxTasksPerRound        = 10000
	defaultWorkerPoolSize          = 16
)

// 分布式锁前缀
const (
	lockKeyPrefix = "visionai:reconcile"
)
