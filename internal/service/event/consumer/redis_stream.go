package consumer

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/event"
	event_handlers "va_visionai_server/internal/service/event/handlers"
	"va_visionai_server/internal/zlog"
)

const (
	PaymentEventQueueKey = "paylinker:payment:event"
	PaymentConsumerGroup = "visionai_server_group"
	PaymentConsumerName  = "consumer_payment" // 生产环境可附加实例ID
)

// RedisStreamConsumer 实现了 Consumer 接口，用于从 Redis Stream 消费事件
type RedisStreamConsumer struct {
	rdb          *redis.Client
	eventService *event.EventService
	stopChan     chan struct{}
	sourceName   string
}

// NewRedisStreamConsumer 创建一个新的 Redis Stream 消费者
func NewRedisStreamConsumer(rdb *redis.Client, eventService *event.EventService) Consumer {
	return &RedisStreamConsumer{
		rdb:          rdb,
		eventService: eventService,
		stopChan:     make(chan struct{}),
		sourceName:   "RedisStream_Payment",
	}
}

func (c *RedisStreamConsumer) SourceName() string {
	return c.sourceName
}

func (c *RedisStreamConsumer) Start() {
	// 在启动时尝试创建消费者组。如果流不存在或组已存在，此调用会失败，但我们可以安全地忽略该错误。
	// 当生产者第一次写入数据时，流会自动创建，下一次消费者循环时，组就会被成功创建。
	c.rdb.XGroupCreate(PaymentEventQueueKey, PaymentConsumerGroup, "0-0").Err()

	zlog.Logger.Info("启动 Redis Stream 消费者", zap.String("source", c.sourceName))
	go c.run()
}

func (c *RedisStreamConsumer) Stop() {
	close(c.stopChan)
	zlog.Logger.Info("停止 Redis Stream 消费者", zap.String("source", c.sourceName))
}

func (c *RedisStreamConsumer) run() {
	for {
		select {
		case <-c.stopChan:
			return
		default:
			streams, err := c.rdb.XReadGroup(&redis.XReadGroupArgs{
				Group:    PaymentConsumerGroup,
				Consumer: PaymentConsumerName,
				Streams:  []string{PaymentEventQueueKey, ">"}, // ">"表示读取从未被投递过的消息
				Count:    1,
				Block:    2 * time.Second,
			}).Result()

			if err != nil {
				// 如果错误是 "NOGROUP"，说明 stream 或 group 尚未创建，这是冷启动时的正常现象。
				// 此时不应作为错误打印，以避免日志噪音。
				if err == redis.Nil || strings.Contains(err.Error(), "NOGROUP") {
					// 可以选择完全静默，或是在 debug 级别打印一个提示
					// zlog.Logger.Debug("Redis stream or group not ready, waiting...")
					continue
				}
				zlog.Logger.Error("从Redis Stream读取支付事件失败", zap.Error(err))
				// 在发生真实错误时，稍微等待一下，避免刷爆日志
				time.Sleep(5 * time.Second)
				continue
			}

			if len(streams) == 0 {
				continue
			}

			// 处理拉取到的消息
			for _, stream := range streams {
				for _, message := range stream.Messages {
					c.processMessage(message)
				}
			}
		}
	}
}

func (c *RedisStreamConsumer) processMessage(msg redis.XMessage) {
	zlog.Logger.Info("处理支付事件", zap.Any("message", msg))
	eventJSON, ok := msg.Values["event_data"].(string)
	if !ok {
		zlog.Logger.Error("无效的支付事件消息：缺少event_data字段", zap.Any("messageID", msg.ID))
		c.rdb.XAck(PaymentEventQueueKey, PaymentConsumerGroup, msg.ID)
		return
	}

	// 解析完整的业务载体
	var payload event_handlers.PaymentPayload
	if err := json.Unmarshal([]byte(eventJSON), &payload); err != nil {
		zlog.Logger.Error("无法解析支付事件业务载体", zap.Error(err), zap.String("data", eventJSON))
		c.rdb.XAck(PaymentEventQueueKey, PaymentConsumerGroup, msg.ID)
		return
	}

	// 根据载体内容推断事件类型，以用于路由
	var eventType string
	if payload.SubscriptionDetails != nil {
		// 暂时将所有会员相关事件归为一个类型用于路由，后续可根据需求细化
		eventType = constants.EventTypeSubscriptionCreated
	} else {
		eventType = constants.EventTypeOnetimePurchase
	}

	// 验证 UserID 是否存在
	if payload.UserID == "" {
		zlog.Logger.Error("支付事件业务载体中缺少 user_id", zap.String("data", eventJSON))
		c.rdb.XAck(PaymentEventQueueKey, PaymentConsumerGroup, msg.ID)
		return
	}

	// 构建用于传递给服务层的标准事件对象
	userEvent := &model.UserEvent{
		UserID:    payload.UserID,
		EventType: eventType,
		EventData: json.RawMessage(eventJSON), // 传递原始JSON
		CreatedAt: time.Now(),
	}

	// 调用事件服务进行处理
	err := c.eventService.HandleBackgroundEvent(context.Background(), userEvent)

	if err != nil {
		zlog.Logger.Error("处理支付事件失败，等待重试", zap.Error(err), zap.String("userID", payload.UserID))
	} else {
		// 处理成功，ACK 消息
		if ackErr := c.rdb.XAck(PaymentEventQueueKey, PaymentConsumerGroup, msg.ID).Err(); ackErr != nil {
			zlog.Logger.Error("ACK消息失败", zap.Error(ackErr), zap.String("messageID", msg.ID))
		}
	}
}
