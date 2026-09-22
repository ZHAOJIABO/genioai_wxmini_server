package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/internal/db"
	"va_visionai_server/internal/zlog"
)

type CacheUtil interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Del(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

type CacheService struct {
	cache CacheUtil
}

func NewCacheService() *CacheService {
	return &CacheService{
		cache: getDefaultCache(),
	}
}

func (s *CacheService) Get(ctx context.Context, key string, dest interface{}) error {
	return s.cache.Get(ctx, key, dest)
}

func (s *CacheService) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return s.cache.Set(ctx, key, value, expiration)
}

func (s *CacheService) Del(ctx context.Context, key string) error {
	return s.cache.Del(ctx, key)
}

func (s *CacheService) Exists(ctx context.Context, key string) (bool, error) {
	return s.cache.Exists(ctx, key)
}

func (s *CacheService) GetOrSet(ctx context.Context, key string, dest interface{}, expiration time.Duration, loader func() (interface{}, error)) error {
	err := s.Get(ctx, key, dest)
	if err == nil {
		return nil
	}

	value, err := loader()
	if err != nil {
		return err
	}

	if setErr := s.Set(ctx, key, value, expiration); setErr != nil {
	}

	return s.cache.Get(ctx, key, dest)
}

type MemoryCacheUtil struct {
	cache     map[string]*cacheItem
	mu        sync.RWMutex
	stopClean chan struct{}
}

type cacheItem struct {
	data      []byte
	expiresAt time.Time
}

func NewMemoryCacheUtil() *MemoryCacheUtil {
	c := &MemoryCacheUtil{
		cache:     make(map[string]*cacheItem),
		stopClean: make(chan struct{}),
	}
	go c.startCleanup()
	return c
}

func (c *MemoryCacheUtil) startCleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.cleanExpired()
		case <-c.stopClean:
			return
		}
	}
}

func (c *MemoryCacheUtil) cleanExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for key, item := range c.cache {
		if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
			delete(c.cache, key)
		}
	}
}

func (c *MemoryCacheUtil) Close() {
	close(c.stopClean)
}

func (c *MemoryCacheUtil) Get(ctx context.Context, key string, dest interface{}) error {
	c.mu.RLock()
	item, exists := c.cache[key]
	c.mu.RUnlock()

	if !exists {
		return ErrCacheNotFound
	}

	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		c.Del(ctx, key)
		return ErrCacheNotFound
	}

	return json.Unmarshal(item.data, dest)
}

func (c *MemoryCacheUtil) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	item := &cacheItem{data: data}
	if expiration > 0 {
		item.expiresAt = time.Now().Add(expiration)
	}

	c.mu.Lock()
	c.cache[key] = item
	c.mu.Unlock()
	return nil
}

func (c *MemoryCacheUtil) Del(ctx context.Context, key string) error {
	c.mu.Lock()
	delete(c.cache, key)
	c.mu.Unlock()
	return nil
}

func (c *MemoryCacheUtil) Exists(ctx context.Context, key string) (bool, error) {
	c.mu.RLock()
	item, exists := c.cache[key]
	c.mu.RUnlock()

	if !exists {
		return false, nil
	}

	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		c.Del(ctx, key)
		return false, nil
	}

	return true, nil
}

type RedisCacheUtil struct {
	client *redis.Client
}

func NewRedisCacheUtil() *RedisCacheUtil {
	return &RedisCacheUtil{
		client: db.GetRedis(),
	}
}

func (c *RedisCacheUtil) Get(ctx context.Context, key string, dest interface{}) error {
	result, err := c.client.Get(key).Result()
	if err != nil {
		if err == redis.Nil {
			return ErrCacheNotFound
		}
		return err
	}
	return json.Unmarshal([]byte(result), dest)
}

func (c *RedisCacheUtil) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(key, data, expiration).Err()
}

func (c *RedisCacheUtil) Del(ctx context.Context, key string) error {
	return c.client.Del(key).Err()
}

func (c *RedisCacheUtil) Exists(ctx context.Context, key string) (bool, error) {
	result, err := c.client.Exists(key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

type HybridCacheUtil struct {
	memCache   *MemoryCacheUtil
	redisCache *RedisCacheUtil
}

func NewHybridCacheUtil() *HybridCacheUtil {
	return &HybridCacheUtil{
		memCache:   NewMemoryCacheUtil(),
		redisCache: NewRedisCacheUtil(),
	}
}

func (c *HybridCacheUtil) Get(ctx context.Context, key string, dest interface{}) error {
	err := c.memCache.Get(ctx, key, dest)
	if err == nil {
		return nil
	}

	err = c.redisCache.Get(ctx, key, dest)
	if err != nil {
		return err
	}

	memExpiration := 5 * time.Minute
	if setErr := c.memCache.Set(ctx, key, dest, memExpiration); setErr != nil {
		zlog.LogWithContext(ctx).Warn("failed to update memory cache",
			zap.String("key", key),
			zap.Error(setErr))
	}

	return nil
}

func (c *HybridCacheUtil) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if err := c.redisCache.Set(ctx, key, value, expiration); err != nil {
		return err
	}

	memExpiration := 5 * time.Minute
	if expiration > 0 && expiration < memExpiration {
		memExpiration = expiration
	}

	if err := c.memCache.Set(ctx, key, value, memExpiration); err != nil {
		zlog.LogWithContext(ctx).Warn("failed to set memory cache",
			zap.String("key", key),
			zap.Error(err))
	}

	return nil
}

func (c *HybridCacheUtil) Del(ctx context.Context, key string) error {
	c.memCache.Del(ctx, key)
	return c.redisCache.Del(ctx, key)
}

func (c *HybridCacheUtil) Exists(ctx context.Context, key string) (bool, error) {
	exists, err := c.memCache.Exists(ctx, key)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}
	return c.redisCache.Exists(ctx, key)
}

var (
	ErrCacheNotFound = redis.Nil
)

func getDefaultCache() CacheUtil {
	return NewHybridCacheUtil()
}
