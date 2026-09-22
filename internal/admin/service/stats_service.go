package service

import (
	"context"
	"time"

	"gorm.io/gorm"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	pb "va_visionai_server/internal/va_interface"
)

type StatsService struct {
	db             *gorm.DB
	pictureTaskDao *dao.PictureTaskDao
}

func NewStatsService(db *gorm.DB, pictureTaskDao *dao.PictureTaskDao) *StatsService {
	return &StatsService{db: db, pictureTaskDao: pictureTaskDao}
}

type DashboardData struct {
	TotalUsers       int64 `json:"total_users"`
	TotalTasks       int64 `json:"total_tasks"`
	CompletedTasks   int64 `json:"completed_tasks"`
	TodayTasks       int64 `json:"today_tasks"`
	TotalCreditsUsed int64 `json:"total_credits_used"`
}

type TaskStatsData struct {
	TotalTasks     int64 `json:"total_tasks"`
	CompletedTasks int64 `json:"completed_tasks"`
	FailedTasks    int64 `json:"failed_tasks"`
	ProcessingTask int64 `json:"processing_tasks"`
}

type DailyTaskData struct {
	Date      string `json:"date"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
	Failed    int64  `json:"failed"`
}

type UserStatsData struct {
	UserID           string `json:"user_id"`
	TotalTasks       int64  `json:"total_tasks"`
	CompletedTasks   int64  `json:"completed_tasks"`
	FailedTasks      int64  `json:"failed_tasks"`
	TotalCreditsUsed int64  `json:"total_credits_used"`
	LastTaskTime     string `json:"last_task_time"`
}

// GetDashboard 获取仪表盘数据
func (s *StatsService) GetDashboard(ctx context.Context) (*DashboardData, error) {
	data := &DashboardData{}

	s.db.Model(&model.UserRecord{}).Count(&data.TotalUsers)
	s.db.Model(&model.PictureTask{}).Count(&data.TotalTasks)
	s.db.Model(&model.PictureTask{}).Where("status = ?", int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED)).Count(&data.CompletedTasks)

	today := time.Now().Format("2006-01-02")
	s.db.Model(&model.PictureTask{}).Where("DATE(created_at) = ?", today).Count(&data.TodayTasks)

	s.db.Model(&model.PictureTask{}).Select("COALESCE(SUM(credit_points), 0)").Scan(&data.TotalCreditsUsed)

	return data, nil
}

// GetTaskStats 获取任务统计
func (s *StatsService) GetTaskStats(ctx context.Context, start, end string) (*TaskStatsData, error) {
	data := &TaskStatsData{}
	query := s.db.Model(&model.PictureTask{})

	if start != "" {
		query = query.Where("created_at >= ?", start)
	}
	if end != "" {
		query = query.Where("created_at <= ?", end)
	}

	query.Count(&data.TotalTasks)
	query.Where("status = ?", int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED)).Count(&data.CompletedTasks)

	query2 := s.db.Model(&model.PictureTask{})
	if start != "" {
		query2 = query2.Where("created_at >= ?", start)
	}
	if end != "" {
		query2 = query2.Where("created_at <= ?", end)
	}
	query2.Where("status = ?", int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)).Count(&data.FailedTasks)

	data.ProcessingTask = data.TotalTasks - data.CompletedTasks - data.FailedTasks

	return data, nil
}

// GetDailyTasks 获取每日任务趋势
func (s *StatsService) GetDailyTasks(ctx context.Context, days int) ([]DailyTaskData, error) {
	if days <= 0 {
		days = 7
	}

	startDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	var results []DailyTaskData
	err := s.db.Model(&model.PictureTask{}).
		Select(`DATE(created_at) as date,
			COUNT(*) as total,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as completed,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as failed`,
			int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED),
			int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED),
		).
		Where("created_at >= ?", startDate).
		Group("DATE(created_at)").
		Order("date ASC").
		Scan(&results).Error

	return results, err
}

// GetUserStats 获取用户生图统计排行
func (s *StatsService) GetUserStats(ctx context.Context, page, size int) ([]UserStatsData, int64, error) {
	stats, err := s.pictureTaskDao.GetAllUserTaskStats(ctx)
	if err != nil {
		return nil, 0, err
	}

	total := int64(len(stats))

	// 手动分页
	start := (page - 1) * size
	if start >= len(stats) {
		return []UserStatsData{}, total, nil
	}
	end := start + size
	if end > len(stats) {
		end = len(stats)
	}

	result := make([]UserStatsData, 0, end-start)
	for _, st := range stats[start:end] {
		lastTime := ""
		if st.LastTaskTime != nil {
			lastTime = st.LastTaskTime.Format("2006-01-02 15:04:05")
		}
		result = append(result, UserStatsData{
			UserID:           st.UserID,
			TotalTasks:       st.TotalTasks,
			CompletedTasks:   st.CompletedTasks,
			FailedTasks:      st.FailedTasks,
			TotalCreditsUsed: st.TotalCreditsUsed,
			LastTaskTime:     lastTime,
		})
	}

	return result, total, nil
}
