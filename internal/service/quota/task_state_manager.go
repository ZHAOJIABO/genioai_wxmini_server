package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

const (
	redisTaskLockPrefix = "visionai:task:lock:"
	lockTTL             = 30 * time.Second
)

// TaskStateManager 任务状态管理器
type TaskStateManager struct {
	rdb          *redis.Client
	taskDao      *dao.PictureTaskDao
	quotaChecker *TaskQuotaChecker
}

// NewTaskStateManager 创建状态管理器
func NewTaskStateManager(
	rdb *redis.Client,
	taskDao *dao.PictureTaskDao,
	quotaChecker *TaskQuotaChecker,
) *TaskStateManager {
	return &TaskStateManager{
		rdb:          rdb,
		taskDao:      taskDao,
		quotaChecker: quotaChecker,
	}
}

// TransitionToProcessing 原子性地将任务从PENDING转换为PROCESSING
// 该方法确保：
// 1. 任务状态转换的原子性
// 2. 并发槽位的正确预留
// 3. 失败时的自动回滚
func (m *TaskStateManager) TransitionToProcessing(
	ctx context.Context,
	taskID string,
	userID string,
	maxConcurrent int,
) (*model.PictureTask, error) {
	log := zlog.LogWithContext(ctx).With(
		zap.String("task_id", taskID),
		zap.String("user_id", userID),
		zap.Int("max_concurrent", maxConcurrent),
	)

	// 1. 获取分布式锁（避免并发调度同一任务）
	lockKey := fmt.Sprintf("%s%s", redisTaskLockPrefix, taskID)
	lockValue, err := m.acquireLock(ctx, lockKey, lockTTL)
	if err != nil {
		log.Debug("failed to acquire lock, task may be processing by another worker")
		return nil, errors.Wrap(err, "acquire lock failed")
	}
	defer m.releaseLock(ctx, lockKey, lockValue)

	// 2. 开启数据库事务
	db := m.taskDao.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return nil, errors.Wrap(tx.Error, "begin transaction failed")
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Error("panic in TransitionToProcessing", zap.Any("panic", r), zap.Stack("stack"))
		}
	}()

	// 3. 在事务内锁定任务记录
	task, err := m.taskDao.GetTaskForUpdate(ctx, tx, taskID)
	if err != nil {
		tx.Rollback()
		return nil, errors.Wrap(err, "get task for update failed")
	}

	// 4. 验证任务状态（只允许PENDING转PROCESSING）
	if task.Status != int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING) {
		tx.Rollback()
		log.Debug("task is not in PENDING state, skip transition",
			zap.Int32("current_status", task.Status))
		return nil, fmt.Errorf("task status is not PENDING: %d", task.Status)
	}

	// 5. 预留并发槽位（Redis原子操作）
	if err := m.quotaChecker.CheckAndReserveConcurrentSlot(ctx, userID, taskID, maxConcurrent); err != nil {
		tx.Rollback()
		log.Debug("failed to reserve concurrent slot", zap.Error(err))
		return nil, err
	}

	// 6. 更新任务状态为PROCESSING
	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING)
	task.UpdatedAt = time.Now()
	if err := tx.Save(task).Error; err != nil {
		// 回滚Redis计数
		_ = m.quotaChecker.ReleaseConcurrentSlot(ctx, userID, taskID)
		tx.Rollback()
		log.Error("failed to update task status", zap.Error(err))
		return nil, errors.Wrap(err, "update task status failed")
	}

	// 7. 提交事务
	if err := tx.Commit().Error; err != nil {
		// 回滚Redis计数
		_ = m.quotaChecker.ReleaseConcurrentSlot(ctx, userID, taskID)
		log.Error("failed to commit transaction", zap.Error(err))
		return nil, errors.Wrap(err, "commit transaction failed")
	}

	log.Info("task transitioned to PROCESSING successfully")
	return task, nil
}

// acquireLock 获取分布式锁
func (m *TaskStateManager) acquireLock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	lockValue := uuid.New().String()
	success, err := m.rdb.SetNX(key, lockValue, ttl).Result()
	if err != nil {
		return "", err
	}
	if !success {
		return "", errors.New("failed to acquire lock: key already exists")
	}
	return lockValue, nil
}

// releaseLock 释放分布式锁（Lua脚本验证token）
func (m *TaskStateManager) releaseLock(ctx context.Context, key, lockValue string) error {
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`
	_, err := m.rdb.Eval(script, []string{key}, lockValue).Result()
	return err
}
