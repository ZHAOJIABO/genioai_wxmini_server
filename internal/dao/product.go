package dao

import (
	"gorm.io/gorm"

	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
)

type ProductDao struct {
	*gorm.DB
}

func NewProductDao() *ProductDao {
	return &ProductDao{
		DB: db.GetDB(),
	}
}

func (d *ProductDao) GetByProductID(productID, lang, os, projectID string) (*model.Product, error) {
	var product model.Product
	query := d.Table("pay_product").Where("product_id = ? and project_id = ?", productID, projectID)
	if lang != "" {
		query = query.Where("lang = ?", lang)
	}
	if os != "" {
		query = query.Where("os = ?", os)
	}
	err := query.First(&product).Error
	if err != nil {
		return nil, err
	}

	return &product, nil
}

func (d *ProductDao) List(lang, os, projectID, paymentWay string) ([]*model.Product, error) {
	var products []*model.Product

	query := d.Table("pay_product").Where("lang = ? and os = ? and project_id = ?", lang, os, projectID)
	if paymentWay != "" {
		query = query.Where("payment_way = ?", paymentWay)
	}
	err := query.Order("score,id").Find(&products).Error

	if err != nil {
		return nil, err
	}

	return products, nil
}

func (d *ProductDao) GetActiveProductsByGroup(groupID int, lang, os, projectID string, fetchAll bool) ([]*model.Product, error) {
	var products []*model.Product

	query := d.Table("pay_product").Where("lang = ? AND os = ? AND project_id = ? AND status = 1", lang, os, projectID)
	if !fetchAll {
		query = query.Where("group_id = ?", groupID)
	}
	err := query.Order("score,id").Find(&products).Error
	if err != nil {
		return nil, err
	}

	return products, nil
}
