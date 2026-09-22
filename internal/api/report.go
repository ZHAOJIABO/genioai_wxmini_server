package api

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/event"
	"va_visionai_server/internal/service/prompt"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type ReportServer struct {
	reportService      *service.ReportService
	eventService       *event.EventService
	watchService       *event.WatchService
	inspirationService *prompt.InspirationService
	// eventRegistry *event.EventRegistry

	vai.UnimplementedReportServiceServer
}

func NewReportServer(reportService *service.ReportService, eventService *event.EventService, watchService *event.WatchService, inspirationService *prompt.InspirationService) *ReportServer {
	return &ReportServer{
		reportService:      reportService,
		eventService:       eventService,
		watchService:       watchService,
		inspirationService: inspirationService,
	}
}

func (s *ReportServer) ReportLLMMsgQuality(ctx context.Context, req *vai.ReportLLMMsgQualityRequest) (*vai.ReportLLMMsgQualityResponse, error) {
	if err := s.reportService.ReportLLMMsgQuality(ctx, req.GetRequestHeader().GetUserId(), req.GetChatId(), req.GetMsgId(), req.GetQuality()); err != nil {
		header := common.BuildHeader(vai.StatusCode_REQUEST_FAILED)
		rsp := &vai.ReportLLMMsgQualityResponse{ResponseHeader: header}
		return rsp, nil
	}
	header := common.BuildHeader(vai.StatusCode_SUCCESS)
	return &vai.ReportLLMMsgQualityResponse{ResponseHeader: header}, nil
}

func (s *ReportServer) ReportUserEvent(ctx context.Context, req *vai.ReportUserEventRequest) (*vai.ReportUserEventResponse, error) {
	if req.GetRequestHeader() == nil || req.GetRequestHeader().GetUserId() == "" {
		return &vai.ReportUserEventResponse{
			ResponseHeader: common.BuildHeader(vai.StatusCode_INVALID_USER),
		}, nil
	}

	// 上报事件
	if err := s.eventService.ReportEvent(ctx, req.GetRequestHeader(), req.GetRequestHeader().GetUserId(), req.GetEventType().String(), req.GetEventData()); err != nil {
		zlog.LogWithContext(ctx).Error("failed to report event",
			zap.String("userID", req.GetRequestHeader().GetUserId()),
			zap.String("eventType", req.GetEventType().String()),
			zap.Error(err))
		return &vai.ReportUserEventResponse{
			ResponseHeader: common.BuildHeader(vai.StatusCode_REQUEST_FAILED),
		}, nil
	}

	// 灵感应用统计逻辑
	if req.GetEventType() == vai.UserEventType_TRACKING_EVENT {
		if trackingData, ok := req.GetEventData().(*vai.ReportUserEventRequest_TrackingData); ok {
			if trackingData.TrackingData.GetEventType() == vai.TrackingEventType_TRACKING_EVENT_TYPE_TOOLS_DETAIL_CREATE_BTN {
				s.handleInspirationTracking(ctx, req.GetRequestHeader().GetUserId(), trackingData.TrackingData)
			}
		}
	}

	return &vai.ReportUserEventResponse{
		ResponseHeader: common.BuildHeader(vai.StatusCode_SUCCESS),
	}, nil
}

func (s *ReportServer) EventWatch(req *vai.EventWatchRequest, stream vai.ReportService_EventWatchServer) error {
	return s.watchService.Watch(req, stream)
}

func (s *ReportServer) UserFeedback(ctx context.Context, req *vai.UserFeedbackRequest) (*vai.UserFeedbackResponse, error) {
	projectID := common.GetProjectID(ctx)
	// header json string
	headerInfo := req.GetRequestHeader().String()
	headerInfoJson, err := json.Marshal(req.GetRequestHeader())
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to marshal header info", zap.Error(err))
	} else {
		headerInfo = string(headerInfoJson)
	}
	screenshotUrls := req.GetScreenshotUrls()
	feedbackType := req.GetFeedbackType()
	feedbackContent := req.GetFeedbackContent()
	userID := req.GetRequestHeader().GetUserId()
	mail := req.GetEmail()

	// 如果提供了邮箱地址,则需要校验其格式
	if mail != "" && !common.IsValidEmail(mail) {
		zlog.LogWithContext(ctx).Error("invalid email format",
			zap.String("userID", userID),
			zap.String("email", mail))
		rsp := &vai.UserFeedbackResponse{
			ResponseHeader: common.BuildHeader(vai.StatusCode_INVALID_PARAM),
		}
		rsp.GetResponseHeader().Msg = "Invalid email format"
		return rsp, nil
	}

	if err := s.reportService.UserFeedback(ctx, projectID, userID, feedbackType, feedbackContent, headerInfo, screenshotUrls, mail); err != nil {
		return &vai.UserFeedbackResponse{
			ResponseHeader: common.BuildHeader(vai.StatusCode_REQUEST_FAILED),
		}, nil
	}
	return &vai.UserFeedbackResponse{
		ResponseHeader: common.BuildHeader(vai.StatusCode_SUCCESS),
	}, nil
}

// handleInspirationTracking 处理灵感应用统计
func (s *ReportServer) handleInspirationTracking(ctx context.Context, userID string, trackingData *vai.TrackingEventData) {
	// 1. 验证 extra_data
	extraData := trackingData.GetExtraData()
	if extraData == "" {
		zlog.LogWithContext(ctx).Warn("inspiration tracking: empty extra_data")
		return
	}

	// 2. 解析 JSON
	var data struct {
		InspirationPromptID string `json:"inspiration_prompt_id"`
		ToolID              string `json:"tool_id"`
		ToolType            string `json:"tool_type"`
	}

	if err := json.Unmarshal([]byte(extraData), &data); err != nil {
		zlog.LogWithContext(ctx).Error("inspiration tracking: failed to parse extra_data",
			zap.Error(err),
			zap.String("extra_data", extraData))
		return
	}

	// 3. 验证必填字段
	if data.InspirationPromptID == "" {
		zlog.LogWithContext(ctx).Warn("inspiration tracking: missing inspiration_prompt_id")
		return
	}

	// 4. 调用服务记录
	if err := s.inspirationService.ApplyInspirationPrompt(ctx, data.InspirationPromptID, userID, data.ToolID, data.ToolType); err != nil {
		zlog.LogWithContext(ctx).Error("inspiration tracking: failed to record application",
			zap.Error(err),
			zap.String("prompt_id", data.InspirationPromptID),
			zap.String("user_id", userID))
		return
	}

	zlog.LogWithContext(ctx).Debug("inspiration tracking: recorded application",
		zap.String("prompt_id", data.InspirationPromptID),
		zap.String("user_id", userID),
		zap.String("tool_id", data.ToolID))
}
