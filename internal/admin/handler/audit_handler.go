package handler

import (
	"net/http"

	admindao "va_visionai_server/internal/admin/dao"
)

type AuditHandler struct {
	auditLogDao *admindao.AuditLogDao
}

func NewAuditHandler(auditLogDao *admindao.AuditLogDao) *AuditHandler {
	return &AuditHandler{auditLogDao: auditLogDao}
}

// ListLogs 获取审计日志列表
func (h *AuditHandler) ListLogs(w http.ResponseWriter, r *http.Request) {
	page, size := ParsePage(r)

	logs, total, err := h.auditLogDao.List(page, size)
	if err != nil {
		Error(w, http.StatusInternalServerError, "查询审计日志失败")
		return
	}

	Success(w, PageData{List: logs, Total: total, Page: page, Size: size})
}
