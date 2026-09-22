package common

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"va_visionai_server/internal/zlog"
)

const (
	// DefaultLockTTL 默认锁过期时间
	DefaultLockTTL = 30 * time.Second
	// DefaultRenewalInterval 默认续约间隔
	DefaultRenewalInterval = 10 * time.Second
)

// 错误定义
var (
	ErrAcquireLockFailed = errors.New("无法获取分布式锁")
	ErrLockNotHeld       = errors.New("未持有该锁")
)

// DistributedLockManager 分布式锁管理器
type DistributedLockManager struct {
	redisClient redis.UniversalClient
	lockPrefix  string
	renewalTTL  time.Duration
}

// NewDistributedLockManager 创建新的分布式锁管理器
func NewDistributedLockManager(redisClient redis.UniversalClient, lockPrefix string) *DistributedLockManager {
	return &DistributedLockManager{
		redisClient: redisClient,
		lockPrefix:  lockPrefix,
		renewalTTL:  DefaultLockTTL,
	}
}

// SetRenewalTTL 设置锁续约TTL
func (lm *DistributedLockManager) SetRenewalTTL(ttl time.Duration) {
	if ttl > 0 {
		lm.renewalTTL = ttl
	}
}

// AcquireLock 获取锁
func (lm *DistributedLockManager) AcquireLock(ctx context.Context, resourceID string, ttl time.Duration) (string, error) {
	lockKey := lm.lockPrefix + ":" + resourceID
	lockValue := uuid.New().String()

	// 使用SET NX命令尝试获取锁
	success, err := lm.redisClient.SetNX(lockKey, lockValue, ttl).Result()
	if err != nil {
		zlog.Logger.Warn("获取分布式锁失败",
			zap.String("resource", resourceID),
			zap.Error(err))
		return "", err
	}

	if !success {
		return "", ErrAcquireLockFailed
	}

	zlog.Logger.Debug("成功获取分布式锁",
		zap.String("resource", resourceID),
		zap.String("lock_value", lockValue))

	return lockValue, nil
}

// ReleaseLock 释放锁
func (lm *DistributedLockManager) ReleaseLock(ctx context.Context, resourceID string, lockValue string) error {
	lockKey := lm.lockPrefix + ":" + resourceID

	// 使用Lua脚本确保只释放自己的锁
	script := `
	if redis.call("GET", KEYS[1]) == ARGV[1] then
		return redis.call("DEL", KEYS[1])
	else
		return 0
	end`

	result, err := lm.redisClient.Eval(script, []string{lockKey}, lockValue).Result()
	if err != nil {
		zlog.Logger.Warn("释放分布式锁失败",
			zap.String("resource", resourceID),
			zap.Error(err))
		return err
	}

	// 如果结果为0，表示锁不存在或不是由当前持有者持有
	if result.(int64) == 0 {
		zlog.Logger.Warn("尝试释放未持有的锁",
			zap.String("resource", resourceID),
			zap.String("lock_value", lockValue))
		return ErrLockNotHeld
	}

	zlog.Logger.Debug("成功释放分布式锁",
		zap.String("resource", resourceID),
		zap.String("lock_value", lockValue))

	return nil
}

// StartRenewal 开始自动续约
func (lm *DistributedLockManager) StartRenewal(ctx context.Context, resourceID string, lockValue string) (chan struct{}, error) {
	lockKey := lm.lockPrefix + ":" + resourceID
	done := make(chan struct{})

	// 检查锁是否存在且由当前持有者持有
	val, err := lm.redisClient.Get(lockKey).Result()
	if err != nil || val != lockValue {
		close(done)
		if err == redis.Nil {
			return nil, ErrLockNotHeld
		}
		if err != nil {
			return nil, err
		}
		return nil, ErrLockNotHeld
	}

	// 启动续约goroutine
	go func() {
		ticker := time.NewTicker(lm.renewalTTL / 3)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 使用Lua脚本续约锁
				script := `
				if redis.call("GET", KEYS[1]) == ARGV[1] then
					return redis.call("EXPIRE", KEYS[1], ARGV[2])
				else
					return 0
				end`

				result, err := lm.redisClient.Eval(script, []string{lockKey}, lockValue, int(lm.renewalTTL.Seconds())).Result()

				// 记录续约结果
				if err != nil {
					zlog.Logger.Warn("续约分布式锁失败",
						zap.String("resource", resourceID),
						zap.Error(err))
				} else if result.(int64) == 0 {
					zlog.Logger.Warn("锁已不再由当前实例持有，停止续约",
						zap.String("resource", resourceID))
					return
				} else {
					zlog.Logger.Debug("成功续约分布式锁",
						zap.String("resource", resourceID))
				}

			case <-done:
				zlog.Logger.Debug("停止续约分布式锁",
					zap.String("resource", resourceID))
				return

			case <-ctx.Done():
				zlog.Logger.Debug("上下文取消，停止续约分布式锁",
					zap.String("resource", resourceID))
				return
			}
		}
	}()

	return done, nil
}
