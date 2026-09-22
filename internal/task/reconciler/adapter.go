package reconciler

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/conf"
	"va_visionai_server/internal/common"
	"va_visionai_server/internal/model"
	pb "va_visionai_server/internal/va_interface"
)

// SlotAdapter 抽象不同类型对账器的差异点
// 注意：当前仅作为骨架，后续逐步替换现有并发/队列对账器中的重复逻辑
type SlotAdapter interface {
	Type() string // "concurrent" | "queue"
	KeyPrefix() string
	KeySuffix() string
	IsActive(status int32) bool
	Release(ctx context.Context, userID, taskID string) error
	IsDegradeMode() bool
	GetTask(ctx context.Context, taskID string) (*model.PictureTask, error)
}

// SlotMetrics 可插拔指标接口，默认提供 no-op 实现
type SlotMetrics interface {
	IncRuns()
	AddKeysScanned(n int)
	AddTasksScanned(n int)
	IncReleaseSuccess()
	IncReleaseError()
	IncSkippedDegrade()
	ObserveDuration(d time.Duration)
}

// 默认 no-op 指标实现
type noopSlotMetrics struct{}

func (noopSlotMetrics) IncRuns()                        {}
func (noopSlotMetrics) AddKeysScanned(n int)            {}
func (noopSlotMetrics) AddTasksScanned(n int)           {}
func (noopSlotMetrics) IncReleaseSuccess()              {}
func (noopSlotMetrics) IncReleaseError()                {}
func (noopSlotMetrics) IncSkippedDegrade()              {}
func (noopSlotMetrics) ObserveDuration(d time.Duration) {}

// SlotReconcilerConfig 对账骨架配置
type SlotReconcilerConfig struct {
	Enabled          bool
	Interval         time.Duration
	RedisScanCount   int64
	MaxTasksPerRound int
	WorkerPoolSize   int
}

// ReconcilerBase 提供通用的对账循环/扫描/派发骨架
type ReconcilerBase struct {
	Cfg      SlotReconcilerConfig
	Rdb      *redis.Client
	LockMgr  *common.DistributedLockManager
	Logger   *zap.Logger
	Adapter  SlotAdapter
	Metrics  SlotMetrics
	stopCh   chan struct{}
	lockName string // 锁资源名，如: "slot_reconcile" / "queue_slot_reconcile"
}

// NewReconcilerBase 构造对账骨架
func NewReconcilerBase(cfg SlotReconcilerConfig, rdb *redis.Client, lm *common.DistributedLockManager, logger *zap.Logger, adapter SlotAdapter, lockName string, metrics SlotMetrics) *ReconcilerBase {
	if metrics == nil {
		metrics = noopSlotMetrics{}
	}
	if logger == nil {
		logger = zap.L().Named("ReconcilerBase")
	}
	return &ReconcilerBase{Cfg: cfg, Rdb: rdb, LockMgr: lm, Logger: logger, Adapter: adapter, Metrics: metrics, stopCh: make(chan struct{}), lockName: lockName}
}

// Start 启动骨架循环（占位，具体实现按需迁移）
func (r *ReconcilerBase) Start() {
	if !r.Cfg.Enabled {
		r.Logger.Info("reconciler disabled")
		return
	}
	go r.loop()
}

// Stop 停止骨架循环
func (r *ReconcilerBase) Stop() {
	select {
	case <-r.stopCh:
		return
	default:
		close(r.stopCh)
	}
}

// loop 主循环（锁获取 + 每轮对账占位逻辑）
func (r *ReconcilerBase) loop() {
	ticker := time.NewTicker(r.Cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stopCh:
			r.Logger.Info("reconciler stopped", zap.String("type", r.Adapter.Type()))
			return
		case <-ticker.C:
			r.safeRunOnce()
		}
	}
}

func (r *ReconcilerBase) safeRunOnce() {
	defer func() {
		if rec := recover(); rec != nil {
			r.Logger.Error("panic in reconciler runOnce", zap.Any("panic", rec), zap.String("type", r.Adapter.Type()))
		}
	}()
	r.runOnce()
}

// runOnce 仅保留获取锁与调用扫描的占位骨架，实际扫描/处理在现有实现中
func (r *ReconcilerBase) runOnce() {
	start := time.Now()
	r.Metrics.IncRuns()
	defer func() { r.Metrics.ObserveDuration(time.Since(start)) }()

	if r.Adapter.IsDegradeMode() {
		r.Metrics.IncSkippedDegrade()
		r.Logger.Warn("reconcile skipped due to degrade mode", zap.String("type", r.Adapter.Type()))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.Cfg.Interval)
	defer cancel()

	lockValue, err := r.LockMgr.AcquireLock(ctx, r.lockName, r.Cfg.Interval)
	if err != nil {
		r.Logger.Debug("lock not acquired", zap.Error(err), zap.String("type", r.Adapter.Type()))
		return
	}
	done, _ := r.LockMgr.StartRenewal(ctx, r.lockName, lockValue)
	defer func() {
		if done != nil {
			close(done)
		}
		_ = r.LockMgr.ReleaseLock(context.Background(), r.lockName, lockValue)
	}()

	// 每轮超时 + 扫描派发
	roundCtx, roundCancel := context.WithTimeout(ctx, time.Duration(float64(r.Cfg.Interval)*0.9))
	defer roundCancel()
	r.scanAndDispatchTasks(roundCtx)
}

// scanAndDispatchTasks 公共扫描/派发骨架（暂未接入现有实现）
func (r *ReconcilerBase) scanAndDispatchTasks(ctx context.Context) {
	jobs := make(chan struct{ userID, taskID string }, r.Cfg.WorkerPoolSize*2)
	var wg sync.WaitGroup
	for i := 0; i < r.Cfg.WorkerPoolSize; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				r.processOne(ctx, j.userID, j.taskID)
			}
		}()
	}
	processed := r.scanRedisKeys(ctx, jobs)
	r.Logger.Debug("reconcile round completed", zap.Int("processed_tasks", processed), zap.String("type", r.Adapter.Type()))
	close(jobs)
	wg.Wait()
}

func (r *ReconcilerBase) scanRedisKeys(ctx context.Context, jobs chan<- struct{ userID, taskID string }) int {
	processed := 0
	var cursor uint64
	pattern := r.Adapter.KeyPrefix() + "*" + r.Adapter.KeySuffix()
	for {
		select {
		case <-ctx.Done():
			r.Logger.Warn("reconcile round context done", zap.Error(ctx.Err()))
			return processed
		default:
		}
		keys, next, err := r.Rdb.Scan(cursor, pattern, r.Cfg.RedisScanCount).Result()
		if err != nil {
			r.Logger.Error("redis SCAN error", zap.Error(err))
			break
		}
		if len(keys) > 0 {
			r.Metrics.AddKeysScanned(len(keys))
		}
		for _, key := range keys {
			userID, ok := ParseUserIDFromKey(key, r.Adapter.KeyPrefix(), r.Adapter.KeySuffix())
			if !ok || userID == "" {
				continue
			}
			processed += r.scanTasksInKey(ctx, key, userID, jobs, processed)
			if processed >= r.Cfg.MaxTasksPerRound {
				break
			}
		}
		if processed >= r.Cfg.MaxTasksPerRound || next == 0 {
			break
		}
		cursor = next
	}
	return processed
}

// ParseUserIDFromKey 通用的从 key 中解析 userID 的工具
// 要求满足前后缀，避免重复实现
func ParseUserIDFromKey(key, prefix, suffix string) (string, bool) {
	if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, suffix) {
		return "", false
	}
	trimmed := strings.TrimPrefix(key, prefix)
	userID := strings.TrimSuffix(trimmed, suffix)
	if userID == "" {
		return "", false
	}
	return userID, true
}

func (r *ReconcilerBase) scanTasksInKey(ctx context.Context, key, userID string, jobs chan<- struct{ userID, taskID string }, currentProcessed int) int {
	processed := 0
	var sc uint64
	for {
		members, nextSc, err := r.Rdb.SScan(key, sc, "", r.Cfg.RedisScanCount).Result()
		if err != nil {
			r.Logger.Error("redis SSCAN error", zap.String("key", key), zap.Error(err))
			break
		}
		for _, taskID := range members {
			select {
			case <-ctx.Done():
				return processed
			default:
			}
			if currentProcessed+processed >= r.Cfg.MaxTasksPerRound {
				return processed
			}
			processed++
			r.Metrics.AddTasksScanned(1)
			jobs <- struct{ userID, taskID string }{userID: userID, taskID: taskID}
		}
		if currentProcessed+processed >= r.Cfg.MaxTasksPerRound || nextSc == 0 {
			break
		}
		sc = nextSc
	}
	return processed
}

func (r *ReconcilerBase) processOne(ctx context.Context, userID, taskID string) {
	if r.Adapter.IsDegradeMode() {
		r.Metrics.IncSkippedDegrade()
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	task, err := r.Adapter.GetTask(cctx, taskID)
	if err != nil || task == nil {
		if relErr := r.Adapter.Release(ctx, userID, taskID); relErr != nil {
			r.Metrics.IncReleaseError()
			r.Logger.Warn("release slot error for missing task", zap.String("user_id", userID), zap.String("task_id", taskID), zap.Error(relErr), zap.String("type", r.Adapter.Type()))
		} else {
			r.Metrics.IncReleaseSuccess()
		}
		return
	}
	// 当状态不再活跃时（不包含 PROCESSING/AWAITING），释放槽位
	// 额外：对于 AWAITING_PROVIDER_COMPLETION 长时间等待（>10分钟），也释放并发槽，以避免无限占用
	isAwaiting := task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION)
	awaitingTooLong := false
	if isAwaiting {
		// 使用配置的超时阈值，默认10分钟
		timeout := 10 * time.Minute
		if conf.GlobalConfig.Reconciler.Concurrent.AwaitingTimeout > 0 {
			timeout = conf.GlobalConfig.Reconciler.Concurrent.AwaitingTimeout
		}
		// 任务更新时间超过阈值，视为等待过久：仅释放并发槽位
		if time.Since(task.UpdatedAt) > timeout {
			awaitingTooLong = true
		}
	}
	if !r.Adapter.IsActive(task.Status) || awaitingTooLong {
		if relErr := r.Adapter.Release(ctx, userID, taskID); relErr != nil {
			r.Metrics.IncReleaseError()
			r.Logger.Warn("release slot error", zap.String("user_id", userID), zap.String("task_id", taskID), zap.Int32("status", task.Status), zap.Bool("awaiting_timeout", awaitingTooLong), zap.Error(relErr), zap.String("type", r.Adapter.Type()))
		} else {
			r.Metrics.IncReleaseSuccess()
		}
	}
}

// ProcessOneForTest 仅用于测试：暴露单个任务处理入口以便在外部包组合适配器进行行为验证。
// 注意：生产代码请使用 Start/Stop 驱动循环，勿直接调用该方法。
func (r *ReconcilerBase) ProcessOneForTest(ctx context.Context, userID, taskID string) {
	r.processOne(ctx, userID, taskID)
}
