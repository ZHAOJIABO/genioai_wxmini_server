package admin

import (
	"net/http"

	"gorm.io/gorm"

	admindao "va_visionai_server/internal/admin/dao"
	adminhandler "va_visionai_server/internal/admin/handler"
	"va_visionai_server/internal/admin/middleware"
	adminservice "va_visionai_server/internal/admin/service"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service/credit"

	"va_visionai_server/admin_ui"
)

// RegisterAdminRoutes 注册后台管理路由
func RegisterAdminRoutes(mux *http.ServeMux, db *gorm.DB, repos *dao.Repositories, creditSvc credit.Service) {
	// 初始化 DAO
	adminUserDao := admindao.NewAdminUserDao(db)
	auditLogDao := admindao.NewAuditLogDao(db)

	// 确保存在默认管理员
	_ = adminUserDao.EnsureDefaultAdmin()

	// 初始化 Service
	authService := adminservice.NewAuthService(adminUserDao)
	userService := adminservice.NewUserService(db, repos.User)
	creditService := adminservice.NewCreditService(db, creditSvc)
	statsService := adminservice.NewStatsService(db, repos.Task)
	configService := adminservice.NewConfigService(db)

	// 初始化 Handler
	authHandler := adminhandler.NewAuthHandler(authService)
	userHandler := adminhandler.NewUserHandler(userService)
	creditHandler := adminhandler.NewCreditHandler(creditService, auditLogDao)
	statsHandler := adminhandler.NewStatsHandler(statsService)
	configHandler := adminhandler.NewConfigHandler(configService, auditLogDao)
	auditHandler := adminhandler.NewAuditHandler(auditLogDao)

	// 公开接口（无需认证）
	mux.HandleFunc("/admin/api/login", authHandler.Login)

	// 需要认证的接口
	authed := http.NewServeMux()
	authed.HandleFunc("/admin/api/change-password", authHandler.ChangePassword)
	authed.HandleFunc("/admin/api/dashboard", statsHandler.Dashboard)
	authed.HandleFunc("/admin/api/users", userHandler.ListUsers)
	authed.HandleFunc("/admin/api/users/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case len(path) > len("/admin/api/users/") && hasSuffix(path, "/ban"):
			userHandler.BanUser(w, r)
		case len(path) > len("/admin/api/users/") && hasSuffix(path, "/unban"):
			userHandler.UnbanUser(w, r)
		default:
			userHandler.GetUser(w, r)
		}
	})
	authed.HandleFunc("/admin/api/credits/add", creditHandler.AddCredits)
	authed.HandleFunc("/admin/api/credits/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if hasSuffix(path, "/history") {
			creditHandler.GetHistory(w, r)
		} else {
			creditHandler.GetBalance(w, r)
		}
	})
	authed.HandleFunc("/admin/api/stats/tasks", statsHandler.TaskStats)
	authed.HandleFunc("/admin/api/stats/tasks/daily", statsHandler.DailyTasks)
	authed.HandleFunc("/admin/api/stats/users", statsHandler.UserStats)
	authed.HandleFunc("/admin/api/configs", configHandler.ListConfigs)
	authed.HandleFunc("/admin/api/configs/", configHandler.UpdateConfig)
	authed.HandleFunc("/admin/api/audit-logs", auditHandler.ListLogs)

	// 用JWT中间件包装所有认证路由
	mux.Handle("/admin/api/", middleware.JWTAuth(authService, authed))

	// 静态文件服务 (SPA)
	mux.Handle("/admin/", http.StripPrefix("/admin/", admin_ui.Handler()))
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
