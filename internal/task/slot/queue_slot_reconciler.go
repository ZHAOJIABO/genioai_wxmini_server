package slot

import (
	"context"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/model"
	reconutil "va_visionai_server/internal/task/reconciler"
	"va_visionai_server/internal/zlog"
)

// TaskGetter 提供任务查询能力
type TaskGetter interface {
	GetTask(ctx context.Context, taskID string) (*model.PictureTask, error)
}

// QueueSlotReconciler 队列槽位对账器
type QueueSlotReconciler struct {
	cfg          SlotReconcilerConfig
	rdb          *redis.Client
	taskDao      TaskGetter
	quotaChecker reconutil.QueueReleaser
	lockMgr      *common.DistributedLockManager
	logger       *zap.Logger
	base         *reconutil.ReconcilerBase
}

const (
	queueReconcileLockResource = "queue_slot_reconcile"
)

// NewQueueSlotReconciler 构造队列对账器
func NewQueueSlotReconciler(
	cfg SlotReconcilerConfig,
	rdb *redis.Client,
	taskDao TaskGetter,
	quotaChecker reconutil.QueueReleaser,
	logger *zap.Logger,
) *QueueSlotReconciler {
	// 设置默认值
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
		logger = zlog.LogWithContext(context.Background()).Named("QueueSlotReconciler")
	}

	lm := common.NewDistributedLockManager(rdb, lockKeyPrefix)
	lm.SetRenewalTTL(cfg.Interval)

	reconciler := &QueueSlotReconciler{
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
		&reconutil.QueueAdapter{TaskGetter: reconciler.taskDao.GetTask, Releaser: reconciler.quotaChecker},
		queueReconcileLockResource,
		nil,
	)
	return reconciler
}

// Start 启动定时对账
func (r *QueueSlotReconciler) Start() {
	if !r.cfg.Enabled {
		r.logger.Info("QueueSlotReconciler is disabled, skip start")
		return
	}
	r.logger.Info("QueueSlotReconciler starting...",
		zap.Duration("interval", r.cfg.Interval),
		zap.Int64("redis_scan_count", r.cfg.RedisScanCount),
		zap.Int("max_tasks_per_round", r.cfg.MaxTasksPerRound),
		zap.Int("worker_pool_size", r.cfg.WorkerPoolSize),
	)
	r.base.Start()
}

// Stop 停止对账
func (r *QueueSlotReconciler) Stop() {
	if r.base != nil {
		r.base.Stop()
	}
}
