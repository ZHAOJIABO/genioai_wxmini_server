package model

import vai "va_visionai_server/internal/va_interface"

// Experiment 表示一个 A/B 测试实验规则
// AB 测试的核心逻辑：
// 1. 根据用户特征（如 user_id 后缀）匹配实验规则
// 2. 按优先级选择最高优先级的匹配实验
// 3. 对商品列表应用实验策略（包含、排除、属性修改）
type Experiment struct {
	Name     string  `json:"name"`     // 实验名称，用于标识和日志记录
	Priority int     `json:"priority"` // 优先级，数字越大优先级越高
	Rule     Rule    `json:"rule"`     // 实验目标用户匹配规则
	Payload  Payload `json:"payload"`  // 实验策略配置
}

// Rule 定义实验的目标用户匹配规则
// 目前支持基于用户ID后缀的匹配，未来可扩展更多规则类型
type Rule struct {
	UserIDSuffixes []int `json:"user_id_suffix"` // 用户ID后缀匹配，例如 [0,1,2] 表示用户ID以0、1、2结尾的用户
}

// Payload 定义实验的具体策略
// 支持三种操作，按以下顺序执行：
// 1. Inclusions: 如果非空，则只保留指定的商品ID
// 2. Exclusions: 从商品列表中排除指定的商品ID
// 3. Modifications: 对指定商品的属性进行覆盖修改
type Payload struct {
	Inclusions    []string                `json:"inclusions"`    // 仅包含这些商品ID，为空则不限制
	Exclusions    []string                `json:"exclusions"`    // 排除这些商品ID
	Modifications map[string]Modification `json:"modifications"` // 商品属性修改，key为商品ID，value为要修改的属性
}

// Modification 定义对商品属性的具体修改
// key 为商品模型中的字段名（Go 结构体字段名），value 为新值
// 例如：{"Price": 29.99, "PriceLabel": "$29.99/mo"}
type Modification map[string]interface{}

// SubscriptionExitConfig 订阅退出流程配置
type SubscriptionExitConfig struct {
	ExitFlowType  vai.SubscriptionExitFlowType `json:"exit_flow_type"`
	ConfigDetails string                       `json:"config_details"`
}

// ExitFlowConfigDetails 退出流程配置详情
type ExitFlowConfigDetails struct {
	// 提示文案
	Message string `json:"message"`
}
