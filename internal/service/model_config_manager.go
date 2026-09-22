package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"va_visionai_server/internal/zlog"

	"go.uber.org/zap"
)

// ModelMetadata 模型元数据
type ModelMetadata struct {
	ModelName      string  `json:"model_name"`       // 模型名称
	DisplayName    string  `json:"display_name"`     // 显示名称
	Provider       string  `json:"provider"`         // 执行器（comfyui/kling/gpt4o等）
	ApiIden        string  `json:"api_iden"`         // API标识
	EffectScene    string  `json:"effect_scene"`     // 效果场景
	TaskType       string  `json:"task_type"`        // 任务类型
	CreditPoints   int     `json:"credit_points"`    // 默认积分消耗
	DefaultWidth   int     `json:"default_width"`    // 默认宽度
	DefaultHeight  int     `json:"default_height"`   // 默认高度
	DefaultSteps   int     `json:"default_steps"`    // 默认推理步数
	DefaultCfgScale float32 `json:"default_cfg_scale"` // 默认CFG scale
	SupportT2I     bool    `json:"support_t2i"`      // 支持文生图
	SupportI2I     bool    `json:"support_i2i"`      // 支持图生图
	MaxImages      int     `json:"max_images"`       // 最大生成图片数
	MaxInputImages int     `json:"max_input_images"` // 最大输入图片数
	SupportQualities []string `json:"support_qualities"` // 支持的质量选项
	Description    string  `json:"description"`      // 模型描述
	Enabled        bool    `json:"enabled"`          // 是否启用
}

// ModelConfigManager 模型配置管理器
type ModelConfigManager struct {
	mu            sync.RWMutex
	configs       map[string]*ModelMetadata
	configFilePath string
}

// NewModelConfigManager 创建模型配置管理器
func NewModelConfigManager(configFilePath string) (*ModelConfigManager, error) {
	manager := &ModelConfigManager{
		configs:        make(map[string]*ModelMetadata),
		configFilePath: configFilePath,
	}

	// 如果配置文件存在，加载配置
	if _, err := os.Stat(configFilePath); err == nil {
		if err := manager.LoadConfig(); err != nil {
			return nil, fmt.Errorf("failed to load model config: %w", err)
		}
	} else {
		// 配置文件不存在，使用默认配置
		manager.initDefaultConfig()
	}

	return manager, nil
}

// LoadConfig 从文件加载配置
func (m *ModelConfigManager) LoadConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.configFilePath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var configs []*ModelMetadata
	if err := json.Unmarshal(data, &configs); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 重建map
	m.configs = make(map[string]*ModelMetadata)
	for _, config := range configs {
		if config.Enabled {
			m.configs[config.ModelName] = config
		}
	}

	return nil
}

// initDefaultConfig 初始化默认配置
func (m *ModelConfigManager) initDefaultConfig() {
	m.configs = map[string]*ModelMetadata{
		"flux-dev": {
			ModelName:       "flux-dev",
			DisplayName:     "Flux Dev",
			Provider:        "comfyui",
			ApiIden:         "flux_dev",
			EffectScene:     "general",
			TaskType:        "text2image",
			CreditPoints:    10,
			DefaultWidth:    1024,
			DefaultHeight:   1024,
			DefaultSteps:    28,
			DefaultCfgScale: 3.5,
			SupportT2I:      true,
			SupportI2I:      true,
			MaxImages:       4,
			SupportQualities: []string{"standard", "hd"},
			Description:     "Flux Dev 模型，适合高质量图片生成",
			Enabled:         true,
		},
		"flux-schnell": {
			ModelName:       "flux-schnell",
			DisplayName:     "Flux Schnell",
			Provider:        "comfyui",
			ApiIden:         "flux_schnell",
			EffectScene:     "general",
			TaskType:        "text2image",
			CreditPoints:    5,
			DefaultWidth:    1024,
			DefaultHeight:   1024,
			DefaultSteps:    4,
			DefaultCfgScale: 1.0,
			SupportT2I:      true,
			SupportI2I:      true,
			MaxImages:       4,
			SupportQualities: []string{"standard"},
			Description:     "Flux Schnell 模型，快速生成",
			Enabled:         true,
		},
		"sd3": {
			ModelName:       "sd3",
			DisplayName:     "Stable Diffusion 3",
			Provider:        "comfyui",
			ApiIden:         "sd3",
			EffectScene:     "general",
			TaskType:        "text2image",
			CreditPoints:    8,
			DefaultWidth:    1024,
			DefaultHeight:   1024,
			DefaultSteps:    28,
			DefaultCfgScale: 7.0,
			SupportT2I:      true,
			SupportI2I:      true,
			MaxImages:       4,
			SupportQualities: []string{"standard", "hd"},
			Description:     "Stable Diffusion 3 模型",
			Enabled:         true,
		},
		"wanx": {
			ModelName:       "wanx",
			DisplayName:     "通义万相",
			Provider:        "comfyui",
			ApiIden:         "wanx_v1",
			EffectScene:     "general",
			TaskType:        "text2image",
			CreditPoints:    6,
			DefaultWidth:    1024,
			DefaultHeight:   1024,
			DefaultSteps:    20,
			DefaultCfgScale: 5.0,
			SupportT2I:      true,
			SupportI2I:      false,
			MaxImages:       1,
			SupportQualities: []string{"standard"},
			Description:     "阿里通义万相模型",
			Enabled:         true,
		},
	}
}

// GetModelMetadata 获取模型元数据
func (m *ModelConfigManager) GetModelMetadata(modelName string) (*ModelMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	config, ok := m.configs[modelName]
	if !ok {
		return nil, fmt.Errorf("model not found or disabled: %s", modelName)
	}

	// 返回副本，避免外部修改
	configCopy := *config
	return &configCopy, nil
}

// ListAvailableModels 列出所有可用模型
func (m *ModelConfigManager) ListAvailableModels() []*ModelMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()

	models := make([]*ModelMetadata, 0, len(m.configs))
	for _, config := range m.configs {
		configCopy := *config
		models = append(models, &configCopy)
	}

	return models
}

// ValidateModelSupport 验证模型是否支持指定的操作
func (m *ModelConfigManager) ValidateModelSupport(ctx context.Context, modelName string, hasInputImages bool) error {
	metadata, err := m.GetModelMetadata(modelName)
	if err != nil {
		return err
	}

	if hasInputImages {
		if !metadata.SupportI2I {
			return fmt.Errorf("model %s does not support image-to-image generation", modelName)
		}
	} else {
		if !metadata.SupportT2I {
			return fmt.Errorf("model %s does not support text-to-image generation", modelName)
		}
	}

	return nil
}

// GetDefaultParameters 获取模型的默认参数
func (m *ModelConfigManager) GetDefaultParameters(modelName string) (map[string]interface{}, error) {
	metadata, err := m.GetModelMetadata(modelName)
	if err != nil {
		return nil, err
	}

	params := map[string]interface{}{
		"width":     metadata.DefaultWidth,
		"height":    metadata.DefaultHeight,
		"steps":     metadata.DefaultSteps,
		"cfg_scale": metadata.DefaultCfgScale,
	}

	return params, nil
}

// ReloadConfig 重新加载配置（热更新）
func (m *ModelConfigManager) ReloadConfig(ctx context.Context) error {
	zlog.LogWithContext(ctx).Info("reloading model config")

	if err := m.LoadConfig(); err != nil {
		zlog.LogWithContext(ctx).Error("failed to reload model config", zap.Error(err))
		return err
	}

	zlog.LogWithContext(ctx).Info("model config reloaded successfully",
		zap.Int("model_count", len(m.configs)))
	return nil
}
