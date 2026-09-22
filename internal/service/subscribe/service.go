package subscribe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/conf"
	"va_visionai_server/internal/utils"
	"va_visionai_server/internal/zlog"
)

// PromotionHistory 促销历史记录
type PromotionHistory struct {
	PaymentChannel string `json:"payment_channel"` // 支付渠道: "googleplay" 或 "apple"
	TransactionID  string `json:"transaction_id"`  // 交易ID
	ProductID      string `json:"product_id"`      // 产品ID
	OfferID        string `json:"offer_id"`        // Google Play 优惠ID (仅 Google Play 用户返回)
	OfferType      *int   `json:"offer_type"`      // iOS 优惠类型 (仅 Apple 用户返回)
	PurchaseDate   int64  `json:"purchase_date"`   // 购买时间戳(毫秒)
}

// Service 订阅历史服务
type Service struct {
	httpClient   *http.Client
	payLinkerURL string
}

// NewService 创建订阅历史服务实例
func NewService() *Service {
	return &Service{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:       50,
				IdleConnTimeout:    90 * time.Second,
				DisableCompression: true,
			},
		},
		payLinkerURL: conf.GlobalConfig.PayLinker.Addr,
	}
}

// GetPromotionHistory 获取用户促销历史
func (s *Service) GetPromotionHistory(ctx context.Context, projectID, userID string) ([]PromotionHistory, error) {
	if projectID == "" || userID == "" {
		return nil, errors.New("invalid parameters: projectID and userID are required")
	}

	url := fmt.Sprintf("%s/promotions/history?project_id=%s&user_id=%s", s.payLinkerURL, projectID, userID)

	//nolint:bodyclose
	_, body, err := utils.Get(ctx, url, nil)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetPromotionHistory request failed",
			zap.String("project_id", projectID),
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get promotion history: %w", err)
	}

	var result []PromotionHistory
	if err := json.Unmarshal(body, &result); err != nil {
		zlog.LogWithContext(ctx).Error("GetPromotionHistory unmarshal failed",
			zap.String("project_id", projectID),
			zap.String("user_id", userID),
			zap.String("body", string(body)),
			zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// GetUsedPromotion 获取用户已使用优惠的商品ID集合
func (s *Service) GetUsedPromotion(ctx context.Context, projectID, userID string) (map[string]struct{}, error) {
	history, err := s.GetPromotionHistory(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}

	usedProducts := make(map[string]struct{}, len(history))
	for _, h := range history {
		usedProducts[h.ProductID] = struct{}{}
	}
	return usedProducts, nil
}
