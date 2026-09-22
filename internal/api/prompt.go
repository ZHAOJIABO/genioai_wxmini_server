package api

import (
	"context"

	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	pf "va_visionai_server/internal/service/picture_forge"
	"va_visionai_server/internal/service/prompt"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type PromptServer struct {
	vai.UnimplementedPromptServiceServer
	PromptService      *prompt.PromptService
	PromptKindService  *prompt.PromptKindService
	Optimizer          *prompt.OptimizerService
	PictureForge       *pf.PictureForgeService
	InspirationService *prompt.InspirationService
}

func NewPromptServer(promptService *prompt.PromptService, promptKindService *prompt.PromptKindService, optimizer *prompt.OptimizerService, pictureForge *pf.PictureForgeService, inspirationService *prompt.InspirationService) *PromptServer {
	return &PromptServer{
		PromptService:      promptService,
		PromptKindService:  promptKindService,
		Optimizer:          optimizer,
		PictureForge:       pictureForge,
		InspirationService: inspirationService,
	}
}
func (s *PromptServer) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, defaultTimeout)
}
func (s *PromptServer) ListPrompt(ctx context.Context, req *vai.ListPromptsRequest) (*vai.ListPromptsResponse, error) {
	rsp := &vai.ListPromptsResponse{}
	defer func(rsp *vai.ListPromptsResponse) {
		zlog.LogWithContext(ctx).Info("List Prompt",
			// zap.String("UserID", req.GetRequestHeader().GetUserId()),
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
		)
	}(rsp)
	language := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	promptList := s.PromptService.ListPromptResp(ctx, "", language)
	promptKindList := s.PromptKindService.ListPromptKindResp(language)
	s.PromptKindService.GroupPromptByKind(promptList, promptKindList)
	rsp.ResponseHeader = &vai.ResponseHeader{
		Code: vai.StatusCode_SUCCESS,
	}
	rsp.Kinds = promptKindList

	return rsp, nil
}

func (s *PromptServer) ListCameraPrompt(ctx context.Context, req *vai.ListCameraPromptRequest) (*vai.ListCameraPromptResponse, error) {
	rsp := &vai.ListCameraPromptResponse{}
	defer func(rsp *vai.ListCameraPromptResponse) {
		zlog.LogWithContext(ctx).Info("List Camera Prompt",
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
		)
	}(rsp)

	language := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	prompts, err := s.PromptService.ListCameraPrompt(ctx, req.GetKindId(), language)
	if err != nil {
		rsp.ResponseHeader = &vai.ResponseHeader{
			Code: vai.StatusCode_REQUEST_FAILED,
			Msg:  err.Error(),
		}
		return rsp, nil
	}

	rsp.ResponseHeader = &vai.ResponseHeader{
		Code: vai.StatusCode_SUCCESS,
	}
	rsp.Prompts = prompts
	return rsp, nil
}

func (s *PromptServer) PromptEnhance(ctx context.Context, req *vai.PromptEnhanceRequest) (*vai.PromptEnhanceResponse, error) {
	rsp := &vai.PromptEnhanceResponse{}
	defer func(rsp *vai.PromptEnhanceResponse) {
		zlog.LogWithContext(ctx).Info("PromptEnhance",
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
		)
	}(rsp)
	// 容错：请求不完整时直接回传原始 prompt
	original := ""
	if req != nil {
		original = req.GetPrompt()
	}
	if req == nil || req.GetRequestHeader() == nil || req.GetRequestHeader().GetUserId() == "" {
		rsp.ResponseHeader = &vai.ResponseHeader{
			Code: vai.StatusCode_INVALID_PARAM,
			Msg:  "invalid request",
		}
		rsp.Prompts = []string{original}
		return rsp, nil
	}

	userID := req.GetRequestHeader().GetUserId()
	toolID := req.GetToolId()
	final := []string{original}
	enhanced := false
	toolType := ""

	if s.Optimizer == nil {
		zlog.LogWithContext(ctx).Warn("PromptEnhance: optimizer service not initialized, fallback to original")
	} else {
		if s.PictureForge != nil && toolID != "" {
			if tool, err := s.PictureForge.GetToolByID(ctx, toolID); err != nil {
				zlog.LogWithContext(ctx).Warn("PromptEnhance: failed to get tool by id, ignore tool_type", zap.Error(err), zap.String("tool_id", toolID))
			} else if tool != nil {
				toolType = tool.ToolType
			}
		}
		enhancedPrompts, err := s.Optimizer.EnhancePrompt(ctx, original, req.GetImageUrls(), toolType)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("PromptEnhance: enhance failed, fallback to original", zap.Error(err))
		} else if len(enhancedPrompts) == 0 {
			zlog.LogWithContext(ctx).Warn("PromptEnhance: got empty enhanced prompt, fallback to original")
		} else {
			final = enhancedPrompts
			enhanced = true
		}
	}

	// 事件上报（通过 Service 层）
	if s.Optimizer != nil {
		s.Optimizer.TrackPromptEnhanceEvent(ctx, prompt.PromptEnhanceEventParams{
			UserID:          userID,
			OriginalPrompt:  original,
			EnhancedPrompts: final,
			ToolID:          toolID,
			ToolType:        toolType,
			ImageCount:      len(req.GetImageUrls()),
			Enhanced:        enhanced,
		})
	}

	rsp.ResponseHeader = &vai.ResponseHeader{
		Code: vai.StatusCode_SUCCESS,
	}
	rsp.Prompts = final
	return rsp, nil
}
func (s *PromptServer) GetUserCustomPromptHistory(ctx context.Context, req *vai.GetUserCustomPromptHistoryRequest) (*vai.GetUserCustomPromptHistoryResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if err := validateRequest(req.GetRequestHeader()); err != nil {
		return BuildErrorResponse[vai.GetUserCustomPromptHistoryResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid request header")
	}
	if req.GetToolId() == "" {
		return BuildErrorResponse[vai.GetUserCustomPromptHistoryResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "tool_id is required")
	}

	userID := common.GetUserID(ctx)
	limit := 10
	items, err := s.PictureForge.GetUserCustomPromptHistoryByToolType(ctx, userID, req.GetToolId(), limit)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get custom prompt history",
			zap.Error(err), zap.String("user_id", userID), zap.String("tool_id", req.GetToolId()))
		return BuildErrorResponse[vai.GetUserCustomPromptHistoryResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get custom prompt history")
	}

	resp := &vai.GetUserCustomPromptHistoryResponse{HistoryItems: items}
	return BuildSuccessResponse(resp)
}

func (s *PromptServer) DeleteCustomPromptHistory(ctx context.Context, req *vai.DeleteCustomPromptHistoryRequest) (*vai.DeleteCustomPromptHistoryResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if err := validateRequest(req.GetRequestHeader()); err != nil {
		return BuildErrorResponse[vai.DeleteCustomPromptHistoryResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid request header")
	}
	if req.GetHistoryId() == "" {
		return BuildErrorResponse[vai.DeleteCustomPromptHistoryResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "history_id is required")
	}

	userID := common.GetUserID(ctx)
	if err := s.PictureForge.DeleteCustomPromptHistory(ctx, userID, req.GetHistoryId()); err != nil {
		zlog.LogWithContext(ctx).Error("failed to delete custom prompt history",
			zap.Error(err), zap.String("user_id", userID), zap.String("history_id", req.GetHistoryId()))
		return BuildErrorResponse[vai.DeleteCustomPromptHistoryResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to delete custom prompt history")
	}

	return BuildSuccessResponse(&vai.DeleteCustomPromptHistoryResponse{})
}

// GetInspirationPrompts 获取灵感推荐
func (s *PromptServer) GetInspirationPrompts(ctx context.Context, req *vai.GetInspirationPromptsRequest) (*vai.GetInspirationPromptsResponse, error) {
	rsp := &vai.GetInspirationPromptsResponse{}

	// 参数校验
	if req == nil || req.GetRequestHeader() == nil {
		rsp.ResponseHeader = &vai.ResponseHeader{
			Code: vai.StatusCode_INVALID_PARAM,
			Msg:  "invalid request",
		}
		return rsp, nil
	}

	if req.GetToolId() == "" {
		rsp.ResponseHeader = &vai.ResponseHeader{
			Code: vai.StatusCode_INVALID_PARAM,
			Msg:  "tool_id is required",
		}
		return rsp, nil
	}

	// 获取工具类型
	toolType := ""
	if s.PictureForge != nil {
		tool, err := s.PictureForge.GetToolByID(ctx, req.GetToolId())
		if err != nil {
			zlog.LogWithContext(ctx).Error("GetInspirationPrompts: failed to get tool by id",
				zap.Error(err),
				zap.String("tool_id", req.GetToolId()))
			rsp.ResponseHeader = &vai.ResponseHeader{
				Code: vai.StatusCode_REQUEST_FAILED,
				Msg:  "failed to get tool info",
			}
			return rsp, nil
		}
		if tool == nil {
			zlog.LogWithContext(ctx).Warn("GetInspirationPrompts: tool not found",
				zap.String("tool_id", req.GetToolId()))
			rsp.ResponseHeader = &vai.ResponseHeader{
				Code: vai.StatusCode_REQUEST_FAILED,
				Msg:  "tool not found",
			}
			return rsp, nil
		}
		toolType = tool.ToolType
	}

	if toolType == "" {
		rsp.ResponseHeader = &vai.ResponseHeader{
			Code: vai.StatusCode_REQUEST_FAILED,
			Msg:  "tool_type not found",
		}
		return rsp, nil
	}

	// 获取灵感列表
	limit := 5
	cursor := req.GetPromptIdCursor() // 提取游标参数

	prompts, err := s.InspirationService.GetInspirationPrompts(ctx, toolType, cursor, int32(limit))
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetInspirationPrompts: service error",
			zap.Error(err),
			zap.String("tool_type", toolType),
			zap.String("cursor", cursor))
		rsp.ResponseHeader = &vai.ResponseHeader{
			Code: vai.StatusCode_REQUEST_FAILED,
			Msg:  err.Error(),
		}
		return rsp, nil
	}

	// 转换为协议格式
	respPrompts := make([]*vai.InspirationPrompt, 0, len(prompts))
	for _, p := range prompts {
		respPrompts = append(respPrompts, &vai.InspirationPrompt{
			PromptId: p.PromptID,
			Title:    p.Title,
			Content:  p.Content,
		})
	}

	rsp.ResponseHeader = &vai.ResponseHeader{
		Code: vai.StatusCode_SUCCESS,
	}
	rsp.Prompts = respPrompts

	// 记录日志
	// 说明: 客户端从返回的prompts中取最后一条的prompt_id作为下次请求的cursor
	zlog.LogWithContext(ctx).Info("GetInspirationPrompts",
		zap.String("tool_id", req.GetToolId()),
		zap.String("cursor", cursor),
		zap.Int("returned_count", len(prompts)),
		zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
		zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
	)

	return rsp, nil
}
