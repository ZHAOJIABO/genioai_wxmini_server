package handler

import (
	"net/http"
	"strings"

	"va_visionai_server/internal/admin/middleware"
	adminservice "va_visionai_server/internal/admin/service"
)

type UserHandler struct {
	userService *adminservice.UserService
}

func NewUserHandler(userService *adminservice.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// ListUsers 用户列表
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	page, size := ParsePage(r)
	search := r.URL.Query().Get("search")

	users, total, err := h.userService.ListUsers(r.Context(), page, size, search)
	if err != nil {
		Error(w, http.StatusInternalServerError, "查询用户失败")
		return
	}

	Success(w, PageData{List: users, Total: total, Page: page, Size: size})
}

// GetUser 用户详情
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	// 从路径中提取 user_id: /admin/api/users/{id}
	path := r.URL.Path
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) < 4 {
		Error(w, http.StatusBadRequest, "缺少用户ID")
		return
	}
	userID := parts[len(parts)-1]

	user, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusNotFound, "用户不存在")
		return
	}

	Success(w, user)
}

// BanUser 封禁用户
func (h *UserHandler) BanUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}

	// /admin/api/users/{id}/ban
	path := r.URL.Path
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) < 5 {
		Error(w, http.StatusBadRequest, "缺少用户ID")
		return
	}
	userID := parts[len(parts)-2]

	_ = middleware.GetAdminID(r.Context())

	err := h.userService.BanUser(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "封禁用户失败")
		return
	}

	Success(w, nil)
}

// UnbanUser 解封用户
func (h *UserHandler) UnbanUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}

	// /admin/api/users/{id}/unban
	path := r.URL.Path
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) < 5 {
		Error(w, http.StatusBadRequest, "缺少用户ID")
		return
	}
	userID := parts[len(parts)-2]

	err := h.userService.UnbanUser(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "解封用户失败")
		return
	}

	Success(w, nil)
}
