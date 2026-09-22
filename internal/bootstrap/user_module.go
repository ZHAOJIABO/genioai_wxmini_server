package bootstrap

import (
	"fmt"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service"
)

// UserModuleDeps 用户模块依赖
type UserModuleDeps struct {
	UserService             *service.UserService
	UserPersonalInfoService *service.UserPersonalInfoService
}

// InitUserModule 初始化用户模块相关服务
func InitUserModule(repos *dao.Repositories) (*UserModuleDeps, error) {
	userService, err := service.NewUserService(repos.User, repos.UserAmount, service.NewDeviceService())
	if err != nil {
		return nil, fmt.Errorf("init user service failed: %v", err)
	}

	userPersonalInfoService := service.NewUserPersonalInfoService(repos.UserPersonalInfo, repos.User, nil)

	return &UserModuleDeps{
		UserService:             userService,
		UserPersonalInfoService: userPersonalInfoService,
	}, nil
}
