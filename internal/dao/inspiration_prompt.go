package dao

import (
	"time"

	"gorm.io/gorm"

	"va_visionai_server/internal/model"
)

type InspirationPromptDao struct {
	db *gorm.DB
}

func NewInspirationPromptDao(db *gorm.DB) *InspirationPromptDao {
	return &InspirationPromptDao{db: db}
}

// ============ Repository Interface Implementation ============
// 以下方法实现 service.InspirationRepository 接口
// DAO层隐式实现Service层定义的接口，遵循Go最佳实践

// GetPromptsByToolType 根据工具类型获取所有启用的灵感（不筛选语言）
func (d *InspirationPromptDao) GetPromptsByToolType(toolType string) ([]*model.InspirationPrompt, error) {
	var prompts []*model.InspirationPrompt
	err := d.db.
		Where("tool_type = ? AND status = 1", toolType).
		Where("deleted_at IS NULL").
		Find(&prompts).Error
	return prompts, err
}

// GetPromptByID 根据 prompt_id 获取灵感
func (d *InspirationPromptDao) GetPromptByID(promptID string) (*model.InspirationPrompt, error) {
	var prompt model.InspirationPrompt
	err := d.db.
		Where("prompt_id = ?", promptID).
		Where("deleted_at IS NULL").
		First(&prompt).Error
	return &prompt, err
}

// CountApplicationsBatch 批量统计多个灵感在指定时间后的应用数
// 返回 map[promptID]count，未统计到的promptID自动填充为0
func (d *InspirationPromptDao) CountApplicationsBatch(promptIDs []string, since time.Time) (map[string]int64, error) {
	if len(promptIDs) == 0 {
		return make(map[string]int64), nil
	}

	type CountResult struct {
		PromptID string
		Count    int64
	}

	var results []CountResult
	err := d.db.Model(&model.InspirationApplication{}).
		Select("prompt_id, COUNT(*) as count").
		Where("prompt_id IN ? AND applied_at >= ?", promptIDs, since).
		Group("prompt_id").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	countMap := make(map[string]int64, len(promptIDs))
	for _, result := range results {
		countMap[result.PromptID] = result.Count
	}

	// 填充未统计到的 promptID (count = 0)
	for _, pid := range promptIDs {
		if _, exists := countMap[pid]; !exists {
			countMap[pid] = 0
		}
	}

	return countMap, nil
}

// CountApplications 统计指定灵感在指定时间后的应用数
func (d *InspirationPromptDao) CountApplications(promptID string, since time.Time) (int64, error) {
	var count int64
	err := d.db.Model(&model.InspirationApplication{}).
		Where("prompt_id = ? AND applied_at >= ?", promptID, since).
		Count(&count).Error
	return count, err
}

// RecordApplication 记录灵感应用
func (d *InspirationPromptDao) RecordApplication(application *model.InspirationApplication) error {
	return d.db.Create(application).Error
}

// ============ Other DAO Methods ============
// 以下方法用于管理端或其他业务场景

// Create 创建灵感提示词
func (d *InspirationPromptDao) Create(prompt *model.InspirationPrompt) error {
	return d.db.Create(prompt).Error
}

// Update 更新灵感提示词
func (d *InspirationPromptDao) Update(prompt *model.InspirationPrompt) error {
	return d.db.Save(prompt).Error
}

// Delete 软删除灵感提示词
func (d *InspirationPromptDao) Delete(promptID string) error {
	return d.db.Where("prompt_id = ?", promptID).Delete(&model.InspirationPrompt{}).Error
}

// List 列出所有灵感提示词(支持分页)
func (d *InspirationPromptDao) List(toolType, language string, offset, limit int) ([]*model.InspirationPrompt, int64, error) {
	var prompts []*model.InspirationPrompt
	var total int64

	query := d.db.Model(&model.InspirationPrompt{}).Where("deleted_at IS NULL")

	if toolType != "" {
		query = query.Where("tool_type = ?", toolType)
	}
	if language != "" {
		query = query.Where("language = ?", language)
	}

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := query.
		Order("sort_weight DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&prompts).Error

	return prompts, total, err
}
