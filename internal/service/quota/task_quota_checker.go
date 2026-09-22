package quota

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/quota"
	"va_visionai_server/internal/zlog"
)

const (
	redisQueueKeyPrefix         = quota.QueueTasksKeyPrefix      // 队列长度计数器前缀
	redisConcurrentKeyPrefix    = quota.ConcurrentTasksKeyPrefix // 并发执行计数器前缀
	redisHealthCheckInterval    = 10 * time.Second
	redisHealthCheckTimeout     = 5 * time.Second
	redisHealthCheckMaxDuration = 5 * time.Minute // 健康检查最大持续时间，防止永久阻塞
)

// 使用统一的配额类型定义（收敛到 internal/quota 包）
type QuotaType = quota.QuotaType

// 移除内置脚本：脚本统一由 task_quota_scripts.go 提供

// TaskQuotaChecker 任务配额检查器
type TaskQuotaChecker struct {
	rdb                *redis.Client
	taskDao            *dao.PictureTaskDao
	degradeMode        atomic.Bool // 降级模式标志
	healthChecker      *RedisHealthChecker
	healthCheckRunning atomic.Bool // 健康检查运行标志，防止goroutine泄漏
}

// NewTaskQuotaChecker 创建配额检查器
func NewTaskQuotaChecker(rdb *redis.Client, taskDao *dao.PictureTaskDao) *TaskQuotaChecker {
	checker := &TaskQuotaChecker{
		rdb:     rdb,
		taskDao: taskDao,
	}
	checker.degradeMode.Store(false)
	checker.healthChecker = NewRedisHealthChecker(rdb, redisHealthCheckInterval, redisHealthCheckTimeout)
	return checker
}

// CheckAndReserveSlot 统一的配额检查和预留接口（支持队列和并发两种类型）
func (c *TaskQuotaChecker) CheckAndReserveSlot(
	ctx context.Context,
	quotaType QuotaType,
	userID string,
	taskID string,
	maxLimit int,
) error {
	log := zlog.LogWithContext(ctx).With(
		zap.String("quota_type", string(quotaType)),
		zap.String("user_id", userID),
		zap.String("task_id", taskID),
		zap.Int("max_limit", maxLimit),
	)

	// 优先使用Redis（快速路径）
	if !c.degradeMode.Load() {
		err := c.checkWithRedis(ctx, quotaType, userID, taskID, maxLimit)
		if err == nil {
			log.Debug("slot reserved via Redis")
			return nil
		}

		// 处理Redis错误
		if c.isRedisError(err) {
			c.handleRedisError(ctx, log)
		} else {
			// 业务错误（如超限），直接返回
			return err
		}
	}

	// 降级模式：使用数据库（慢速路径）
	log.Info("using database for quota check (degrade mode)")
	return c.checkWithDB(ctx, quotaType, userID, maxLimit)
}

// CheckAndReserveConcurrentSlot 检查并预留并发槽位（保留兼容性）
func (c *TaskQuotaChecker) CheckAndReserveConcurrentSlot(
	ctx context.Context,
	userID string,
	taskID string,
	maxConcurrent int,
) error {
	return c.CheckAndReserveSlot(ctx, quota.QuotaTypeConcurrent, userID, taskID, maxConcurrent)
}

// CheckAndReserveQueueSlot 检查并预留队列槽位（新增）
func (c *TaskQuotaChecker) CheckAndReserveQueueSlot(
	ctx context.Context,
	userID string,
	taskID string,
	maxQueueSize int,
) error {
	return c.CheckAndReserveSlot(ctx, quota.QuotaTypeQueue, userID, taskID, maxQueueSize)
}

// handleRedisError 处理Redis错误并启动健康检查
func (c *TaskQuotaChecker) handleRedisError(ctx context.Context, log *zap.Logger) {
	log.Warn("redis error detected, switching to degrade mode")

	// 使用CAS确保只有一个goroutine执行降级切换
	if !c.degradeMode.CompareAndSwap(false, true) {
		return // 已经在降级模式
	}

	// 使用CAS确保只有一个健康检查goroutine运行
	if !c.healthCheckRunning.CompareAndSwap(false, true) {
		return // 健康检查已在运行
	}

	go c.startHealthCheckWithTimeout(log)
}

// startHealthCheckWithTimeout 启动带超时保护的健康检查
func (c *TaskQuotaChecker) startHealthCheckWithTimeout(log *zap.Logger) {
	defer c.healthCheckRunning.Store(false)

	// 添加超时保护，防止健康检查永久运行
	ctx, cancel := context.WithTimeout(context.Background(), redisHealthCheckMaxDuration)
	defer cancel()

	c.healthChecker.StartHealthCheck(ctx, func() {
		c.degradeMode.Store(false)
		log.Info("redis recovered, exiting degrade mode")
	})

	// 如果超时退出，记录警告
	if ctx.Err() == context.DeadlineExceeded {
		log.Warn("redis health check timeout, stopping health check",
			zap.Duration("max_duration", redisHealthCheckMaxDuration))
	}
}

// checkWithRedis 使用Redis进行配额检查（原子操作）
func (c *TaskQuotaChecker) checkWithRedis(
	ctx context.Context,
	quotaType QuotaType,
	userID string,
	taskID string,
	maxLimit int,
) error {
	key := c.buildKey(quotaType, userID)
	// 优先尝试使用统一脚本常量，兼容保留内置版本
	result, err := c.rdb.Eval(LuaCheckAndReserveScript,
		[]string{key},
		taskID,
		maxLimit,
		RedisKeyTTL(),
	).Result()

	if err != nil {
		return errors.Wrap(err, "redis eval failed")
	}

	return c.parseReserveResult(ctx, result, quotaType, userID, maxLimit)
}

// buildKey 根据配额类型构建Redis key
func (c *TaskQuotaChecker) buildKey(quotaType QuotaType, userID string) string {
	switch quotaType {
	case quota.QuotaTypeQueue:
		return fmt.Sprintf("%s%s", redisQueueKeyPrefix, userID)
	case quota.QuotaTypeConcurrent:
		return fmt.Sprintf("%s%s", redisConcurrentKeyPrefix, userID)
	default:
		return fmt.Sprintf("%s%s", redisConcurrentKeyPrefix, userID) // 默认并发
	}
}

// parseReserveResult 解析预留槽位的Redis返回结果
func (c *TaskQuotaChecker) parseReserveResult(
	ctx context.Context,
	result interface{},
	quotaType QuotaType,
	userID string,
	maxLimit int,
) error {
	vals, ok := result.([]interface{})
	if !ok || len(vals) != 2 {
		return errors.New("unexpected redis response format")
	}

	success, ok := vals[0].(int64)
	if !ok {
		return errors.New("unexpected success flag type")
	}

	if success == 0 {
		currentCount, _ := vals[1].(int64)

		// 根据配额类型返回不同的错误
		if quotaType == quota.QuotaTypeQueue {
			zlog.LogWithContext(ctx).Debug("queue limit exceeded",
				zap.String("user_id", userID),
				zap.Int64("current_count", currentCount),
				zap.Int("max_queue_size", maxLimit))
			return constants.ERR_TASK_QUEUE_LIMIT_EXCEEDED
		} else {
			zlog.LogWithContext(ctx).Debug("concurrent limit exceeded",
				zap.String("user_id", userID),
				zap.Int64("current_count", currentCount),
				zap.Int("max_concurrent", maxLimit))
			return constants.ERR_TASK_CONCURRENT_LIMIT_EXCEEDED
		}
	}

	return nil
}

// checkWithDB 使用数据库进行配额检查（降级方案）
func (c *TaskQuotaChecker) checkWithDB(ctx context.Context, quotaType QuotaType, userID string, maxLimit int) error {
	var count int64
	var err error

	// 根据配额类型选择不同的查询方法
	if quotaType == quota.QuotaTypeQueue {
		count, err = c.taskDao.CountActiveTasksByUser(ctx, nil, userID)
	} else {
		count, err = c.taskDao.CountProcessingTasksByUser(ctx, nil, userID)
	}

	if err != nil {
		return errors.Wrap(err, "failed to count tasks")
	}

	if count >= int64(maxLimit) {
		if quotaType == quota.QuotaTypeQueue {
			zlog.LogWithContext(ctx).Debug("queue limit exceeded (DB check)",
				zap.String("user_id", userID),
				zap.Int64("active_count", count),
				zap.Int("max_queue_size", maxLimit))
			return constants.ERR_TASK_QUEUE_LIMIT_EXCEEDED
		} else {
			zlog.LogWithContext(ctx).Debug("concurrent limit exceeded (DB check)",
				zap.String("user_id", userID),
				zap.Int64("processing_count", count),
				zap.Int("max_concurrent", maxLimit))
			return constants.ERR_TASK_CONCURRENT_LIMIT_EXCEEDED
		}
	}

	return nil
}

// IsDegradeMode 对外暴露降级模式查询，供外部流程（如对账器）判定是否跳过Redis写操作
func (c *TaskQuotaChecker) IsDegradeMode() bool {
	return c.degradeMode.Load()
}

// ReleaseSlot 统一的配额释放接口（支持队列和并发两种类型）
func (c *TaskQuotaChecker) ReleaseSlot(ctx context.Context, quotaType QuotaType, userID, taskID string) error {
	log := zlog.LogWithContext(ctx).With(
		zap.String("quota_type", string(quotaType)),
		zap.String("user_id", userID),
		zap.String("task_id", taskID),
	)

	// 如果在降级模式，跳过Redis操作
	if c.degradeMode.Load() {
		log.Debug("skip release slot in degrade mode")
		return nil
	}

	key := c.buildKey(quotaType, userID)

	result, err := c.rdb.Eval(LuaReleaseSlotScript, []string{key}, taskID).Int()
	if err != nil {
		log.Error("failed to release slot", zap.Error(err))
		return err
	}

	c.logReleaseResult(log, quotaType, result)
	return nil
}

// ReleaseConcurrentSlot 释放并发槽位（保留兼容性）
func (c *TaskQuotaChecker) ReleaseConcurrentSlot(ctx context.Context, userID, taskID string) error {
	return c.ReleaseSlot(ctx, quota.QuotaTypeConcurrent, userID, taskID)
}

// ReleaseQueueSlot 释放队列槽位（新增）
func (c *TaskQuotaChecker) ReleaseQueueSlot(ctx context.Context, userID, taskID string) error {
	return c.ReleaseSlot(ctx, quota.QuotaTypeQueue, userID, taskID)
}

// logReleaseResult 记录释放结果日志
func (c *TaskQuotaChecker) logReleaseResult(log *zap.Logger, quotaType QuotaType, result int) {
	slotTypeName := "concurrent slot"
	if quotaType == quota.QuotaTypeQueue {
		slotTypeName = "queue slot"
	}

	if result == 1 {
		log.Debug(slotTypeName + " released successfully")
	} else {
		log.Debug(slotTypeName + " was not released (task_id not found or count already 0)")
	}
}

// isRedisError 判断是否为Redis连接错误
func (c *TaskQuotaChecker) isRedisError(err error) bool {
	if err == nil {
		return false
	}
	// Redis连接错误、超时错误、网络错误
	errStr := err.Error()
	return err == redis.Nil ||
		errors.Is(err, redis.TxFailedErr) ||
		errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "no route to host") ||
		strings.Contains(errStr, "EOF")
}

// RedisHealthChecker Redis健康检查器
type RedisHealthChecker struct {
	rdb      *redis.Client
	interval time.Duration
	timeout  time.Duration
}

// NewRedisHealthChecker 创建健康检查器
func NewRedisHealthChecker(rdb *redis.Client, interval, timeout time.Duration) *RedisHealthChecker {
	return &RedisHealthChecker{
		rdb:      rdb,
		interval: interval,
		timeout:  timeout,
	}
}

// StartHealthCheck 启动健康检查（阻塞式，应在goroutine中调用）
func (h *RedisHealthChecker) StartHealthCheck(ctx context.Context, onRecovered func()) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	zlog.LogWithContext(ctx).Info("redis health check started",
		zap.Duration("interval", h.interval))

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if h.checkHealth(ctx) {
				zlog.LogWithContext(ctx).Info("redis health check passed, calling recovery callback")
				onRecovered()
				return // 恢复成功，退出健康检查
			}
		}
	}
}

// checkHealth 检查Redis健康状态
func (h *RedisHealthChecker) checkHealth(ctx context.Context) bool {
	err := h.rdb.Ping().Err()
	if err != nil {
		zlog.LogWithContext(ctx).Debug("redis health check failed", zap.Error(err))
		return false
	}

	return true
}
