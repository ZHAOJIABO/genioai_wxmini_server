package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type ConfigService struct{}

var Config ConfigService

func NewConfigService() *ConfigService {
	return &ConfigService{}
}

func (*ConfigService) GetConfValue(key string) (string, error) {
	cdao := dao.NewConfigDao()
	confValue, err := cdao.GetConfigValue(key)
	if err != nil {
		zlog.Logger.Error("Get Config Error", zap.Error(err))
	}
	return confValue, err
}

func (*ConfigService) SetConfValue(key string, value string) error {
	cdao := dao.NewConfigDao()
	return cdao.SetConfigValue(key, value)
}

func (*ConfigService) GetFeedBackConfig() (model.FeecBackConfig, error) {
	cdao := dao.NewConfigDao()
	var conf model.FeecBackConfig
	feedbackConfigStr, err := cdao.GetConfigValue(constants.ConfigKeyFeedback)
	if err != nil {
		zlog.Logger.Error("Get Feed Back Config Error", zap.Error(err))
		return conf, err
	}
	err = json.Unmarshal([]byte(feedbackConfigStr), &conf)
	return conf, err
}

func (s *ConfigService) GetPopAdsConfig(dst interface{}) error {
	return s.GetJSONConfig(constants.ConfigKeyPopAdsConfig, dst)
}

func (s *ConfigService) GetPopSubscribeConfig(dst interface{}) error {
	return s.GetJSONConfig(constants.ConfigKeyPopSubscribeConfig, dst)
}

func (s *ConfigService) GetAppConfiguration(ctx context.Context, projectID, os string) (*vai.Configuration, error) {
	config := &vai.Configuration{}

	// 平台标识字符串标准化处理（可选）
	platform := strings.ToLower(os)

	// 获取最新版本号
	latestVersionKey := constants.GetProjectConfig(projectID, platform, constants.BaseConfigKeyLatestVersion)
	latestVersion, err := s.GetInt32Config(latestVersionKey)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Get App Configuration Error", zap.Error(err))
	}
	config.LatestVersion = latestVersion

	// 获取更新文案
	updateTextKey := constants.GetProjectConfig(projectID, platform, constants.BaseConfigKeyUpdateText)
	updateText, err := s.GetStringConfig(updateTextKey)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Get App Configuration Error", zap.Error(err))
	}
	config.UpdateText = updateText

	// 获取是否强制更新
	forceUpdateKey := constants.GetProjectConfig(projectID, platform, constants.BaseConfigKeyForceUpdate)
	forceUpdate, err := s.GetBoolConfig(forceUpdateKey)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Get App Configuration Error", zap.Error(err))
	}
	config.ForceUpdate = forceUpdate
	// 获取全局load sdk 开关
	globalLoadSdkEnabledKey := constants.GetProjectConfig(projectID, platform, constants.BaseConfigKeyGlobalLoadSdkEnabled)
	globalLoadSdkEnabled, err := s.GetBoolConfig(globalLoadSdkEnabledKey)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Get App Configuration Error", zap.Error(err))
	}
	config.GlobalLoadSdkEnabled = globalLoadSdkEnabled

	return config, nil
}

func (s *ConfigService) ShouldPopSubscribe(ctx context.Context, userID string) (bool, error) {
	popConfig, err := s.getPopConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("获取弹窗配置失败: %w", err)
	}
	if popConfig.MaxPopCount <= 0 {
		return false, nil
	}
	key := constants.RedisKeyUserPopSubscribePage + ":" + userID
	rdb := db.GetRedis()

	script := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local windowSeconds = tonumber(ARGV[2])
		local maxCount = tonumber(ARGV[3])
	
		local windowStart = now - windowSeconds
		redis.call('ZREMRANGEBYSCORE', key, '-inf', windowStart)
		
		local count = redis.call('ZCARD', key)
		
		if count < maxCount then
			redis.call('ZADD', key, now, tostring(now))
			redis.call('EXPIRE', key, windowSeconds)
			return {1, count + 1}
		end
		
		return {0, count}
	`

	windowSeconds := int64(popConfig.DayInterval * 24 * 3600)
	now := time.Now().Unix()
	result, err := rdb.Eval(script, []string{key},
		now,
		windowSeconds,
		popConfig.MaxPopCount,
	).Result()

	if err != nil {
		return false, fmt.Errorf("Redis操作失败: %w", err)
	}
	resultArray, ok := result.([]interface{})
	if !ok || len(resultArray) != 2 {
		return false, fmt.Errorf("unexpected result format: %v", result)
	}
	shouldPop := resultArray[0].(int64) == 1
	return shouldPop, nil
}

// popConfig 存储弹窗相关的配置参数
type popConfig struct {
	MaxPopCount int `json:"pop_subscribe_page_count"`
	DayInterval int `json:"pop_subscribe_interval"`
}

// getPopConfig 获取并验证弹窗配置
func (s *ConfigService) getPopConfig(ctx context.Context) (*popConfig, error) {
	var config popConfig
	if err := s.GetPopSubscribeConfig(&config); err != nil {
		return nil, err
	}

	if config.DayInterval <= 0 {
		return nil, fmt.Errorf(
			"配置参数无效: max_count=%d, day_interval=%d",
			config.MaxPopCount,
			config.DayInterval,
		)
	}

	return &config, nil
}

// GetStringConfig 获取字符串类型配置
func (s *ConfigService) GetStringConfig(key string) (string, error) {
	value, err := s.GetConfValue(key)
	if err != nil {
		return "", fmt.Errorf("获取配置[%s]失败: %w", key, err)
	}
	return value, nil
}

// GetIntConfig 获取整数类型配置
func (s *ConfigService) GetIntConfig(key string) (int, error) {
	value, err := s.GetConfValue(key)
	if err != nil {
		return 0, fmt.Errorf("获取配置[%s]失败: %w", key, err)
	}
	if value == "" {
		return 0, nil
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("配置[%s]值[%s]转换为整数失败: %w", key, value, err)
	}
	return intValue, nil
}

// GetInt32Config 获取int32类型配置
func (s *ConfigService) GetInt32Config(key string) (int32, error) {
	value, err := s.GetConfValue(key)
	if err != nil {
		return 0, fmt.Errorf("获取配置[%s]失败: %w", key, err)
	}
	if value == "" {
		return 0, nil
	}
	// 直接解析为int32，避免溢出
	intValue, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("配置[%s]值[%s]转换为整数失败: %w", key, value, err)
	}
	return int32(intValue), nil
}

// GetBoolConfig 获取布尔类型配置
func (s *ConfigService) GetBoolConfig(key string) (bool, error) {
	value, err := s.GetConfValue(key)
	if err != nil {
		return false, fmt.Errorf("获取配置[%s]失败: %w", key, err)
	}
	if value == "" {
		return false, nil
	}
	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("配置[%s]值[%s]转换为布尔值失败: %w", key, value, err)
	}
	return boolValue, nil
}

// GetJSONConfig 获取JSON类型配置并解析到目标结构
func (s *ConfigService) GetJSONConfig(key string, dst interface{}) error {
	// 检查dst是否为nil
	if dst == nil {
		return fmt.Errorf("配置[%s]目标对象不能为nil", key)
	}

	// 检查dst是否为指针类型
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("配置[%s]目标对象必须是非nil指针", key)
	}

	// 获取配置值
	value, err := s.GetConfValue(key)
	if err != nil {
		return fmt.Errorf("获取配置[%s]失败: %w", key, err)
	}

	// 处理空值情况
	if value == "" {
		// 可以选择设置一个明确的标志或日志
		zlog.Logger.Debug("配置值为空", zap.String("key", key))
		return nil
	}

	// JSON反序列化
	if err := json.Unmarshal([]byte(value), dst); err != nil {
		// 增强错误信息，但避免在错误中包含过长的JSON
		valuePreview := value
		if len(value) > 100 {
			valuePreview = value[:97] + "..."
		}
		return fmt.Errorf("配置[%s]值解析JSON失败: %w, 值预览: %s", key, err, valuePreview)
	}

	return nil
}

// GetEmotionalStateList 获取情感状态列表
func (s *ConfigService) GetEmotionalStateList(ctx context.Context, language string) ([]*vai.EmotionalState, error) {
	var emotionalStateList []*vai.EmotionalState
	key := constants.GetConfigKeyWithLanguage(language, constants.BaseConfigKeyEmotionalStateList)
	err := s.GetJSONConfig(key, &emotionalStateList)

	if err != nil {
		zlog.LogWithContext(ctx).Error("Get Emotional State List Error", zap.Error(err))
		return emotionalStateList, err
	}
	return emotionalStateList, nil
}

// GetRelationList 获取关系列表
func (s *ConfigService) GetRelationList(ctx context.Context, language string) ([]*vai.Relation, error) {
	var relationList []*vai.Relation
	key := constants.GetConfigKeyWithLanguage(language, constants.BaseConfigKeyRelationList)
	err := s.GetJSONConfig(key, &relationList)

	if err != nil {
		// 降级到EN
		key = constants.GetConfigKeyWithLanguage(constants.EN, constants.BaseConfigKeyRelationList)
		err = s.GetJSONConfig(key, &relationList)
		if err != nil {
			zlog.LogWithContext(ctx).Error("Get Relation List Error", zap.Error(err))
			return relationList, err
		}
		zlog.LogWithContext(ctx).Error("Get Relation List Error", zap.Error(err))
		return relationList, err
	}
	return relationList, nil
}

// CreationRestrictionRules defines the structure for configurable content creation restrictions
type CreationRestrictionRules struct {
	AllowedTypes      []string `json:"allowed_types"`      // e.g., ["IMAGE", "VIDEO"]
	AllowedCategories []string `json:"allowed_categories"` // e.g., ["PORTRAIT", "PET", "PRANK"]
	AllowedChannels   []string `json:"allowed_channels"`   // e.g., ["gpt4o", "comfyui", "kling", "minimax"]
	AllowedKindIDs    []string `json:"allowed_kind_ids"`   // e.g., ["person_kind_id", "pet_kind_id"]
}

// GetCreationRestrictionRules retrieves and parses the creation restriction rules from configuration
func (s *ConfigService) GetCreationRestrictionRules(ctx context.Context) (*CreationRestrictionRules, error) {
	var rules CreationRestrictionRules
	err := s.GetJSONConfig(constants.ConfigKeyCreationRestrictionRules, &rules)
	if err != nil {
		// If configuration is missing or invalid, return nil to indicate no restrictions
		zlog.LogWithContext(ctx).Warn("Creation restriction rules not found or invalid, no restrictions will be applied", zap.Error(err))
		return nil, nil
	}
	return &rules, nil
}

// PiclibAppConfig piclib 应用全局配置（对应数据库 piclib_app_config）
// 包含 proto 定义的字段 + 额外的服务端字段
type PiclibAppConfig struct {
	// proto 定义的字段（用于客户端）
	BlindBoxBackgroundUrl string `json:"blind_box_background_url"`
	BlindBoxTitle         string `json:"blind_box_title"`
	BlindBoxSubtitle      string `json:"blind_box_subtitle"`
	Theme                 int32  `json:"theme"`
	HomeIcon              string `json:"home_icon"`
	SupportEmail          string `json:"support_email"`

	// 额外字段（服务端使用）
	DefaultFloatToolBarToolId string `json:"default_float_tool_bar_tool_id"`
}

// ToPiclibConfig 转换为 proto 类型（给客户端）
func (c *PiclibAppConfig) ToPiclibConfig() *vai.PiclibConfig {
	return &vai.PiclibConfig{
		BlindBoxBackgroundUrl: c.BlindBoxBackgroundUrl,
		BlindBoxTitle:         c.BlindBoxTitle,
		BlindBoxSubtitle:      c.BlindBoxSubtitle,
		Theme:                 vai.Theme(c.Theme),
		HomeIcon:              c.HomeIcon,
		SupportEmail:          c.SupportEmail,
	}
}

// GetPiclibAppConfig 获取 piclib 应用全局配置（内部使用）
func (s *ConfigService) GetPiclibAppConfig(ctx context.Context) *PiclibAppConfig {
	var config PiclibAppConfig
	if err := s.GetJSONConfig(constants.ConfigKeyPiclibAppConfig, &config); err != nil {
		zlog.LogWithContext(ctx).Error("Get Piclib App Config Error", zap.Error(err))
	}
	return &config
}

// GetPiclibConfigration 获取 piclib 配置（返回 proto 类型给客户端）
func (s *ConfigService) GetPiclibConfigration(ctx context.Context) *vai.PiclibConfig {
	return s.GetPiclibAppConfig(ctx).ToPiclibConfig()
}

// GetUserConfig 获取用户配置（包括首页弹窗逻辑）
func (s *ConfigService) GetUserConfig(ctx context.Context, projectID, userID string, subscribeInfo *vai.SubscribeInfo) *vai.UserConfig {
	userConfig := &vai.UserConfig{}

	// 处理首页弹窗逻辑
	s.processHomePopupForUserConfig(ctx, projectID, userID, userConfig, subscribeInfo)

	// 获取悬浮工具栏默认工具ID（通过统一入口）
	piclibAppConfig := s.GetPiclibAppConfig(ctx)
	userConfig.FloatToolBarToolId = piclibAppConfig.DefaultFloatToolBarToolId

	return userConfig
}

// processHomePopupForUserConfig 处理首页弹窗配置
func (s *ConfigService) processHomePopupForUserConfig(ctx context.Context, projectID, userID string, userConfig *vai.UserConfig, subscribeInfo *vai.SubscribeInfo) {
	// 1. 检查弹窗功能是否开启
	enabled, err := s.GetBoolConfig(constants.ConfigKeyPiclibHomePopEnabled)
	if err != nil || !enabled {
		userConfig.ShouldShow = false
		return
	}

	// 2. 如果用户未登录（userID为空），不显示弹窗
	if userID == "" {
		userConfig.ShouldShow = false
		return
	}

	// 3. 检查用户是否有过付费历史
	hasPaid := false
	if subscribeInfo != nil {
		status := subscribeInfo.GetStatus()
		if status == vai.SubscribeStatus_SubscribeActivate || status == vai.SubscribeStatus_SubscribeExpired {
			hasPaid = true
		}
	}

	// 4. 只有从未订阅过的用户才显示弹窗（有订阅历史的不弹）
	if hasPaid {
		userConfig.ShouldShow = false
		zlog.LogWithContext(ctx).Debug("User has subscription history, skip popup",
			zap.String("user_id", userID))
		return
	}

	// 5. 获取弹窗详细信息
	var popupInfo vai.PopupInfo
	if err := s.GetJSONConfig(constants.ConfigKeyPiclibHomePopInfo, &popupInfo); err != nil {
		zlog.LogWithContext(ctx).Error("Get home popup info error", zap.Error(err))
		userConfig.ShouldShow = false
		return
	}

	// 6. 设置弹窗信息（仅对从未订阅的用户）
	userConfig.ShouldShow = true
	userConfig.HomePopup = &popupInfo

	zlog.LogWithContext(ctx).Info("Show home popup for new user",
		zap.String("user_id", userID))
}

// ToolRecommendConfig 工具推荐配置
type ToolRecommendConfig struct {
	// ToolRecommendFirstToolID 固定推荐的第一个工具ID
	// 如果为空或工具不可用,则随机选择
	ToolRecommendFirstToolID string `json:"tool_recommend_first_tool_id"`

	// BanToolIDs 禁止推荐的工具ID列表
	// 这些工具不会出现在推荐结果中
	BanToolIDs []string `json:"ban_tool_id"`
}

// GetToolRecommendConfig 获取工具推荐配置
// 配置读取失败时返回默认配置,确保服务降级可用
func (s *ConfigService) GetToolRecommendConfig(ctx context.Context) (*ToolRecommendConfig, error) {
	var cfg ToolRecommendConfig
	if err := s.GetJSONConfig(constants.ConfigKeyRecommendToolConfig, &cfg); err != nil {
		zlog.LogWithContext(ctx).Error("Failed to get tool recommend config, using defaults",
			zap.Error(err),
			zap.String("config_key", constants.ConfigKeyRecommendToolConfig))
		// 返回默认配置而不是 nil,支持降级
		return &ToolRecommendConfig{
			ToolRecommendFirstToolID: "",
			BanToolIDs:               []string{},
		}, err
	}
	return &cfg, nil
}
