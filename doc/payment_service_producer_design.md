# 支付服务事件生产者设计文档

## 1. 概述

本文档为**支付服务**设计了一个事件生产者（`EventProducer`）模块。其唯一职责是在支付流程成功完成后，构建一条标准的事件消息，并将其**可靠地**推送到 Redis Stream 中，以供下游服务（如 `visionai-server`）进行消费。

本文档是 `payment_event_processing_design.md` 的姊妹篇，共同构成完整的事件驱动流程。

## 2. 架构与技术选型

- **角色**: 生产者 (Producer)
- **消息中间件**: Redis Stream
- **核心命令**: `XADD`
- **数据格式**: JSON

支付服务通过 `XADD` 命令将事件消息添加到 Stream 的末尾。我们明确选择 **Stream** 而非 Pub/Sub，因为它提供了**持久化**能力。即使下游的消费者服务暂时宕机，事件消息也会安全地保留在队列中，直到消费者恢复并处理它，这保证了交易的最终一致性。

## 3. 生产者实现

我们建议在支付服务中创建一个 `producer` 包来封装事件发送的逻辑。

### 3.1. 接口定义

一个清晰的接口有助于将事件发送的实现与业务逻辑解耦。

```go
package producer

type EventProducer interface {
	// SendSubscriptionEvent 用于发送新订阅或续订成功的事件
	SendSubscriptionEvent(ctx context.Context, eventType string, data SubscriptionEventData) error

	// SendOnetimePurchaseEvent 用于发送一次性内购成功的事件
	SendOnetimePurchaseEvent(ctx context.Context, data OnetimePurchaseEventData) error
}
```

### 3.2. 实现细节

`EventProducer` 的具体实现（例如 `redisEventProducer`）将负责：
1.  接收业务数据结构（`SubscriptionEventData` 或 `OnetimePurchaseEventData`）。
2.  将其转换为两层嵌套的 JSON 结构，这与消费者端 `model.UserEvent` 的约定完全一致。
3.  调用 Redis 客户端，执行 `XADD` 命令将消息推送到指定的 Stream Key (`visionai:events:payment`)。

### 3.3. 调用时机

为保证数据一致性，`EventProducer` 的方法**必须**在核心支付业务的数据库事务**成功提交之后**被调用。这确保了我们绝不会为一个失败或未完成的支付发送"成功"事件。

推荐以"Fire and Forget"的方式异步调用（`go s.eventProducer.Send...`），以避免阻塞主支付流程。

## 4. 事件消息规范

`EventProducer` 发送到 Redis Stream 的消息体 (`Values`) 应包含一个 key 为 `event_data` 的字段，其值是一个 JSON 字符串，可以被下游服务反序列化为 `model.UserEvent` 对象。

**`event_data` 字段的值 (示例):**
```json
{
  "userID": "usr_aBcDeF12345",
  "eventType": "subscription.created",
  "eventData": "{\"project_id\":\"com.visionai.app.ios\",\"order_id\":\"ord_GHIjkl67890\",\"membership_level\":2,\"subscription_expires_at\":\"2024-10-27T10:00:00Z\"}"
}
```
*注意: `eventData` 字段的值是一个内嵌的、转义后的JSON字符串。*

## 5. 可靠性考量

- **Redis 连接失败**: `EventProducer` 的实现应能处理 Redis 暂时不可用的情况。推荐的策略是：
  1.  使用带重试和指数退避策略的逻辑来执行 `XADD` 命令。
  2.  如果最终发送失败，必须记录**严重错误（Critical Error）**级别的日志，并触发监控告警，以便运维人员介入。这表示一笔成功的支付未能通知下游系统，需要人工补发事件。
- **数据中心同步**: 在多数据中心部署时，需确保 Redis Stream 的数据能够可靠地同步或路由到消费者所在的数据中心。

## 6. 总结

该生产者设计简单、专注且可靠，与消费者端的解耦设计相结合，形成了一个健壮的、端到端的异步事件处理系统，能够满足金融级操作的可靠性要求。 