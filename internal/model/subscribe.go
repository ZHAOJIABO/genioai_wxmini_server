package model

import (
	"time"

	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	vai "va_visionai_server/internal/va_interface"
)

type Order struct {
	gorm.Model

	ProjectID       string            `gorm:"type:varchar(255)"`
	UserID          string            `gorm:"type:varchar(255)"`
	ProductID       string            `gorm:"type:varchar(255)"`
	OrderID         string            `gorm:"type:varchar(255);unique"`
	PaymentWay      string            `gorm:"payment_way"`
	Status          vai.PaymentStatus `gorm:"type:int"`
	AlipayPayParams string            `gorm:"type:text"`
}

type OrderDetail struct {
	gorm.Model

	OrderID         string    `gorm:"order_id;type:varchar(255);not null"`
	DeviceNum       string    `gorm:"device_num;type:varchar(255);not null"`
	PaymentWay      string    `gorm:"payment_way;type:varchar(255);not null"`
	PayTime         time.Time `gorm:"pay_time;type:datetime;not null"`
	PayCallbackData string    `gorm:"pay_callback_data;type:text"`
}

type Product struct {
	gorm.Model

	ProjectID         string                  `gorm:"project_id;type:varchar(255);not null"`
	ProductID         string                  `gorm:"product_id;type:varchar(255);not null"`
	Name              string                  `gorm:"name;type:varchar(255);not null"`
	Description       string                  `gorm:"description;type:varchar(255);not null"`
	Score             int                     `gorm:"score;type:int;not null"`
	Lang              string                  `gorm:"lang;type:varchar(255);not null"`
	Price             float64                 `gorm:"price;type:decimal(10,2);not null"`
	PriceUnit         string                  `gorm:"price_unit;type:varchar(255);not null"`
	PriceLabel        string                  `gorm:"price_label;type:varchar(255);not null"`
	Duration          uint                    `gorm:"duration;type:int;not null"`
	Status            constants.ProductStatus `gorm:"status;type:int;not null"`
	Discount          float64                 `gorm:"discount;type:decimal(10,2);not null"`
	DiscountLabel     string                  `gorm:"discount_label;type:varchar(255);not null"`
	AveragePrice      float64                 `gorm:"average_price;type:decimal(10,2);not null"`
	AveragePriceUnit  string                  `gorm:"average_price_unit;type:varchar(255);not null"`
	AveragePriceLabel string                  `gorm:"average_price_label;type:varchar(255);not null"`
	IsTrial           bool                    `gorm:"is_trial;type:boolean;not null"`
	Level             int                     `gorm:"level;type:int;not null"`
	PaymentWay        string                  `gorm:"payment_way;type:varchar(255);not null"`
	// ProductType 决定了支付方式 (Subscribe-订阅, OneTimeCharge-一次性购买)
	ProductType string `gorm:"product_type;type:varchar(255);not null"`
	// BenefitType 决定了商品性质/权益类型 (membership-会员, credits-次数/积分)
	BenefitType string `gorm:"benefit_type;type:varchar(255);not null;default:'membership'"`
	Credits     int    `gorm:"credits;type:int;not null;default:0"` // 权益包中包含的次数
	GroupID     int    `gorm:"group_id;type:int;not null;index"`
	//Icon
	Icon string `gorm:"icon;type:varchar(255);not null"`
	// product group id
	ProductGroupID string `gorm:"product_group_id;type:varchar(255);not null"`
}

type Subscription struct {
	gorm.Model

	ReceiptID   string                `json:"receipt_id"`
	DeviceID    string                `json:"device_id"`    // 设备唯一标识符
	ProductID   string                `json:"product_id"`   // 产品ID (例如: com.yourapp.monthly_subscription)
	ReceiptData string                `json:"receipt_data"` // 购买凭证数据 (从 iOS 设备获取)
	Status      constants.OrderStatus `json:"status"`       // 订阅状态 (例如: active, expired, pending)
}
type Transactions struct {
	ProductID   string
	ExpiresDate int64
}

type UserSubscription struct {
	gorm.Model
	UserID    string `gorm:"type:varchar(255);not null"`
	ProjectID string `gorm:"type:varchar(255);not null"`
	ProductID string `gorm:"type:varchar(255);not null"`
	// SubscriptionID: TransactionID （后续其他支付服务商的ID）
	SubscriptionID string `gorm:"type:varchar(255);not null"`
	// 订阅状态
	SubscriptionStatus string `gorm:"type:varchar(255);not null"`
	StartDate          int64  `gorm:"type:bigint;not null"`
	ExpireDate         int64  `gorm:"type:bigint;not null"`
}
