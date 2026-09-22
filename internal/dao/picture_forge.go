package dao

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/model"
)

type PictureForgeDao struct {
	db *gorm.DB
}

func NewPictureForgeDao(db *gorm.DB) *PictureForgeDao {
	return &PictureForgeDao{
		db: db,
	}
}

func (d *PictureForgeDao) IncrementWorkflowRefCount(ctx context.Context, tx *gorm.DB, workflowID string) error {
	return errors.Wrap(
		tx.WithContext(ctx).
			Model(&model.Workflow{}).
			Where("workflow_id = ?", workflowID).
			Update("ref_count", gorm.Expr("ref_count + ?", 1)).Error,
		"increment workflow ref count",
	)
}

// ListWorkflowKinds gets all workflow kinds with multi-theme support
func (d *PictureForgeDao) ListWorkflowKinds(ctx context.Context, lang, kindType, theme string) ([]*model.WorkflowKind, error) {
	var kinds []*model.WorkflowKind
	query := d.db.WithContext(ctx).Model(&model.WorkflowKind{})

	if lang != "" {
		query = query.Where("language = ?", lang)
	}
	if kindType != "" {
		query = query.Where("kind_type = ?", kindType)
	}
	if theme != "" {
		// Support multi-theme query using MySQL JSON_CONTAINS
		query = query.Where("JSON_CONTAINS(theme, JSON_QUOTE(?))", theme)
	}

	query = query.Where("banner_type = ? and status = ?", constants.WorkflowKindTypeNormal, 1)
	err := query.Order("sort_order ASC").Find(&kinds).Error
	return kinds, errors.Wrap(err, "list workflow kinds")
}

// ListAllWorkflows gets all workflows with given kind IDs
func (d *PictureForgeDao) ListAllWorkflows(ctx context.Context, kindIDs []string) ([]*model.Workflow, error) {
	var workflows []*model.Workflow
	if err := d.db.WithContext(ctx).
		Where("kind_id IN ? AND status = ?", kindIDs, 1).
		Order("sort_order asc").
		Find(&workflows).Error; err != nil {
		return nil, errors.Wrap(err, "query workflows")
	}
	return workflows, nil
}

// GetWorkflow gets single workflow by ID
func (d *PictureForgeDao) GetWorkflow(ctx context.Context, tx *gorm.DB, workflowID string) (*model.Workflow, error) {
	var workflow model.Workflow
	if err := tx.WithContext(ctx).
		Where("workflow_id = ? AND status = ?", workflowID, 1).
		First(&workflow).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.New("workflow not found")
		}
		return nil, errors.Wrap(err, "query workflow")
	}
	return &workflow, nil
}

// ListWorkflowsByIDs 根据workflow ID列表批量获取工作流
func (d *PictureForgeDao) ListWorkflowsByIDs(ctx context.Context, workflowIDs []string) ([]*model.Workflow, error) {
	if len(workflowIDs) == 0 {
		return []*model.Workflow{}, nil
	}

	var workflows []*model.Workflow
	err := d.db.WithContext(ctx).
		Where("workflow_id IN ? AND status = ?", workflowIDs, 1).
		Find(&workflows).Error
	return workflows, errors.Wrap(err, "list workflows by ids")
}

// ListWorkflows 获取工作流列表（分页，支持多种筛选）
func (d *PictureForgeDao) ListWorkflows(ctx context.Context, kindID, theme, kindType string, offset, limit int) ([]*model.Workflow, int64, error) {
	var total int64
	var workflows []*model.Workflow

	query := d.db.WithContext(ctx).Model(&model.Workflow{})

	// 始终 JOIN workflow_kind 表以确保可以检查 kind 的状态
	query = query.Joins("JOIN va_workflow_kind ON va_workflow_kind.kind_id = va_workflow.kind_id")

	if kindID != "" {
		query = query.Where("va_workflow.kind_id = ?", kindID)
	}
	if theme != "" {
		// Support multi-theme query for workflows using MySQL JSON_CONTAINS
		query = query.Where("JSON_CONTAINS(va_workflow_kind.theme, JSON_QUOTE(?))", theme)
	}
	if kindType != "" {
		query = query.Where("va_workflow_kind.kind_type = ?", kindType)
	}

	// 检查 workflow 和 workflow_kind 的状态都为 1（启用状态）
	query = query.Where("va_workflow.status = ? AND va_workflow_kind.status = ?", 1, 1)

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "count workflows")
	}

	// 获取分页数据
	if limit != -1 {
		query = query.Offset(offset).Limit(limit)
	}

	err = query.Order("va_workflow.sort_order ASC").Find(&workflows).Error

	return workflows, total, errors.Wrap(err, "list workflows")
}

// ListBannerWorkflowKinds 获取banner工作流分类列表
func (d *PictureForgeDao) ListBannerWorkflowKinds(ctx context.Context, lang string) ([]*model.WorkflowKind, error) {
	var kinds []*model.WorkflowKind
	query := d.db.WithContext(ctx).
		Where("banner_type != ?", constants.WorkflowKindTypeNormal)

	if lang != "" {
		query = query.Where("language = ?", lang)
	}

	err := query.Find(&kinds).Error
	return kinds, errors.Wrap(err, "list banner workflow kinds")
}

// ListWorkflowKindsByIDs gets workflow kinds by a list of IDs.
func (d *PictureForgeDao) ListWorkflowKindsByIDs(ctx context.Context, kindIDs []string) ([]*model.WorkflowKind, error) {
	var kinds []*model.WorkflowKind
	if len(kindIDs) == 0 {
		return kinds, nil
	}
	err := d.db.WithContext(ctx).
		Where("kind_id IN ?", kindIDs).
		Find(&kinds).Error
	return kinds, errors.Wrap(err, "list workflow kinds by ids")
}

// GetWorkflowKind 根据ID获取工作流分类信息
func (d *PictureForgeDao) GetWorkflowKind(ctx context.Context, kindID string) (*model.WorkflowKind, error) {
	var kind model.WorkflowKind
	if err := d.db.
		Where("kind_id = ?", kindID).
		First(&kind).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.New("workflow kind not found")
		}
		return nil, errors.Wrap(err, "query workflow kind")
	}
	return &kind, nil
}

// GetWorkflowUsageCounts 获取指定时间以来的工作流使用次数
func (d *PictureForgeDao) GetWorkflowUsageCounts(ctx context.Context, since time.Time) (map[string]int64, error) {
	var results []struct {
		WorkflowID string `json:"workflow_id"`
		Count      int64  `json:"count"`
	}

	err := d.db.WithContext(ctx).Model(&model.PictureTask{}).
		Select("workflow_id, COUNT(*) as count").
		Where("created_at >= ? AND deleted_at IS NULL", since).
		Group("workflow_id").
		Find(&results).Error

	if err != nil {
		return nil, errors.Wrap(err, "query workflow usage counts")
	}

	usageCounts := make(map[string]int64)
	for _, result := range results {
		usageCounts[result.WorkflowID] = result.Count
	}

	return usageCounts, nil
}

// ListPictureTools lists all picture tools
func (d *PictureForgeDao) ListPictureTools(ctx context.Context) ([]*model.PictureTools, error) {
	var tools []*model.PictureTools
	err := d.db.WithContext(ctx).
		Order("sort_order ASC").
		Find(&tools).Error
	return tools, errors.Wrap(err, "list picture tools")
}

// GetPictureToolByID gets a picture tool by tool_id
func (d *PictureForgeDao) GetPictureToolByID(ctx context.Context, toolID string) (*model.PictureTools, error) {
	var tool model.PictureTools
	err := d.db.WithContext(ctx).
		Where("tool_id = ? AND deleted_at IS NULL", toolID).
		First(&tool).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.New("picture tool not found")
		}
		return nil, errors.Wrap(err, "get picture tool by tool_id")
	}
	return &tool, nil
}

// GetPictureToolByWorkflowID gets a picture tool by workflow ID
func (d *PictureForgeDao) GetPictureToolByWorkflowID(ctx context.Context, workflowID string) (*model.PictureTools, error) {
	var tool model.PictureTools
	err := d.db.WithContext(ctx).
		Where("ref_workflow_id = ? AND deleted_at IS NULL", workflowID).
		First(&tool).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.New("picture tool not found")
		}
		return nil, errors.Wrap(err, "get picture tool by workflow id")
	}
	return &tool, nil
}

// ListPictureToolCapabilities 根据工具ID获取工具能力列表
func (d *PictureForgeDao) ListPictureToolCapabilities(ctx context.Context, toolID string) ([]*model.PictureToolCapability, error) {
	var capabilities []*model.PictureToolCapability
	err := d.db.WithContext(ctx).
		Where("tool_id = ? AND deleted_at IS NULL", toolID).
		Order("sort_order ASC, id ASC").
		Find(&capabilities).Error
	return capabilities, errors.Wrap(err, "list picture tool capabilities")
}

// CreatePictureToolCapability 创建工具能力
func (d *PictureForgeDao) CreatePictureToolCapability(ctx context.Context, capability *model.PictureToolCapability) error {
	return errors.Wrap(
		d.db.WithContext(ctx).Create(capability).Error,
		"create picture tool capability",
	)
}

// UpdatePictureToolCapability 更新工具能力
func (d *PictureForgeDao) UpdatePictureToolCapability(ctx context.Context, capability *model.PictureToolCapability) error {
	return errors.Wrap(
		d.db.WithContext(ctx).Save(capability).Error,
		"update picture tool capability",
	)
}

// DeletePictureToolCapability 删除工具能力（软删除）
func (d *PictureForgeDao) DeletePictureToolCapability(ctx context.Context, id uint) error {
	return errors.Wrap(
		d.db.WithContext(ctx).Delete(&model.PictureToolCapability{}, id).Error,
		"delete picture tool capability",
	)
}

// CreateCustomPromptHistory 创建自定义 Prompt 历史
func (d *PictureForgeDao) CreateCustomPromptHistory(ctx context.Context, history *model.CustomPromptHistory) error {
	return errors.Wrap(
		d.db.WithContext(ctx).Create(history).Error,
		"create custom prompt history",
	)
}

// ListCustomPromptHistoryByToolType 按用户与工具类型查询历史（倒序，限制数量）
func (d *PictureForgeDao) ListCustomPromptHistoryByToolType(ctx context.Context, userID, toolType string, limit int) ([]*model.CustomPromptHistory, error) {
	var list []*model.CustomPromptHistory
	q := d.db.WithContext(ctx).Model(&model.CustomPromptHistory{}).
		Where("user_id = ? AND tool_type = ?", userID, toolType).
		Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&list).Error
	return list, errors.Wrap(err, "list custom prompt history by tool type")
}

// DeleteCustomPromptHistory 按用户与历史ID删除
func (d *PictureForgeDao) DeleteCustomPromptHistory(ctx context.Context, userID, historyID string) error {
	result := d.db.WithContext(ctx).
		Where("user_id = ? AND history_id = ?", userID, historyID).
		Delete(&model.CustomPromptHistory{})

	if result.Error != nil {
		return errors.Wrap(result.Error, "delete custom prompt history")
	}

	if result.RowsAffected == 0 {
		return errors.New("custom prompt history not found")
	}

	return nil
}

// CheckPromptHashExists 检查指定用户和工具类型下是否存在相同哈希值的 Prompt
// 用于快速判断 Prompt 是否重复，避免全文对比的性能开销
//
// 参数:
//   - userID: 用户ID
//   - toolType: 工具类型
//   - promptHash: Prompt 的 SHA256 哈希值
//
// 返回:
//   - bool: true 表示存在重复，false 表示不存在
//   - error: 查询错误
func (d *PictureForgeDao) CheckPromptHashExists(ctx context.Context, userID, toolType, promptHash string) (bool, error) {
	var count int64
	err := d.db.WithContext(ctx).
		Model(&model.CustomPromptHistory{}).
		Where("user_id = ? AND tool_type = ? AND prompt_hash = ?", userID, toolType, promptHash).
		Count(&count).Error

	if err != nil {
		return false, errors.Wrap(err, "check prompt hash exists")
	}

	return count > 0, nil
}

// ==================== Banner相关DAO方法 ====================

// ListBanners 获取Banner列表
func (d *PictureForgeDao) ListBanners(ctx context.Context) ([]*model.BannerConfig, error) {
	var banners []*model.BannerConfig

	err := d.db.WithContext(ctx).
		Where("status = ?", 1). // 上架状态
		Order("sort_order ASC, id ASC").
		Find(&banners).Error

	return banners, errors.Wrap(err, "list banners")
}

// GetBannerByKey 根据banner_key获取Banner配置
func (d *PictureForgeDao) GetBannerByKey(ctx context.Context, bannerKey string) (*model.BannerConfig, error) {
	var banner model.BannerConfig
	err := d.db.WithContext(ctx).
		Where("banner_key = ?", bannerKey).
		First(&banner).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("banner not found")
		}
		return nil, errors.Wrap(err, "get banner by key")
	}

	return &banner, nil
}

// GetBannerClickRecord 获取用户的Banner点击记录
func (d *PictureForgeDao) GetBannerClickRecord(ctx context.Context, userID, bannerKey string) (*model.BannerClickRecord, error) {
	var record model.BannerClickRecord
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND banner_key = ?", userID, bannerKey).
		First(&record).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 没有记录返回nil，不返回错误
		}
		return nil, errors.Wrap(err, "get banner click record")
	}

	return &record, nil
}

// CreateBannerClickRecord 创建Banner点击记录（使用事务）
func (d *PictureForgeDao) CreateBannerClickRecord(ctx context.Context, tx *gorm.DB, record *model.BannerClickRecord) error {
	return errors.Wrap(
		tx.WithContext(ctx).Create(record).Error,
		"create banner click record",
	)
}

// GetDB 获取数据库实例（用于事务）
func (d *PictureForgeDao) GetDB() *gorm.DB {
	return d.db
}

// ==================== ToolGroup相关DAO方法 ====================

// ListToolGroups 获取所有工具分组（按排序）
func (d *PictureForgeDao) ListToolGroups(ctx context.Context) ([]*model.ToolGroup, error) {
	var groups []*model.ToolGroup
	err := d.db.WithContext(ctx).
		Order("sort_order ASC, id ASC").
		Find(&groups).Error
	return groups, errors.Wrap(err, "list tool groups")
}

// GetToolGroupByID 根据分组ID获取分组
func (d *PictureForgeDao) GetToolGroupByID(ctx context.Context, groupID string) (*model.ToolGroup, error) {
	var group model.ToolGroup
	err := d.db.WithContext(ctx).
		Where("group_id = ?", groupID).
		First(&group).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("tool group not found")
		}
		return nil, errors.Wrap(err, "get tool group by id")
	}

	return &group, nil
}
