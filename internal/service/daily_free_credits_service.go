package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/service/cache"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/zlog"
)

// DailyFreeCreditsService 负责每日免费积分的发放逻辑
type DailyFreeCreditsService struct {
	creditService    credit.Service
	configService    *ConfigService
	subscribeService *SubscribeService
	cacheService     *cache.CacheService
}

// NewDailyFreeCreditsService 创建每日免费积分服务实例
func NewDailyFreeCreditsService(
	creditService credit.Service,
	configService *ConfigService,
	subscribeService *SubscribeService,
	cacheService *cache.CacheService,
) *DailyFreeCreditsService {
	return &DailyFreeCreditsService{
		creditService:    creditService,
		configService:    configService,
		subscribeService: subscribeService,
		cacheService:     cacheService,
	}
}

// CheckAndGrantDailyCredits 检查并发放每日免费积分
// 返回值: 是否发放, 发放金额, 错误
func (s *DailyFreeCreditsService) CheckAndGrantDailyCredits(
	ctx context.Context,
	projectID string,
	userID string,
) (bool, int, error) {
	// 1. 校验功能开关
	enabled, err := s.isFeatureEnabled(ctx)
	if err != nil {
		return false, 0, err
	}
	if !enabled {
		return false, 0, nil
	}

	// 2. VIP 用户跳过
	if s.isVIPUser(ctx, userID) {
		zlog.LogWithContext(ctx).Debug("skip daily credits for vip user",
			zap.String("user_id", userID))
		return false, 0, nil
	}

	// 3. 计算配置额度
	amount := s.getDailyCreditsAmount(ctx)
	if amount <= 0 {
		return false, 0, nil
	}

	// 4. 构建 sourceID 做幂等控制
	today := time.Now().Format("2006-01-02")
	sourceID := fmt.Sprintf("daily_free_%s_%s_%s", projectID, userID, today)

	// 5. 计算过期时间
	expiresAt := s.getTodayEndTime()

	// 6. 发放积分
	err = s.creditService.AddCredits(
		ctx,
		projectID,
		userID,
		sourceID,
		constants.CreditTypeDailyFree,
		constants.TransactionTypeDailyGrant,
		int64(amount),
		expiresAt,
		"每日免费积分",
		0,
	)
	if err != nil {
		if s.isDuplicateError(err) {
			zlog.LogWithContext(ctx).Debug("daily credits already granted",
				zap.String("user_id", userID),
				zap.String("source_id", sourceID))
			return false, 0, nil
		}
		zlog.LogWithContext(ctx).Error("grant daily credits failed",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("project_id", projectID))
		return false, 0, err
	}

	zlog.LogWithContext(ctx).Info("daily credits granted",
		zap.String("user_id", userID),
		zap.String("project_id", projectID),
		zap.Int("amount", amount),
		zap.Time("expires_at", expiresAt))
	return true, amount, nil
}

// CheckAndGrantDailyCreditsWithTx 带事务版本，由调用方传入 isMember，避免二次判定
func (s *DailyFreeCreditsService) CheckAndGrantDailyCreditsWithTx(
	ctx context.Context,
	tx *gorm.DB,
	projectID string,
	userID string,
	isMember bool,
) (bool, int, error) {
	// 1. 开关
	enabled, err := s.isFeatureEnabled(ctx)
	if err != nil || !enabled {
		return false, 0, err
	}

	// 2. 会员跳过
	if isMember {
		return false, 0, nil
	}

	// 3. 配额
	amount := s.getDailyCreditsAmount(ctx)
	if amount <= 0 {
		return false, 0, nil
	}

	// 4. 幂等 sourceID
	today := time.Now().Format("2006-01-02")
	sourceID := fmt.Sprintf("daily_free_%s_%s_%s", projectID, userID, today)

	// 5. 过期时间（当天 23:59:59）
	expiresAt := s.getTodayEndTime()

	// 6. 发放（同一事务）
	if err := s.creditService.AddCreditsWithTx(
		ctx, tx, projectID, userID, sourceID,
		constants.CreditTypeDailyFree,
		constants.TransactionTypeDailyGrant,
		int64(amount), expiresAt, "每日免费积分", 0,
	); err != nil {
		if s.isDuplicateError(err) {
			return false, 0, nil
		}
		return false, 0, err
	}
	return true, amount, nil
}

// GrantRegisterBonus 为新注册用户发放注册奖励积分
// 返回值: 是否发放, 发放金额, 错误
func (s *DailyFreeCreditsService) GrantRegisterBonus(
	ctx context.Context,
	projectID string,
	userID string,
) (bool, int, error) {
	const bonusAmount = 5

	sourceID := fmt.Sprintf("register_bonus_%s_%s", projectID, userID)

	// 100 年后过期（永不过期）
	expiresAt := time.Now().AddDate(100, 0, 0)

	err := s.creditService.AddCredits(
		ctx,
		projectID,
		userID,
		sourceID,
		constants.CreditTypeEventGrant,
		constants.TransactionTypeRegisterBonus,
		int64(bonusAmount),
		expiresAt,
		"新用户注册奖励",
		0,
	)
	if err != nil {
		if s.isDuplicateError(err) {
			zlog.LogWithContext(ctx).Debug("register bonus already granted",
				zap.String("user_id", userID),
				zap.String("source_id", sourceID))
			return false, 0, nil
		}
		zlog.LogWithContext(ctx).Error("grant register bonus failed",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("project_id", projectID))
		return false, 0, err
	}

	zlog.LogWithContext(ctx).Info("register bonus granted",
		zap.String("user_id", userID),
		zap.String("project_id", projectID),
		zap.Int("amount", bonusAmount))
	return true, bonusAmount, nil
}

func (s *DailyFreeCreditsService) isFeatureEnabled(ctx context.Context) (bool, error) {
	if s.configService == nil {
		return false, nil
	}

	enabled, err := s.configService.GetBoolConfig(constants.ConfigKeyDailyFreeCreditsEnabled)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("failed to load daily credits switch",
			zap.Error(err))
		return false, nil
	}

	return enabled, nil
}

func (s *DailyFreeCreditsService) isVIPUser(ctx context.Context, userID string) bool {
	if s.subscribeService == nil {
		return false
	}

	info, err := s.subscribeService.GetSubscribeInfo(ctx, userID, "", "", "")
	if err != nil || info == nil {
		return false
	}

	return info.GetSubscribeLevel() > 0
}

func (s *DailyFreeCreditsService) getDailyCreditsAmount(ctx context.Context) int {
	const defaultAmount = 30
	if s.configService == nil {
		return defaultAmount
	}

	amount, err := s.configService.GetIntConfig(constants.ConfigKeyDailyFreeCreditsAmount)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("failed to load daily credits amount",
			zap.Error(err))
		return defaultAmount
	}

	if amount <= 0 {
		return defaultAmount
	}

	return amount
}

func (s *DailyFreeCreditsService) getTodayEndTime() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
}

func (s *DailyFreeCreditsService) isDuplicateError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	return strings.Contains(errMsg, "Duplicate entry") ||
		strings.Contains(errMsg, "duplicate key") ||
		strings.Contains(errMsg, "UNIQUE constraint")
}
