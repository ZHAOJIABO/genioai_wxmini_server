package google

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/internal/zlog"
)

const (
	googleJWKSURL = "https://www.googleapis.com/oauth2/v3/certs"
	// Redis key for Google public keys
	redisGoogleKeysKey = "google:public_keys"
	// Redis key for distributed lock
	redisLockKey = "google:keys:lock"
	// Lock timeout
	lockTimeout = 10 * time.Second
	// Key refresh interval
	refreshInterval = 72 * time.Hour
)

// JWKS represents Google's JSON Web Key Set
type JWKS struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		Alg string `json:"alg"`
		Use string `json:"use"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

// KeyManager manages Google's public keys
type KeyManager struct {
	redis      *redis.Client
	httpClient *http.Client
	mu         sync.RWMutex
	keys       JWKS
}

// NewKeyManager creates a new KeyManager instance
func NewKeyManager(redisClient *redis.Client) *KeyManager {
	return &KeyManager{
		redis: redisClient,
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

// StartKeyRefresher starts the background key refresher
func (km *KeyManager) StartKeyRefresher(ctx context.Context) {
	// 启动时若 Redis 中无缓存，则立即刷新
	exists, err := km.redis.Exists(redisGoogleKeysKey).Result()
	if err != nil {
		zlog.LogWithContext(ctx).Warn("check google keys exist failed", zap.Error(err))
	} else if exists == 0 {
		go func() {
			if err := km.refreshKeys(ctx); err != nil {
				zlog.LogWithContext(ctx).Error("Failed to refresh Google public keys on start", zap.Error(err))
			}
		}()
	}

	go func() {
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := km.refreshKeys(ctx); err != nil {
					zlog.LogWithContext(ctx).Error("Failed to refresh Google public keys", zap.Error(err))
				}
			}
		}
	}()
}

// acquireLock tries to acquire a distributed lock
func (km *KeyManager) acquireLock(ctx context.Context) (bool, error) {
	return km.redis.SetNX(redisLockKey, "1", lockTimeout).Result()
}

// releaseLock releases the distributed lock
func (km *KeyManager) releaseLock(ctx context.Context) error {
	return km.redis.Del(redisLockKey).Err()
}

// refreshKeys refreshes the Google public keys
func (km *KeyManager) refreshKeys(ctx context.Context) error {
	// Try to acquire lock
	locked, err := km.acquireLock(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		return nil // Another instance is refreshing the keys
	}
	defer km.releaseLock(ctx)

	// Fetch new keys from Google
	resp, err := km.httpClient.Get(googleJWKSURL)
	if err != nil {
		return fmt.Errorf("failed to fetch Google public keys: %w", err)
	}
	defer resp.Body.Close()

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to decode JWKS: %w", err)
	}

	// Store keys in Redis
	keysJSON, err := json.Marshal(jwks)
	if err != nil {
		return fmt.Errorf("failed to marshal JWKS: %w", err)
	}

	if err := km.redis.Set(redisGoogleKeysKey, keysJSON, refreshInterval+time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to store keys in Redis: %w", err)
	}

	// Update local cache
	km.mu.Lock()
	km.keys = jwks
	km.mu.Unlock()

	return nil
}

// GetKeys returns the current public keys
func (km *KeyManager) GetKeys(ctx context.Context) (JWKS, error) {
	// Try to get from local cache first
	km.mu.RLock()
	if len(km.keys.Keys) > 0 {
		keys := km.keys
		km.mu.RUnlock()
		return keys, nil
	}
	km.mu.RUnlock()

	// Try to get from Redis
	keysJSON, err := km.redis.Get(redisGoogleKeysKey).Bytes()
	if err != nil && err != redis.Nil {
		return JWKS{}, fmt.Errorf("failed to get keys from Redis: %w", err)
	}

	if err == nil {
		var jwks JWKS
		if err := json.Unmarshal(keysJSON, &jwks); err != nil {
			return JWKS{}, fmt.Errorf("failed to unmarshal JWKS: %w", err)
		}

		// Update local cache
		km.mu.Lock()
		km.keys = jwks
		km.mu.Unlock()

		return jwks, nil
	}

	// If not in Redis, fetch from Google
	if err := km.refreshKeys(ctx); err != nil {
		return JWKS{}, err
	}

	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.keys, nil
}
