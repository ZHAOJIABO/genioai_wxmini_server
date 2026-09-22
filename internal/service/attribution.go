package service

import (
	"time"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
)

type attributionService struct {
}

func NewattributionService() *attributionService {
	return &attributionService{}
}

func (s *attributionService) RecordEventInfo(header *vai.RequestHeader, eventType string) (id uint, err error) {
	event := model.AttributionEvent{
		EventType: eventType,
		Platform:  constants.MappingOS(header.GetDevice().GetOs()),
		Date:      time.Now().Format("2006-01-02"),
		Hour:      time.Now().Format("15:04:05"),
		Idfv:      header.GetDevice().GetIdfv(),
		Idfa:      header.GetDevice().GetIdfa(),
		Oaid:      header.GetDevice().GetOaid(),
		AndroidId: header.GetDevice().GetAndroidId(),
		Userid:    header.GetUserId(),
		CreatedAt: time.Now(),
	}
	ddao := dao.AttributionDAO()
	return event.ID, ddao.CreateAttribution(&event)
}
