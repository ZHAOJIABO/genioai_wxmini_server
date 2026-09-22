package handler

import (
	"encoding/json"
	"net/http"

	"va_visionai_server/internal/admin/middleware"
	adminservice "va_visionai_server/internal/admin/service"
)

type AuthHandler struct {
	authService *adminservice.AuthService
}

func NewAuthHandler(authService *adminservice.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token       string `json:"token"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// Login 管理员登录
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "请求参数错误")
		return
	}

	if req.Username == "" || req.Password == "" {
		Error(w, http.StatusBadRequest, "用户名和密码不能为空")
		return
	}

	ip := GetClientIP(r)
	token, user, err := h.authService.Login(req.Username, req.Password, ip)
	if err != nil {
		Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	Success(w, loginResponse{
		Token:       token,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
	})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ChangePassword 修改密码
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "请求参数错误")
		return
	}

	if req.OldPassword == "" || req.NewPassword == "" {
		Error(w, http.StatusBadRequest, "旧密码和新密码不能为空")
		return
	}

	if len(req.NewPassword) < 6 {
		Error(w, http.StatusBadRequest, "新密码长度至少6位")
		return
	}

	adminID := middleware.GetAdminID(r.Context())
	if err := h.authService.ChangePassword(adminID, req.OldPassword, req.NewPassword); err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}

	Success(w, nil)
}
