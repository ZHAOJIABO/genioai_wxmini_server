package bootstrap

import (
	"gorm.io/gorm"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/service/invite"
)

// InviteModuleDeps 邀请模块依赖
type InviteModuleDeps struct {
	InviteService *invite.InviteService
}

// InitInviteModule 初始化邀请模块
func InitInviteModule(
	repos *dao.Repositories,
	creditService credit.Service,
	configService invite.ConfigService,
	db *gorm.DB,
) *InviteModuleDeps {
	inviteService := invite.NewInviteService(
		repos.Invite,
		creditService,
		configService,
		db,
	)

	return &InviteModuleDeps{
		InviteService: inviteService,
	}
}
