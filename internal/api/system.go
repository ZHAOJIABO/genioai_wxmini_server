package api

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type SystemServer struct {
	configService        *service.ConfigService
	popupNotificationDao *dao.PopupNotificationDao
	vai.UnimplementedSystemServiceServer
}

func NewSystemServer(popupNotificationDao *dao.PopupNotificationDao) *SystemServer {
	return &SystemServer{
		configService:        service.NewConfigService(),
		popupNotificationDao: popupNotificationDao,
	}
}

// GetAppConfiguration implements the GetAppConfiguration RPC method
func (s *SystemServer) GetAppConfiguration(ctx context.Context, req *vai.ConfigurationRequest) (*vai.ConfigurationResponse, error) {
	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.ConfigurationResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}
	os := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())
	projectID := req.GetRequestHeader().GetApp().GetPackageName()

	config, err := s.configService.GetAppConfiguration(ctx, projectID, os)
	if err != nil {
		return BuildErrorResponse[vai.ConfigurationResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get app configuration")
	}

	piclibConfig := s.configService.GetPiclibConfigration(ctx)

	return BuildSuccessResponse(&vai.ConfigurationResponse{
		Configuration: config,
		PiclibConfig:  piclibConfig,
	})
}

// GetFeedBackConfig implements the GetFeedBackConfig RPC method
func (s *SystemServer) GetFeedBackConfig(ctx context.Context, req *vai.FeedbackConfigRequest) (*vai.FeedbackConfigResponse, error) {
	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.FeedbackConfigResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}

	feedbackConf, err := service.Config.GetFeedBackConfig()
	if err != nil {
		return BuildErrorResponse[vai.FeedbackConfigResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get feedback config")
	}

	return BuildSuccessResponse(&vai.FeedbackConfigResponse{
		PopupCount:    feedbackConf.PopupCount,
		PopupInterval: feedbackConf.PopupInterval,
	})
}

// GetEmotionalStateList implements the GetEmotionalStateList RPC method
func (s *SystemServer) GetEmotionalStateList(ctx context.Context, req *vai.EmotionalStateListRequest) (*vai.EmotionalStateListResponse, error) {
	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.EmotionalStateListResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}
	language := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	emotionalStateList, err := s.configService.GetEmotionalStateList(ctx, language)
	if len(emotionalStateList) == 0 || err != nil {
		emotionalStateList, err = s.configService.GetEmotionalStateList(ctx, constants.EN)
		if err != nil {
			zlog.LogWithContext(ctx).Error("GetEmotionalStateList", zap.Error(err))
			return BuildErrorResponse[vai.EmotionalStateListResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get emotional state list")
		}
	}

	return BuildSuccessResponse(&vai.EmotionalStateListResponse{
		EmotionalStateList: emotionalStateList,
	})
}

// GetRelationList implements the GetRelationList RPC method
func (s *SystemServer) GetRelationList(ctx context.Context, req *vai.RelationListRequest) (*vai.RelationListResponse, error) {
	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.RelationListResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}
	language := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	relationList, err := s.configService.GetRelationList(ctx, language)
	if err != nil || len(relationList) == 0 {
		relationList, err = s.configService.GetRelationList(ctx, constants.EN)
		if err != nil {
			zlog.LogWithContext(ctx).Error("GetRelationList", zap.Error(err))
			return BuildErrorResponse[vai.RelationListResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get relation list")
		}
	}

	return BuildSuccessResponse(&vai.RelationListResponse{
		RelationList: relationList,
	})
}

// GetFixedTextData implements the GetFixedTextData RPC method
func (s *SystemServer) GetFixedTextData(ctx context.Context, req *vai.FixedTextDataRequest) (*vai.FixedTextDataResponse, error) {
	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.FixedTextDataResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}
	projectID := req.GetRequestHeader().GetApp().GetPackageName()
	language := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	configKey := fmt.Sprintf("%s_%s_fixed_text_data", projectID, language)
	var fixedTextData []*vai.FixedTextData
	err := s.configService.GetJSONConfig(configKey, &fixedTextData)
	if err != nil {
		// 如果获取失败，则使用默认语言
		configKey = fmt.Sprintf("%s_%s_fixed_text_data", projectID, constants.EN)
		err = s.configService.GetJSONConfig(configKey, &fixedTextData)
		if err != nil {
			return BuildErrorResponse[vai.FixedTextDataResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get fixed text data")
		}
	}

	return BuildSuccessResponse(&vai.FixedTextDataResponse{
		FixedTextData: fixedTextData,
	})
}

// GetPopupNotifications implements the GetPopupNotifications RPC method
func (s *SystemServer) GetPopupNotifications(ctx context.Context, req *vai.GetPopupNotificationsRequest) (*vai.GetPopupNotificationsResponse, error) {
	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.GetPopupNotificationsResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}

	// 从设备信息获取平台
	platform := strings.ToLower(constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs()))

	notifications, err := s.popupNotificationDao.GetActiveNotifications(ctx, platform)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetPopupNotifications", zap.Error(err))
		return BuildErrorResponse[vai.GetPopupNotificationsResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get popup notifications")
	}

	items := make([]*vai.PopupNotificationItem, 0, len(notifications))
	for _, n := range notifications {
		items = append(items, &vai.PopupNotificationItem{
			Id:               uint32(n.ID),
			Title:            n.Title,
			Content:          n.Content,
			ImageUrl:         n.ImageURL,
			LinkUrl:          n.LinkURL,
			NotificationType: n.NotificationType,
			Priority:         int32(n.Priority),
		})
	}

	return BuildSuccessResponse(&vai.GetPopupNotificationsResponse{
		Notifications: items,
	})
}
