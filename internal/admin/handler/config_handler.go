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

type ConfigHandler struct {
	configService *adminservice.ConfigService
	auditLogDao   *admindao.AuditLogDao
}

func NewConfigHandler(configService *adminservice.ConfigService, auditLogDao *admindao.AuditLogDao) *ConfigHandler {
	return &ConfigHandler{configService: configService, auditLogDao: auditLogDao}
}

// ListConfigs 配置列表
func (h *ConfigHandler) ListConfigs(w http.ResponseWriter, r *http.Request) {
	configs, err := h.configService.ListConfigs(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "获取配置失败")
		return
	}
	Success(w, configs)
}

type updateConfigRequest struct {
	Value string `json:"value"`
}

// UpdateConfig 更新配置
func (h *ConfigHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		Error(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}

	// /admin/api/configs/{key}
	path := r.URL.Path
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) < 4 {
		Error(w, http.StatusBadRequest, "缺少配置key")
		return
	}
	key := parts[len(parts)-1]

	var req updateConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "请求参数错误")
		return
	}

	err := h.configService.UpdateConfig(r.Context(), key, req.Value)
	if err != nil {
		Error(w, http.StatusInternalServerError, "更新配置失败")
		return
	}

	// 记录审计日志
	adminID := middleware.GetAdminID(r.Context())
	adminName := middleware.GetAdminUsername(r.Context())
	h.auditLogDao.Create(&adminmodel.AdminAuditLog{
		AdminID:    adminID,
		AdminName:  adminName,
		Action:     "update_config",
		Resource:   "config",
		ResourceID: key,
		Detail:     fmt.Sprintf(`{"key":"%s","value":"%s"}`, key, req.Value),
		IP:         GetClientIP(r),
	})

	Success(w, nil)
}
