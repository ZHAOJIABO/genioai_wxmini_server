package constants

// apple notify type
const (
	// 订阅
	SUBSCRIBED = "SUBSCRIBED"
	// 更改订阅偏好
	DID_CHANGE_RENEWAL_PREF = "DID_CHANGE_RENEWAL_PREF"
	// 更改订阅状态
	DID_CHANGE_RENEWAL_STATUS = "DID_CHANGE_RENEWAL_STATUS"
	// 优惠兑换
	OFFER_REDEEMED = "OFFER_REDEEMED"
	// 续订
	DID_RENEW = "DID_RENEW"
	// 过期
	EXPIRED = "EXPIRED"
	// 续订失败
	DID_FAIL_TO_RENEW = "DID_FAIL_TO_RENEW"
	// 宽限期过期
	GRACE_PERIOD_EXPIRED = "GRACE_PERIOD_EXPIRED"
	// 价格上涨
	PRICE_INCREASE = "PRICE_INCREASE"
	// 退款
	REFUND = "REFUND"
	// 退款拒绝
	REFUND_DECLINED = "REFUND_DECLINED"
	// 消耗请求
	CONSUMPTION_REQUEST = "CONSUMPTION_REQUEST"
	// 续订延长
	RENEWAL_EXTENDED = "RENEWAL_EXTENDED"
	// 撤销
	REVOKE = "REVOKE"
	// 测试
	TEST = "TEST"
	// 续订扩展
	RENEWAL_EXTENSION = "RENEWAL_EXTENSION"
	// 退款反转
	REFUND_REVERSED = "REFUND_REVERSED"
	// 外部购买令牌
	EXTERNAL_PURCHASE_TOKEN = "EXTERNAL_PURCHASE_TOKEN"
	// 一次性收费
	ONE_TIME_CHARGE = "ONE_TIME_CHARGE"

	// 订阅状态
	// 订阅
	SubscriptionStatusSubscribed = "SUBSCRIBED"
	// 过期
	SubscriptionStatusExpired = "EXPIRED"

	ProductTypeSubscribe     = "Subscribe"
	ProductTypeOneTimeCharge = "OneTimeCharge"
)

type OrderStatus int

const (
	Pending       OrderStatus = iota // 待支付
	Processing                       // 支付中
	Paid                             // 支付成功
	PaymentFailed                    // 支付失败
	Cancelled                        // 已取消
	PendingReview                    // 待审核
	Reviewed                         // 已审核
	Activated                        // 订阅激活
	Expired                          // 订阅到期
	Renewing                         // 续订中
	Refunded                         // 已退款
	Refunding                        // 退款处理中
	ExpiredUnpaid                    // 过期未支付
)

func (s OrderStatus) String() string {
	switch s {
	case Pending:
		return "待支付"
	case Processing:
		return "支付中"
	case Paid:
		return "支付成功"
	case PaymentFailed:
		return "支付失败"
	case Cancelled:
		return "已取消"
	case PendingReview:
		return "待审核"
	case Reviewed:
		return "已审核"
	case Activated:
		return "订阅激活"
	case Expired:
		return "订阅到期"
	case Renewing:
		return "续订中"
	case Refunded:
		return "已退款"
	case Refunding:
		return "退款处理中"
	case ExpiredUnpaid:
		return "过期未支付"
	default:
		return "未知状态"
	}
}

type ProductStatus int

const (
	Normal ProductStatus = iota
	Unlisted
)
