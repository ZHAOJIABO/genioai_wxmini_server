package dao

import (
	"context"

	"gorm.io/gorm"
)

// UserDoc 用户文档模型
type UserDoc struct {
	gorm.Model
	ProjectID string
	UserID    string
	DocID     string
	Title     string
	Content   string
	Type      int    // 文档类型
	Status    int    // 状态 0-正常 1-删除
	Tags      string // 标签，用逗号分隔
}

// UserDocDao 用于处理用户文档相关数据访问
type UserDocDao struct {
	db *gorm.DB
}

// NewUserDocDao 创建 UserDocDao 实例
func NewUserDocDao(db *gorm.DB) *UserDocDao {
	return &UserDocDao{db: db}
}

// GetByID 根据ID获取用户文档
func (d *UserDocDao) GetByID(ctx context.Context, projectID, docID string) (*UserDoc, error) {
	var doc UserDoc
	err := d.db.Where("project_id = ? AND doc_id = ? AND status = 0", projectID, docID).First(&doc).Error
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// GetUserDocs 获取用户的所有文档
func (d *UserDocDao) GetUserDocs(ctx context.Context, projectID, userID string, count, offset int) ([]*UserDoc, error) {
	var docs []*UserDoc

	if offset < 1 {
		offset = 1
	}

	err := d.db.Where("project_id = ? AND user_id = ? AND status = 0", projectID, userID).
		Order("updated_at desc").
		Limit(count).
		Offset((offset - 1) * count).
		Find(&docs).Error

	return docs, err
}

// SaveDoc 保存用户文档
func (d *UserDocDao) SaveDoc(ctx context.Context, doc *UserDoc) error {
	return d.db.Save(doc).Error
}

// DeleteDoc 删除用户文档（标记删除）
func (d *UserDocDao) DeleteDoc(ctx context.Context, projectID, userID, docID string) error {
	return d.db.Model(&UserDoc{}).
		Where("project_id = ? AND user_id = ? AND doc_id = ?", projectID, userID, docID).
		Update("status", 1).Error
}

// SearchDocs 搜索用户文档
func (d *UserDocDao) SearchDocs(ctx context.Context, projectID, userID, keyword string, count, offset int) ([]*UserDoc, error) {
	var docs []*UserDoc

	if offset < 1 {
		offset = 1
	}

	// 在标题或内容中搜索关键词
	query := d.db.Where("project_id = ? AND user_id = ? AND status = 0", projectID, userID).
		Where("title LIKE ? OR content LIKE ?", "%"+keyword+"%", "%"+keyword+"%")

	err := query.Order("updated_at desc").
		Limit(count).
		Offset((offset - 1) * count).
		Find(&docs).Error

	return docs, err
}
