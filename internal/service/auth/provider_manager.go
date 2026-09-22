package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"va_visionai_server/internal/service/auth/types"
	"va_visionai_server/internal/zlog"
)

// ProviderFactory 是一个创建登录提供商的工厂函数类型
type ProviderFactory func(config types.ProviderConfig) (types.LoginProvider, error)

// ProviderConfig 存储登录提供商的配置
type ProviderConfig struct {
	ProjectID    string                 // 项目ID
	ProviderType string                 // 提供商类型 (normal, apple, google, 等)
	ClientID     string                 // OAuth客户端ID
	ClientSecret string                 // OAuth客户端密钥
	RedirectURI  string                 // OAuth重定向URI
	ExtraConfig  map[string]interface{} // 额外配置
}

// ProviderManager 管理各种登录提供商和配置
type ProviderManager struct {
	factories    map[string]ProviderFactory         // 提供商工厂映射
	configs      map[configKey]types.ProviderConfig // 提供商配置映射
	providers    map[configKey]types.LoginProvider  // 实例化的提供商映射
	mutex        sync.RWMutex                       // 读写锁，用于并发控制
	defaultProjs map[string]string                  // 项目类型到默认项目ID的映射
}

// configKey 用于配置映射的键
type configKey struct {
	projectID    string
	providerType string
}

// NewProviderManager 创建一个新的提供商管理器
func NewProviderManager() *ProviderManager {
	return &ProviderManager{
		factories:    make(map[string]ProviderFactory),
		configs:      make(map[configKey]types.ProviderConfig),
		providers:    make(map[configKey]types.LoginProvider),
		defaultProjs: make(map[string]string),
	}
}

// RegisterFactory 注册提供商工厂函数
func (pm *ProviderManager) RegisterFactory(providerType string, factory ProviderFactory) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	pm.factories[providerType] = factory
}

// RegisterConfig 注册提供商配置
func (pm *ProviderManager) RegisterConfig(config types.ProviderConfig) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// 检查该类型的提供商工厂是否存在
	factory, ok := pm.factories[config.ProviderType]
	if !ok {
		return fmt.Errorf("provider factory not found for type: %s", config.ProviderType)
	}

	// 创建一个新的配置键
	key := configKey{config.ProjectID, config.ProviderType}

	// 保存配置
	pm.configs[key] = config

	// 如果项目ID为"default"，设置为该类型的默认
	if config.ProjectID == "default" {
		pm.defaultProjs[config.ProviderType] = "default"
	}

	// 初始化提供商
	provider, err := factory(config)
	if err != nil {
		return fmt.Errorf("failed to initialize provider: %w", err)
	}

	// 保存提供商实例
	pm.providers[key] = provider
	return nil
}

// SetDefaultProject 设置特定类型的默认项目
func (pm *ProviderManager) SetDefaultProject(providerType, projectID string) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	pm.defaultProjs[providerType] = projectID
}

// GetProvider 获取提供商实例
func (pm *ProviderManager) GetProvider(projectID, providerType string) (types.LoginProvider, error) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	// 尝试获取特定项目和类型的提供商
	key := configKey{projectID, providerType}
	if provider, ok := pm.providers[key]; ok {
		return provider, nil
	}

	// 如果未找到，尝试使用默认项目
	if defaultProj, ok := pm.defaultProjs[providerType]; ok {
		key = configKey{defaultProj, providerType}
		if provider, ok := pm.providers[key]; ok {
			return provider, nil
		}
	}

	return nil, fmt.Errorf("provider not found for project: %s, type: %s", projectID, providerType)
}

// GetProviderConfig 获取提供商配置
func (pm *ProviderManager) GetProviderConfig(projectID, providerType string) (types.ProviderConfig, error) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	// 尝试获取特定项目和类型的配置
	key := configKey{projectID, providerType}
	if config, ok := pm.configs[key]; ok {
		return config, nil
	}

	// 如果未找到，尝试使用默认项目
	if defaultProj, ok := pm.defaultProjs[providerType]; ok {
		key = configKey{defaultProj, providerType}
		if config, ok := pm.configs[key]; ok {
			return config, nil
		}
	}

	return types.ProviderConfig{}, fmt.Errorf("provider config not found for project: %s, type: %s", projectID, providerType)
}

// LoadConfigsFromStore 从配置存储中加载配置
func (m *ProviderManager) LoadConfigsFromStore(ctx context.Context, store ConfigStore) error {
	configs, err := store.ListConfigs(ctx)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to load login provider configs", zap.Error(err))
		return err
	}

	for _, config := range configs {
		if err := m.RegisterConfig(config); err != nil {
			zlog.LogWithContext(ctx).Error("Failed to register login provider config",
				zap.String("project_id", config.ProjectID),
				zap.String("provider_type", config.ProviderType),
				zap.Error(err))
			return err
		}
	}

	zlog.LogWithContext(ctx).Info("Loaded login provider configs", zap.Int("count", len(configs)))

	return nil
}

// ConfigStore 配置存储接口
type ConfigStore interface {
	// ListConfigs 列出所有配置
	ListConfigs(ctx context.Context) ([]types.ProviderConfig, error)

	// GetConfig 获取配置
	GetConfig(ctx context.Context, projectID, providerType string) (*types.ProviderConfig, error)

	// SaveConfig 保存配置
	SaveConfig(ctx context.Context, config types.ProviderConfig) error

	// DeleteConfig 删除配置
	DeleteConfig(ctx context.Context, projectID, providerType string) error
}

// InMemoryConfigStore 内存配置存储
type InMemoryConfigStore struct {
	configs []types.ProviderConfig
	mu      sync.RWMutex
}

// NewInMemoryConfigStore 创建内存配置存储
func NewInMemoryConfigStore() *InMemoryConfigStore {
	return &InMemoryConfigStore{
		configs: make([]types.ProviderConfig, 0),
	}
}

// ListConfigs 列出所有配置
func (s *InMemoryConfigStore) ListConfigs(ctx context.Context) ([]types.ProviderConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.ProviderConfig, len(s.configs))
	copy(result, s.configs)

	return result, nil
}

// GetConfig 获取配置
func (s *InMemoryConfigStore) GetConfig(ctx context.Context, projectID, providerType string) (*types.ProviderConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, config := range s.configs {
		if config.ProjectID == projectID && config.ProviderType == providerType {
			return &config, nil
		}
	}

	return nil, errors.New("provider config not found")
}

// SaveConfig 保存配置
func (s *InMemoryConfigStore) SaveConfig(ctx context.Context, config types.ProviderConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, c := range s.configs {
		if c.ProjectID == config.ProjectID && c.ProviderType == config.ProviderType {
			s.configs[i] = config
			return nil
		}
	}

	s.configs = append(s.configs, config)
	return nil
}

// DeleteConfig 删除配置
func (s *InMemoryConfigStore) DeleteConfig(ctx context.Context, projectID, providerType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, c := range s.configs {
		if c.ProjectID == projectID && c.ProviderType == providerType {
			s.configs = append(s.configs[:i], s.configs[i+1:]...)
			return nil
		}
	}

	return errors.New("provider config not found")
}
