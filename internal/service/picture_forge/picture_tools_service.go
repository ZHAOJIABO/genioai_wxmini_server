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

type ToolParamConfigItem struct {
	Label  string                   `json:"label"`
	Detail []*ToolParamConfigDetail `json:"detail"`
}

type ToolParamConfigDetail struct {
	Label       string `json:"label"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Credit      int32  `json:"credit"`
	SourceGroup string `json:"source_group,omitempty"`
}

type PictureToolsService struct {
	picForgeDao *dao.PictureForgeDao
}

func NewPictureToolsService(picForgeDao *dao.PictureForgeDao) *PictureToolsService {
	return &PictureToolsService{
		picForgeDao: picForgeDao,
	}
}

func (s *PictureToolsService) batchLoadWorkflowsAndKinds(ctx context.Context, tools []*model.PictureTools) (map[string]*model.Workflow, map[string]*model.WorkflowKind, error) {
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

func (s *PictureToolsService) buildToolParamInfos(ctx context.Context, tool *model.PictureTools) ([]*vai.ToolParamsInfo, error) {
	if tool.ParamsJSON == "" {
		return []*vai.ToolParamsInfo{}, nil
	}
	return s.parseToolParamsFromJSON(ctx, tool.ParamsJSON)
}

func (s *PictureToolsService) parseToolParamsFromJSON(ctx context.Context, paramsJSON string) ([]*vai.ToolParamsInfo, error) {
	var configItems []*ToolParamConfigItem
	if err := json.Unmarshal([]byte(paramsJSON), &configItems); err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to unmarshal tool params JSON")
	}

	result := make([]*vai.ToolParamsInfo, 0, len(configItems))
	for _, configItem := range configItems {
		if configItem != nil {
			result = append(result, s.convertConfigToToolParam(configItem))
		}
	}

	return result, nil
}

func (s *PictureToolsService) convertConfigToToolParam(configItem *ToolParamConfigItem) *vai.ToolParamsInfo {
	if configItem == nil {
		return nil
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

	return &vai.ToolParamsInfo{
		Label:  configItem.Label,
		Detail: details,
	}
}

func (s *PictureToolsService) ListPictureTools(ctx context.Context, appVersion string, platform string) ([]*vai.PictureToolsInfo, error) {
	tools, err := s.picForgeDao.ListPictureTools(ctx)
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to list picture tools")
	}

	if len(tools) == 0 {
		return []*vai.PictureToolsInfo{}, nil
	}

	// 应用版本过滤逻辑（参考ThemeManagementService的实现）
	filteredTools := s.filterToolsByVersion(ctx, tools, appVersion)
	if len(filteredTools) == 0 {
		return []*vai.PictureToolsInfo{}, nil
	}

	// 应用平台过滤逻辑
	filteredTools = s.filterToolsByPlatform(ctx, filteredTools, platform)
	if len(filteredTools) == 0 {
		return []*vai.PictureToolsInfo{}, nil
	}

	// 应用首页隐藏过滤逻辑
	filteredTools = s.filterToolsForHome(ctx, filteredTools)
	if len(filteredTools) == 0 {
		return []*vai.PictureToolsInfo{}, nil
	}

	return s.buildPictureToolsInfo(ctx, filteredTools)
}

func (s *PictureToolsService) buildPictureToolsInfo(ctx context.Context, tools []*model.PictureTools) ([]*vai.PictureToolsInfo, error) {
	workflowMap, kindMap, err := s.batchLoadWorkflowsAndKinds(ctx, tools)
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to batch load workflows and kinds")
	}

	result := make([]*vai.PictureToolsInfo, 0, len(tools))
	for _, tool := range tools {
		var coverPicture *vai.PictureInfo
		if tool.Cover != "" {
			err := json.Unmarshal([]byte(tool.Cover), &coverPicture)
			if err != nil {
				zlog.LogWithContext(ctx).Warn("failed to unmarshal cover",
					zap.Any("toolID", tool.ID),
					zap.Error(err))
			}
		}

		inputCount := int32(0)
		var resultType vai.WorkflowKindType = vai.WorkflowKindType_WORKFLOW_KIND_TYPE_UNKNOWN

		if tool.RefWorkflowID != "" {
			if workflow, exists := workflowMap[tool.RefWorkflowID]; exists {
				if kindInfo, kindExists := kindMap[workflow.KindID]; kindExists {
					resultType = ConvertKindTypeEnum(kindInfo.KindType)
				}
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

		toolParamInfos, err := s.buildToolParamInfos(ctx, tool)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("failed to build tool param infos",
				zap.String("toolID", tool.ToolID),
				zap.Error(err))
			toolParamInfos = []*vai.ToolParamsInfo{}
		}

		toolInfo := &vai.PictureToolsInfo{
			Id:                        tool.ToolID,
			SimpleTitle:               tool.SimpleTitle,
			Title:                     tool.Title,
			Description:               tool.Description,
			Icon:                      tool.Icon,
			RecommendCover:            tool.RecommendCover,
			Cover:                     coverPicture,
			InputCount:                inputCount,
			ResultType:                resultType,
			SupportCustomPrompt:       tool.SupportCustomPrompt,
			IsVipTool:                 tool.IsVipTool,
			SupportPromptOptimization: tool.SupportPromptOptimization,
			ToolParamInfos:            toolParamInfos,
			ToolIcon:                  tool.ToolIcon,
			TagIcon:                   tool.TagIcon,
		}
		result = append(result, toolInfo)
	}

	return result, nil
}

func (s *PictureToolsService) GetToolEnhance(ctx context.Context, toolID string) ([]*vai.ToolEnhance, error) {
	tool, err := s.picForgeDao.GetPictureToolByID(ctx, toolID)
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to get picture tool", zap.String("toolID", toolID))
	}
	capabilities, err := s.picForgeDao.ListPictureToolCapabilities(ctx, tool.ToolID)
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to get picture tool capabilities", zap.String("toolID", toolID))
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

	return enhances, nil
}

func (s *PictureToolsService) ValidateAndResolvePictureTool(ctx context.Context, req *vai.SubmitPictureToolsTaskRequest) (*model.PictureTools, error) {
	tool, err := s.picForgeDao.GetPictureToolByID(ctx, req.GetPictureToolId())
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to get picture tool", zap.String("toolID", req.GetPictureToolId()))
	}

	if tool.RefWorkflowID == "" {
		return nil, LogAndWrapError(ctx, NewValidationError("tool has no associated workflow"), "picture tool has no associated workflow", zap.String("toolID", req.GetPictureToolId()))
	}

	return tool, nil
}

// GetToolByID 通过 tool_id 获取工具信息
func (s *PictureToolsService) GetToolByID(ctx context.Context, toolID string) (*model.PictureTools, error) {
	tool, err := s.picForgeDao.GetPictureToolByID(ctx, toolID)
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to get picture tool by id", zap.String("toolID", toolID))
	}
	return tool, nil
}

func (s *PictureToolsService) ListPictureToolsTaskResult(ctx context.Context, page, pageSize int32) ([]*vai.PictureToolsTaskDetail, int64, error) {
	tools, err := s.picForgeDao.ListPictureTools(ctx)
	if err != nil {
		return nil, 0, LogAndWrapError(ctx, err, "failed to list picture tools")
	}

	workflowIDs := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool.RefWorkflowID != "" {
			workflowIDs = append(workflowIDs, tool.RefWorkflowID)
		}
	}

	return []*vai.PictureToolsTaskDetail{}, 0, nil
}

func NewValidationError(message string) error {
	return &ValidationError{Message: message}
}

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// filterToolsByVersion filters picture tools based on version constraints
// 参考ThemeManagementService中的版本过滤逻辑
func (s *PictureToolsService) filterToolsByVersion(ctx context.Context, tools []*model.PictureTools, appVersion string) []*model.PictureTools {
	if appVersion == "" {
		return tools
	}

	filteredTools := make([]*model.PictureTools, 0, len(tools))
	for _, tool := range tools {
		// 如果工具没有版本限制，则包含在结果中
		if tool.SupportVersions == "" {
			filteredTools = append(filteredTools, tool)
			continue
		}

		// 检查app版本是否满足工具的版本要求
		ok, err := utils.CheckVersionRange(appVersion, tool.SupportVersions)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("failed to check version range for picture tool",
				zap.String("toolID", tool.ToolID),
				zap.String("appVersion", appVersion),
				zap.String("supportVersions", tool.SupportVersions),
				zap.Error(err))
			// 如果版本检查出错，为了安全起见，跳过这个工具
			continue
		}

		if ok {
			filteredTools = append(filteredTools, tool)
		}
	}

	return filteredTools
}

// filterToolsByPlatform filters picture tools based on platform constraints
// platform: constants.IOS, constants.ANDROID 或其他值
// PlatformDisplay: 0-全平台, 1-仅iOS, 2-仅Android
func (s *PictureToolsService) filterToolsByPlatform(ctx context.Context, tools []*model.PictureTools, platform string) []*model.PictureTools {
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

// filterToolsForHome 过滤首页隐藏的工具
// HideOnHome: true-首页隐藏(聚合页仍显示), false-首页显示(默认)
func (s *PictureToolsService) filterToolsForHome(ctx context.Context, tools []*model.PictureTools) []*model.PictureTools {
	filteredTools := make([]*model.PictureTools, 0, len(tools))
	for _, tool := range tools {
		if tool.HideOnHome {
			zlog.LogWithContext(ctx).Debug("tool hidden on home page",
				zap.String("toolID", tool.ToolID))
			continue
		}
		filteredTools = append(filteredTools, tool)
	}
	return filteredTools
}
