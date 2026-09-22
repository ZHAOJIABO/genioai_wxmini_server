package model

import (
	"time"

	"gorm.io/gorm"
)

type UserAmount struct {
	gorm.Model

	// SourceID 是授予这些积分的事件的唯一标识符（例如 payment_id、order_id）。
	// 它确保了积分授予的幂等性。
	SourceID string `gorm:"type:varchar(255);not null;uniqueIndex"`

	// 额度类型标识, e.g., "purchased", "subscription_gift"
	// Identifier for the credit type.
	AmountIden string `gorm:"type:varchar(50);not null;index"`
	// 项目ID
	// Project ID
	ProjectID string `gorm:"type:varchar(255);not null;index"`
	// 用户ID
	// User ID
	UserID string `gorm:"type:varchar(255);not null;index"`
	// 剩余额度
	// Remaining credits.
	RemainingAmount int64 `gorm:"type:bigint;not null"`
	// 额度有效期. For "purchased" type, it could be a very far future date.
	// Expiration date of the credits.
	ExpiredAt time.Time `gorm:"not null;index"`
	// 信用积分等级
	SubscribeLevel int `gorm:"type:int;not null"`
}
