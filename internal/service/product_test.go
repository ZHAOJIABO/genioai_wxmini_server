package service

import (
	"context"
	"testing"

	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
)

// 创建测试用的商品数据
func createTestProducts() []*model.Product {
	return []*model.Product{
		{
			ProductID:   "monthly",
			Name:        "Monthly Plan",
			Price:       9.99,
			PriceLabel:  "$9.99/mo",
			PaymentWay:  "AppStore",
			ProductType: "Subscribe",
			Score:       1,
			Status:      1,
			Lang:        "en",
			ProjectID:   "test_project",
			GroupID:     1,
			Level:       20, // 月卡级别20
		},
		{
			ProductID:   "yearly",
			Name:        "Yearly Plan",
			Price:       99.99,
			PriceLabel:  "$99.99/yr",
			PaymentWay:  "AppStore",
			ProductType: "Subscribe",
			Score:       2,
			Status:      1,
			Lang:        "en",
			ProjectID:   "test_project",
			GroupID:     1,
			Level:       30, // 年卡级别30
		},
		{
			ProductID:   "weekly",
			Name:        "Weekly Plan",
			Price:       2.99,
			PriceLabel:  "$2.99/wk",
			PaymentWay:  "AppStore",
			ProductType: "Subscribe",
			Score:       3,
			Status:      1,
			Lang:        "en",
			ProjectID:   "test_project",
			GroupID:     1,
			Level:       10, // 周卡级别10
		},
	}
}

// TestProductService_checkRule 测试用户规则匹配逻辑
func TestProductService_checkRule(t *testing.T) {
	service := &ProductService{}
	ctx := context.Background()

	tests := []struct {
		name     string
		userID   string
		rule     model.Rule
		expected bool
	}{
		{
			name:   "匹配成功 - 用户ID以5结尾",
			userID: "user12345",
			rule: model.Rule{
				UserIDSuffixes: []int{5, 6, 7},
			},
			expected: true,
		},
		{
			name:   "匹配失败 - 用户ID以4结尾",
			userID: "user12344",
			rule: model.Rule{
				UserIDSuffixes: []int{5, 6, 7},
			},
			expected: false,
		},
		{
			name:   "空用户ID",
			userID: "",
			rule: model.Rule{
				UserIDSuffixes: []int{5, 6, 7},
			},
			expected: false,
		},
		{
			name:   "非数字后缀",
			userID: "userABC",
			rule: model.Rule{
				UserIDSuffixes: []int{5, 6, 7},
			},
			expected: false,
		},
		{
			name:   "空规则",
			userID: "user12345",
			rule: model.Rule{
				UserIDSuffixes: []int{},
			},
			expected: false,
		},
		{
			name:   "单字符用户ID",
			userID: "5",
			rule: model.Rule{
				UserIDSuffixes: []int{5, 6, 7},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.checkRule(ctx, tt.userID, tt.rule)
			if result != tt.expected {
				t.Errorf("checkRule() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestProductService_matchExperiment 测试实验匹配逻辑
func TestProductService_matchExperiment(t *testing.T) {
	service := &ProductService{}
	ctx := context.Background()

	experiments := []model.Experiment{
		{
			Name:     "experiment_1",
			Priority: 10,
			Rule: model.Rule{
				UserIDSuffixes: []int{0, 1, 2},
			},
		},
		{
			Name:     "experiment_2",
			Priority: 20,
			Rule: model.Rule{
				UserIDSuffixes: []int{5, 6, 7},
			},
		},
		{
			Name:     "experiment_3",
			Priority: 15,
			Rule: model.Rule{
				UserIDSuffixes: []int{8, 9},
			},
		},
	}

	tests := []struct {
		name            string
		userID          string
		expectedExpName string
	}{
		{
			name:            "匹配高优先级实验",
			userID:          "user1235",
			expectedExpName: "experiment_2", // 优先级 20
		},
		{
			name:            "匹配中优先级实验",
			userID:          "user1238",
			expectedExpName: "experiment_3", // 优先级 15
		},
		{
			name:            "匹配低优先级实验",
			userID:          "user1230",
			expectedExpName: "experiment_1", // 优先级 10
		},
		{
			name:            "无匹配实验",
			userID:          "user1234",
			expectedExpName: "", // 无匹配
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.matchExperiment(ctx, tt.userID, experiments)
			if tt.expectedExpName == "" {
				if result != nil {
					t.Errorf("matchExperiment() = %v, expected nil", result.Name)
				}
			} else {
				if result == nil {
					t.Errorf("matchExperiment() = nil, expected %v", tt.expectedExpName)
				} else if result.Name != tt.expectedExpName {
					t.Errorf("matchExperiment() = %v, expected %v", result.Name, tt.expectedExpName)
				}
			}
		})
	}
}

// TestProductService_applyModification 测试商品属性修改逻辑
func TestProductService_applyModification(t *testing.T) {
	service := &ProductService{}

	product := &model.Product{
		ProductID:   "test_product",
		Name:        "Test Product",
		Price:       9.99,
		PriceLabel:  "$9.99",
		PaymentWay:  "AppStore",
		ProductType: "Subscribe",
	}

	modification := model.Modification{
		"Price":      19.99,
		"PriceLabel": "$19.99",
		"Name":       "Modified Product",
	}

	result, err := service.applyModification(product, modification)

	if err != nil {
		t.Errorf("applyModification() error = %v", err)
		return
	}

	if result == nil {
		t.Error("applyModification() returned nil")
		return
	}

	// 验证修改的字段
	if result.Price != 19.99 {
		t.Errorf("Price = %v, expected 19.99", result.Price)
	}
	if result.PriceLabel != "$19.99" {
		t.Errorf("PriceLabel = %v, expected $19.99", result.PriceLabel)
	}
	if result.Name != "Modified Product" {
		t.Errorf("Name = %v, expected Modified Product", result.Name)
	}

	// 验证未修改的字段保持不变
	if result.ProductID != "test_product" {
		t.Errorf("ProductID = %v, expected test_product", result.ProductID)
	}
	if result.PaymentWay != "AppStore" {
		t.Errorf("PaymentWay = %v, expected AppStore", result.PaymentWay)
	}
}

// TestProductService_applyExperiment_Exclusions 测试排除逻辑
func TestProductService_applyExperiment_Exclusions(t *testing.T) {
	service := &ProductService{}
	ctx := context.Background()

	products := createTestProducts()
	payload := model.Payload{
		Exclusions: []string{"weekly"},
	}

	result, err := service.applyExperiment(ctx, products, payload)

	if err != nil {
		t.Errorf("applyExperiment() error = %v", err)
		return
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 products, got %d", len(result))
		return
	}

	// 验证 weekly 被排除
	for _, p := range result {
		if p.ProductID == "weekly" {
			t.Error("weekly product should be excluded")
		}
	}

	// 验证其他商品存在
	productIDs := make(map[string]bool)
	for _, p := range result {
		productIDs[p.ProductID] = true
	}

	if !productIDs["monthly"] {
		t.Error("monthly product should be included")
	}
	if !productIDs["yearly"] {
		t.Error("yearly product should be included")
	}
}

// TestProductService_applyExperiment_Inclusions 测试包含逻辑
func TestProductService_applyExperiment_Inclusions(t *testing.T) {
	service := &ProductService{}
	ctx := context.Background()

	products := createTestProducts()
	payload := model.Payload{
		Inclusions: []string{"monthly", "yearly"},
	}

	result, err := service.applyExperiment(ctx, products, payload)

	if err != nil {
		t.Errorf("applyExperiment() error = %v", err)
		return
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 products, got %d", len(result))
		return
	}

	// 验证只包含指定商品
	productIDs := make(map[string]bool)
	for _, p := range result {
		productIDs[p.ProductID] = true
	}

	if !productIDs["monthly"] {
		t.Error("monthly product should be included")
	}
	if !productIDs["yearly"] {
		t.Error("yearly product should be included")
	}
	if productIDs["weekly"] {
		t.Error("weekly product should not be included")
	}
}

// TestProductService_applyExperiment_Modifications 测试属性修改逻辑
func TestProductService_applyExperiment_Modifications(t *testing.T) {
	service := &ProductService{}
	ctx := context.Background()

	products := createTestProducts()
	payload := model.Payload{
		Modifications: map[string]model.Modification{
			"monthly": {
				"Price":      19.99,
				"PriceLabel": "$19.99/mo",
			},
			"yearly": {
				"Price": 79.99,
			},
		},
	}

	result, err := service.applyExperiment(ctx, products, payload)

	if err != nil {
		t.Errorf("applyExperiment() error = %v", err)
		return
	}

	if len(result) != 3 {
		t.Errorf("Expected 3 products, got %d", len(result))
		return
	}

	// 验证修改
	for _, p := range result {
		switch p.ProductID {
		case "monthly":
			if p.Price != 19.99 {
				t.Errorf("monthly price = %v, expected 19.99", p.Price)
			}
			if p.PriceLabel != "$19.99/mo" {
				t.Errorf("monthly price_label = %v, expected $19.99/mo", p.PriceLabel)
			}
		case "yearly":
			if p.Price != 79.99 {
				t.Errorf("yearly price = %v, expected 79.99", p.Price)
			}
			// PriceLabel 应该保持原值
			if p.PriceLabel != "$99.99/yr" {
				t.Errorf("yearly price_label = %v, expected $99.99/yr", p.PriceLabel)
			}
		case "weekly":
			// weekly 应该保持原值
			if p.Price != 2.99 {
				t.Errorf("weekly price = %v, expected 2.99", p.Price)
			}
		}
	}
}

// TestProductService_applyExperiment_Combined 测试组合逻辑
func TestProductService_applyExperiment_Combined(t *testing.T) {
	service := &ProductService{}
	ctx := context.Background()

	products := createTestProducts()
	payload := model.Payload{
		Inclusions: []string{"monthly", "yearly", "weekly"},
		Exclusions: []string{"weekly"},
		Modifications: map[string]model.Modification{
			"monthly": {
				"Price": 19.99,
			},
		},
	}

	result, err := service.applyExperiment(ctx, products, payload)

	if err != nil {
		t.Errorf("applyExperiment() error = %v", err)
		return
	}

	// 应该先执行 inclusions（保留所有），再执行 exclusions（排除 weekly），最后执行 modifications
	if len(result) != 2 {
		t.Errorf("Expected 2 products, got %d", len(result))
		return
	}

	// 验证 weekly 被排除
	for _, p := range result {
		if p.ProductID == "weekly" {
			t.Error("weekly product should be excluded")
		}
	}

	// 验证 monthly 价格被修改
	var monthlyProduct *model.Product
	for _, p := range result {
		if p.ProductID == "monthly" {
			monthlyProduct = p
			break
		}
	}

	if monthlyProduct == nil {
		t.Error("monthly product should be included")
	} else if monthlyProduct.Price != 19.99 {
		t.Errorf("monthly price = %v, expected 19.99", monthlyProduct.Price)
	}
}

// TestProductService_filterProductsByMembershipLevel 测试会员级别过滤逻辑
func TestProductService_filterProductsByMembershipLevel(t *testing.T) {
	service := &ProductService{}
	ctx := context.Background()

	products := createTestProducts()

	tests := []struct {
		name             string
		subscribeLevel   int32
		expectedCount    int
		expectedProducts []string
	}{
		{
			name:             "无订阅用户看到所有商品",
			subscribeLevel:   0,
			expectedCount:    3,
			expectedProducts: []string{"monthly", "yearly", "weekly"},
		},
		{
			name:             "10级会员只看到高于10级的商品",
			subscribeLevel:   10,
			expectedCount:    2,
			expectedProducts: []string{"monthly", "yearly"},
		},
		{
			name:             "20级会员只看到高于20级的商品",
			subscribeLevel:   20,
			expectedCount:    1,
			expectedProducts: []string{"yearly"},
		},
		{
			name:             "30级会员看不到任何商品",
			subscribeLevel:   30,
			expectedCount:    0,
			expectedProducts: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subscribeInfo := &vai.SubscribeInfo{
				SubscribeLevel: tt.subscribeLevel,
			}

			result := service.filterProductsByMembershipLevel(ctx, products, subscribeInfo)

			if len(result) != tt.expectedCount {
				t.Errorf("Expected %d products, got %d", tt.expectedCount, len(result))
				return
			}

			// 验证返回的商品ID
			resultIDs := make(map[string]bool)
			for _, p := range result {
				resultIDs[p.ProductID] = true
			}

			for _, expectedID := range tt.expectedProducts {
				if !resultIDs[expectedID] {
					t.Errorf("Expected product %s not found in result", expectedID)
				}
			}

			// 验证没有不应该出现的商品
			for _, p := range result {
				found := false
				for _, expectedID := range tt.expectedProducts {
					if p.ProductID == expectedID {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Unexpected product %s found in result", p.ProductID)
				}
			}
		})
	}
}

// TestResolveOffTagIcon 测试 resolveOffTagIcon 纯函数
func TestResolveOffTagIcon(t *testing.T) {
	cfg := OffTagIconConfig{
		Default: "default_icon",
		NewUser: "new_user_icon",
	}

	tests := []struct {
		name            string
		productType     string
		cfg             OffTagIconConfig
		usedPromotion   bool
		fallbackDefault bool
		expected        string
	}{
		// 内购商品测试用例
		{
			name:            "内购商品_未用优惠_正常状态",
			productType:     "OneTimeCharge",
			cfg:             cfg,
			usedPromotion:   false,
			fallbackDefault: false,
			expected:        "new_user_icon",
		},
		{
			name:            "内购商品_已用优惠_正常状态",
			productType:     "OneTimeCharge",
			cfg:             cfg,
			usedPromotion:   true,
			fallbackDefault: false,
			expected:        "new_user_icon",
		},
		{
			name:            "内购商品_未用优惠_状态未知",
			productType:     "OneTimeCharge",
			cfg:             cfg,
			usedPromotion:   false,
			fallbackDefault: true,
			expected:        "new_user_icon",
		},
		{
			name:            "内购商品_已用优惠_状态未知",
			productType:     "OneTimeCharge",
			cfg:             cfg,
			usedPromotion:   true,
			fallbackDefault: true,
			expected:        "new_user_icon",
		},
		// 订阅商品测试用例
		{
			name:            "订阅商品_未用优惠_正常状态",
			productType:     "Subscribe",
			cfg:             cfg,
			usedPromotion:   false,
			fallbackDefault: false,
			expected:        "new_user_icon",
		},
		{
			name:            "订阅商品_已用优惠_正常状态",
			productType:     "Subscribe",
			cfg:             cfg,
			usedPromotion:   true,
			fallbackDefault: false,
			expected:        "default_icon",
		},
		{
			name:            "订阅商品_未用优惠_状态未知",
			productType:     "Subscribe",
			cfg:             cfg,
			usedPromotion:   false,
			fallbackDefault: true,
			expected:        "default_icon",
		},
		{
			name:            "订阅商品_已用优惠_状态未知",
			productType:     "Subscribe",
			cfg:             cfg,
			usedPromotion:   true,
			fallbackDefault: true,
			expected:        "default_icon",
		},
		// 边界情况
		{
			name:            "未知商品类型",
			productType:     "Unknown",
			cfg:             cfg,
			usedPromotion:   false,
			fallbackDefault: false,
			expected:        "default_icon",
		},
		{
			name:            "内购商品_NewUser配置为空",
			productType:     "OneTimeCharge",
			cfg:             OffTagIconConfig{Default: "default_icon", NewUser: ""},
			usedPromotion:   false,
			fallbackDefault: false,
			expected:        "default_icon",
		},
		{
			name:            "订阅商品_NewUser配置为空_未用优惠",
			productType:     "Subscribe",
			cfg:             OffTagIconConfig{Default: "default_icon", NewUser: ""},
			usedPromotion:   false,
			fallbackDefault: false,
			expected:        "default_icon",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveOffTagIcon(tt.productType, tt.cfg, tt.usedPromotion, tt.fallbackDefault)
			if result != tt.expected {
				t.Errorf("resolveOffTagIcon() = %v, want %v", result, tt.expected)
			}
		})
	}
}
