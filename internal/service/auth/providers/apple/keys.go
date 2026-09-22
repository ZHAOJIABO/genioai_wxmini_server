package apple

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/internal/zlog"
)

// ApplePublicKey 存储从 Apple 获取的公钥信息
type ApplePublicKey struct {
	Keys []struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

const (
	appleJWKSURL            = "https://appleid.apple.com/auth/keys"
	redisAppleKeysKey       = "apple:public_keys"
	redisAppleLockKey       = "apple:keys:lock"
	appleKeyLockTimeout     = 10 * time.Second
	appleKeyRefreshBefore   = 72 * time.Hour // TTL ≤ 3 天时开始刷新
	appleKeyRefreshInterval = time.Hour      // 失败后每小时重试
)

// KeyManager 管理 Apple 公钥
type KeyManager struct {
	redis      *redis.Client
	httpClient *http.Client
	mu         sync.RWMutex
	keys       ApplePublicKey
}

// NewKeyManager 创建新的 KeyManager 实例
func NewKeyManager(redisClient *redis.Client) *KeyManager {
	return &KeyManager{
		redis:      redisClient,
		httpClient: &http.Client{Timeout: 3 * time.Second},
	}
}

// StartKeyRefresher 启动后台定时重试，并在启动时检查 Redis 缓存
func (km *KeyManager) StartKeyRefresher(ctx context.Context) {
	// 启动时若 Redis 中无缓存，则立即刷新
	exists, err := km.redis.Exists(redisAppleKeysKey).Result()
	if err != nil {
		zlog.LogWithContext(ctx).Warn("check apple keys exist failed", zap.Error(err))
	} else if exists == 0 {
		go km.tryRefreshKeys(ctx)
	}

	go func() {
		ticker := time.NewTicker(appleKeyRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				km.tryRefreshKeys(ctx)
			}
		}
	}()
}

// GetKeys 获取当前可用 keys，优先本地，其次 Redis，不足时拉取
func (km *KeyManager) GetKeys(ctx context.Context) (ApplePublicKey, error) {
	// 1. 本地缓存
	km.mu.RLock()
	if len(km.keys.Keys) > 0 {
		keys := km.keys
		km.mu.RUnlock()
		// 剩余 TTL ≤ 阈值时异步刷新
		if ttl, _ := km.redis.TTL(redisAppleKeysKey).Result(); ttl <= appleKeyRefreshBefore {
			go km.tryRefreshKeys(ctx)
		}
		return keys, nil
	}
	km.mu.RUnlock()

	// 2. Redis 缓存
	data, err := km.redis.Get(redisAppleKeysKey).Bytes()
	if err == nil {
		var keys ApplePublicKey
		if err2 := json.Unmarshal(data, &keys); err2 == nil {
			km.mu.Lock()
			km.keys = keys
			km.mu.Unlock()
			if ttl, _ := km.redis.TTL(redisAppleKeysKey).Result(); ttl <= appleKeyRefreshBefore {
				go km.tryRefreshKeys(ctx)
			}
			return keys, nil
		}
	} else if err != redis.Nil {
		return ApplePublicKey{}, fmt.Errorf("redis get keys error: %w", err)
	}

	// 3. 缓存 Miss → 立即拉取
	if err := km.refreshKeys(ctx); err != nil {
		return ApplePublicKey{}, err
	}
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.keys, nil
}

// tryRefreshKeys 尝试刷新（TTL 到阈值时或定时器触发）
func (km *KeyManager) tryRefreshKeys(ctx context.Context) {
	ttl, err := km.redis.TTL(redisAppleKeysKey).Result()
	if err != nil || ttl > appleKeyRefreshBefore {
		return
	}
	if err := km.refreshKeys(ctx); err != nil {
		zlog.LogWithContext(ctx).Error("refresh apple keys failed", zap.Error(err))
	}
}

// refreshKeys 真正去 Apple 获取、写入 Redis 并更新本地
func (km *KeyManager) refreshKeys(ctx context.Context) error {
	ok, err := km.redis.SetNX(redisAppleLockKey, "1", appleKeyLockTimeout).Result()
	if err != nil || !ok {
		return err
	}
	defer km.redis.Del(redisAppleLockKey)

	resp, err := km.httpClient.Get(appleJWKSURL)
	if err != nil {
		return fmt.Errorf("fetch apple keys fail: %w", err)
	}
	defer resp.Body.Close()

	var keys ApplePublicKey
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		return fmt.Errorf("decode apple keys fail: %w", err)
	}

	ttl := parseCacheControl(resp.Header.Get("Cache-Control"))
	if ttl <= 0 {
		ttl = appleKeyRefreshBefore
	}

	buf, _ := json.Marshal(keys)
	if err := km.redis.Set(redisAppleKeysKey, buf, ttl).Err(); err != nil {
		return fmt.Errorf("store keys to redis fail: %w", err)
	}

	km.mu.Lock()
	km.keys = keys
	km.mu.Unlock()
	return nil
}

// parseCacheControl 从 Cache-Control: max-age=xxx 中解析 TTL
func parseCacheControl(cc string) time.Duration {
	for _, part := range strings.Split(cc, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "max-age=") {
			if secs, err := strconv.ParseInt(strings.TrimPrefix(part, "max-age="), 10, 64); err == nil {
				return time.Duration(secs) * time.Second
			}
		}
	}
	return 0
}
