package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/service/cache"
	"va_visionai_server/internal/zlog"
)

// ConfigServiceIface 抽象配置服务接口，避免跨包循环依赖
type ConfigServiceIface interface {
	GetStringConfig(key string) (string, error)
}

const (
	taskConfigCacheKeyPrefix = "quota:task_limits"
	taskConfigCacheTTL       = 5 * time.Minute
)

var (
	fallbackMemberConfig = TaskLimitsConfig{MaxQueueSize: 2, MaxConcurrent: 1}
	//fallbackMemberConfig    = TaskLimitsConfig{MaxQueueSize: 6, MaxConcurrent: 2}
	fallbackNonMemberConfig = TaskLimitsConfig{MaxQueueSize: 2, MaxConcurrent: 1}
)

type taskLimitsConfigSet struct {
	Member    TaskLimitsConfig `json:"member"`
	NonMember TaskLimitsConfig `json:"non_member"`
}

type TaskConfigManager struct {
	cacheService  *cache.CacheService
	configService ConfigServiceIface
}

// NewTaskConfigManager 创建配置管理器
func NewTaskConfigManager(configService ConfigServiceIface, cacheService *cache.CacheService) *TaskConfigManager {
	return &TaskConfigManager{
		cacheService:  cacheService,
		configService: configService,
	}
}

// LoadConfig 加载任务提交相关配置（示例方法，保持接口不变）
func (m *TaskConfigManager) LoadConfig(ctx context.Context, isMember bool) TaskLimitsConfig {
	if m.configService == nil {
		zlog.LogWithContext(ctx).Warn("configService is nil, using fallback limits")
		return m.getFallbackConfig(isMember)
	}

	cacheKey := m.buildCacheKey(isMember)
	if cfg, ok := m.loadFromCache(ctx, cacheKey); ok {
		return cfg
	}

	cfgSet, err := m.loadConfigSet(ctx)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("failed to load task limits config, using fallback",
			zap.Error(err),
		)
		return m.getFallbackConfig(isMember)
	}

	m.persistToCache(ctx, cfgSet)

	if isMember {
		return cfgSet.Member
	}
	return cfgSet.NonMember
}

// TaskLimitsConfig 任务提交与执行限制配置
type TaskLimitsConfig struct {
	MaxQueueSize  int `json:"max_queue_size"`
	MaxConcurrent int `json:"max_concurrent"`
}

func (m *TaskConfigManager) ParseBoolConfig(val string) bool {
	return strings.ToLower(strings.TrimSpace(val)) == "true"
}

func (m *TaskConfigManager) loadFromCache(ctx context.Context, key string) (TaskLimitsConfig, bool) {
	if m.cacheService == nil {
		return TaskLimitsConfig{}, false
	}

	var cfg TaskLimitsConfig
	if err := m.cacheService.Get(ctx, key, &cfg); err == nil && cfg.MaxQueueSize > 0 && cfg.MaxConcurrent > 0 {
		return cfg, true
	}
	return TaskLimitsConfig{}, false
}

func (m *TaskConfigManager) persistToCache(ctx context.Context, cfg taskLimitsConfigSet) {
	if m.cacheService == nil {
		return
	}

	memberKey := m.buildCacheKey(true)
	nonMemberKey := m.buildCacheKey(false)

	_ = m.cacheService.Set(ctx, memberKey, cfg.Member, taskConfigCacheTTL)
	_ = m.cacheService.Set(ctx, nonMemberKey, cfg.NonMember, taskConfigCacheTTL)
}

func (m *TaskConfigManager) buildCacheKey(isMember bool) string {
	category := "non_member"
	if isMember {
		category = "member"
	}
	return fmt.Sprintf("%s:%s", taskConfigCacheKeyPrefix, category)
}

func (m *TaskConfigManager) loadConfigSet(ctx context.Context) (taskLimitsConfigSet, error) {
	// 仅读取新键
	raw, err := m.configService.GetStringConfig(constants.ConfigKeyTaskConcurrencyLimits)
	if err != nil {
		return taskLimitsConfigSet{}, err
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return taskLimitsConfigSet{}, fmt.Errorf("config %s is empty", constants.ConfigKeyTaskConcurrencyLimits)
	}

	var parsed struct {
		Member    *TaskLimitsConfig `json:"member"`
		NonMember *TaskLimitsConfig `json:"non_member"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return taskLimitsConfigSet{}, fmt.Errorf("parse task limits config failed: %w", err)
	}

	result := taskLimitsConfigSet{
		Member:    sanitizeLimits(parsed.Member, fallbackMemberConfig),
		NonMember: sanitizeLimits(parsed.NonMember, fallbackNonMemberConfig),
	}

	return result, nil
}

func sanitizeLimits(cfg *TaskLimitsConfig, fallback TaskLimitsConfig) TaskLimitsConfig {
	if cfg == nil {
		return fallback
	}

	result := *cfg
	if result.MaxQueueSize <= 0 {
		result.MaxQueueSize = fallback.MaxQueueSize
	}
	if result.MaxConcurrent <= 0 {
		result.MaxConcurrent = fallback.MaxConcurrent
	}

	return result
}

func (m *TaskConfigManager) getFallbackConfig(isMember bool) TaskLimitsConfig {
	if isMember {
		return fallbackMemberConfig
	}
	return fallbackNonMemberConfig
}
