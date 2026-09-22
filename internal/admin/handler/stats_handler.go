package handler

import (
	"net/http"
	"strconv"

	adminservice "va_visionai_server/internal/admin/service"
)

type StatsHandler struct {
	statsService *adminservice.StatsService
}

func NewStatsHandler(statsService *adminservice.StatsService) *StatsHandler {
	return &StatsHandler{statsService: statsService}
}

// Dashboard 仪表盘
func (h *StatsHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	data, err := h.statsService.GetDashboard(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "获取仪表盘数据失败")
		return
	}
	Success(w, data)
}

// TaskStats 任务统计
func (h *StatsHandler) TaskStats(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")

	data, err := h.statsService.GetTaskStats(r.Context(), start, end)
	if err != nil {
		Error(w, http.StatusInternalServerError, "获取任务统计失败")
		return
	}
	Success(w, data)
}

// DailyTasks 每日任务趋势
func (h *StatsHandler) DailyTasks(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days, _ := strconv.Atoi(daysStr)
	if days <= 0 {
		days = 7
	}

	data, err := h.statsService.GetDailyTasks(r.Context(), days)
	if err != nil {
		Error(w, http.StatusInternalServerError, "获取每日趋势失败")
		return
	}
	Success(w, data)
}

// UserStats 用户生图统计排行
func (h *StatsHandler) UserStats(w http.ResponseWriter, r *http.Request) {
	page, size := ParsePage(r)

	data, total, err := h.statsService.GetUserStats(r.Context(), page, size)
	if err != nil {
		Error(w, http.StatusInternalServerError, "获取用户统计失败")
		return
	}
	Success(w, PageData{List: data, Total: total, Page: page, Size: size})
}
