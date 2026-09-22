package dao

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/zlog"
)

// UserAmountDao 用户额度数据访问层
type UserAmountDao struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewUserAmountDao 创建新的用户额度DAO实例
func NewUserAmountDao() *UserAmountDao {
	return &UserAmountDao{
		db:    db.GetDB(),
		redis: db.GetRedis(),
	}
}

// GetUserAmount 获取用户文本和图片聊天额度
func (d *UserAmountDao) GetUserAmount(ctx context.Context, projectID, userID string) (textAmount int64, imageAmount int64, err error) {
	if projectID == "" || userID == "" {
		return 0, 0, errors.New("项目ID和用户ID不能为空")
	}

	textKey := fmt.Sprintf("%s:%s:%s", projectID, userID, constants.RedisKeyUserAmountHashFieldText)
	imageKey := fmt.Sprintf("%s:%s:%s", projectID, userID, constants.RedisKeyUserAmountHashFieldImage)

	pipe := d.redis.Pipeline()
	textCmd := pipe.HGet(constants.RedisKeyUserAmount, textKey)
	imageCmd := pipe.HGet(constants.RedisKeyUserAmount, imageKey)

	_, err = pipe.Exec()
	if err == nil {
		textAmount, _ = textCmd.Int64()
		imageAmount, _ = imageCmd.Int64()
		return textAmount, imageAmount, nil
	}

	if err != redis.Nil && err != nil {
		return 300000, 3, errors.Wrap(err, "获取用户额度失败")
	}

	// 如果Redis中没有数据，检查用户是否有订阅
	var count int64
	err = d.db.Table("pay_user_subscription").
		Where("project_id = ? AND user_id = ? AND expire_date > ?", projectID, userID, time.Now().UnixMilli()).
		Count(&count).Error
	if err != nil {
		return 0, 0, errors.Wrap(err, "检查用户订阅失败")
	}

	// 根据订阅状态设置用户额度并返回
	if count > 0 {
		if err := d.SetUserAmount(ctx, projectID, userID, true); err != nil {
			return 0, 0, errors.Wrap(err, "设置订阅用户额度失败")
		}
		return 300000, 300000, nil
	}

	if err := d.SetUserAmount(ctx, projectID, userID, false); err != nil {
		return 300000, 3, errors.Wrap(err, "设置非订阅用户额度失败")
	}
	return 300000, 3, nil
}

// SetUserAmount 设置用户聊天额度
func (d *UserAmountDao) SetUserAmount(ctx context.Context, projectID, userID string, isSubscribeUser bool) error {
	if projectID == "" || userID == "" {
		return errors.New("项目ID和用户ID不能为空")
	}

	var subscribeUserIDs, notSubscribeUserIDs []string
	if isSubscribeUser {
		subscribeUserIDs = []string{userID}
	} else {
		notSubscribeUserIDs = []string{userID}
	}

	zlog.LogWithContext(ctx).Info("设置用户额度",
		zap.String("projectID", projectID),
		zap.String("userID", userID),
		zap.Bool("isSubscribeUser", isSubscribeUser))

	return d.updateUserAmountsInRedis(ctx, projectID, subscribeUserIDs, notSubscribeUserIDs, false)
}

// DecrUserAmount 减少用户聊天额度
func (d *UserAmountDao) DecrUserAmount(ctx context.Context, projectID, userID string, isTextChat bool) error {
	if projectID == "" || userID == "" {
		return errors.New("项目ID和用户ID不能为空")
	}

	// 加锁确保原子操作
	lockKey := fmt.Sprintf("%s:%s:lock", projectID, userID)
	ok, err := d.redis.SetNX(lockKey, "1", 5*time.Second).Result()
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取锁失败", zap.Error(err))
		return errors.Wrap(err, "获取锁失败")
	}
	if !ok {
		zlog.LogWithContext(ctx).Error("无法获取锁")
		return errors.New("无法获取锁")
	}
	defer d.redis.Del(lockKey)

	amountType := constants.RedisKeyUserAmountHashFieldImage
	if isTextChat {
		amountType = constants.RedisKeyUserAmountHashFieldText
	}

	key := fmt.Sprintf("%s:%s:%s", projectID, userID, amountType)

	// 原子减少并验证
	result, err := d.redis.HIncrBy(constants.RedisKeyUserAmount, key, -1).Result()
	if err != nil {
		zlog.LogWithContext(ctx).Error("减少额度失败", zap.Error(err))
		return errors.Wrap(err, "减少额度失败")
	}

	if result < 0 {
		// 回滚减少操作
		d.redis.HIncrBy(constants.RedisKeyUserAmount, key, 1)
		zlog.LogWithContext(ctx).Error("减少额度失败, 剩余额度为0", zap.Error(err))
		return constants.ERR_DAILY_TOLEN_LIMIT
	}

	return nil
}

// updateUserAmountsInRedis 使用Lua脚本原子性地更新Redis中的用户额度
func (d *UserAmountDao) updateUserAmountsInRedis(ctx context.Context, projectID string, subscribeUserIDs, notSubscribeUserIDs []string, shouldDeleteInvalid bool) error {
	if subscribeUserIDs == nil {
		subscribeUserIDs = []string{}
	}
	if notSubscribeUserIDs == nil {
		notSubscribeUserIDs = []string{}
	}
	// 特权packge ,所有用户都是订阅用户
	//if projectID == "com.bluex.visualai" {
	//	subscribeUserIDs = append(subscribeUserIDs, notSubscribeUserIDs...)
	//	notSubscribeUserIDs = []string{}
	//}

	subscribeUsersJSON, err := json.Marshal(subscribeUserIDs)
	if err != nil {
		return errors.Wrap(err, "序列化订阅用户失败")
	}

	notSubscribeUsersJSON, err := json.Marshal(notSubscribeUserIDs)
	if err != nil {
		return errors.Wrap(err, "序列化非订阅用户失败")
	}

	luaScript := `
		local projectID = ARGV[1]
		local subscribeUsers = cjson.decode(ARGV[2])
		local notSubscribeUsers = cjson.decode(ARGV[3])
		local textChatField = ARGV[4]
		local imageChatField = ARGV[5]
		local shouldDeleteInvalid = ARGV[6] == "true"
		local dietAIProjectID = ARGV[7]
		
		local result = {success = true, error = nil, deleted = 0, updated = 0}
		
		local function getKey(userID, field)
			return projectID .. ":" .. userID .. ":" .. field
		end
		
		local validUsers = {}
		for _, userID in ipairs(subscribeUsers) do validUsers[userID] = "subscribe" end
		for _, userID in ipairs(notSubscribeUsers) do validUsers[userID] = "not_subscribe" end
		
		if shouldDeleteInvalid then
			local keys = redis.call('HKEYS', KEYS[1])
			for _, key in ipairs(keys) do
				if string.match(key, "^" .. projectID .. ":") then
					local userID = string.match(key, projectID .. ":([^:]+):")
					if not validUsers[userID] then
						redis.call('HDEL', KEYS[1], key)
						result.deleted = result.deleted + 1
					end
				end
			end
		end
		
		for userID, userType in pairs(validUsers) do
			local textKey = getKey(userID, textChatField)
			local imageKey = getKey(userID, imageChatField)
			
			if projectID == dietAIProjectID then
				if userType == "subscribe" then
					redis.call('HSET', KEYS[1], textKey, 30000)
					redis.call('HSET', KEYS[1], imageKey, 30000)
				else
					redis.call('HSET', KEYS[1], textKey, 1)
					redis.call('HSET', KEYS[1], imageKey, 30000)
				end
			else
				if userType == "subscribe" then
					redis.call('HSET', KEYS[1], textKey, 300000)
					redis.call('HSET', KEYS[1], imageKey, 300000)
				else
					redis.call('HSET', KEYS[1], textKey, 30000)
					redis.call('HSET', KEYS[1], imageKey, 3)
				end
			end
			result.updated = result.updated + 2
		end
		
		return cjson.encode(result)
	`

	result, err := d.redis.Eval(luaScript, []string{constants.RedisKeyUserAmount},
		projectID,
		string(subscribeUsersJSON),
		string(notSubscribeUsersJSON),
		constants.RedisKeyUserAmountHashFieldText,
		constants.RedisKeyUserAmountHashFieldImage,
		strconv.FormatBool(shouldDeleteInvalid),
		constants.ProjectIdDietAI,
	).Result()

	if err != nil {
		return errors.Wrap(err, "执行Lua脚本失败")
	}

	var luaResult struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
		Deleted int    `json:"deleted"`
		Updated int    `json:"updated"`
	}

	if err := json.Unmarshal([]byte(result.(string)), &luaResult); err != nil {
		return errors.Wrap(err, "解析Lua结果失败")
	}

	if !luaResult.Success {
		return fmt.Errorf("Lua执行失败: %s", luaResult.Error)
	}

	zlog.LogWithContext(ctx).Info("更新用户额度成功",
		zap.String("projectID", projectID),
		zap.Int("deleted", luaResult.Deleted),
		zap.Int("updated", luaResult.Updated))

	return nil
}

// UpdateUserAmountsInRedis 使用Lua脚本原子性地更新Redis中的用户额度
func (d *UserAmountDao) UpdateUserAmountsInRedis(ctx context.Context, projectID string, subscribeUserIDs, notSubscribeUserIDs []string, shouldDeleteInvalid bool) error {
	return d.updateUserAmountsInRedis(ctx, projectID, subscribeUserIDs, notSubscribeUserIDs, shouldDeleteInvalid)
}

func (d *UserAmountDao) UpdateUserAmountByID(ctx context.Context, userAmount model.UserAmount) error {
	err := d.db.Model(&model.UserAmount{}).Where("id = ?", userAmount.ID).Updates(userAmount).Error
	if err != nil {
		return err
	}
	return nil
}

func (d *UserAmountDao) CreateUserAmount(ctx context.Context, userAmount model.UserAmount) error {
	err := d.db.Create(&userAmount).Error
	if err != nil {
		return err
	}
	return nil
}
