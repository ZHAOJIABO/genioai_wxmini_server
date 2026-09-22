package event_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/cache"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/zlog"
)

// SubscriptionDetails 定义了与会员资格相关的具体信息 (此结构体在此处理器中将被忽略)
type SubscriptionDetails struct {
	Level     int       `json:"level"`
	ExpiresAt time.Time `json:"expires_at"`
}

// PaymentPayload 定义了支付事件消息的统一结构 (V2)
type PaymentPayload struct {
	EventID             string                      `json:"event_id"`
	SubscriptionType    SubscriptionType            `json:"subscription_type"`
	UserID              string                      `json:"user_id"`
	ProjectID           string                      `json:"project_id"`
	OrderID             string                      `json:"order_id"`
	CreditDetails       credit.CreditDetails        `json:"credit_details"`
	SubscriptionDetails *credit.SubscriptionDetails `json:"subscription_details,omitempty"` // 仅为兼容性保留
	// IOS 新 WebOrderLineItemID
	IOSNewWebOrderLineItemID bool `json:"ios_new_web_order_line_item_id"`
}

// PaymentEventHandler 负责处理所有与支付相关的事件
type PaymentEventHandler struct {
	creditService credit.Service
	cacheService  *cache.CacheService
}

type SubscriptionType int

const (
	// 未知
	SubscriptionTypeUnknown SubscriptionType = iota
	// 订阅
	SubscriptionTypeSubscription
	// 续订
	SubscriptionTypeRenewal
	// 过期
	SubscriptionTypeExpired
	// 取消
	SubscriptionTypeCanceled
	// 升级
	SubscriptionTypeUpgrade
	// 一次性购买
	SubscriptionTypeOneTimePurchase
	// 降级
	SubscriptionTypeDowngrade
	// 重订阅
	SubscriptionTypeResubscribe

	// 优惠券兑换
	SubscriptionTypeCouponRedemption SubscriptionType = 100

	UserSubscribeInfoCacheKeyPrefix = "project:%s:user:%s:subscribe_info"
)

// NewPaymentEventHandler 创建一个新的支付事件处理器
func NewPaymentEventHandler(cs credit.Service, cacheService *cache.CacheService) *PaymentEventHandler {
	return &PaymentEventHandler{
		creditService: cs,
		cacheService:  cacheService,
	}
}

// EventType 返回此 Handler 负责处理的事件类型
func (h *PaymentEventHandler) EventType() []string {
	// 此处理器将统一处理所有支付相关事件
	return []string{
		constants.EventTypeSubscriptionCreated,
		constants.EventTypeSubscriptionRenewed,
		constants.EventTypeOnetimePurchase,
	}
}

// HandleEvent 是事件处理的核心逻辑。
// V2 版本中，此方法只负责积分管理，忽略所有会员状态相关信息。
func (h *PaymentEventHandler) HandleEvent(ctx context.Context, event *model.UserEvent) error {
	var payload PaymentPayload
	if err := json.Unmarshal(event.EventData, &payload); err != nil {
		return errors.Wrap(err, "failed to unmarshal payment event data")
	}
	if payload.SubscriptionType == SubscriptionTypeUnknown ||
		payload.SubscriptionType == SubscriptionTypeCanceled ||
		payload.SubscriptionType == SubscriptionTypeExpired ||
		payload.SubscriptionType == SubscriptionTypeDowngrade {
		return nil
	}
	isCouponRedemption := payload.SubscriptionType == SubscriptionTypeCouponRedemption

	if payload.ProjectID == constants.ProjectIdPicLib && !payload.IOSNewWebOrderLineItemID &&
		payload.SubscriptionType != SubscriptionTypeOneTimePurchase && !isCouponRedemption {
		zlog.LogWithContext(ctx).Info("PaymentEventHandler HandleEvent Picflow Restore, will not add credits", zap.Any("payload", payload))
		return nil
	}

	creditInfo := payload.CreditDetails
	// Picflow 项目会员升级特殊逻辑
	if payload.ProjectID == constants.ProjectPicflow &&
		constants.CreditTransactionType(creditInfo.TransactionType) == constants.TransactionTypeSubscriptionUpgrade {
		return h.creditService.HandleMembershipUpgrade(
			ctx,
			payload.ProjectID,
			event.UserID,
			payload.EventID,
			creditInfo,
			*payload.SubscriptionDetails,
		)
	}

	var subscribeLevel int
	if payload.SubscriptionDetails != nil {
		subscribeLevel = payload.SubscriptionDetails.Level
	} else if payload.SubscriptionType == SubscriptionTypeOneTimePurchase {
		subscribeLevel = 99
	}

	// 基础验证
	if creditInfo.Amount == 0 { // 0额度操作直接忽略
		return nil
	}
	if creditInfo.Type == "" || creditInfo.TransactionType == "" {
		return errors.New("credit type and transaction type must be specified")
	}
	if payload.OrderID == "" && !isCouponRedemption {
		return errors.New("order_id is required for transaction tracking")
	}
	if creditInfo.ExpiresAt.IsZero() {
		creditInfo.ExpiresAt = time.Date(9990, 12, 31, 23, 59, 59, 0, time.UTC)
	}
	userSubscribeInfoCacheKey := fmt.Sprintf(UserSubscribeInfoCacheKeyPrefix, payload.ProjectID, event.UserID)
	h.cacheService.Del(context.Background(), userSubscribeInfoCacheKey)
	return h.creditService.AddCredits(
		ctx,
		payload.ProjectID,
		event.UserID,
		payload.EventID,
		constants.CreditType(creditInfo.Type),
		constants.CreditTransactionType(creditInfo.TransactionType),
		creditInfo.Amount,
		creditInfo.ExpiresAt,
		creditInfo.Description,
		subscribeLevel,
	)
}
