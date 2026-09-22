package event_handlers

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type AsaHandler struct {
	attributionDao *dao.AttributionDao
}

func NewAsaHandler(attributionDao *dao.AttributionDao) *AsaHandler {
	return &AsaHandler{
		attributionDao: attributionDao,
	}
}

func (a *AsaHandler) HandleEvent(ctx context.Context, event *model.UserEvent) error {
	zlog.LogWithContext(ctx).Info("handling Asa event",
		zap.String(constants.CtxEventType, event.EventType))
	var asaEvent vai.AttributionAsaEventData
	err := json.Unmarshal(event.EventData, &asaEvent)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to unmarshal asa attribution event",
			zap.Error(err))
		return err
	}

	newAsaEvent := &model.AsaEvent{
		Date:           event.CreatedAt.Format("2006-01-02"),
		Hour:           event.CreatedAt.Format("15:04:05"),
		Userid:         event.UserID,
		CreatedAt:      event.CreatedAt,
		Attribution:    asaEvent.GetAttribution(),
		OrgId:          asaEvent.GetOrgId(),
		CampaignId:     asaEvent.GetCampaignId(),
		ConversionType: asaEvent.GetConversionType(),
		ClickDate:      asaEvent.GetClickDate(),
		ClaimType:      asaEvent.GetClaimType(),
		AdGroupId:      asaEvent.GetAdGroupId(),
		CountyOrRegion: asaEvent.GetCountryOrRegion(),
		KeywordId:      asaEvent.GetKeywordId(),
		AdId:           asaEvent.GetAdId(),
	}
	err = a.attributionDao.CreateAsaEvent(newAsaEvent)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to upsert asa attribution event count",
			zap.String("event type", event.EventType),
			zap.Error(err))
		return err
	}
	return nil
}

func (a *AsaHandler) EventType() []string {
	return []string{
		vai.UserEventType_ATTRIBUTION_EVENT_ASA.String(),
	}
}
