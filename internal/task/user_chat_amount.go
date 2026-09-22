package task

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/zlog"
)

// UserChatAmountProcessor 用户聊天额度任务处理器V2版本
type UserChatAmountProcessor struct {
	userDao       *dao.UserDao
	userAmountDao *dao.UserAmountDao
}

// NewUserChatAmountProcessor 创建新的用户额度任务处理器
func NewUserChatAmountProcessor(userDao *dao.UserDao, userAmountDao *dao.UserAmountDao) *UserChatAmountProcessor {
	return &UserChatAmountProcessor{
		userDao:       userDao,
		userAmountDao: userAmountDao,
	}
}

// Start 启动额度处理任务
func (p *UserChatAmountProcessor) Start(ctx context.Context) {
	// 启动定时重置额度任务
	go p.scheduleReset(ctx)

	// 启动订阅检查任务
	// go p.startSubscriptionChecker(ctx)
}

// scheduleReset 定时调度重置用户额度
func (p *UserChatAmountProcessor) scheduleReset(ctx context.Context) {
	// 程序启动时检查并初始化重置
	if err := p.checkAndInitialReset(ctx); err != nil {
		zlog.LogWithContext(ctx).Error("初始重置失败",
			zap.Error(err),
			zap.Any(constants.ServiceEvent, "InitialReset"))
	}

	// 创建每日定时器
	ticker := p.createDailyTicker()

	// 立即进行一次全量重置
	p.resetAllUserAmountsWithLock(ctx)

	defer ticker.Stop()

	// 持续监听定时器和上下文取消信号
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.resetAllUserAmountsWithLock(ctx)
		}
	}
}

// createDailyTicker 创建每日定时器，在每天0点触发
func (p *UserChatAmountProcessor) createDailyTicker() *time.Ticker {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	duration := next.Sub(now)
	time.Sleep(duration)
	return time.NewTicker(24 * time.Hour)
}

// checkAndInitialReset 检查并执行初始重置
func (p *UserChatAmountProcessor) checkAndInitialReset(ctx context.Context) error {
	exists, err := db.GetRedis().Exists(constants.RedisKeyUserAmount).Result()
	if err != nil {
		return fmt.Errorf("检查用户额度键: %w", err)
	}

	if exists == 0 {
		zlog.LogWithContext(ctx).Info("执行初始重置用户额度")
		p.resetAllUserAmountsWithLock(ctx)
		return nil
	}

	zlog.LogWithContext(ctx).Info("用户额度已存在，跳过初始重置")
	return nil
}

// resetAllUserAmountsWithLock 使用分布式锁重置所有用户额度
func (p *UserChatAmountProcessor) resetAllUserAmountsWithLock(ctx context.Context) {
	// 获取所有活跃项目
	projects, err := p.getAllProjects(ctx)
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取项目列表失败",
			zap.Error(err),
			zap.Any(constants.ServiceEvent, constants.EventResetUserAmounts))
		return
	}

	// 全局锁，防止多个实例同时运行重置操作
	globalLockKey := constants.RedisKeyUserAmountLock
	ok, err := db.GetRedis().SetNX(globalLockKey, "1", 30*time.Minute).Result()
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取全局锁失败",
			zap.Error(err),
			zap.Any(constants.ServiceEvent, constants.EventResetUserAmounts))
		return
	}

	if !ok {
		zlog.LogWithContext(ctx).Info("另一个实例正在处理重置，跳过")
		return
	}
	defer db.GetRedis().Del(globalLockKey)

	// 并发处理每个项目的用户额度重置
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5) // 限制并发数

	for _, projectID := range projects {
		projectID := projectID // 创建副本避免闭包问题
		g.Go(func() error {
			return p.resetProjectUserAmounts(gctx, projectID)
		})
	}

	if err := g.Wait(); err != nil {
		zlog.LogWithContext(ctx).Error("重置部分项目的用户额度失败",
			zap.Error(err),
			zap.Any(constants.ServiceEvent, constants.EventResetUserAmounts))
	}
}

// resetProjectUserAmounts 重置指定项目的用户额度
func (p *UserChatAmountProcessor) resetProjectUserAmounts(ctx context.Context, projectID string) error {
	startTime := time.Now()

	// 获取项目下所有用户
	allUsers, err := p.userDao.GetAllUsersByProject(projectID)
	if err != nil {
		return errors.Wrap(err, "获取项目用户列表失败")
	}

	// 获取有效订阅用户
	subscribedUsers, err := p.userDao.GetSubscribedUsersByProject(projectID)
	if err != nil {
		return errors.Wrap(err, "获取订阅用户列表失败")
	}

	// 创建订阅用户集合，便于快速查找
	subscribedUserSet := make(map[string]struct{}, len(subscribedUsers))
	for _, userID := range subscribedUsers {
		subscribedUserSet[userID] = struct{}{}
	}

	// 分离非订阅用户
	var nonSubscribedUsers []string
	for _, userID := range allUsers {
		if _, isSubscribed := subscribedUserSet[userID]; !isSubscribed {
			nonSubscribedUsers = append(nonSubscribedUsers, userID)
		}
	}

	// 更新Redis中的用户额度
	err = p.userAmountDao.UpdateUserAmountsInRedis(
		ctx,
		projectID,
		subscribedUsers,
		nonSubscribedUsers,
		true, // 应该删除无效用户
	)
	if err != nil {
		return errors.Wrap(err, "更新Redis中的用户额度失败")
	}

	elapsed := time.Since(startTime)
	zlog.LogWithContext(ctx).Info("成功重置项目用户额度",
		zap.String("projectID", projectID),
		zap.Int("订阅用户数", len(subscribedUsers)),
		zap.Int("非订阅用户数", len(nonSubscribedUsers)),
		zap.Duration("耗时", elapsed))

	return nil
}

// getAllProjects 获取所有项目ID
func (p *UserChatAmountProcessor) getAllProjects(ctx context.Context) ([]string, error) {
	dbInst := db.GetDB()
	var projects []string

	// 从项目表获取项目ID
	err := dbInst.Table("va_project").
		Select("project_id").
		Where("project_id IS NOT NULL AND project_id != ''").
		Find(&projects).Error
	if err != nil {
		return nil, errors.Wrap(err, "从项目表获取项目失败")
	}

	return projects, nil
}
