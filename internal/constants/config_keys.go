package constants

import (
	"fmt"
)

// 平台类型
type Platform string

const (
	PlatformAndroid Platform = "android"
	PlatformIOS     Platform = "ios"
	PlatformWeb     Platform = "web"
	// 未来可以添加更多平台
)

// 平台类型对应的整数值
const (
	PlatformTypeAndroid = 1
	PlatformTypeIOS     = 2
	PlatformTypeWeb     = 3
)

// 根据整数值获取平台类型
func GetPlatformByType(platformType int) (Platform, error) {
	switch platformType {
	case PlatformTypeAndroid:
		return PlatformAndroid, nil
	case PlatformTypeIOS:
		return PlatformIOS, nil
	case PlatformTypeWeb:
		return PlatformWeb, nil
	default:
		return "", fmt.Errorf("不支持的平台类型: %d", platformType)
	}
}

// GetPlatformByString 根据字符串获取平台类型
func GetPlatformByString(platformStr string) (Platform, error) {
	switch platformStr {
	case "android", "Android", "ANDROID":
		return PlatformAndroid, nil
	case "ios", "iOS", "IOS":
		return PlatformIOS, nil
	case "web", "Web", "WEB":
		return PlatformWeb, nil
	default:
		return "", fmt.Errorf("不支持的平台类型: %s", platformStr)
	}
}

// 应用配置相关键
const (
	// 版本相关基础键
	BaseConfigKeyLatestVersion        = "latest_version"          // 最新版本号
	BaseConfigKeyUpdateText           = "update_text"             // 更新文案
	BaseConfigKeyForceUpdate          = "force_update"            // 是否强制更新
	BaseConfigKeyGlobalLoadSdkEnabled = "global_load_sdk_enabled" // 全局load sdk 开关
	// 情感状态列表
	BaseConfigKeyEmotionalStateList = "emotional_state_list" // 情感状态列表
	BaseConfigKeyRelationList       = "relation_list"        // 关系列表

	// 弹窗相关
	ConfigKeyPopAdsConfig       = "pop_ads_config"       // 广告弹窗配置
	ConfigKeyPopSubscribeConfig = "pop_subscribe_config" // 订阅弹窗配置

	// 反馈相关
	ConfigKeyFeedback = "feedback_config" // 反馈配置

	// 用户文档聊天相关配置键
	ConfigKeyPersonalDocChatPromptTemplate = "personal_doc_chat_prompt_template"
	ConfigKeyPersonalDocChatModel          = "personal_doc_chat_model"

	// 图生视频相关配置
	ConfigKeyKlingImage2VideoConfig = "kling_image2video_config"
	ConfigKeyKlingEffectsConfig     = "kling_effects_config"

	// Minimax 视频生成相关配置
	ConfigKeyMinimaxText2VideoConfig    = "minimax_text2video_config"
	ConfigKeyMinimaxImage2VideoConfig   = "minimax_image2video_config"
	ConfigKeyMinimaxTemplateVideoConfig = "minimax_template_video_config"

	// 商品实验相关配置
	BaseConfigKeyProductExperiments = "product_experiments" // 商品A/B测试实验配置
	// 商品配置（可扩展），包含优惠图标等
	ConfigKeyProductConfig = "product_config"

	// 图片锻造相关配置
	BaseConfigKeyBuiltInPictures = "built_in_pictures" // 内置图片配置
	BaseConfigKeyThemeMetadata   = "theme_metadata"    // 主题元数据配置
	// 工具推荐配置
	ConfigKeyRecommendToolConfig = "recommend_tool_config"
	// ComfyUI 相关配置
	ConfigKeyComfyUIObjectInfo = "comfyui_object_info" // ComfyUI object_info 缓存

	// 盲盒创作限制规则配置
	ConfigKeyCreationRestrictionRules = "creation_restriction_rules"

	// 图片引导生成视频相关配置
	ConfigKeyPictureToVideoGuideWorkflowID = "picture_to_video_guide_workflow_id"

	// 默认图片转视频工具ID配置
	ConfigKeyDefaultI2VToolID = "default_i2v_tool_id"

	// Prompt优化相关配置
	ConfigKeyPromptOptimizationInfo          = "prompt_optimization_info"
	ConfigKeyPromptOptimizationDefaultModel  = "prompt_optimization_default_model"
	ConfigKeyPromptOptimizationDefaultPrompt = "prompt_optimization_default_prompt"

	// 任务并发控制配置（新配置键）
	ConfigKeyTaskConcurrencyLimits = "task_concurrency_limits"

	// 工作流默认额外参数配置
	ConfigKeyDefaultWorkflowExtraParams = "default_workflow_extra_params"

	// WANX 相关配置
	ConfigKeyWANX22ResolutionMap = "wanx2_2_resolution_map" // WANX2.2分辨率映射配置

	// 订阅退出流程A/B测试配置
	BaseConfigKeySubscriptionExitFlow = "subscription_exit_flow" // 订阅退出流程A/B测试配置

	// 工作流分类避审机制配置
	ConfigKeyWorkflowKindAuditFilter = "workflow_kind_audit_filter"

	// piclib 应用配置
	ConfigKeyPiclibAppConfig = "piclib_app_config"

	// Cloud Comfy workflow replacement (GOOGLENICE)
	ConfigKeyCloudComfyVideoReplaceBy             = "cloud_comfy_video_replace_by"
	ConfigKeyCloudComfyMultiImageToVideoReplaceBy = "cloud_comfy_multi_image_to_video_replace_by"
	ConfigKeyCloudComfyImageToImageReplaceBy      = "cloud_comfy_image_to_image_replace_by"

	//首页弹窗相关配置
	ConfigKeyPiclibHomePopEnabled = "home_pop.enabled"
	ConfigKeyPiclibHomePopInfo    = "home_pop.info"

	// 任务完成推送通知配置
	ConfigKeyTaskPushEnabled = "task_push_enabled" // 任务完成推送开关
	ConfigKeyTaskPushTitle   = "task_push_title"   // 任务完成通知标题
	ConfigKeyTaskPushBody    = "task_push_body"    // 任务完成通知内容

	// 订阅优惠推送配置（全局，用于向未订阅用户定时推送）
	ConfigKeySubscriptionPromoPushEnabled = "subscription_promo_push_enabled" // 订阅优惠推送开关
	ConfigKeySubscriptionPromoPushTitle   = "subscription_promo_push_title"   // 订阅优惠推送标题
	ConfigKeySubscriptionPromoPushBody    = "subscription_promo_push_body"    // 订阅优惠推送内容
	ConfigKeySubscriptionPromoPushTime    = "subscription_promo_push_time"    // 推送时间 (如 "10:30")
)

// GetConfigKey 根据平台和基础键构建完整的配置键
// 例如：GetConfigKey("ios", BaseConfigKeyLatestVersion) 返回 "ios_latest_version"
func GetConfigKey(platform string, baseKey string) string {
	return platform + "_" + baseKey
}

// getProjectConfig 获取项目配置
func GetProjectConfig(project, platform, baseKey string) string {
	return project + "_" + platform + "_" + baseKey
}

// 多语言获取构建完整配置键
func GetConfigKeyWithLanguage(language, baseKey string) string {
	return language + "_" + baseKey
}

// 为了向后兼容，保留一些常用的直接常量
const (
	ConfigKeyLatestVersionAndroid = "android_latest_version"
	ConfigKeyUpdateTextAndroid    = "android_update_text"
	ConfigKeyForceUpdateAndroid   = "android_force_update"

	ConfigKeyLatestVersionIOS = "ios_latest_version"
	ConfigKeyUpdateTextIOS    = "ios_update_text"
	ConfigKeyForceUpdateIOS   = "ios_force_update"

	ConfigKeyAstroLLMModel = "astro_llm_model"
)

// GoCalConfigKeyReviewingVersion 大健康相关ConfigKey常量
const GoCalConfigKeyReviewingVersion = ProjectIdDietAI + "_ios_reviewing_version"
const GoCalConfigKeyTrialFilter = ProjectIdDietAI + "_ios_trial_filter_config"
const SolaceConfigKeyReviewingVersion = ProjectIdSolacex + "_ios_reviewing_version"
