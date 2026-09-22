package dao

import (
	"gorm.io/gorm"

	"va_visionai_server/internal/model"
	"va_visionai_server/internal/va_interface"
)

type OrderDao struct {
	DB *gorm.DB
}

func NewOrderDao(db *gorm.DB) *OrderDao {
	return &OrderDao{DB: db}
}

func (d *OrderDao) ListOrderByUserID(userID string) ([]*model.Order, error) {
	var orders []*model.Order
	err := d.DB.Table("pay_order").Where("user_id = ?  and status = ", userID, va_interface.PaymentStatus_PaymentStatusSuccess).
		Find(&orders).Error
	return orders, err
}

func (d *OrderDao) GetSubscribeByTransaction(tx *gorm.DB, userID, orderID, transactionID string) (string, error) {
	// get apple_notify by transaction_id
	var productID string
	err := tx.Table("pay_apple_notify").Select("product_id").Where("transaction_id = ?", transactionID).First(&productID).Error
	if err != nil {
		return "", err
	}
	return productID, nil
}

func (d *OrderDao) GetOrderByID(orderID string) (*model.Order, error) {
	var order *model.Order
	err := d.DB.Table("pay_order").Where("order_id = ?", orderID).First(&order).Error
	if err != nil {
		return nil, err
	}
	return order, nil
}
