package picture_workflow_task

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/picture_generate"
	pb "va_visionai_server/internal/va_interface"
)

const (
	DefaultProviderSyncPollInterval  = 2 * time.Second
	DefaultProviderSyncBatchSize     = 10
	DefaultProviderTaskTimeout       = 15 * time.Minute
	providerStatusSyncLockKey        = "visionai:provider_status_sync_lock"
	providerStatusSyncLockTTL        = 30 * time.Second
	taskStatusSyncLockResourcePrefix = "task_status_sync:"
	// 终态处理（下载/上传、退款、完成推送）非幂等，且整个过程中 DB 状态仍是
	// AWAITING_PROVIDER_COMPLETION、会被下一轮重新捞出，只能靠这个 TTL 自然过期挡住重入，
	// 因此要留足一次下载+上传的余量。
	// 进度轮询那条廉价分支（仅查状态+写进度）会在返回前显式释放锁，不受这个 TTL 节流。
	taskStatusSyncLockTTL       = 15 * time.Second
	userProcessingLockKeyPrefix = "visionai:user_processing_lock:"
)

type ProviderStatusSyncConfig struct {
	PollInterval time.Duration
	BatchSize    int
	TaskTimeout  time.Duration
}

type ProviderStatusSyncProcessor struct {
	taskDao          *dao.PictureTaskDao
	executorProvider func(executorName string) picture_generate.TaskExecutorForSync
	cfg              ProviderStatusSyncConfig
	ticker           *time.Ticker
	done             chan struct{}
	logger           *zap.Logger
	rdb              *redis.Client
	taskService      *service.PictureTaskService

	// 使用通用分布式锁管理器，保证所有权校验与续期
	lockMgr *common.DistributedLockManager
}

func NewProviderStatusSyncProcessor(
	taskDao *dao.PictureTaskDao,
	executorProvider func(executorName string) picture_generate.TaskExecutorForSync,
	config *ProviderStatusSyncConfig,
	baseLogger *zap.Logger,
	rdb *redis.Client,
	taskService *service.PictureTaskService,
) *ProviderStatusSyncProcessor {
	pConf := ProviderStatusSyncConfig{
		PollInterval: DefaultProviderSyncPollInterval,
		BatchSize:    DefaultProviderSyncBatchSize,
		TaskTimeout:  DefaultProviderTaskTimeout,
	}
	if config != nil {
		if config.PollInterval > 0 {
			pConf.PollInterval = config.PollInterval
		}
		if config.BatchSize > 0 {
			pConf.BatchSize = config.BatchSize
		}
		if config.TaskTimeout > 0 {
			pConf.TaskTimeout = config.TaskTimeout
		}
	}
	p := &ProviderStatusSyncProcessor{
		taskDao:          taskDao,
		executorProvider: executorProvider,
		cfg:              pConf,
		done:             make(chan struct{}),
		logger:           baseLogger.With(zap.String("processor", "ProviderStatusSync")),
		rdb:              rdb,
		taskService:      taskService,
	}

	// 初始化分布式锁管理器
	p.lockMgr = common.NewDistributedLockManager(rdb, "visionai:provider_sync")
	p.lockMgr.SetRenewalTTL(providerStatusSyncLockTTL)

	return p
}

func (p *ProviderStatusSyncProcessor) Start() {
	if p.ticker != nil {
		p.logger.Warn("Processor already started or not properly stopped.")
		return
	}
	p.logger.Info("Starting provider status sync processor",
		zap.Duration("pollInterval", p.cfg.PollInterval),
		zap.Int("batchSize", p.cfg.BatchSize),
		zap.Duration("taskTimeout", p.cfg.TaskTimeout),
	)
	// 启动即执行一次，减少重启后的同步延迟
	go func() {
		p.logger.Info("Executing initial provider status sync on startup")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		p.processTasks(ctx)
	}()

	p.ticker = time.NewTicker(p.cfg.PollInterval)
	go p.run()
}

func (p *ProviderStatusSyncProcessor) run() {
	defer p.logger.Info("Provider status sync processor run loop stopped.")
	for {
		select {
		case <-p.done:
			return
		case <-p.ticker.C:
			p.logger.Debug("Provider status sync processor tick")
			// 设定明确的一轮处理超时，避免 p.cfg.PollInterval - 5s 为 0 的问题
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			p.processTasks(ctx)
			cancel()
		}
	}
}

func (p *ProviderStatusSyncProcessor) Stop() {
	if p.ticker == nil {
		p.logger.Warn("Processor not started or already stopped.")
		return
	}
	p.logger.Info("Stopping provider status sync processor...")
	p.ticker.Stop()
	p.ticker = nil
	close(p.done)
}

func (p *ProviderStatusSyncProcessor) processTasks(ctx context.Context) {
	// panic 保护，避免单轮异常中断循环
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("Panic in processTasks, recovered", zap.Any("panic", r), zap.Stack("stack"))
		}
	}()

	// 使用通用分布式锁管理器尝试获取锁
	lockValue, err := p.lockMgr.AcquireLock(ctx, "provider_status_sync_lock", providerStatusSyncLockTTL)
	if err != nil {
		p.logger.Debug("Acquire provider status sync lock failed or not acquired", zap.Error(err))
		return
	}
	// 启动续期，确保长时间处理时锁不丢失
	done, _ := p.lockMgr.StartRenewal(ctx, "provider_status_sync_lock", lockValue)
	defer func() {
		if done != nil {
			close(done)
		}
		// 使用独立上下文，确保释放锁不受父 ctx 取消影响
		if rerr := p.lockMgr.ReleaseLock(context.Background(), "provider_status_sync_lock", lockValue); rerr != nil {
			p.logger.Warn("release provider status sync lock error", zap.Error(rerr))
		}
	}()

	p.logger.Debug("Processing tasks for provider status sync")
	tasks, err := p.taskDao.GetTasksByStatus(
		ctx,
		pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION,
		p.cfg.BatchSize,
	)
	if err != nil {
		p.logger.Error("Failed to get tasks for provider sync", zap.Error(err))
		return
	}

	if len(tasks) == 0 {
		p.logger.Debug("No tasks found for provider status sync")
		return
	}
	p.logger.Debug("Found tasks for provider status sync", zap.Int("count", len(tasks)))

	for _, task := range tasks {
		taskCtx, taskCancel := context.WithTimeout(ctx, 30*time.Second)
		p.handleSingleTask(taskCtx, task)
		taskCancel()
	}
}

func (p *ProviderStatusSyncProcessor) handleSingleTask(ctx context.Context, task *model.PictureTask) {
	taskLogger := p.logger.With(
		zap.String("taskID", task.TaskID),
		zap.String("executer", task.Executer),
		zap.String("providerTaskID", task.ExecuterTaskID),
		zap.Time("taskUpdatedAt", task.UpdatedAt),
	)
	taskLogger.Debug("Attempting to sync provider status for task")

	// 为具体任务获取同步锁，避免同一任务被多次同步
	taskSyncResource := taskStatusSyncLockResourcePrefix + task.TaskID
	taskLogger.Debug("Attempting to acquire task status sync lock", zap.String("lock_resource", taskSyncResource))

	lockValue, err := p.lockMgr.AcquireLock(ctx, taskSyncResource, taskStatusSyncLockTTL)
	if err != nil {
		if errors.Is(err, common.ErrAcquireLockFailed) {
			taskLogger.Debug("Another instance is syncing this task, skipping.", zap.String("lock_resource", taskSyncResource))
		} else {
			taskLogger.Error("Failed to attempt acquiring task status sync lock", zap.Error(err), zap.String("lock_resource", taskSyncResource))
		}
		return
	}
	taskLogger.Debug("Successfully acquired task status sync lock", zap.String("lock_resource", taskSyncResource))

	// 默认保留锁到 TTL 过期：终态处理非幂等，必须挡住下一轮重入。
	// 只有"仍在进行中"的廉价分支才把 releaseNow 置为 true，提前释放以免节流进度更新。
	releaseNow := false
	defer func() {
		if !releaseNow {
			return
		}
		// 使用独立上下文，确保释放锁不受父 ctx 取消影响
		if rerr := p.lockMgr.ReleaseLock(context.Background(), taskSyncResource, lockValue); rerr != nil && !errors.Is(rerr, common.ErrLockNotHeld) {
			taskLogger.Warn("release task status sync lock error", zap.Error(rerr))
		}
	}()

	if time.Since(task.UpdatedAt) > p.cfg.TaskTimeout && task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION) {
		taskLogger.Error("Task has been awaiting provider completion for too long, marking as FAILED",
			zap.Duration("elapsed", time.Since(task.UpdatedAt)),
			zap.Duration("timeoutThreshold", p.cfg.TaskTimeout),
		)
		// FailTask 包含事务、退款等操作,设置 60 秒超时
		failCtx, failCancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer failCancel()
		if err := p.taskService.FailTask(failCtx, task, "Provider task processing timeout."); err != nil {
			taskLogger.Error("Failed to handle task failure after provider timeout", zap.Error(err))
		}
		return
	}

	executor := p.executorProvider(task.Executer)
	if executor == nil {
		taskLogger.Error("No executor found for task, marking as FAILED")
		failCtx, failCancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer failCancel()
		if err := p.taskService.FailTask(failCtx, task, "No suitable executor found for status sync."); err != nil {
			taskLogger.Error("Failed to handle task failure after no executor for sync", zap.Error(err))
		}
		return
	}
	// 保持每任务的 30 秒超时控制，沿用父级 ctx 传入执行器
	// 注意：若需要更长的超时，应使用 context.WithTimeout(ctx, X) 而不是 WithoutCancel，确保父级取消能正确传播
	syncCtx, syncCancel := context.WithTimeout(context.WithoutCancel(ctx), 120*time.Second)
	defer syncCancel()
	newStatus, syncErr := executor.SyncProviderStatus(syncCtx, task)
	if syncErr != nil {
		taskLogger.Warn("SyncProviderStatus returned an error", zap.Error(syncErr), zap.Int32("newStatus", int32(newStatus)))

		// 改进的逻辑：只要执行器返回的状态不是明确的失败，就认为是可重试的。
		// 这是因为执行器最了解其内部状态，调用方应信任其判断。
		if newStatus != pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED {
			taskLogger.Warn("Task status is not FAILED despite sync error. It will be retried on the next cycle.", zap.Error(syncErr))
			// 仅回写错误信息与失败计数，幂等且廉价，可立即释放锁让下一轮继续推进
			releaseNow = true
			// 执行器已经在内存中更新了 task 的错误信息和失败计数，在这里我们负责将这些更新持久化到数据库。
			// UpdateTask 为数据库写入操作,设置 30 秒超时
			updateCtx, updateCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer updateCancel()
			if err := p.taskDao.UpdateTask(updateCtx, task); err != nil {
				taskLogger.Error("Failed to update task status after a retryable sync error",
					zap.String("taskID", task.TaskID),
					zap.Error(err),
				)
			}
			return
		}

		// 对于明确返回 FAILED 状态的错误，我们委托给统一的失败处理器。
		taskLogger.Error("Executor's SyncProviderStatus reported a definitive failure", zap.Error(syncErr))
		failCtx, failCancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer failCancel()
		if err := p.taskService.FailTask(failCtx, task, fmt.Sprintf("Executor sync failed: %v", syncErr)); err != nil {
			taskLogger.Error("Failed to handle task failure after executor sync error", zap.Error(err))
		}
		return // 任务已处理，直接返回
	}

	// 当任务达到终态（完成或失败）时
	if newStatus == pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED ||
		newStatus == pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED {
		taskLogger.Info("Task reached terminal state after sync", zap.Int32("status", int32(newStatus)))

		// 只有在任务成功时，才继续处理结果并释放锁。失败情况已由 FailTask 处理。
		if newStatus == pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED {
			// ProcessSuccessfulResult 包含下载图片、上传 OSS 等 IO 操作,可能较慢,设置 3 分钟超时
			processCtx, processCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
			defer processCancel()
			if err := executor.ProcessSuccessfulResult(processCtx, task); err != nil {
				taskLogger.Error("Failed to process successful result, marking task as FAILED", zap.Error(err))
				// 如果结果处理失败，则调用统一的失败处理器
				failCtx, failCancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
				defer failCancel()
				if failErr := p.taskService.FailTask(failCtx, task, fmt.Sprintf("Failed to process successful result: %v", err)); failErr != nil {
					taskLogger.Error("Failed to handle task failure after result processing error", zap.Error(failErr))
				}
			} else {
				p.releaseUserProcessingLock(task.UserID, task.TaskID, taskLogger)
				// 调用完成Hook，确保链式任务在同步完成场景也能触发
				picture_generate.TriggerOnTaskCompleted(task)
				// 发送任务完成推送通知
				go p.taskService.SendTaskCompletionPush(context.Background(), task)
			}
		}
		// 对于FAILED状态，锁已在FailTask中释放，此处无需操作。
	} else {
		// 任务仍在进行中：本轮只查了状态、写了进度，幂等且廉价，立即释放锁，
		// 否则单任务的进度更新会被 taskStatusSyncLockTTL 节流成一格。
		releaseNow = true
		taskLogger.Debug("Task still ongoing after sync or status unchanged.", zap.Int32("status", int32(newStatus)))
	}
}

func (p *ProviderStatusSyncProcessor) releaseUserProcessingLock(userID string, taskID string, logger *zap.Logger) {
	lockKey := fmt.Sprintf("%s%s", userProcessingLockKeyPrefix, userID)

	// 检查锁的值是否仍然是当前任务ID，避免错误地释放了为后续任务加的锁
	currentLockVal, err := p.rdb.Get(lockKey).Result()
	if err != nil {
		if err != redis.Nil {
			logger.Error("Failed to get user processing lock before releasing",
				zap.String("user_id", userID),
				zap.String("task_id", taskID),
				zap.String("lock_key", lockKey),
				zap.Error(err),
			)
		}
		// 如果锁不存在，说明已经释放或过期，无需操作
		return
	}

	if currentLockVal == taskID {
		if err := p.rdb.Del(lockKey).Err(); err != nil {
			logger.Error("Failed to release user processing lock",
				zap.String("user_id", userID),
				zap.String("task_id", taskID),
				zap.String("lock_key", lockKey),
				zap.Error(err),
			)
		} else {
			logger.Info("Successfully released user processing lock",
				zap.String("user_id", userID),
				zap.String("task_id", taskID),
				zap.String("lock_key", lockKey),
			)
		}
	} else {
		logger.Warn("Did not release user processing lock: lock is held by another task",
			zap.String("user_id", userID),
			zap.String("current_task_id", taskID),
			zap.String("lock_holder_task_id", currentLockVal),
		)
	}
}
