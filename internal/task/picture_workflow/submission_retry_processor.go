package picture_workflow_task

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/picture_generate"
	pb "va_visionai_server/internal/va_interface"
)

const (
	DefaultSubmissionRetryPollInterval = 1 * time.Minute
	DefaultSubmissionMaxRetryAttempts  = 3
	DefaultSubmissionBatchSize         = 10
	submissionRetryLockKey             = "visionai:submission_retry_lock"
	submissionRetryLockTTL             = 5 * time.Minute
)

type SubmissionRetryProcessorConfig struct {
	PollInterval     time.Duration
	MaxRetryAttempts int32
	BatchSize        int
}

type SubmissionRetryProcessor struct {
	taskDao          *dao.PictureTaskDao
	executorProvider func(executorName string) picture_generate.TaskExecutorForRetry
	cfg              SubmissionRetryProcessorConfig
	ticker           *time.Ticker
	done             chan struct{}
	logger           *zap.Logger
	rdb              *redis.Client
}

func NewSubmissionRetryProcessor(
	taskDao *dao.PictureTaskDao,
	executorProvider func(executorName string) picture_generate.TaskExecutorForRetry,
	config *SubmissionRetryProcessorConfig,
	baseLogger *zap.Logger,
	rdb *redis.Client,
) *SubmissionRetryProcessor {
	pConf := SubmissionRetryProcessorConfig{
		PollInterval:     DefaultSubmissionRetryPollInterval,
		MaxRetryAttempts: DefaultSubmissionMaxRetryAttempts,
		BatchSize:        DefaultSubmissionBatchSize,
	}
	if config != nil {
		if config.PollInterval > 0 {
			pConf.PollInterval = config.PollInterval
		}
		if config.MaxRetryAttempts > 0 {
			pConf.MaxRetryAttempts = config.MaxRetryAttempts
		}
		if config.BatchSize > 0 {
			pConf.BatchSize = config.BatchSize
		}
	}

	return &SubmissionRetryProcessor{
		taskDao:          taskDao,
		executorProvider: executorProvider,
		cfg:              pConf,
		done:             make(chan struct{}),
		logger:           baseLogger.With(zap.String("processor", "SubmissionRetry")),
		rdb:              rdb,
	}
}

func (p *SubmissionRetryProcessor) Start() {
	if p.ticker != nil {
		p.logger.Warn("Processor already started or not properly stopped.")
		return
	}
	p.logger.Info("Starting submission retry processor",
		zap.Duration("pollInterval", p.cfg.PollInterval),
		zap.Int32("maxRetries", p.cfg.MaxRetryAttempts),
		zap.Int("batchSize", p.cfg.BatchSize),
	)
	p.ticker = time.NewTicker(p.cfg.PollInterval)
	go p.run()
}

func (p *SubmissionRetryProcessor) run() {
	defer p.logger.Info("Submission retry processor run loop stopped.")
	for {
		select {
		case <-p.done:
			return
		case <-p.ticker.C:
			p.logger.Debug("Submission retry processor tick")
			ctx, cancel := context.WithTimeout(context.Background(), p.cfg.PollInterval-5*time.Second)
			p.processTasks(ctx)
			cancel()
		}
	}
}

func (p *SubmissionRetryProcessor) Stop() {
	if p.ticker == nil {
		p.logger.Warn("Processor not started or already stopped.")
		return
	}
	p.logger.Info("Stopping submission retry processor...")
	p.ticker.Stop()
	p.ticker = nil
	close(p.done)
}

func (p *SubmissionRetryProcessor) processTasks(ctx context.Context) {
	// 尝试获取分布式锁
	p.logger.Debug("Attempting to acquire submission retry lock")
	acquired, err := p.rdb.SetNX(submissionRetryLockKey, "1", submissionRetryLockTTL).Result()
	if err != nil {
		p.logger.Error("Failed to attempt acquiring submission retry lock", zap.Error(err))
		return
	}
	if !acquired {
		p.logger.Debug("Another instance holds the submission retry lock, skipping.")
		return
	}
	p.logger.Debug("Successfully acquired submission retry lock")

	// 确保锁被释放
	defer func() {
		if err := p.rdb.Del(submissionRetryLockKey).Err(); err != nil {
			p.logger.Error("Failed to release submission retry lock", zap.Error(err))
		} else {
			p.logger.Debug("Successfully released submission retry lock")
		}
	}()

	p.logger.Debug("Processing tasks for submission retry")
	tasks, err := p.taskDao.GetTasksByStatusAndMaxRetries(
		ctx,
		pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING_SUBMISSION_RETRY,
		p.cfg.MaxRetryAttempts,
		p.cfg.BatchSize,
	)
	if err != nil {
		p.logger.Error("Failed to get tasks for submission retry", zap.Error(err))
		return
	}

	if len(tasks) == 0 {
		p.logger.Debug("No tasks found for submission retry")
		return
	}

	p.logger.Info("Found tasks for submission retry", zap.Int("count", len(tasks)))

	for _, task := range tasks {
		taskCtx, taskCancel := context.WithTimeout(ctx, 30*time.Second)
		p.handleSingleTask(taskCtx, task)
		taskCancel()
	}
}

func (p *SubmissionRetryProcessor) handleSingleTask(ctx context.Context, task *model.PictureTask) {
	taskLogger := p.logger.With(zap.String("taskID", task.TaskID), zap.String("executer", task.Executer), zap.Int32("retryCount", task.RetryCount))
	taskLogger.Info("Attempting to retry submission for task")

	var taskErr error
	needsDBUpdate := false
	originalStatus := task.Status
	originalError := task.Error

	defer func() {
		if ctx.Err() != nil {
			taskLogger.Warn("Context cancelled or timed out before final task update in defer.", zap.Error(ctx.Err()))
			return
		}
		if taskErr != nil {
			task.Error = taskErr.Error()
			if task.Status != int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION) &&
				task.Status != int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
				task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			}
		}
		if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
			task.Progress = -1
		}
		if needsDBUpdate || taskErr != nil || task.Status != originalStatus || task.Error != originalError {
			task.UpdatedAt = time.Now()
			if updateErr := p.taskDao.UpdateTask(context.Background(), task); updateErr != nil {
				taskLogger.Error("Failed to update task after retry attempt in defer", zap.Error(updateErr),
					zap.Int32("finalStatus", task.Status),
					zap.String("finalError", task.Error),
					zap.Int32("finalProgress", task.Progress))
			} else {
				taskLogger.Info("Task updated successfully in defer.",
					zap.Int32("finalStatus", task.Status),
					zap.String("finalError", task.Error),
					zap.Int32("finalProgress", task.Progress))
				// 进入终态(FAILED)后，触发终态钩子以统一释放并发/队列槽位
				if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
					picture_generate.TriggerOnTaskTerminated(context.Background(), task)
				}
			}
		}
	}()

	task.RetryCount++
	if err := p.taskDao.UpdateTask(ctx, &model.PictureTask{TaskID: task.TaskID, RetryCount: task.RetryCount, UpdatedAt: time.Now()}); err != nil {
		taskLogger.Error("Failed to increment retry_count", zap.Error(err))
	} else {
		needsDBUpdate = true
	}

	executor := p.executorProvider(task.Executer)
	if executor == nil {
		taskLogger.Error("No executor found for task, marking as FAILED")
		task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
		taskErr = errors.New("no suitable executor found for retry")
		return
	}

	err := executor.RetrySubmission(ctx, task)
	if err != nil {
		taskLogger.Error("Executor failed to retry submission", zap.Error(err), zap.Int32("currentRetryCount", task.RetryCount))
		taskErr = err

		if !executor.IsSubmissionErrorRetryable(err) {
			taskLogger.Warn("Error is not retryable, marking task as FAILED", zap.Error(err))
			task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			return
		}

		if task.RetryCount >= p.cfg.MaxRetryAttempts {
			taskLogger.Warn("Max retry attempts reached, task will be marked as FAILED", zap.Int32("retryCount", task.RetryCount))
			task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
		}
		return
	}

	taskLogger.Info("Executor successfully retried submission.", zap.Int32("newStatus", task.Status))
	needsDBUpdate = true
}
