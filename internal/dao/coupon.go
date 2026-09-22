package dao

import (
	"context"

	"gorm.io/gorm"
)

type CouponDao struct {
	db *gorm.DB
}

func NewCouponDao(db *gorm.DB) *CouponDao {
	return &CouponDao{db: db}
}

const (
	TableCouponLog = "pay_coupon_redemption_log"
)

// 兑换过优惠券
func (d *CouponDao) RedeemedCoupon(ctx context.Context, projectID, userID string, couponCode []string) bool {
	var count int64
	err := d.db.Table(TableCouponLog).Where("project_id = ? AND user_id = ? AND coupon_code in (?)", projectID, userID, couponCode).Count(&count).Error
	if err != nil {
		return false
	}

	return count > 0
}
