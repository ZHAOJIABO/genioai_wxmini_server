package task

import (
	"context"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/zlog"
)

// CreditExpiryProcessor 处理会员额度过期清理的定时任务
type CreditExpiryProcessor struct {
	membershipService credit.MembershipService
	userDao           *dao.UserDao
	running           bool
	stopChan          chan struct{}
}

// NewCreditExpiryProcessor 创建新的额度过期处理器
func NewCreditExpiryProcessor(membershipService credit.MembershipService, userDao *dao.UserDao) *CreditExpiryProcessor {
	return &CreditExpiryProcessor{
		membershipService: membershipService,
		userDao:           userDao,
		stopChan:          make(chan struct{}),
	}
}

// Start 启动定时任务
func (p *CreditExpiryProcessor) Start(ctx context.Context) {
	if p.running {
		zlog.Logger.Warn("CreditExpiryProcessor 已经在运行中")
		return
	}

	p.running = true
	zlog.Logger.Info("启动 CreditExpiryProcessor")

	// 每小时执行一次清理任务
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// 启动时立即执行一次
	go p.processExpiredCredits(ctx)

	for {
		select {
		case <-ticker.C:
			go p.processExpiredCredits(ctx)
		case <-p.stopChan:
			zlog.Logger.Info("CreditExpiryProcessor 停止")
			p.running = false
			return
		case <-ctx.Done():
			zlog.Logger.Info("CreditExpiryProcessor 因上下文取消而停止")
			p.running = false
			return
		}
	}
}

// Stop 停止定时任务
func (p *CreditExpiryProcessor) Stop() {
	if !p.running {
		return
	}
	close(p.stopChan)
}

// processExpiredCredits 处理过期额度清理
func (p *CreditExpiryProcessor) processExpiredCredits(ctx context.Context) {
	zlog.Logger.Info("开始处理过期会员额度清理")

	// 这里可以根据实际需求来获取需要处理的用户列表
	// 目前简化处理，可以后续优化为批量处理

	// TODO: 实现批量获取有会员额度的用户列表
	// 目前这是一个简化的实现，实际使用时需要根据业务需求调整

	zlog.Logger.Info("会员额度过期清理任务完成")
}

// processUserExpiredCredits 处理单个用户的过期额度
func (p *CreditExpiryProcessor) processUserExpiredCredits(ctx context.Context, projectID, userID string) {
	clearedAmount, err := p.membershipService.ClearExpiredMembershipCredits(ctx, projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("清理用户过期额度失败",
			zap.String("userID", userID),
			zap.String("projectID", projectID),
			zap.Error(err))
		return
	}

	if clearedAmount > 0 {
		zlog.LogWithContext(ctx).Info("清理用户过期额度成功",
			zap.String("userID", userID),
			zap.String("projectID", projectID),
			zap.Int64("clearedAmount", clearedAmount))
	}
}
