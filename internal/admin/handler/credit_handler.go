package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	admindao "va_visionai_server/internal/admin/dao"
	"va_visionai_server/internal/admin/middleware"
	adminmodel "va_visionai_server/internal/admin/model"
	adminservice "va_visionai_server/internal/admin/service"
)

type CreditHandler struct {
	creditService *adminservice.CreditService
	auditLogDao   *admindao.AuditLogDao
}

func NewCreditHandler(creditService *adminservice.CreditService, auditLogDao *admindao.AuditLogDao) *CreditHandler {
	return &CreditHandler{creditService: creditService, auditLogDao: auditLogDao}
}

type addCreditsRequest struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	Amount      int64  `json:"amount"`
	Description string `json:"description"`
}

// AddCredits 给用户加积分
func (h *CreditHandler) AddCredits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}

	var req addCreditsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "请求参数错误")
		return
	}

	if req.Amount <= 0 {
		Error(w, http.StatusBadRequest, "积分数量必须大于0")
		return
	}

	userID := req.UserID
	if userID == "" && req.Email != "" {
		id, err := h.creditService.FindUserIDByEmail(r.Context(), req.Email)
		if err != nil {
			Error(w, http.StatusBadRequest, "未找到该邮箱对应的用户")
			return
		}
		userID = id
	}

	if userID == "" {
		Error(w, http.StatusBadRequest, "请提供 user_id 或 email")
		return
	}

	projectID := "com.web.genioai"
	description := req.Description
	if description == "" {
		description = "管理员手动添加积分"
	}

	err := h.creditService.AddCredits(r.Context(), projectID, userID, req.Amount, description)
	if err != nil {
		Error(w, http.StatusInternalServerError, "添加积分失败: "+err.Error())
		return
	}

	// 记录审计日志
	adminID := middleware.GetAdminID(r.Context())
	adminName := middleware.GetAdminUsername(r.Context())
	h.auditLogDao.Create(&adminmodel.AdminAuditLog{
		AdminID:    adminID,
		AdminName:  adminName,
		Action:     "add_credits",
		Resource:   "credit",
		ResourceID: userID,
		Detail:     fmt.Sprintf(`{"amount":%d,"description":"%s"}`, req.Amount, description),
		IP:         GetClientIP(r),
	})

	Success(w, map[string]interface{}{"user_id": userID, "added": req.Amount})
}

// GetBalance 获取用户积分余额
func (h *CreditHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	// /admin/api/credits/{user_id}
	path := r.URL.Path
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) < 4 {
		Error(w, http.StatusBadRequest, "缺少用户ID")
		return
	}
	userID := parts[len(parts)-1]

	projectID := "com.web.genioai"
	balance, err := h.creditService.GetBalance(r.Context(), projectID, userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "查询积分失败")
		return
	}

	Success(w, balance)
}

// GetHistory 获取积分流水
func (h *CreditHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	// /admin/api/credits/{user_id}/history
	path := r.URL.Path
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) < 5 {
		Error(w, http.StatusBadRequest, "缺少用户ID")
		return
	}
	userID := parts[len(parts)-2]

	page, size := ParsePage(r)
	projectID := "com.web.genioai"

	records, total, err := h.creditService.GetHistory(r.Context(), projectID, userID, page, size)
	if err != nil {
		Error(w, http.StatusInternalServerError, "查询积分流水失败")
		return
	}

	Success(w, PageData{List: records, Total: total, Page: page, Size: size})
}
