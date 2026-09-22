package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/subscribe"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

const (
	cacheTTL = 5 * time.Minute
)

type ProductService struct {
	productDao       *dao.ProductDao
	configService    *ConfigService
	redisCli         *redis.Client
	subscribeService *subscribe.Service
}

// OffTagIconConfig 配置商品优惠标识图标
type OffTagIconConfig struct {
	// 默认展示的优惠图标
	Default string `json:"default"`
	// 新用户优惠图标
	NewUser string `json:"new_user"`
}

// ProductConfig 商品配置集合，后续可扩展
type ProductConfig struct {
	OffTagIcon OffTagIconConfig `json:"off_tag_icon"`
}

func NewProductService() *ProductService {
	return &ProductService{
		productDao:       dao.NewProductDao(),
		configService:    NewConfigService(),
		redisCli:         db.GetRedis(),
		subscribeService: subscribe.NewService(),
	}
}

// NewProductServiceWithConfig 使用注入的 ConfigService 创建 ProductService
func NewProductServiceWithConfig(configService *ConfigService) *ProductService {
	return &ProductService{
		productDao:       dao.NewProductDao(),
		configService:    configService,
		redisCli:         db.GetRedis(),
		subscribeService: subscribe.NewService(),
	}
}

func (s *ProductService) GetProduct(ctx context.Context, productID, lang, os, projectID string) (*model.Product, error) {
	if productID == "" {
		return nil, constants.ERR_INVALID_PARAM
	}
	os = strings.ToLower(os)
	productInfo, err := s.productDao.GetByProductID(productID, lang, os, projectID)

	// 如果没有查询到数据且当前语言不是英语，降级为英语再查询一次
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) && lang != constants.EN {
		zlog.LogWithContext(ctx).Info("product not found for current language, fallback to en",
			zap.String("productID", productID),
			zap.String("lang", lang))
		productInfo, err = s.productDao.GetByProductID(productID, constants.EN, os, projectID)
		if err != nil {
			zlog.LogWithContext(ctx).Error("get product by productID with fallback lang 'en' failed",
				zap.String("productID", productID),
				zap.Error(err))
		}
	}

	return productInfo, err
}

// ListProducts 获取商品列表，支持基于场景和 A/B 测试的动态商品展示
// 核心流程：
// 1. 从上下文提取用户信息（userID, projectID, lang, os）
// 2. 根据 groupID 获取基础商品列表
// 3. 特殊处理：对于PicLib项目的groupID=1场景，根据会员级别过滤商品
// 4. 获取该项目的 A/B 测试实验配置
// 5. 匹配用户对应的实验规则
// 6. 应用实验策略（包含、排除、属性修改）
func (s *ProductService) ListProducts(ctx context.Context, userID, os string, groupID int, subscribeInfo *vai.SubscribeInfo, fetchAll bool, appVersion string) ([]*model.Product, error) {
	// 1. Extract info from context
	// userID := common.GetUserID(ctx)
	projectID := common.GetProjectID(ctx)
	lang := common.CtxGetStrValue(ctx, constants.CtxLang)
	// os := common.CtxGetStrValue(ctx, constants.CtxOSName)

	// 2. Get base products
	products, err := s.productDao.GetActiveProductsByGroup(groupID, lang, os, projectID, fetchAll)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		zlog.LogWithContext(ctx).Error("get active products by group failed", zap.Error(err), zap.Int("groupID", groupID))
		return nil, err
	}

	if len(products) == 0 && lang != constants.EN {
		zlog.LogWithContext(ctx).Info("no products found for current language, fallback to en", zap.String("lang", lang))
		products, err = s.productDao.GetActiveProductsByGroup(groupID, constants.EN, os, projectID, fetchAll)
		if err != nil {
			zlog.LogWithContext(ctx).Error("get active products by group with fallback lang 'en' failed", zap.Error(err), zap.Int("groupID", groupID))
			return nil, err
		}
	}

	// 3. 特殊处理：PicLib项目的groupID=1场景，根据会员级别过滤商品
	if projectID == constants.ProjectIdPicFlow && groupID == 1 && subscribeInfo != nil {
		products = s.filterProductsByMembershipLevel(ctx, products, subscribeInfo)
	}

	//4.特殊处理：goCal项目product筛选
	if projectID == constants.ProjectIdDietAI {
		products = s.GoCalFilterProducts(ctx, products, os, appVersion, userID)
	}
	//5.特殊处理：solace项目product筛选
	if projectID == constants.ProjectIdSolacex {
		products = s.filterSolaceReviewingProducts(ctx, products, os, appVersion)
	}
	// 如果fetchAll为true，则返回所有商品
	if fetchAll {
		return products, nil
	}

	// 4. Get experiment config
	experiments, err := s.getExperiments(ctx, projectID)
	if err != nil {
		// If config fails or no experiments found for this project, return base products
		zlog.LogWithContext(ctx).Info("get experiments failed or no experiments configured, returning base products",
			zap.Error(err), zap.String("projectID", projectID))
		return products, nil
	}
	if len(experiments) == 0 {
		return products, nil
	}

	// 5. Match experiment
	matchedExperiment := s.matchExperiment(ctx, userID, experiments)
	if matchedExperiment == nil {
		return products, nil // No experiment matched
	}

	zlog.LogWithContext(ctx).Info("user matched experiment", zap.String("userID", userID), zap.String("experiment", matchedExperiment.Name))

	// 6. Apply experiment payload
	finalProducts, err := s.applyExperiment(ctx, products, matchedExperiment.Payload)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to apply experiment, returning base products", zap.Error(err), zap.String("experiment", matchedExperiment.Name))
		return products, nil // Fail gracefully
	}

	return finalProducts, nil
}

// GetProductConfig 获取商品配置
func (s *ProductService) GetProductConfig(ctx context.Context) (ProductConfig, error) {
	var cfg ProductConfig
	if err := s.configService.GetJSONConfig(constants.ConfigKeyProductConfig, &cfg); err != nil {
		return ProductConfig{}, err
	}
	return cfg, nil
}

// resolveOffTagIcon 根据商品类型和优惠使用状态决定 icon
// productType: 商品类型
// cfg: icon 配置
// usedPromotion: 是否使用过优惠
// fallbackDefault: 状态获取失败时强制返回 default（仅影响订阅商品）
func resolveOffTagIcon(productType string, cfg OffTagIconConfig, usedPromotion bool, fallbackDefault bool) string {
	switch productType {
	case constants.ProductTypeOneTimeCharge:
		// 内购商品：始终展示 new_user，不受 fallbackDefault 影响
		if cfg.NewUser != "" {
			return cfg.NewUser
		}
		return cfg.Default

	case constants.ProductTypeSubscribe:
		// 订阅商品：状态未知时保守返回 default
		if fallbackDefault {
			return cfg.Default
		}
		// 已用优惠返回 default，未用返回 new_user
		if usedPromotion {
			return cfg.Default
		}
		if cfg.NewUser != "" {
			return cfg.NewUser
		}
		return cfg.Default

	default:
		return cfg.Default
	}
}

// getOffTagIconState 获取商品优惠标识所需状态
// 返回：配置、是否使用过优惠、错误
func (s *ProductService) getOffTagIconState(ctx context.Context, projectID, userID string) (OffTagIconConfig, bool, error) {
	cfg, err := s.GetProductConfig(ctx)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("get product_config failed", zap.Error(err))
		return OffTagIconConfig{}, false, err
	}

	// 无 userID/projectID 时视为未使用过优惠，但上层需通过 fallbackDefault 保守处理
	if userID == "" || projectID == "" {
		return cfg.OffTagIcon, false, nil
	}

	usedProducts, err := s.subscribeService.GetUsedPromotion(ctx, projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("get promotion history failed", zap.Error(err))
		return cfg.OffTagIcon, false, err
	}

	return cfg.OffTagIcon, len(usedProducts) > 0, nil
}

// BuildProductInfos 将商品列表转换为接口返回结构
func (s *ProductService) BuildProductInfos(ctx context.Context, products []*model.Product, projectID, userID string) []*vai.ProductInfo {
	cfg, usedPromotion, err := s.getOffTagIconState(ctx, projectID, userID)
	fallbackDefault := err != nil || userID == "" || projectID == ""

	productInfos := make([]*vai.ProductInfo, 0, len(products))
	for _, product := range products {
		if product == nil {
			continue
		}
		offTagIcon := resolveOffTagIcon(product.ProductType, cfg, usedPromotion, fallbackDefault)
		productInfos = append(productInfos, s.buildProductInfoWithIcon(product, offTagIcon))
	}
	return productInfos
}

// BuildProductInfo 构建单个 ProductInfo
func (s *ProductService) BuildProductInfo(ctx context.Context, product *model.Product, projectID, userID string) *vai.ProductInfo {
	cfg, usedPromotion, err := s.getOffTagIconState(ctx, projectID, userID)
	fallbackDefault := err != nil || userID == "" || projectID == ""
	offTagIcon := resolveOffTagIcon(product.ProductType, cfg, usedPromotion, fallbackDefault)
	return s.buildProductInfoWithIcon(product, offTagIcon)
}

// buildProductInfoWithIcon 内部方法：将 model.Product 转换为 vai.ProductInfo
func (s *ProductService) buildProductInfoWithIcon(product *model.Product, offTagIcon string) *vai.ProductInfo {
	purchaseType := vai.PurchaseType_PURCHASE_TYPE_UNKNOWN
	switch product.ProductType {
	case constants.ProductTypeSubscribe:
		purchaseType = vai.PurchaseType_PURCHASE_TYPE_SUBSCRIPTION
	case constants.ProductTypeOneTimeCharge:
		purchaseType = vai.PurchaseType_PURCHASE_TYPE_IN_APP
	}

	return &vai.ProductInfo{
		ProductId:              product.ProductID,
		ProductName:            product.Name,
		Description:            product.Description,
		Price:                  product.Price,
		PriceUnit:              product.PriceUnit,
		PriceLabel:             product.PriceLabel,
		PurchaseType:           purchaseType,
		Discount:               product.Discount,
		DiscountLabel:          product.DiscountLabel,
		AveragePrice:           product.AveragePrice,
		AveragePriceUnit:       product.AveragePriceUnit,
		AveragePriceLabel:      product.AveragePriceLabel,
		IsTrial:                product.IsTrial,
		SubscribeLevel:         int32(product.Level),
		MemberGrantCreditPoint: int32(product.Credits),
		Icon:                   product.Icon,
		ProductGroupId:         product.ProductGroupID,
		OffTagIcon:             offTagIcon,
	}
}

// filterProductsByMembershipLevel 根据用户会员级别过滤商品
// 业务逻辑：不展示小于等于当前会员级别的商品
// 例如：用户是20级会员，则不展示10级和20级的商品，只展示30级及以上的商品
func (s *ProductService) filterProductsByMembershipLevel(ctx context.Context, products []*model.Product, subscribeInfo *vai.SubscribeInfo) []*model.Product {
	userLevel := int(subscribeInfo.GetSubscribeLevel())

	// 如果用户没有订阅（级别为0），返回所有商品
	if userLevel == 0 {
		zlog.LogWithContext(ctx).Debug("user has no subscription, showing all products",
			zap.Int("userLevel", userLevel))
		return products
	}

	var filteredProducts []*model.Product
	excludedCount := 0

	for _, product := range products {
		// 只保留级别大于当前用户级别的商品
		if product.Level > userLevel {
			filteredProducts = append(filteredProducts, product)
		} else {
			excludedCount++
		}
	}

	zlog.LogWithContext(ctx).Info("filtered products by membership level",
		zap.Int("userLevel", userLevel),
		zap.Int("originalCount", len(products)),
		zap.Int("filteredCount", len(filteredProducts)),
		zap.Int("excludedCount", excludedCount))

	return filteredProducts
}

// getExperiments 获取指定项目的 A/B 测试实验配置
// 支持 Redis 缓存以提高性能，缓存时间为 5 分钟
func (s *ProductService) getExperiments(ctx context.Context, projectID string) ([]model.Experiment, error) {
	// Generate cache key based on project
	cacheKey := "cache:product_experiments:" + projectID

	// Try cache first
	cachedData, err := s.redisCli.Get(cacheKey).Result()
	if err == nil {
		var experiments []model.Experiment
		if err := json.Unmarshal([]byte(cachedData), &experiments); err == nil {
			zlog.LogWithContext(ctx).Debug("get experiments from cache hit", zap.String("projectID", projectID))
			return experiments, nil
		}
	}

	// Get config key based on project using GetConfigKey
	configKey := constants.GetConfigKey(projectID, constants.BaseConfigKeyProductExperiments)

	// Use ConfigService.GetJSONConfig to fetch and parse experiments
	var experiments []model.Experiment
	err = s.configService.GetJSONConfig(configKey, &experiments)
	if err != nil {
		// If no config found for this project, it means AB testing is not enabled
		zlog.LogWithContext(ctx).Info("no product experiments config found for project",
			zap.String("projectID", projectID), zap.String("configKey", configKey), zap.Error(err))
		return []model.Experiment{}, nil
	}

	// Update cache if we got valid experiments
	if len(experiments) > 0 {
		if experimentsData, err := json.Marshal(experiments); err == nil {
			if err := s.redisCli.Set(cacheKey, experimentsData, cacheTTL).Err(); err != nil {
				zlog.LogWithContext(ctx).Error("failed to set experiments to cache", zap.Error(err))
			}
		}
	}

	zlog.LogWithContext(ctx).Debug("get experiments from config service",
		zap.String("projectID", projectID), zap.Int("count", len(experiments)))
	return experiments, nil
}

// matchExperiment 匹配用户对应的实验规则
// 按优先级降序排序，返回第一个匹配的实验
// 如果用户同时命中多个实验，优先级高的实验生效
func (s *ProductService) matchExperiment(ctx context.Context, userID string, experiments []model.Experiment) *model.Experiment {
	// Sort by priority (descending)
	sort.Slice(experiments, func(i, j int) bool {
		return experiments[i].Priority > experiments[j].Priority
	})

	for i := range experiments {
		exp := &experiments[i]
		if s.checkRule(ctx, userID, exp.Rule) {
			return exp
		}
	}
	return nil
}

// checkRule 检查用户是否匹配实验规则
// 目前支持基于用户ID后缀的匹配规则
// 规则逻辑：提取用户ID的最后一位数字，检查是否在指定的后缀列表中
//
// 示例：
// - 用户ID "12345"，后缀为 5
// - 如果规则中 user_id_suffix 包含 [5, 6, 7]，则匹配成功
// - 如果规则中 user_id_suffix 包含 [0, 1, 2]，则匹配失败
func (s *ProductService) checkRule(ctx context.Context, userID string, rule model.Rule) bool {
	// Currently only supports user_id_suffix
	if len(rule.UserIDSuffixes) > 0 {
		if userID == "" {
			return false
		}
		// Extract the last digit of userID
		lastChar := userID[len(userID)-1:]
		suffix, err := strconv.Atoi(lastChar)
		if err != nil {
			// not a numeric suffix
			zlog.LogWithContext(ctx).Warn("user id suffix is not a number", zap.String("userID", userID))
			return false
		}

		for _, s := range rule.UserIDSuffixes {
			if s == suffix {
				return true
			}
		}
	}
	return false
}

// applyExperiment 对商品列表应用实验策略
// 按以下顺序执行三种操作：
// 1. Inclusions（包含）: 如果指定了 inclusions，则只保留这些商品ID，其他商品被过滤掉
// 2. Exclusions（排除）: 从商品列表中移除指定的商品ID
// 3. Modifications（修改）: 对指定商品的属性进行覆盖修改
//
// 示例场景：
// - 新用户实验：inclusions=["monthly", "yearly"] 只展示月卡和年卡
// - 隐藏周卡实验：exclusions=["weekly"] 隐藏周卡
// - 价格测试实验：modifications={"monthly": {"Price": 29.99}} 修改月卡价格
func (s *ProductService) applyExperiment(ctx context.Context, products []*model.Product, payload model.Payload) ([]*model.Product, error) {
	var filteredProducts []*model.Product

	// 1. Handle inclusions (仅包含指定商品)
	if len(payload.Inclusions) > 0 {
		inclusionSet := make(map[string]struct{})
		for _, productID := range payload.Inclusions {
			inclusionSet[productID] = struct{}{}
		}

		for _, p := range products {
			if _, ok := inclusionSet[p.ProductID]; ok {
				// Create a copy to avoid modifying the original
				newP := *p
				filteredProducts = append(filteredProducts, &newP)
			}
		}
		zlog.LogWithContext(ctx).Debug("applied inclusions filter",
			zap.Strings("inclusions", payload.Inclusions),
			zap.Int("before", len(products)),
			zap.Int("after", len(filteredProducts)))
	} else {
		// No inclusions specified, copy all products
		for _, p := range products {
			newP := *p
			filteredProducts = append(filteredProducts, &newP)
		}
	}

	// 2. Handle exclusions (排除指定商品)
	if len(payload.Exclusions) > 0 {
		exclusionSet := make(map[string]struct{})
		for _, productID := range payload.Exclusions {
			exclusionSet[productID] = struct{}{}
		}

		var remainingProducts []*model.Product
		for _, p := range filteredProducts {
			if _, ok := exclusionSet[p.ProductID]; !ok {
				remainingProducts = append(remainingProducts, p)
			}
		}

		zlog.LogWithContext(ctx).Debug("applied exclusions filter",
			zap.Strings("exclusions", payload.Exclusions),
			zap.Int("before", len(filteredProducts)),
			zap.Int("after", len(remainingProducts)))
		filteredProducts = remainingProducts
	}

	// 3. Handle modifications (属性修改)
	modificationCount := 0
	for i, p := range filteredProducts {
		if mod, ok := payload.Modifications[p.ProductID]; ok {
			modifiedProduct, err := s.applyModification(p, mod)
			if err != nil {
				zlog.LogWithContext(ctx).Error("failed to apply modification",
					zap.String("productID", p.ProductID), zap.Error(err))
				return nil, err
			}
			filteredProducts[i] = modifiedProduct
			modificationCount++
		}
	}

	if modificationCount > 0 {
		zlog.LogWithContext(ctx).Debug("applied modifications",
			zap.Int("modifiedCount", modificationCount))
	}

	return filteredProducts, nil
}

// applyModification 对单个商品应用属性修改
// 使用 JSON 序列化/反序列化的方式来动态修改商品属性
// 这种方式灵活且类型安全，支持修改任何可序列化的字段
func (s *ProductService) applyModification(product *model.Product, modification model.Modification) (*model.Product, error) {
	// Convert product to map
	productBytes, err := json.Marshal(product)
	if err != nil {
		return nil, err
	}
	var productMap map[string]interface{}
	if err := json.Unmarshal(productBytes, &productMap); err != nil {
		return nil, err
	}

	// Apply modifications
	for key, value := range modification {
		// The keys in the modification payload must match the Go struct field names
		productMap[key] = value
	}

	// Convert map back to product
	modifiedBytes, err := json.Marshal(productMap)
	if err != nil {
		return nil, err
	}
	var modifiedProduct model.Product
	if err := json.Unmarshal(modifiedBytes, &modifiedProduct); err != nil {
		return nil, err
	}

	return &modifiedProduct, nil
}

// GetSubscriptionExitConfig 获取用户订阅退出流程配置
// 基于现有A/B测试框架，为不同用户组返回不同的退出流程配置
func (s *ProductService) GetSubscriptionExitConfig(ctx context.Context, userID, projectID string) (*model.SubscriptionExitConfig, error) {
	// 1. 获取退出流程实验配置
	experiments, err := s.getExitFlowExperiments(ctx, projectID)
	if err != nil {
		// 如果获取实验配置失败，返回默认配置
		zlog.LogWithContext(ctx).Info("get exit flow experiments failed, using default config",
			zap.Error(err), zap.String("projectID", projectID))
		return s.getDefaultExitConfig(), nil
	}

	if len(experiments) == 0 {
		return s.getDefaultExitConfig(), nil
	}

	// 2. 匹配用户对应的实验
	matchedExperiment := s.matchExperiment(ctx, userID, experiments)
	if matchedExperiment == nil {
		return s.getDefaultExitConfig(), nil
	}

	zlog.LogWithContext(ctx).Info("user matched exit flow experiment",
		zap.String("userID", userID),
		zap.String("experiment", matchedExperiment.Name))

	// 3. 解析实验配置
	exitConfig, err := s.parseExitFlowPayload(matchedExperiment.Payload)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to parse exit flow payload, using default config",
			zap.Error(err), zap.String("experiment", matchedExperiment.Name))
		return s.getDefaultExitConfig(), nil
	}

	return exitConfig, nil
}

// getExitFlowExperiments 获取退出流程A/B测试实验配置
func (s *ProductService) getExitFlowExperiments(ctx context.Context, projectID string) ([]model.Experiment, error) {
	// 生成缓存键
	cacheKey := "cache:subscription_exit_flow:" + projectID

	// 尝试从缓存获取
	cachedData, err := s.redisCli.Get(cacheKey).Result()
	if err == nil {
		var experiments []model.Experiment
		if err := json.Unmarshal([]byte(cachedData), &experiments); err == nil {
			zlog.LogWithContext(ctx).Debug("get exit flow experiments from cache hit", zap.String("projectID", projectID))
			return experiments, nil
		}
	}

	// 从配置服务获取
	configKey := constants.GetConfigKey(projectID, constants.BaseConfigKeySubscriptionExitFlow)
	var experiments []model.Experiment
	err = s.configService.GetJSONConfig(configKey, &experiments)
	if err != nil {
		zlog.LogWithContext(ctx).Info("no subscription exit flow config found for project",
			zap.String("projectID", projectID), zap.String("configKey", configKey), zap.Error(err))
		return []model.Experiment{}, nil
	}

	// 更新缓存
	if len(experiments) > 0 {
		if experimentsData, err := json.Marshal(experiments); err == nil {
			if err := s.redisCli.Set(cacheKey, experimentsData, cacheTTL).Err(); err != nil {
				zlog.LogWithContext(ctx).Error("failed to set exit flow experiments to cache", zap.Error(err))
			}
		}
	}

	zlog.LogWithContext(ctx).Debug("get exit flow experiments from config service",
		zap.String("projectID", projectID), zap.Int("count", len(experiments)))
	return experiments, nil
}

// getDefaultExitConfig 获取默认退出流程配置
func (s *ProductService) getDefaultExitConfig() *model.SubscriptionExitConfig {
	defaultDetails := model.ExitFlowConfigDetails{
		Message: "DefaultExitConfig",
	}

	detailsJSON, _ := json.Marshal(defaultDetails)
	return &model.SubscriptionExitConfig{
		ExitFlowType:  vai.SubscriptionExitFlowType_EXIT_FLOW_DEFAULT,
		ConfigDetails: string(detailsJSON),
	}
}

// parseExitFlowPayload 解析退出流程实验配置
func (s *ProductService) parseExitFlowPayload(payload model.Payload) (*model.SubscriptionExitConfig, error) {
	if exitFlowConfig, ok := payload.Modifications["exit_flow_config"]; ok {
		exitConfig := &model.SubscriptionExitConfig{}

		// 手动解析字段
		if exitFlowType, exists := exitFlowConfig["exit_flow_type"]; exists {
			if typeInt, ok := exitFlowType.(float64); ok {
				exitConfig.ExitFlowType = vai.SubscriptionExitFlowType(int32(typeInt))
			}
		}

		if configDetails, exists := exitFlowConfig["config_details"]; exists {
			// configDetails 应该是一个对象，需要序列化为 JSON 字符串
			if detailsBytes, err := json.Marshal(configDetails); err == nil {
				exitConfig.ConfigDetails = string(detailsBytes)
			} else {
				// 如果序列化失败，尝试直接转换为字符串（兼容旧格式）
				if detailsStr, ok := configDetails.(string); ok {
					exitConfig.ConfigDetails = detailsStr
				}
			}
		}

		return exitConfig, nil
	}

	return s.getDefaultExitConfig(), nil
}

// 判断是否是GoCal审核中版本
func (s *ProductService) IsGoCalReviewingVersion(ctx context.Context, appVersion string) bool {
	reviewingVersion, err := s.configService.GetConfValue(constants.GoCalConfigKeyReviewingVersion)

	if err != nil {
		zlog.LogWithContext(ctx).Error("Get App Configuration Error", zap.Error(err))
	}
	if err != nil {
		zlog.LogWithContext(ctx).Error("get reviewing Version failed", zap.Error(err))
		return false
	}

	return appVersion == reviewingVersion
}

// IsSolaceReviewingVersion 判断是否是Solace审核中版本
func (s *ProductService) IsSolaceReviewingVersion(ctx context.Context, appVersion string) bool {
	reviewingVersion, err := s.configService.GetConfValue(constants.SolaceConfigKeyReviewingVersion)

	if err != nil {
		zlog.LogWithContext(ctx).Error("Get App Configuration Error", zap.Error(err))
	}
	if err != nil {
		zlog.LogWithContext(ctx).Error("get reviewing Version failed", zap.Error(err))
		return false
	}

	return appVersion == reviewingVersion
}

// GoCalFilterProducts 筛选审核中的商品列表
// 业务逻辑：
// 1. 如果是审核中版本，显示 GroupID = 1 的商品
// 2. 如果是非审核中版本，根据 trial_filter_config 配置决定显示 GroupID = 0 或 GroupID = 2 的商品
func (s *ProductService) GoCalFilterProducts(ctx context.Context, products []*model.Product, os, appVersion string, userID string) []*model.Product {
	isReviewing := os == constants.IOS && s.IsGoCalReviewingVersion(ctx, appVersion)
	targetGroupID := 0
	logMessage := "filtered noReviewing products"
	countKey := "NoReviewingCount"

	if isReviewing {
		targetGroupID = 1
		logMessage = "filtered reviewing products"
		countKey = "reviewingCount"
	} else {
		// 非审核版本，根据 trial_filter_config 配置决定用户分组
		projectID := common.GetProjectID(ctx)
		if projectID == constants.ProjectIdDietAI {
			// 获取试用过滤配置
			trialFilterConfig, err := s.getTrialFilterConfig(ctx, projectID, userID)
			if err != nil {
				zlog.LogWithContext(ctx).Warn("get trial filter config failed, using default groupID 0",
					zap.Error(err), zap.String("userID", userID))
				targetGroupID = 0
			} else {
				targetGroupID = trialFilterConfig.TargetGroupID
				logMessage = fmt.Sprintf("filtered products by trial config, groupID=%d", targetGroupID)
				countKey = fmt.Sprintf("trialFilterGroupID%dCount", targetGroupID)
			}
		}
	}

	filteredProducts := make([]*model.Product, 0, len(products))
	for _, product := range products {
		if product.GroupID == targetGroupID {
			filteredProducts = append(filteredProducts, product)
		}
	}

	zlog.LogWithContext(ctx).Info(logMessage,
		zap.Int("originalCount", len(products)),
		zap.Int(countKey, len(filteredProducts)),
		zap.String("userID", userID),
		zap.Int("targetGroupID", targetGroupID))

	return filteredProducts
}

// TrialFilterConfig 试用过滤配置结构
type TrialFilterConfig struct {
	TargetGroupID int `json:"target_group_id"` // 目标GroupID，0或2
}

// getTrialFilterConfig 获取试用过滤配置
// 根据配置中的用户ID后缀匹配规则决定用户应该看到哪个GroupID的商品
func (s *ProductService) getTrialFilterConfig(ctx context.Context, projectID, userID string) (*TrialFilterConfig, error) {
	// 1. 获取试用过滤实验配置
	experiments, err := s.getTrialFilterExperiments(ctx, projectID)
	if err != nil {
		return &TrialFilterConfig{TargetGroupID: 0}, err
	}

	if len(experiments) == 0 {
		return &TrialFilterConfig{TargetGroupID: 0}, nil
	}

	// 2. 匹配用户对应的实验
	matchedExperiment := s.matchExperiment(ctx, userID, experiments)
	if matchedExperiment == nil {
		return &TrialFilterConfig{TargetGroupID: 0}, nil
	}

	zlog.LogWithContext(ctx).Info("user matched trial filter experiment",
		zap.String("userID", userID),
		zap.String("experiment", matchedExperiment.Name))

	// 3. 解析实验配置
	trialConfig, err := s.parseTrialFilterPayload(matchedExperiment.Payload)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to parse trial filter payload, using default groupID 0",
			zap.Error(err), zap.String("experiment", matchedExperiment.Name))
		return &TrialFilterConfig{TargetGroupID: 0}, nil
	}

	return trialConfig, nil
}

// getTrialFilterExperiments 获取试用过滤A/B测试实验配置
func (s *ProductService) getTrialFilterExperiments(ctx context.Context, projectID string) ([]model.Experiment, error) {
	// 生成缓存键
	cacheKey := "cache:trial_filter:" + projectID

	// 尝试从缓存获取
	cachedData, err := s.redisCli.Get(cacheKey).Result()
	if err == nil {
		var experiments []model.Experiment
		if err := json.Unmarshal([]byte(cachedData), &experiments); err == nil {
			zlog.LogWithContext(ctx).Debug("get trial filter experiments from cache hit", zap.String("projectID", projectID))
			return experiments, nil
		}
	}

	// 从配置服务获取
	configKey := constants.GoCalConfigKeyTrialFilter
	var experiments []model.Experiment
	err = s.configService.GetJSONConfig(configKey, &experiments)
	if err != nil {
		zlog.LogWithContext(ctx).Info("no trial filter config found for project",
			zap.String("projectID", projectID), zap.String("configKey", configKey), zap.Error(err))
		return []model.Experiment{}, nil
	}

	// 更新缓存
	if len(experiments) > 0 {
		if experimentsData, err := json.Marshal(experiments); err == nil {
			if err := s.redisCli.Set(cacheKey, experimentsData, cacheTTL).Err(); err != nil {
				zlog.LogWithContext(ctx).Error("failed to set trial filter experiments to cache", zap.Error(err))
			}
		}
	}

	zlog.LogWithContext(ctx).Debug("get trial filter experiments from config service",
		zap.String("projectID", projectID), zap.Int("count", len(experiments)))
	return experiments, nil
}

// parseTrialFilterPayload 解析试用过滤实验配置
func (s *ProductService) parseTrialFilterPayload(payload model.Payload) (*TrialFilterConfig, error) {
	if trialFilterConfig, ok := payload.Modifications["trial_filter_config"]; ok {
		config := &TrialFilterConfig{}

		// 手动解析字段
		if targetGroupID, exists := trialFilterConfig["target_group_id"]; exists {
			if groupIDFloat, ok := targetGroupID.(float64); ok {
				config.TargetGroupID = int(groupIDFloat)
			} else if groupIDInt, ok := targetGroupID.(int); ok {
				config.TargetGroupID = groupIDInt
			}
		}

		return config, nil
	}

	return &TrialFilterConfig{TargetGroupID: 0}, nil
}

// filterSolaceReviewingProducts 筛选审核中的商品列表
// groupID 为 1，即审核中显示的商品
// 业务逻辑：只返回 groupID 为 1 的商品，用于应用商店审核期间展示
func (s *ProductService) filterSolaceReviewingProducts(ctx context.Context, products []*model.Product, os, appVersion string) []*model.Product {
	isReviewing := os == constants.IOS && s.IsSolaceReviewingVersion(ctx, appVersion)
	targetGroupID := 0
	logMessage := "filtered noReviewing products"
	countKey := "NoReviewingCount"

	if isReviewing {
		targetGroupID = 1
		logMessage = "filtered reviewing products"
		countKey = "reviewingCount"
	}

	filteredProducts := make([]*model.Product, 0, len(products))
	for _, product := range products {
		if product.GroupID == targetGroupID {
			filteredProducts = append(filteredProducts, product)
		}
	}

	zlog.LogWithContext(ctx).Info(logMessage,
		zap.Int("originalCount", len(products)),
		zap.Int(countKey, len(filteredProducts)))

	return filteredProducts
}
