package event

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	event_handlers "va_visionai_server/internal/service/event/handlers"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type EventService struct {
	eventDao       *dao.EventDao
	pictureTaskDao *dao.PictureTaskDao
	trackingDao    *dao.TrackingDao
	attributionDao *dao.AttributionDao
	registry       *Registry
	eventChan      chan *model.UserEvent
}

func NewEventService(eventDao *dao.EventDao, pictureTaskDao *dao.PictureTaskDao, trackingDao *dao.TrackingDao, attributionDao *dao.AttributionDao) *EventService {
	s := &EventService{
		eventDao:       eventDao,
		pictureTaskDao: pictureTaskDao,
		trackingDao:    trackingDao,
		attributionDao: attributionDao,
		registry:       NewRegistry(),
		eventChan:      make(chan *model.UserEvent, 1000),
	}
	s.registry.Register(event_handlers.NewViewPictureHandler(pictureTaskDao))
	s.registry.Register(event_handlers.NewDefaultHandler())
	s.registry.Register(event_handlers.NewTrackingHandler(trackingDao))
	s.registry.Register(event_handlers.NewAsaHandler(attributionDao))
	go s.processEvents()

	return s
}

// Register 注册一个新的事件处理器
func (s *EventService) Register(h Handler) {
	s.registry.Register(h)
}

// ReportEvent 上报事件
func (s *EventService) ReportEvent(ctx context.Context, req *vai.RequestHeader, userID, eventType string, eventData interface{}) error {
	if eventType == vai.UserEventType_ATTRIBUTION_EVENT_ASA_TOKEN.String() {
		var asaTokenData string
		if ASATokenData, ok := eventData.(*vai.ReportUserEventRequest_AsaTokenData); ok {
			asaTokenData = ASATokenData.AsaTokenData.GetToken()
		}
		zlog.LogWithContext(ctx).Info("Get ASA Token",
			zap.Any(constants.ServiceEvent, constants.EventGetASAToken),
			zap.String("asaTokenData", asaTokenData),
		)
		return nil
	}

	// 处理 tracking 事件
	if trackingData, ok := eventData.(*vai.ReportUserEventRequest_TrackingData); ok {
		var brand string
		if req.GetDevice().GetBrand() == "iPhone" {
			brand = req.GetDevice().GetModel()
		} else {
			brand = fmt.Sprintf("%s %s", req.GetDevice().GetBrand(), req.GetDevice().GetModel())
		}
		trackingEvent := &model.TrackingEvent{
			UserID:              userID,
			TrackingEventType:   trackingData.TrackingData.GetEventType().String(),
			TrackingEventSource: trackingData.TrackingData.GetEventSource().String(),
			OS:                  constants.MappingOS(trackingData.TrackingData.GetOs()),
			AppStore:            trackingData.TrackingData.GetAppStore().String(),
			ExtraData:           trackingData.TrackingData.GetExtraData(),
			ProjectID:           req.GetApp().GetPackageName(),
			Version:             req.GetApp().GetAppVersion(),
			IP:                  req.GetDevice().GetIp(),
			Brand:               brand,
			Language:            req.GetDevice().GetLanguage().String(),
		}
		if err := s.trackingDao.CreateTrackingEvent(ctx, trackingEvent); err != nil {
			return errors.Wrap(err, "create tracking event")
		}
		return nil
	}
	// 创建事件记录
	event := &model.UserEvent{
		UserID:    userID,
		EventType: eventType,
	}
	switch data := eventData.(type) {
	case *vai.ReportUserEventRequest_TaskProgressData:
		jsonBytes, err := json.Marshal(data.TaskProgressData)
		if err != nil {
			return errors.Wrap(err, "marshal view picture data")
		}
		event.EventData = jsonBytes
	case *vai.ReportUserEventRequest_CustomData:
		event.EventData = []byte(data.CustomData.GetData())
	case *vai.ReportUserEventRequest_AsaData:
		jsonBytes, err := json.Marshal(data.AsaData)
		if err != nil {
			return errors.Wrap(err, "marshal ASA data")
		}
		event.EventData = jsonBytes
	default:
		return errors.New("invalid event data")
	}

	// 保存事件
	if err := s.eventDao.CreateEvent(ctx, event); err != nil {
		return errors.Wrap(err, "create event")
	}

	// 发送到事件通道
	select {
	case s.eventChan <- event:
	default:
		zlog.LogWithContext(ctx).Warn("event channel is full, event dropped",
			zap.String("userID", userID),
			zap.String("eventType", eventType))
	}

	return nil
}

// processEvents 处理事件的协程
func (s *EventService) processEvents() {
	for event := range s.eventChan {
		handler := s.registry.GetHandler(event.EventType)
		if handler == nil {
			continue
		}
		if err := handler.HandleEvent(context.Background(), event); err != nil {
			zlog.LogWithContext(context.Background()).Error("failed to handle event",
				zap.String("eventType", event.EventType),
				zap.Error(err))
		}
	}
}

// HandleBackgroundEvent 是一个专门给后台消费者调用的新方法
func (s *EventService) HandleBackgroundEvent(ctx context.Context, event *model.UserEvent) error {
	// 直接将事件推送到事件通道
	select {
	case s.eventChan <- event:
		return nil
	default:
		// 如果通道满了，返回错误，让消费者可以重试
		err := errors.New("event channel is full, event processing delayed")
		zlog.LogWithContext(ctx).Warn(err.Error(),
			zap.String("userID", event.UserID),
			zap.String("eventType", event.EventType))
		return err
	}
}
