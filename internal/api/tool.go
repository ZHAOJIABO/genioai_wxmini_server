package api

import (
	"context"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/service/picture_forge"
	vai "va_visionai_server/internal/va_interface"
)

// ToolServer 工具服务 gRPC 实现
type ToolServer struct {
	vai.UnimplementedToolServiceServer
	toolAggregateService *picture_forge.ToolAggregateService
}

// NewToolServer 创建工具服务实例
func NewToolServer(toolAggregateService *picture_forge.ToolAggregateService) *ToolServer {
	return &ToolServer{
		toolAggregateService: toolAggregateService,
	}
}

// ListToolsGroupByType 按分组返回工具列表
func (s *ToolServer) ListToolsGroupByType(
	ctx context.Context,
	req *vai.ListToolsGroupByTypeRequest,
) (*vai.ListToolsGroupByTypeResponse, error) {
	appVersion := req.GetRequestHeader().GetApp().GetAppVersion()
	platform := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())

	toolsGroupByType, err := s.toolAggregateService.ListToolsGroupByType(ctx, appVersion, platform)
	if err != nil {
		return BuildErrorResponse[vai.ListToolsGroupByTypeResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list tools group by type")
	}

	return &vai.ListToolsGroupByTypeResponse{
		ResponseHeader:        common.BuildResponseHeader(vai.StatusCode_SUCCESS, "success"),
		ToolsGroupByTypeInfos: toolsGroupByType,
	}, nil
}
