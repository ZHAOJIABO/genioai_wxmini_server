package slot

import (
	"context"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/dao"
	reconutil "va_visionai_server/internal/task/reconciler"
	"va_visionai_server/internal/zlog"
)

// SlotReconcilerConfig 对账服务配置
type SlotReconcilerConfig = reconutil.SlotReconcilerConfig

// Deprecated: 请迁移至 reconciler.SlotReconcilerConfig

// ConcurrentSlotReconciler 并发槽位对账器
type ConcurrentSlotReconciler struct {
	cfg          SlotReconcilerConfig
	rdb          *redis.Client
	taskDao      *dao.PictureTaskDao
	quotaChecker reconutil.ConcurrentReleaser
	lockMgr      *common.DistributedLockManager
	logger       *zap.Logger
	base         *reconutil.ReconcilerBase
}

const (
	reconcileLockResource = "slot_reconcile"
)

// NewConcurrentSlotReconciler 构造对账器
func NewConcurrentSlotReconciler(
	cfg SlotReconcilerConfig,
	rdb *redis.Client,
	taskDao *dao.PictureTaskDao,
	quotaChecker reconutil.ConcurrentReleaser,
	logger *zap.Logger,
) *ConcurrentSlotReconciler {
	if cfg.Interval == 0 {
		cfg.Interval = defaultReconcileInterval
	}
	if cfg.RedisScanCount <= 0 {
		cfg.RedisScanCount = defaultRedisScanCount
	}
	if cfg.MaxTasksPerRound <= 0 {
		cfg.MaxTasksPerRound = defaultMaxTasksPerRound
	}
	if cfg.WorkerPoolSize <= 0 {
		cfg.WorkerPoolSize = defaultWorkerPoolSize
	}

	if logger == nil {
		logger = zlog.LogWithContext(context.Background()).Named("ConcurrentSlotReconciler")
	}

	lm := common.NewDistributedLockManager(rdb, lockKeyPrefix)
	lm.SetRenewalTTL(cfg.Interval)

	reconciler := &ConcurrentSlotReconciler{
		cfg:          cfg,
		rdb:          rdb,
		taskDao:      taskDao,
		quotaChecker: quotaChecker,
		lockMgr:      lm,
		logger:       logger,
	}
	// 预先构造骨架并保留引用，便于生命周期绑定
	reconciler.base = reconutil.NewReconcilerBase(
		reconciler.cfg,
		reconciler.rdb,
		reconciler.lockMgr,
		reconciler.logger,
		&reconutil.ConcurrentAdapter{TaskDao: reconciler.taskDao, Releaser: reconciler.quotaChecker},
		reconcileLockResource,
		nil,
	)
	return reconciler
}

// Start 启动定时对账
func (r *ConcurrentSlotReconciler) Start() {
	if !r.cfg.Enabled {
		r.logger.Info("ConcurrentSlotReconciler is disabled, skip start")
		return
	}
	r.logger.Info("ConcurrentSlotReconciler starting...",
		zap.Duration("interval", r.cfg.Interval),
		zap.Int64("redis_scan_count", r.cfg.RedisScanCount),
		zap.Int("max_tasks_per_round", r.cfg.MaxTasksPerRound),
		zap.Int("worker_pool_size", r.cfg.WorkerPoolSize),
	)
	// 使用构造时准备好的骨架，保持生命周期一致
	r.base.Start()
}

// Stop 停止对账
func (r *ConcurrentSlotReconciler) Stop() {
	if r.base != nil {
		r.base.Stop()
	}
}

// 旧的循环/扫描/释放逻辑由 ReconcilerBase 接管，兼容壳已移除。
