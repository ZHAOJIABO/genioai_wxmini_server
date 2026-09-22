package picture_forge

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// ToolAggregateService 工具聚合服务，处理按分组展示工具的业务逻辑
type ToolAggregateService struct {
	picForgeDao *dao.PictureForgeDao
}

// NewToolAggregateService 创建工具聚合服务实例
func NewToolAggregateService(picForgeDao *dao.PictureForgeDao) *ToolAggregateService {
	return &ToolAggregateService{
		picForgeDao: picForgeDao,
	}
}

// ListToolsGroupByType 按分组返回工具列表
func (s *ToolAggregateService) ListToolsGroupByType(
	ctx context.Context,
	appVersion, platform string,
) ([]*vai.ToolsGroupByTypeInfo, error) {
	// 1. 获取所有分组（按 sort_order 排序）
	groups, err := s.picForgeDao.ListToolGroups(ctx)
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to list tool groups")
	}

	if len(groups) == 0 {
		return []*vai.ToolsGroupByTypeInfo{}, nil
	}

	// 2. 获取工具列表
	tools, err := s.picForgeDao.ListPictureTools(ctx)
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to list picture tools")
	}

	if len(tools) == 0 {
		return []*vai.ToolsGroupByTypeInfo{}, nil
	}

	// 3. 应用版本过滤
	tools = s.filterToolsByVersion(ctx, tools, appVersion)
	if len(tools) == 0 {
		return []*vai.ToolsGroupByTypeInfo{}, nil
	}

	// 4. 应用平台过滤
	tools = s.filterToolsByPlatform(ctx, tools, platform)
	if len(tools) == 0 {
		return []*vai.ToolsGroupByTypeInfo{}, nil
	}

	// 5. 批量加载 workflow 和 kind 信息
	workflowMap, kindMap, err := s.batchLoadWorkflowsAndKinds(ctx, tools)
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to batch load workflows and kinds")
	}

	// 6. 按 group_id 分组工具
	toolsByGroup := make(map[string][]*model.PictureTools)
	for _, tool := range tools {
		if tool.GroupID != "" {
			toolsByGroup[tool.GroupID] = append(toolsByGroup[tool.GroupID], tool)
		}
	}

	// 7. 构建响应（遍历分组，跳过空分组）
	result := make([]*vai.ToolsGroupByTypeInfo, 0, len(groups))
	for _, group := range groups {
		groupTools := toolsByGroup[group.GroupID]
		if len(groupTools) == 0 {
			continue // 空分组不返回
		}

		toolsInfos := s.buildToolsInfos(ctx, groupTools, workflowMap, kindMap)
		result = append(result, &vai.ToolsGroupByTypeInfo{
			ToolTypeLabel: group.GroupName,
			ToolsInfos:    toolsInfos,
		})
	}

	return result, nil
}

// batchLoadWorkflowsAndKinds 批量加载工作流和分类信息
func (s *ToolAggregateService) batchLoadWorkflowsAndKinds(
	ctx context.Context,
	tools []*model.PictureTools,
) (map[string]*model.Workflow, map[string]*model.WorkflowKind, error) {
	workflowIDSet := make(map[string]struct{})
	for _, tool := range tools {
		if tool.RefWorkflowID != "" {
			workflowIDSet[tool.RefWorkflowID] = struct{}{}
		}
	}

	if len(workflowIDSet) == 0 {
		return make(map[string]*model.Workflow), make(map[string]*model.WorkflowKind), nil
	}

	workflowIDs := make([]string, 0, len(workflowIDSet))
	for workflowID := range workflowIDSet {
		workflowIDs = append(workflowIDs, workflowID)
	}

	workflows, err := s.picForgeDao.ListWorkflowsByIDs(ctx, workflowIDs)
	if err != nil {
		return nil, nil, LogAndWrapError(ctx, err, "failed to list workflows by IDs")
	}

	workflowMap := make(map[string]*model.Workflow, len(workflows))
	kindIDSet := make(map[string]struct{})

	for _, workflow := range workflows {
		if err := ParseWorkflowJSONFields(workflow); err != nil {
			zlog.LogWithContext(ctx).Warn("failed to parse workflow JSON fields",
				zap.String("workflowID", workflow.WorkflowID),
				zap.Error(err))
		}
		workflowMap[workflow.WorkflowID] = workflow
		if workflow.KindID != "" {
			kindIDSet[workflow.KindID] = struct{}{}
		}
	}

	if len(kindIDSet) == 0 {
		return workflowMap, make(map[string]*model.WorkflowKind), nil
	}

	kindIDs := make([]string, 0, len(kindIDSet))
	for kindID := range kindIDSet {
		kindIDs = append(kindIDs, kindID)
	}

	kinds, err := s.picForgeDao.ListWorkflowKindsByIDs(ctx, kindIDs)
	if err != nil {
		return nil, nil, LogAndWrapError(ctx, err, "failed to list workflow kinds by IDs")
	}

	kindMap := make(map[string]*model.WorkflowKind, len(kinds))
	for _, kind := range kinds {
		kindMap[kind.KindID] = kind
	}

	return workflowMap, kindMap, nil
}

// buildToolsInfos 构建工具信息列表
func (s *ToolAggregateService) buildToolsInfos(
	ctx context.Context,
	tools []*model.PictureTools,
	workflowMap map[string]*model.Workflow,
	kindMap map[string]*model.WorkflowKind,
) []*vai.ToolsInfo {
	result := make([]*vai.ToolsInfo, 0, len(tools))

	for _, tool := range tools {
		toolInfo := s.buildSingleToolInfo(ctx, tool, workflowMap, kindMap)
		result = append(result, toolInfo)
	}

	return result
}

// buildSingleToolInfo 构建单个工具信息
func (s *ToolAggregateService) buildSingleToolInfo(
	ctx context.Context,
	tool *model.PictureTools,
	workflowMap map[string]*model.Workflow,
	kindMap map[string]*model.WorkflowKind,
) *vai.ToolsInfo {
	inputCount := int32(0)
	var resultType vai.ToolResultType = vai.ToolResultType_TOOL_RESULT_TYPE_UNKNOWN
	credit := int32(0)

	// 从关联的 workflow 和 kind 获取信息
	if tool.RefWorkflowID != "" {
		if workflow, exists := workflowMap[tool.RefWorkflowID]; exists {
			// 获取结果类型
			if kindInfo, kindExists := kindMap[workflow.KindID]; kindExists {
				resultType = s.convertKindTypeToResultType(kindInfo.KindType)
			}
			// 获取积分
			credit = int32(workflow.CreditPoints)
			// 统计图片输入数量
			if err := ParseWorkflowJSONFields(workflow); err == nil {
				for _, input := range workflow.Inputs {
					if input.InputType == int32(vai.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE) {
						inputCount++
					}
				}
			} else {
				zlog.LogWithContext(ctx).Warn("failed to parse workflow JSON fields",
					zap.String("workflowID", workflow.WorkflowID),
					zap.Error(err))
			}
		}
	}

	// 构建工具参数信息
	toolParamInfos := s.buildToolParamInfos(ctx, tool)

	// 构建工具能力信息
	toolEnhances := s.buildToolEnhances(ctx, tool)

	return &vai.ToolsInfo{
		ToolId:          tool.ToolID,
		Title:           tool.Title,
		Icon:            tool.AggregateIcon,    // 使用聚合页icon
		TagIcon:         tool.AggregateTagIcon, // 使用聚合页标签图片
		ImageInputCount: inputCount,
		ResultType:      resultType,
		IsVipTool:       tool.IsVipTool,
		Credit:          credit,
		ToolParamInfos:  toolParamInfos,
		ToolEnhances:    toolEnhances,
	}
}

// convertKindTypeToResultType 将 KindType 转换为 ToolResultType
func (s *ToolAggregateService) convertKindTypeToResultType(kindType string) vai.ToolResultType {
	switch kindType {
	case vai.WorkflowKindType_WORKFLOW_KIND_TYPE_VIDEO.String():
		return vai.ToolResultType_TOOL_RESULT_TYPE_VIDEO
	case vai.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE.String():
		return vai.ToolResultType_TOOL_RESULT_TYPE_IMAGE
	default:
		return vai.ToolResultType_TOOL_RESULT_TYPE_UNKNOWN
	}
}

// buildToolParamInfos 构建工具参数信息
func (s *ToolAggregateService) buildToolParamInfos(ctx context.Context, tool *model.PictureTools) []*vai.ToolParamsInfo {
	if tool.ParamsJSON == "" {
		return []*vai.ToolParamsInfo{}
	}

	var configItems []*ToolParamConfigItem
	if err := json.Unmarshal([]byte(tool.ParamsJSON), &configItems); err != nil {
		zlog.LogWithContext(ctx).Warn("failed to unmarshal tool params JSON",
			zap.String("toolID", tool.ToolID),
			zap.Error(err))
		return []*vai.ToolParamsInfo{}
	}

	result := make([]*vai.ToolParamsInfo, 0, len(configItems))
	for _, configItem := range configItems {
		if configItem == nil {
			continue
		}

		details := make([]*vai.ToolParamDetail, 0, len(configItem.Detail))
		for _, detail := range configItem.Detail {
			pbDetail := &vai.ToolParamDetail{
				Label:  detail.Label,
				Value:  detail.Value,
				Credit: detail.Credit,
			}
			details = append(details, pbDetail)
		}

		result = append(result, &vai.ToolParamsInfo{
			Label:  configItem.Label,
			Detail: details,
		})
	}

	return result
}

// buildToolEnhances 构建工具能力信息
func (s *ToolAggregateService) buildToolEnhances(ctx context.Context, tool *model.PictureTools) []*vai.ToolEnhance {
	capabilities, err := s.picForgeDao.ListPictureToolCapabilities(ctx, tool.ToolID)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("failed to list tool capabilities",
			zap.String("toolID", tool.ToolID),
			zap.Error(err))
		return []*vai.ToolEnhance{}
	}

	enhances := make([]*vai.ToolEnhance, 0, len(capabilities))
	for _, capability := range capabilities {
		enhance := &vai.ToolEnhance{
			Title:  capability.Title,
			Cover:  capability.Cover,
			Prompt: capability.Prompt,
		}
		enhances = append(enhances, enhance)
	}

	return enhances
}

// filterToolsByVersion 按版本过滤工具
func (s *ToolAggregateService) filterToolsByVersion(ctx context.Context, tools []*model.PictureTools, appVersion string) []*model.PictureTools {
	if appVersion == "" {
		return tools
	}

	filteredTools := make([]*model.PictureTools, 0, len(tools))
	for _, tool := range tools {
		if tool.SupportVersions == "" {
			filteredTools = append(filteredTools, tool)
			continue
		}

		ok, err := utils.CheckVersionRange(appVersion, tool.SupportVersions)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("failed to check version range for picture tool",
				zap.String("toolID", tool.ToolID),
				zap.String("appVersion", appVersion),
				zap.String("supportVersions", tool.SupportVersions),
				zap.Error(err))
			continue
		}

		if ok {
			filteredTools = append(filteredTools, tool)
		}
	}

	return filteredTools
}

// filterToolsByPlatform 按平台过滤工具
func (s *ToolAggregateService) filterToolsByPlatform(ctx context.Context, tools []*model.PictureTools, platform string) []*model.PictureTools {
	if platform == "" {
		return tools
	}

	filteredTools := make([]*model.PictureTools, 0, len(tools))
	for _, tool := range tools {
		// 0 表示全平台，无需过滤
		if tool.PlatformDisplay == 0 {
			filteredTools = append(filteredTools, tool)
			continue
		}

		// 1 表示仅iOS
		if tool.PlatformDisplay == 1 && platform == constants.IOS {
			filteredTools = append(filteredTools, tool)
			continue
		}

		// 2 表示仅Android
		if tool.PlatformDisplay == 2 && platform == constants.ANDROID {
			filteredTools = append(filteredTools, tool)
			continue
		}

		zlog.LogWithContext(ctx).Debug("tool filtered by platform",
			zap.String("toolID", tool.ToolID),
			zap.String("platform", platform),
			zap.Int32("platformDisplay", tool.PlatformDisplay))
	}

	return filteredTools
}
