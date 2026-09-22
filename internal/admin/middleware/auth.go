package middleware

import (
	"context"
	"net/http"
	"strings"

	adminservice "va_visionai_server/internal/admin/service"
)

type contextKey string

const (
	ContextKeyAdminID   contextKey = "admin_id"
	ContextKeyUsername  contextKey = "admin_username"
	ContextKeyRole      contextKey = "admin_role"
)

// JWTAuth JWT认证中间件
func JWTAuth(authService *adminservice.AuthService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"code":401,"msg":"未提供认证信息"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, `{"code":401,"msg":"认证格式错误"}`, http.StatusUnauthorized)
			return
		}

		claims, err := authService.ValidateToken(parts[1])
		if err != nil {
			http.Error(w, `{"code":401,"msg":"token无效或已过期"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), ContextKeyAdminID, claims.AdminID)
		ctx = context.WithValue(ctx, ContextKeyUsername, claims.Username)
		ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetAdminID 从context获取管理员ID
func GetAdminID(ctx context.Context) uint {
	if v, ok := ctx.Value(ContextKeyAdminID).(uint); ok {
		return v
	}
	return 0
}

// GetAdminUsername 从context获取管理员用户名
func GetAdminUsername(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyUsername).(string); ok {
		return v
	}
	return ""
}
