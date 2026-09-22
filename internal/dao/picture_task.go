package dao

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"va_visionai_server/internal/model"
	pb "va_visionai_server/internal/va_interface"
)

type PictureTaskDao struct {
	db *gorm.DB
}

// PictureTaskBrief 用于积分明细中展示的任务摘要信息
type PictureTaskBrief struct {
	TaskID     string
	UserPrompt string
	ResultURLs []string
}

func NewPictureTaskDao(db *gorm.DB) *PictureTaskDao {
	return &PictureTaskDao{db: db}
}

// CreateTask 创建任务
func (d *PictureTaskDao) CreateTask(ctx context.Context, tx *gorm.DB, task *model.PictureTask) error {
	return errors.Wrap(
		tx.WithContext(ctx).Create(task).Error,
		"create picture task",
	)
}

// GetTask 获取任务
func (d *PictureTaskDao) GetTask(ctx context.Context, taskID string) (*model.PictureTask, error) {
	var task model.PictureTask
	err := d.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		First(&task).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.New("task not found")
		}
		return nil, errors.Wrap(err, "get picture task")
	}
	return &task, nil
}

// GetTaskForUpdate 获取任务并锁定记录以供更新
func (d *PictureTaskDao) GetTaskForUpdate(ctx context.Context, tx *gorm.DB, taskID string) (*model.PictureTask, error) {
	var task model.PictureTask
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("task_id = ?", taskID).
		First(&task).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.New("task not found")
		}
		return nil, errors.Wrap(err, "get picture task for update")
	}
	return &task, nil
}

// UpdateTask 更新任务
func (d *PictureTaskDao) UpdateTask(ctx context.Context, task *model.PictureTask) error {
	return errors.Wrap(
		d.db.WithContext(context.Background()).
			Select("*").
			Omit("created_at", "deleted_at").
			Save(task).Error,
		"update picture task",
	)
}

// UpdateTaskPublishStatus 更新发布状态
func (d *PictureTaskDao) UpdateTaskPublishStatus(ctx context.Context, taskID string, isPublished bool) error {
	return errors.Wrap(
		d.db.WithContext(ctx).
			Model(&model.PictureTask{}).
			Where("task_id = ?", taskID).
			Updates(map[string]interface{}{
				"is_published": isPublished,
				"updated_at":   time.Now(),
			}).Error,
		"update publish status",
	)
}

// ListPublicPictures 获取公开的图片列表
func (d *PictureTaskDao) ListPublicPictures(ctx context.Context, offset, limit int) ([]*model.PictureTask, int64, error) {
	var total int64
	var tasks []*model.PictureTask

	// 先获取总数
	err := d.db.WithContext(ctx).
		Model(&model.PictureTask{}).
		Where("is_published = ? AND status = ?", 1, int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED)).
		Count(&total).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "count public pictures")
	}

	// 获取列表
	err = d.db.WithContext(ctx).
		Where("is_published = ? AND status = ?", 1, int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED)).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "list public pictures")
	}

	return tasks, total, nil
}

// ListUserPictures 获取用户的图片列表
func (d *PictureTaskDao) ListUserPictures(ctx context.Context, userID string) ([]*model.PictureTask, error) {
	var tasks []*model.PictureTask
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED)).
		Order("created_at DESC").
		Limit(10).
		Find(&tasks).Error

	return tasks, errors.Wrap(err, "list user pictures")
}

// DeleteTask 删除任务
func (d *PictureTaskDao) DeleteTask(ctx context.Context, taskID, userID string) error {
	return d.DeleteTasks(ctx, []string{taskID}, userID)
}

// DeleteTasks 批量删除任务
func (d *PictureTaskDao) DeleteTasks(ctx context.Context, taskIDs []string, userID string) error {
	if len(taskIDs) == 0 {
		return errors.New("no task IDs provided")
	}

	// 使用事务确保原子性
	return errors.Wrap(
		d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// 先检查任务是否存在且属于该用户
			var count int64
			if err := tx.Model(&model.PictureTask{}).
				Where("task_id IN ? AND user_id = ?", taskIDs, userID).
				Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return errors.New("no tasks found or no permission")
			}
			if int(count) != len(taskIDs) {
				return errors.New("some tasks not found or no permission")
			}

			// 执行批量软删除
			return tx.Where("task_id IN ? AND user_id = ?", taskIDs, userID).
				Delete(&model.PictureTask{}).Error
		}),
		"delete tasks",
	)
}

func (d *PictureTaskDao) GetRecentTasks(ctx context.Context, userID string, theme string, kindType string, taskMode *int8, page, pageSize int32) ([]*model.PictureTask, int64, error) {
	var total int64
	var tasks []*model.PictureTask

	query := d.db.WithContext(ctx).Model(&model.PictureTask{}).Where("user_id = ?", userID)

	// 过滤掉失败的任务
	query = query.Where("status != ?", int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED))

	if theme != "" {
		query = query.Where("theme = ?", theme)
	}

	// 添加 WorkflowKindType 过滤
	if kindType != "" {
		// 直接使用 workflow_type 字段过滤
		query = query.Where("workflow_type = ?", kindType)
	}

	// 添加 TaskMode 过滤
	if taskMode != nil {
		query = query.Where("task_mode = ?", *taskMode)
	}

	// 先获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "count user tasks")
	}

	offset := int((page - 1) * pageSize)
	limit := int(pageSize)
	if limit == -1 {
		limit = int(total)
	}

	// 获取列表
	err = query.
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&tasks).Error

	return tasks, total, errors.Wrap(err, "list recent tasks")
}

// UpdateTaskCounts 一次性更新任务的真实浏览次数、放大后的浏览次数和随机数种子
func (d *PictureTaskDao) UpdateTaskCounts(ctx context.Context, taskID string, viewCount, fakeViewCount int32, viewCountSeed int64) error {
	return errors.Wrap(
		d.db.WithContext(ctx).
			Model(&model.PictureTask{}).
			Where("task_id = ?", taskID).
			Updates(map[string]interface{}{
				"view_count":      viewCount,
				"fake_view_count": fakeViewCount,
				"view_count_seed": viewCountSeed,
			}).Error,
		"update task counts",
	)
}

// CountActiveTasksByUser 统计用户活跃（未完成）任务数
func (d *PictureTaskDao) CountActiveTasksByUser(ctx context.Context, tx *gorm.DB, userID string) (int64, error) {
	var count int64
	activeStatuses := []int32{
		int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING),
		int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING),
		int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING_SUBMISSION_RETRY),
		int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION),
	}
	dbToUse := d.db
	if tx != nil {
		dbToUse = tx
	}
	err := dbToUse.WithContext(ctx).Model(&model.PictureTask{}).
		Where("user_id = ? AND status IN (?)", userID, activeStatuses).
		Count(&count).Error
	if err != nil {
		return 0, errors.Wrap(err, "failed to count active tasks by user")
	}
	return count, nil
}

// GetPendingTasks 获取处于 PENDING 状态的任务，按创建时间升序排列
func (d *PictureTaskDao) GetPendingTasks(ctx context.Context, limit int) ([]*model.PictureTask, error) {
	var tasks []*model.PictureTask
	err := d.db.WithContext(ctx).
		Where("status = ?", int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING)).
		Order("created_at ASC").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil, errors.Wrap(err, "get pending tasks")
	}
	return tasks, nil
}

// GetTasksByStatusAndMaxRetries retrieves tasks with a specific status and retry count less than maxRetries.
func (d *PictureTaskDao) GetTasksByStatusAndMaxRetries(ctx context.Context, status pb.WorkflowTaskStatus, maxRetries int32, limit int) ([]*model.PictureTask, error) {
	var tasks []*model.PictureTask
	err := d.db.WithContext(ctx).
		Where("status = ? AND retry_count < ?", int32(status), maxRetries).
		Order("updated_at ASC").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil, errors.Wrap(err, "failed to get tasks by status and max retries")
	}
	return tasks, nil
}

// GetTasksByStatus retrieves tasks with a specific status.
func (d *PictureTaskDao) GetTasksByStatus(ctx context.Context, status pb.WorkflowTaskStatus, limit int) ([]*model.PictureTask, error) {
	var tasks []*model.PictureTask
	err := d.db.WithContext(ctx).
		Where("status = ?", int32(status)).
		Order("updated_at ASC").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil, errors.Wrap(err, "failed to get tasks by status")
	}
	return tasks, nil
}

// CountProcessingTasksByUser 统计用户正在执行的任务数（仅PROCESSING状态）
func (d *PictureTaskDao) CountProcessingTasksByUser(ctx context.Context, tx *gorm.DB, userID string) (int64, error) {
	var count int64
	dbToUse := d.db
	if tx != nil {
		dbToUse = tx
	}
	err := dbToUse.WithContext(ctx).Model(&model.PictureTask{}).
		Where("user_id = ? AND status = ?", userID, int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING)).
		Count(&count).Error
	if err != nil {
		return 0, errors.Wrap(err, "failed to count processing tasks by user")
	}
	return count, nil
}

// GetDB 返回数据库连接（用于TaskStateManager访问）
func (d *PictureTaskDao) GetDB() *gorm.DB {
	return d.db
}

// WorkflowUsageStats 工作流使用统计
type WorkflowUsageStats struct {
	WorkflowID string
	UsageCount int64
}

// GetTop10HotWorkflowsLast7Days 获取近7天使用率Top10的热门workflow
// 统计近7天内状态为processing或success的任务,按workflow_id分组统计使用次数
func (d *PictureTaskDao) GetTop10HotWorkflowsLast7Days(ctx context.Context, limit int) ([]WorkflowUsageStats, error) {
	var results []WorkflowUsageStats

	// 计算7天前的时间
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)

	// 有效状态: processing和success
	validStatuses := []int32{
		int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING),
		int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED),
	}

	err := d.db.WithContext(ctx).
		Model(&model.PictureTask{}).
		Select("workflow_id, COUNT(*) as usage_count").
		Where("created_at >= ?", sevenDaysAgo).
		Where("status IN ?", validStatuses).
		Group("workflow_id").
		Order("usage_count DESC").
		Limit(limit).
		Scan(&results).Error

	if err != nil {
		return nil, errors.Wrap(err, "failed to get top hot workflows")
	}

	return results, nil
}

// GetTasksByTaskIDs 根据 task_id 列表批量查询任务（仅返回 task_id、user_prompt、result_json）
func (d *PictureTaskDao) GetTasksByTaskIDs(ctx context.Context, taskIDs []string) ([]*model.PictureTask, error) {
	if len(taskIDs) == 0 {
		return nil, nil
	}
	var tasks []*model.PictureTask
	err := d.db.WithContext(ctx).
		Select("task_id, user_prompt, result_json").
		Where("task_id IN ?", taskIDs).
		Find(&tasks).Error
	if err != nil {
		return nil, errors.Wrap(err, "get tasks by task IDs")
	}
	return tasks, nil
}

// UserTaskStats 用户生图统计
type UserTaskStats struct {
	UserID              string
	TotalTasks          int64
	CompletedTasks      int64
	FailedTasks         int64
	ProcessingTasks     int64
	TotalCreditsUsed    int64
	LastTaskTime        *time.Time
}

// GetAllUserTaskStats 获取所有用户的生图统计数据
func (d *PictureTaskDao) GetAllUserTaskStats(ctx context.Context) ([]UserTaskStats, error) {
	var results []UserTaskStats

	err := d.db.WithContext(ctx).
		Model(&model.PictureTask{}).
		Select(`user_id,
			COUNT(*) as total_tasks,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as completed_tasks,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as failed_tasks,
			SUM(CASE WHEN status IN (?,?,?,?) THEN 1 ELSE 0 END) as processing_tasks,
			COALESCE(SUM(credit_points), 0) as total_credits_used,
			MAX(created_at) as last_task_time`,
			int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED),
			int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED),
			int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING),
			int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING),
			int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING_SUBMISSION_RETRY),
			int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION),
		).
		Group("user_id").
		Order("total_tasks DESC").
		Scan(&results).Error

	if err != nil {
		return nil, errors.Wrap(err, "failed to get all user task stats")
	}

	return results, nil
}
