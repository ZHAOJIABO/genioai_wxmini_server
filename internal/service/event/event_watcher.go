package event

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/db"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

const (
	eventChannelPrefix = "visionai:event:"
	userEventPrefix    = "visionai:user_event:"
)

// EventRegistry 管理事件订阅
type EventRegistry struct {
	mu sync.RWMutex
	// 用于跟踪活跃订阅，key是clientID
	activeSubscriptions map[string]*eventSubscription
	rdb                 *redis.Client
}

// eventSubscription 表示一个活跃的事件订阅
type eventSubscription struct {
	eventType     vai.WatchEventType
	pubSub        *redis.PubSub
	messageStream chan *vai.EventWatchResponse
	done          chan struct{}
}

// NewEventRegistry 创建事件注册中心
func NewEventRegistry() *EventRegistry {
	return &EventRegistry{
		activeSubscriptions: make(map[string]*eventSubscription),
		rdb:                 db.GetRedis(),
	}
}

// Register 添加新的事件订阅
func (r *EventRegistry) Register(eventType vai.WatchEventType, clientID string) common.EventChannel {
	r.mu.Lock()
	defer r.mu.Unlock()

	msgChan := make(common.EventChannel, 10000)
	done := make(chan struct{})
	allChannel := fmt.Sprintf("%s%d", eventChannelPrefix, eventType)
	userChannel := fmt.Sprintf("%s%s:%d", userEventPrefix, clientID, eventType)
	pubSub := r.rdb.Subscribe(
		allChannel,
		userChannel,
	)

	sub := &eventSubscription{
		eventType:     eventType,
		pubSub:        pubSub,
		messageStream: msgChan,
		done:          done,
	}
	r.activeSubscriptions[clientID] = sub
	go r.handleSubscription(clientID, sub)

	return msgChan
}

// handleSubscription 处理订阅消息
func (r *EventRegistry) handleSubscription(clientID string, sub *eventSubscription) {
	defer func() {
		if err := recover(); err != nil {
			zlog.Logger.Error("Recovered from panic in handleSubscription",
				zap.Any("Error", err),
				zap.String("ClientID", clientID))
		}
	}()

	ch := sub.pubSub.Channel()
	for {
		select {
		case <-sub.done:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var response vai.EventWatchResponse
			if err := proto.Unmarshal([]byte(msg.Payload), &response); err != nil {
				zlog.Logger.Error("Failed to unmarshal event message",
					zap.Error(err),
					zap.String("ClientID", clientID),
					zap.String("Channel", msg.Channel),
					zap.String("Payload", msg.Payload),
				)
				continue
			}

			select {
			case sub.messageStream <- &response:
				zlog.Logger.Debug("event sent",
					zap.String("ClientID", clientID),
					zap.Int32("EventType", int32(response.GetEventType())),
					zap.String(constants.ServiceEvent, response.GetEventType().String()),
				)
			default:
				zlog.Logger.Warn("message channel is full, skip message",
					zap.String("ClientID", clientID))
			}
		}
	}
}

// Unregister 取消事件订阅
func (r *EventRegistry) Unregister(eventType vai.WatchEventType, clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sub, exists := r.activeSubscriptions[clientID]; exists {
		close(sub.done)
		sub.pubSub.Close()
		delete(r.activeSubscriptions, clientID)
	}
}

// Dispatch 广播事件给所有订阅者
func (r *EventRegistry) Dispatch(eventType vai.WatchEventType, data interface{}) {
	response := r.createEventResponse(eventType, data)
	if response == nil {
		return
	}

	payload, err := json.Marshal(response)
	if err != nil {
		zlog.Logger.Error("Failed to marshal event response",
			zap.Error(err),
			zap.Int32("eventType", int32(eventType)))
		return
	}

	channel := fmt.Sprintf("%s%d", eventChannelPrefix, eventType)
	if err := r.rdb.Publish(channel, payload).Err(); err != nil {
		zlog.Logger.Error("Failed to publish event",
			zap.Error(err),
			zap.String("channel", channel))
	}
}

// DispatchToUser 发送事件给指定用户
func (r *EventRegistry) DispatchToUser(eventType vai.WatchEventType, data interface{}, projectID, userID string) {
	if userID == "" || projectID == "" {
		return
	}

	response := r.createEventResponse(eventType, data)
	if response == nil {
		return
	}

	payload, err := proto.Marshal(response)
	if err != nil {
		zlog.Logger.Error("Failed to marshal event response",
			zap.Error(err),
			zap.String(constants.CtxUserID, userID),
			zap.String(constants.CtxProjectID, projectID),
		)

		return
	}

	channel := fmt.Sprintf("%s%s:%s:%d", userEventPrefix, projectID, userID, eventType)
	if err := r.rdb.Publish(channel, payload).Err(); err != nil {
		zlog.Logger.Error("Failed to publish user event",
			zap.Error(err),
			zap.String("Channel", channel),
			zap.String(constants.CtxUserID, userID),
			zap.String(constants.CtxProjectID, projectID),
		)
	}
}

// createEventResponse 创建事件响应对象
func (r *EventRegistry) createEventResponse(eventType vai.WatchEventType, data interface{}) *vai.EventWatchResponse {
	response := &vai.EventWatchResponse{
		ResponseHeader: &vai.ResponseHeader{
			Code:           vai.StatusCode_SUCCESS,
			Msg:            "success",
			ResponseTimeMs: time.Now().UnixMilli(),
		},
		EventType: eventType,
	}

	switch d := data.(type) {
	case *vai.TaskProgressEventData:
		response.EventData = &vai.EventWatchResponse_TaskProgressData{
			TaskProgressData: d,
		}
	case *vai.CustomEventData:
		response.EventData = &vai.EventWatchResponse_CustomData{
			CustomData: d,
		}
	default:
		zlog.Logger.Error("不支持的事件数据类型",
			zap.String("type", fmt.Sprintf("%T", data)))
		return nil
	}

	return response
}

// WatchService 实现Watch RPC服务
type WatchService struct {
	registry *EventRegistry
}

// NewWatchService 创建Watch服务实例
func NewWatchService() *WatchService {
	return &WatchService{
		registry: GetEventRegistry(),
	}
}

func (s *WatchService) Watch(req *vai.EventWatchRequest, stream vai.ReportService_EventWatchServer) error {
	if req == nil || req.GetRequestHeader() == nil {
		return errors.New("invalid request")
	}

	ctx := stream.Context()
	projectID := common.GetProjectID(ctx)
	clientID := projectID + ":" + req.GetRequestHeader().GetUserId()
	eventType := req.GetEventType()

	zlog.LogWithContext(ctx).Debug("New event watch connection established",
		zap.Int32("EventType", int32(eventType)))

	ch := s.registry.Register(eventType, clientID)
	defer func() {
		s.registry.Unregister(eventType, clientID)
		zlog.LogWithContext(ctx).Debug("Event watch connection closed",
			zap.Int32("EventType", int32(eventType)))
	}()

	rsp := &vai.EventWatchResponse{
		ResponseHeader: &vai.ResponseHeader{
			Code: vai.StatusCode_SUCCESS,
			Msg:  "Connected successfully",
		},
		EventType: eventType,
	}

	if err := stream.Send(rsp); err != nil {
		return errors.Wrap(err, "failed to send initial response")
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				return errors.Wrap(err, "failed to send event")
			}
		}
	}
}

var (
	globalEventRegistry *EventRegistry
	once                sync.Once
)

func GetEventRegistry() *EventRegistry {
	once.Do(func() {
		globalEventRegistry = NewEventRegistry()
	})
	return globalEventRegistry
}

// DispatchEvent 发送事件给所有订阅者
func DispatchEvent(eventType vai.WatchEventType, data interface{}) {
	GetEventRegistry().Dispatch(eventType, data)
}

// DispatchUserEvent 发送事件给指定用户
func DispatchUserEvent(eventType vai.WatchEventType, data interface{}, projectID, userID string) {
	GetEventRegistry().DispatchToUser(eventType, data, projectID, userID)
}
