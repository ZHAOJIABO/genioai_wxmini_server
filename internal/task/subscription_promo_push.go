package task

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service/push_gateway"
	"va_visionai_server/internal/zlog"
)

const (
	// SubscriptionPromoPushLockKey 分布式锁键（1分钟过期）
	SubscriptionPromoPushLockKey = "visionai:subscription_promo_push:lock"
	// SubscriptionPromoPushLockTimeout 锁超时时间
	SubscriptionPromoPushLockTimeout = 1 * time.Minute
	// SubscriptionPromoPushSentKeyFormat 今日已推送用户集合键格式（24小时过期）
	SubscriptionPromoPushSentKeyFormat = "visionai:subscription_promo_push:sent:%s"
	// DefaultPushTime 默认推送时间
	DefaultPushTime = "10:30"
	// PushTimeToleranceMinutes 推送时间容差（分钟），前后各允许的误差
	PushTimeToleranceMinutes = 5
	// MaxUsersPerBatch 每批最大处理用户数，防止单次处理时间过长
	MaxUsersPerBatch = 500
)

// SubscriptionPromoPushConfigService 配置服务接口（避免导入循环）
type SubscriptionPromoPushConfigService interface {
	GetBoolConfig(key string) (bool, error)
	GetStringConfig(key string) (string, error)
}

// SubscriptionPromoPushProcessor 订阅优惠推送处理器
// 定时向未订阅用户推送订阅优惠通知
type SubscriptionPromoPushProcessor struct {
	rdb               *redis.Client
	userDao           *dao.UserDao
	pushGatewayClient *push_gateway.Client
	configService     SubscriptionPromoPushConfigService
	log               *zap.Logger
}

// NewSubscriptionPromoPushProcessor 创建订阅优惠推送处理器
func NewSubscriptionPromoPushProcessor(
	rdb *redis.Client,
	userDao *dao.UserDao,
	pushGatewayClient *push_gateway.Client,
	configService SubscriptionPromoPushConfigService,
) *SubscriptionPromoPushProcessor {
	return &SubscriptionPromoPushProcessor{
		rdb:               rdb,
		userDao:           userDao,
		pushGatewayClient: pushGatewayClient,
		configService:     configService,
		log:               zlog.Logger.Named("subscription_promo_push"),
	}
}

// Start 启动订阅优惠推送处理器
func (p *SubscriptionPromoPushProcessor) Start(ctx context.Context) {
	p.log.Info("订阅优惠推送处理器已启动")

	// 每分钟检查一次
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.log.Info("订阅优惠推送处理器已停止")
			return
		case <-ticker.C:
			p.processPush(ctx)
		}
	}
}

// processPush 处理推送逻辑
func (p *SubscriptionPromoPushProcessor) processPush(ctx context.Context) {
	// 1. 检查推送开关
	enabled, err := p.configService.GetBoolConfig(constants.ConfigKeySubscriptionPromoPushEnabled)
	if err != nil || !enabled {
		return
	}

	// 2. 检查 push gateway 是否启用
	if p.pushGatewayClient == nil || !p.pushGatewayClient.IsEnabled() {
		return
	}

	// 3. 获取分布式锁（防止多实例重复执行）
	ok, err := p.rdb.SetNX(SubscriptionPromoPushLockKey, "1", SubscriptionPromoPushLockTimeout).Result()
	if err != nil {
		p.log.Error("获取分布式锁失败", zap.Error(err))
		return
	}
	if !ok {
		// 其他实例正在处理
		return
	}
	defer p.rdb.Del(SubscriptionPromoPushLockKey)

	// 4. 获取配置的推送时间
	pushTime := p.getPushTime()

	// 5. 计算当前 UTC 时间对应哪些时区正好是目标时间
	matchingTimezones := p.getMatchingTimezones(pushTime)
	if len(matchingTimezones) == 0 {
		return
	}

	p.log.Info("找到匹配的时区",
		zap.String("target_time", pushTime),
		zap.Strings("timezones", matchingTimezones))

	// 6. 获取推送标题和内容
	title, _ := p.configService.GetStringConfig(constants.ConfigKeySubscriptionPromoPushTitle)
	body, _ := p.configService.GetStringConfig(constants.ConfigKeySubscriptionPromoPushBody)

	if title == "" && body == "" {
		p.log.Warn("推送标题和内容均为空，跳过推送")
		return
	}

	// 7. 今日已推送用户集合的键
	today := time.Now().Format("2006-01-02")
	sentKey := fmt.Sprintf(SubscriptionPromoPushSentKeyFormat, today)

	// 8. 按时区并发处理，每个时区独立处理
	var wg sync.WaitGroup
	for _, tz := range matchingTimezones {
		wg.Add(1)
		go func(timezone string) {
			defer wg.Done()
			p.processTimezone(ctx, timezone, title, body, sentKey)
		}(tz)
	}
	wg.Wait()
}

// processTimezone 处理单个时区的用户推送（批量优化版）
func (p *SubscriptionPromoPushProcessor) processTimezone(ctx context.Context, timezone, title, body, sentKey string) {
	// 查询该时区的未订阅用户
	users, err := p.userDao.GetUnsubscribedUsersByTimezones([]string{timezone})
	if err != nil {
		p.log.Error("查询未订阅用户失败",
			zap.String("timezone", timezone),
			zap.Error(err))
		return
	}

	if len(users) == 0 {
		return
	}

	// 限制每批最大处理数量，超出的留到下一分钟处理
	if len(users) > MaxUsersPerBatch {
		p.log.Info("用户数量超过批次限制，本次只处理部分",
			zap.String("timezone", timezone),
			zap.Int("total", len(users)),
			zap.Int("processing", MaxUsersPerBatch))
		users = users[:MaxUsersPerBatch]
	}

	// 1. 先过滤已推送的用户
	var pendingUserIDs []string
	for _, user := range users {
		isSent, err := p.rdb.SIsMember(sentKey, user.UserId).Result()
		if err != nil {
			p.log.Error("检查已推送状态失败",
				zap.String("user_id", user.UserId),
				zap.Error(err))
			continue
		}
		if !isSent {
			pendingUserIDs = append(pendingUserIDs, user.UserId)
		}
	}

	if len(pendingUserIDs) == 0 {
		p.log.Debug("所有用户已推送，跳过",
			zap.String("timezone", timezone))
		return
	}

	p.log.Info("处理时区用户推送",
		zap.String("timezone", timezone),
		zap.Int("pending_count", len(pendingUserIDs)))

	today := time.Now().Format("2006-01-02")
	successCount := 0
	failCount := 0

	// 2. 按批次发送（每批最多 100 个，与 push_gateway 限制一致）
	batchSize := push_gateway.MaxPushBatchSize
	for i := 0; i < len(pendingUserIDs); i += batchSize {
		end := i + batchSize
		if end > len(pendingUserIDs) {
			end = len(pendingUserIDs)
		}
		batch := pendingUserIDs[i:end]

		_, err := p.pushGatewayClient.SendPush(ctx, &push_gateway.SendPushRequest{
			UserIDs: batch,
			Title:   title,
			Body:    body,
			Data: map[string]interface{}{
				"target": "subscription_page",
			},
			BizID: fmt.Sprintf("subscription_promo_%s_%s_batch%d", today, timezone, i/batchSize),
		})

		if err != nil {
			p.log.Error("批量发送推送失败",
				zap.String("timezone", timezone),
				zap.Int("batch_index", i/batchSize),
				zap.Int("batch_size", len(batch)),
				zap.Error(err))
			failCount += len(batch)
			continue
		}

		// 3. 批量记录已推送
		members := make([]interface{}, len(batch))
		for j, uid := range batch {
			members[j] = uid
		}
		if err := p.rdb.SAdd(sentKey, members...).Err(); err != nil {
			p.log.Error("批量记录已推送状态失败",
				zap.String("timezone", timezone),
				zap.Error(err))
		}

		successCount += len(batch)
	}

	// 设置集合过期时间（24小时）
	p.rdb.Expire(sentKey, 24*time.Hour)

	p.log.Info("时区推送完成",
		zap.String("timezone", timezone),
		zap.Int("success", successCount),
		zap.Int("fail", failCount))
}

// getPushTime 获取配置的推送时间，默认 "10:30"
func (p *SubscriptionPromoPushProcessor) getPushTime() string {
	pushTime, err := p.configService.GetStringConfig(constants.ConfigKeySubscriptionPromoPushTime)
	if err != nil || pushTime == "" {
		return DefaultPushTime
	}
	return pushTime
}

// getMatchingTimezones 获取当前时刻等于目标推送时间的所有时区
// 允许 ±PushTimeToleranceMinutes 分钟的误差
func (p *SubscriptionPromoPushProcessor) getMatchingTimezones(targetTime string) []string {
	// 解析目标时间 "10:30" -> hour=10, minute=30
	parts := strings.Split(targetTime, ":")
	if len(parts) != 2 {
		p.log.Error("无效的推送时间格式", zap.String("target_time", targetTime))
		return nil
	}

	targetHour := 0
	targetMinute := 0
	_, err := fmt.Sscanf(targetTime, "%d:%d", &targetHour, &targetMinute)
	if err != nil {
		p.log.Error("解析推送时间失败", zap.String("target_time", targetTime), zap.Error(err))
		return nil
	}

	// 从数据库获取所有用户实际使用的时区
	allTimezones, err := p.userDao.GetDistinctTimezones()
	if err != nil {
		p.log.Error("获取用户时区列表失败", zap.Error(err))
		return nil
	}

	if len(allTimezones) == 0 {
		return nil
	}

	// 计算目标时间的分钟数（从0点开始）
	targetTotalMinutes := targetHour*60 + targetMinute

	var matchedTimezones []string
	now := time.Now().UTC()

	for _, tzName := range allTimezones {
		loc, err := time.LoadLocation(tzName)
		if err != nil {
			p.log.Debug("加载时区失败", zap.String("timezone", tzName), zap.Error(err))
			continue
		}

		localTime := now.In(loc)
		localTotalMinutes := localTime.Hour()*60 + localTime.Minute()

		// 计算与目标时间的差值（考虑跨天情况）
		diff := localTotalMinutes - targetTotalMinutes
		if diff > 720 { // 超过12小时，说明跨天了
			diff -= 1440
		} else if diff < -720 {
			diff += 1440
		}

		// 在容差范围内（±5分钟）
		if diff >= -PushTimeToleranceMinutes && diff <= PushTimeToleranceMinutes {
			matchedTimezones = append(matchedTimezones, tzName)
		}
	}

	return matchedTimezones
}
