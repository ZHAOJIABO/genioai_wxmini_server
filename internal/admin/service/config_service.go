package service

import (
	"context"

	"gorm.io/gorm"

	"va_visionai_server/internal/model"
)

type ConfigService struct {
	db *gorm.DB
}

func NewConfigService(db *gorm.DB) *ConfigService {
	return &ConfigService{db: db}
}

type ConfigItem struct {
	ID         uint   `json:"id"`
	ConfigName string `json:"config_name"`
	Value      string `json:"value"`
	Comment    string `json:"comment"`
}

// ListConfigs 获取所有配置
func (s *ConfigService) ListConfigs(ctx context.Context) ([]ConfigItem, error) {
	var configs []model.Config
	err := s.db.Find(&configs).Error
	if err != nil {
		return nil, err
	}

	items := make([]ConfigItem, 0, len(configs))
	for _, c := range configs {
		items = append(items, ConfigItem{
			ID:         c.ID,
			ConfigName: c.ConfigName,
			Value:      c.Value,
			Comment:    c.Comment,
		})
	}
	return items, nil
}

// UpdateConfig 更新配置
func (s *ConfigService) UpdateConfig(ctx context.Context, key, value string) error {
	return s.db.Model(&model.Config{}).Where("config_name = ?", key).Update("value", value).Error
}
