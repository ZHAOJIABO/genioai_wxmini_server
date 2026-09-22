package picture_forge

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	parentService "va_visionai_server/internal/service"
	"va_visionai_server/internal/service/cache"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// CreditRule 通用积分规则结构
type CreditRule struct {
	SourceGroup string `json:"source_group"` // duration, resolution等
	Value       string `json:"value"`        // 具体值
	Credit      int64  `json:"credit"`       // 积分
}

// CreditCalculationResult 积分计算结果
type CreditCalculationResult struct {
	FinalCredits      int    `json:"final_credits"`
	BaseCredits       int    `json:"base_credits"`
	ToolCredits       int    `json:"tool_credits,omitempty"`
	ExtraCredits      int    `json:"extra_credits,omitempty"`
	ExtraImageCredits int    `json:"extra_image_credits,omitempty"` // 多图额外积分
	CalculationType   string `json:"calculation_type"`              // "tool" 或 "workflow"
	ToolID            string `json:"tool_id,omitempty"`
	WorkflowID        string `json:"workflow_id,omitempty"`
}

type PictureForgeService struct {
	picForgeDao            *dao.PictureForgeDao
	taskDao                *dao.PictureTaskDao
	configService          *parentService.ConfigService
	workflowQueryService   *WorkflowQueryService
	themeManagementService *ThemeManagementService
	pictureToolsService    *PictureToolsService
}

func NewPictureForgeService(picForgeDao *dao.PictureForgeDao, taskDao *dao.PictureTaskDao, configService *parentService.ConfigService, cacheService *cache.CacheService) *PictureForgeService {
	service := &PictureForgeService{
		picForgeDao:   picForgeDao,
		taskDao:       taskDao,
		configService: configService,
	}

	service.workflowQueryService = NewWorkflowQueryService(picForgeDao, configService, cacheService)
	service.themeManagementService = NewThemeManagementService(picForgeDao, configService)
	service.pictureToolsService = NewPictureToolsService(picForgeDao)

	return service
}

func (s *PictureForgeService) FetchWorkflowsWithKinds(ctx context.Context, kindID, themeFilter, kindTypeFilter string, offset, limit int) ([]*model.Workflow, map[string]*model.WorkflowKind, int64, error) {
	return s.workflowQueryService.FetchWorkflowsWithKinds(ctx, kindID, themeFilter, kindTypeFilter, offset, limit, s.applyWorkflowTags)
}

func (s *PictureForgeService) ListWorkflowsPage(ctx context.Context, kindID string, theme vai.WorkflowTheme, kindType vai.WorkflowKindType, page, pageSize int32) ([]*vai.Workflow, int64, error) {
	return s.workflowQueryService.ListWorkflowsPage(ctx, kindID, theme, kindType, page, pageSize, s.applyWorkflowTags)
}

func (s *PictureForgeService) ListKindWithWorkflows(ctx context.Context, limit int, theme vai.WorkflowTheme) ([]*vai.WorkflowKind, error) {
	return s.workflowQueryService.ListKindWithWorkflows(ctx, limit, theme, s.applyWorkflowTags, s.applyWorkflowSortingRules)
}

func (s *PictureForgeService) ListBannerKindWithWorkflows(ctx context.Context) (*vai.WorkflowKind, *vai.WorkflowKind, error) {
	return s.workflowQueryService.ListBannerKindWithWorkflows(ctx, s.applyWorkflowTags, s.applyWorkflowSortingRules)
}

func (s *PictureForgeService) GetWorkflowByID(ctx context.Context, workflowID string) (*model.Workflow, error) {
	return s.workflowQueryService.GetWorkflowByID(ctx, workflowID)
}

func (s *PictureForgeService) GetKindInfoMap(ctx context.Context, kindIDs []string) (map[string]*model.WorkflowKind, error) {
	return s.workflowQueryService.GetKindInfoMap(ctx, kindIDs)
}

func (s *PictureForgeService) GetBuiltInPictures(ctx context.Context) ([]*vai.PictureInfo, error) {
	builtInPics, err := s.configService.GetConfValue(constants.BaseConfigKeyBuiltInPictures)
	if err != nil {
		return nil, errors.Wrap(err, "get built-in pictures")
	}

	var picInfos []*vai.PictureInfo
	if err := json.Unmarshal([]byte(builtInPics), &picInfos); err != nil {
		return nil, errors.Wrap(err, "unmarshal built-in pictures")
	}

	return picInfos, nil
}

func (s *PictureForgeService) GetCreationRestrictionRules(ctx context.Context) (*parentService.CreationRestrictionRules, error) {
	return s.configService.GetCreationRestrictionRules(ctx)
}

func (s *PictureForgeService) ListThemeWithWorkflows(ctx context.Context, lang string, limit int, kindType vai.WorkflowKindType, withoutKind bool) ([]*vai.WorkflowThemeInfoWithWorkflowKind, error) {
	return s.themeManagementService.ListThemeWithWorkflows(ctx, lang, limit, kindType, withoutKind, s.loadAndGroupWorkflows, s.toProtoWorkflows, s.applyWorkflowSortingRules, s.toProtoWorkflowKinds)
}

func (s *PictureForgeService) ListAllThemeInfo(ctx context.Context) ([]*vai.WorkflowThemeInfo, error) {
	return s.themeManagementService.ListAllThemeInfo(ctx)
}

func (s *PictureForgeService) ListPictureTools(ctx context.Context, appVersion string, platform string) ([]*vai.PictureToolsInfo, error) {
	return s.pictureToolsService.ListPictureTools(ctx, appVersion, platform)
}

func (s *PictureForgeService) GetToolEnhance(ctx context.Context, toolID string) ([]*vai.ToolEnhance, error) {
	return s.pictureToolsService.GetToolEnhance(ctx, toolID)
}

func (s *PictureForgeService) ValidateAndResolvePictureTool(ctx context.Context, req *vai.SubmitPictureToolsTaskRequest) (*model.PictureTools, error) {
	return s.pictureToolsService.ValidateAndResolvePictureTool(ctx, req)
}

func (s *PictureForgeService) GetToolByID(ctx context.Context, toolID string) (*model.PictureTools, error) {
	return s.pictureToolsService.GetToolByID(ctx, toolID)
}

// GetToolRecommend 生成工具推荐列表
//
// 推荐策略:
//  1. 固定首位: 从配置 ToolRecommendFirstToolID 指定的工具作为第一个推荐
//  2. 随机第二位: 从剩余候选中随机选择第二个推荐
//  3. 过滤规则: 排除配置中 BanToolIDs 指定的工具
//  4. 版本过滤: 仅返回与 appVersion 和 platform 兼容的工具
//
// 配置格式示例:
//
//	{
//	  "tool_recommend_first_tool_id": "tool_xxx",
//	  "ban_tool_id": ["tool_aaa", "tool_bbb"]
//	}
//
// 降级策略:
//   - 配置读取失败: 使用空配置(无固定工具,无ban列表)
//   - 固定工具不可用: 从候选中随机选择替代
//   - 候选工具不足: 返回实际可用数量(可能 < 2)
//
// 参数:
//   - appVersion: 应用版本,用于工具兼容性过滤
//   - platform: 平台类型,用于工具兼容性过滤
//
// 返回:
//   - 推荐工具列表,目标长度为2,实际可能少于2(当候选不足时)
//   - 错误信息
func (s *PictureForgeService) GetToolRecommend(ctx context.Context, appVersion, platform string) ([]*vai.PictureToolsInfo, error) {
	// Go 1.20+ math/rand 已自动初始化,无需手动设置种子
	// 读取推荐配置
	var fixedToolID string
	banned := make(map[string]struct{})
	if s.configService != nil {
		cfg, _ := s.configService.GetToolRecommendConfig(ctx)
		// GetToolRecommendConfig 总是返回配置(失败时返回默认值)
		if cfg != nil {
			fixedToolID = cfg.ToolRecommendFirstToolID
			for _, id := range cfg.BanToolIDs {
				if id != "" {
					banned[id] = struct{}{}
				}
			}
		}
	}

	// 获取可用工具（已按版本/平台过滤）
	toolsInfo, err := s.ListPictureTools(ctx, appVersion, platform)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list picture tools")
	}

	// 构建候选集合并剔除ban
	candidates := make([]*vai.PictureToolsInfo, 0, len(toolsInfo))
	toolMap := make(map[string]*vai.PictureToolsInfo, len(toolsInfo))
	for _, t := range toolsInfo {
		if t == nil || t.GetId() == "" {
			continue
		}
		toolMap[t.GetId()] = t
		if _, blocked := banned[t.GetId()]; blocked {
			continue
		}
		candidates = append(candidates, t)
	}

	// 选择推荐工具
	selected := make([]*vai.PictureToolsInfo, 0, 2)

	// 第一个: 尝试使用固定工具,如果不可用则随机选择
	firstTool := selectFixedTool(toolMap, fixedToolID)
	// 固定工具不在ban列表中才使用
	if firstTool != nil {
		if _, blocked := banned[fixedToolID]; !blocked {
			selected = append(selected, firstTool)
		}
	}
	// 如果固定工具不可用,从候选中随机选择
	if len(selected) == 0 && len(candidates) > 0 {
		selected = append(selected, selectRandomTool(candidates, ""))
	}

	// 第二个: 从剩余候选中随机选择
	if len(candidates) > 1 {
		excludeID := ""
		if len(selected) > 0 && selected[0] != nil {
			excludeID = selected[0].GetId()
		}
		secondTool := selectRandomTool(candidates, excludeID)
		if secondTool != nil {
			selected = append(selected, secondTool)
		}
	}

	// 确保不超过2个
	if len(selected) > 2 {
		selected = selected[:2]
	}

	return selected, nil
}

// selectFixedTool 根据配置选择固定推荐工具
// 如果固定工具不在候选列表中,返回 nil
func selectFixedTool(toolMap map[string]*vai.PictureToolsInfo, fixedID string) *vai.PictureToolsInfo {
	if fixedID == "" {
		return nil
	}
	if tool, ok := toolMap[fixedID]; ok {
		return tool
	}
	return nil
}

// selectRandomTool 从候选中随机选择一个工具,排除指定ID
func selectRandomTool(candidates []*vai.PictureToolsInfo, excludeID string) *vai.PictureToolsInfo {
	if len(candidates) == 0 {
		return nil
	}

	// 构建可选列表(排除已选)
	available := make([]*vai.PictureToolsInfo, 0, len(candidates))
	for _, tool := range candidates {
		if tool != nil && tool.GetId() != excludeID {
			available = append(available, tool)
		}
	}

	if len(available) == 0 {
		return nil
	}

	// 随机选择
	return available[rand.Intn(len(available))]
}

func (s *PictureForgeService) ListPictureToolsTaskResult(ctx context.Context, page, pageSize int32) ([]*vai.PictureToolsTaskDetail, int64, error) {
	return s.pictureToolsService.ListPictureToolsTaskResult(ctx, page, pageSize)
}

func (s *PictureForgeService) loadAndGroupWorkflows(ctx context.Context, kinds []*model.WorkflowKind) error {
	return s.workflowQueryService.loadAndGroupWorkflows(ctx, kinds, s.applyWorkflowTags)
}

func (s *PictureForgeService) toProtoWorkflows(workflows []*model.Workflow, kind *model.WorkflowKind, theme string) []*vai.Workflow {
	// 为单次调用构建轻量上下文，复用服务级缓存
	cc, _ := s.workflowQueryService.getTaskChainCreditCost(context.Background())
	params := s.workflowQueryService.getDefaultWorkflowExtraParams(context.Background())
	tctx := &workflowTransformContext{taskChainCreditCost: cc, defaultToolParams: params}
	return s.workflowQueryService.toProtoWorkflows(workflows, kind, theme, tctx)
}

func (s *PictureForgeService) toProtoWorkflowKinds(kinds []*model.WorkflowKind, limit int, theme string) []*vai.WorkflowKind {
	// 在该层构建一次上下文，保证当前聚合流程内仅获取一次
	cc, _ := s.workflowQueryService.getTaskChainCreditCost(context.Background())
	params := s.workflowQueryService.getDefaultWorkflowExtraParams(context.Background())
	tctx := &workflowTransformContext{taskChainCreditCost: cc, defaultToolParams: params}

	// 适配器：将无 tctx 的处理函数包装为带 tctx 的签名
	adapter := func(ws []*model.Workflow, k *model.WorkflowKind, th string, inner *workflowTransformContext) []*vai.Workflow {
		return s.workflowQueryService.toProtoWorkflows(ws, k, th, inner)
	}

	return s.workflowQueryService.toProtoWorkflowKinds(kinds, limit, theme, adapter, s.applyWorkflowSortingRules, tctx)
}

func (s *PictureForgeService) applyWorkflowSortingRules(workflows []*vai.Workflow) []*vai.Workflow {
	if len(workflows) <= 2 {
		shuffled := make([]*vai.Workflow, len(workflows))
		copy(shuffled, workflows)
		rand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		return shuffled
	}

	var taggedWorkflows []*vai.Workflow
	var untaggedWorkflows []*vai.Workflow

	for _, wf := range workflows {
		if wf.GetIsHot() || wf.GetIsNew() {
			taggedWorkflows = append(taggedWorkflows, wf)
		} else {
			untaggedWorkflows = append(untaggedWorkflows, wf)
		}
	}

	rand.Shuffle(len(untaggedWorkflows), func(i, j int) {
		untaggedWorkflows[i], untaggedWorkflows[j] = untaggedWorkflows[j], untaggedWorkflows[i]
	})

	if len(taggedWorkflows) <= 2 {
		rand.Shuffle(len(taggedWorkflows), func(i, j int) {
			taggedWorkflows[i], taggedWorkflows[j] = taggedWorkflows[j], taggedWorkflows[i]
		})
		result := make([]*vai.Workflow, 0, len(workflows))
		result = append(result, taggedWorkflows...)
		result = append(result, untaggedWorkflows...)
		return result
	}

	rand.Shuffle(len(taggedWorkflows), func(i, j int) {
		taggedWorkflows[i], taggedWorkflows[j] = taggedWorkflows[j], taggedWorkflows[i]
	})
	topTagged := taggedWorkflows[:2]
	remainingTagged := taggedWorkflows[2:]

	allUntagged := append(untaggedWorkflows, remainingTagged...)
	rand.Shuffle(len(allUntagged), func(i, j int) {
		allUntagged[i], allUntagged[j] = allUntagged[j], allUntagged[i]
	})

	result := make([]*vai.Workflow, 0, len(workflows))
	result = append(result, topTagged...)
	result = append(result, allUntagged...)
	return result
}

func (s *PictureForgeService) applyWorkflowTags(ctx context.Context, workflows []*model.Workflow) error {
	if len(workflows) == 0 {
		return nil
	}

	since := time.Now().AddDate(0, 0, -7)

	usageCounts, err := s.picForgeDao.GetWorkflowUsageCounts(context.Background(), since)
	if err != nil {
		return errors.Wrap(err, "get workflow usage counts")
	}

	kindMaxUsage := make(map[string]int64)
	for _, workflow := range workflows {
		if usage, exists := usageCounts[workflow.WorkflowID]; exists {
			if current, exists := kindMaxUsage[workflow.KindID]; !exists || usage > current {
				kindMaxUsage[workflow.KindID] = usage
			}
		}
	}

	for _, workflow := range workflows {
		workflow.Tags = []string{}

		if workflow.CreatedAt.After(since) {
			workflow.Tags = append(workflow.Tags, "New")
		}

		if usage, exists := usageCounts[workflow.WorkflowID]; exists {
			if maxUsage, exists := kindMaxUsage[workflow.KindID]; exists && maxUsage > 0 && usage == maxUsage {
				workflow.Tags = append(workflow.Tags, "Hot")
			}
		}
	}

	return nil
}

func (s *PictureForgeService) CalculateToolTaskCredits(ctx context.Context, toolID, duration, resolution string) (int, bool, error) {
	if toolID == "" {
		return 0, false, nil
	}

	tool, err := s.picForgeDao.GetPictureToolByID(ctx, toolID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			zlog.LogWithContext(ctx).Warn("tool not found for credit calculation", zap.String("tool_id", toolID))
			return 0, false, fmt.Errorf("invalid tool id: %s", toolID)
		}
		zlog.LogWithContext(ctx).Error("failed to get tool for credit calculation", zap.Error(err), zap.String("tool_id", toolID))
		return 0, false, fmt.Errorf("failed to retrieve tool details for id: %s", toolID)
	}

	if tool.ParamsJSON == "" {
		zlog.LogWithContext(ctx).Info("tool has no params_json for credit calculation", zap.String("tool_id", toolID))
		return 0, false, nil
	}

	var rules []ToolParamConfigItem
	if err := json.Unmarshal([]byte(tool.ParamsJSON), &rules); err != nil {
		zlog.LogWithContext(ctx).Error("failed to unmarshal tool params_json", zap.Error(err), zap.String("tool_id", toolID))
		return 0, true, fmt.Errorf("failed to parse credit rules for tool %s", toolID)
	}

	totalCredits := 0
	userSelections := make(map[string]string)
	userSelections["duration"] = duration
	userSelections["resolution"] = resolution

	for _, rule := range rules {
		for _, detail := range rule.Detail {
			if userValue, ok := userSelections[detail.SourceGroup]; ok {
				if userValue == detail.Value {
					totalCredits += int(detail.Credit)
					break
				}
			}
		}
	}

	zlog.LogWithContext(ctx).Info("calculated tool task credits",
		zap.String("tool_id", toolID),
		zap.Any("user_selections", userSelections),
		zap.Int("total_credits", totalCredits))

	return totalCredits, true, nil
}

// CalculateWorkflowExtraCredits 计算workflow额外积分
// 参考CalculateToolTaskCredits的实现方式，基于duration和resolution参数进行匹配计算
func (s *PictureForgeService) CalculateWorkflowExtraCredits(ctx context.Context, duration, resolution string, supportExtraParams []*model.WorkflowToolParam) (int, bool, error) {
	// 如果duration和resolution都为空，跳过计算
	if duration == "" && resolution == "" {
		return 0, false, nil
	}

	var toolParams []*model.WorkflowToolParam

	// 1. 检查workflow本身是否有SupportExtraParams配置
	if len(supportExtraParams) > 0 {
		toolParams = supportExtraParams
	} else {
		// 2. 如果没有，从默认配置读取
		defaultConfig, err := s.configService.GetConfValue(constants.ConfigKeyDefaultWorkflowExtraParams)
		if err != nil || defaultConfig == "" {
			return 0, false, nil
		}

		if err := json.Unmarshal([]byte(defaultConfig), &toolParams); err != nil {
			zlog.LogWithContext(ctx).Error("failed to unmarshal default workflow extra params", zap.Error(err))
			return 0, false, nil
		}
	}

	if len(toolParams) == 0 {
		return 0, false, nil
	}

	// 3. 根据用户选择的duration和resolution进行匹配计算
	totalCredits := 0
	userSelections := make(map[string]string)
	userSelections["duration"] = duration
	userSelections["resolution"] = resolution

	for _, param := range toolParams {
		if param == nil || param.SourceGroup == "" {
			continue
		}

		// 根据Label匹配对应的用户选择
		if userValue, ok := userSelections[param.SourceGroup]; ok && userValue != "" {
			// 如果用户输入值与配置值匹配，累加积分
			if userValue == param.Value {
				totalCredits += int(param.Credit)
			}
		}
	}

	if totalCredits > 0 {
		zlog.LogWithContext(ctx).Info("calculated workflow extra credits",
			zap.Any("user_selections", userSelections),
			zap.Int("total_credits", totalCredits))
		return totalCredits, true, nil
	}

	return 0, false, nil
}

// CalculateFinalCredits 统一计算最终积分
// 整合工具积分和工作流额外积分的计算逻辑
// quality: 模型直连模式下用户选择的质量参数，用于从 QualityCreditRules 查找额外积分
// imageCount: 用户上传的图片数量，每额外2张图片增加1积分
func (s *PictureForgeService) CalculateFinalCredits(ctx context.Context,
	workflowInfo *model.Workflow,
	toolID, duration, resolution, quality string,
	imageCount int) (*CreditCalculationResult, error) {

	if workflowInfo == nil {
		return nil, errors.New("workflow info cannot be nil")
	}

	result := &CreditCalculationResult{
		BaseCredits:     workflowInfo.CreditPoints,
		FinalCredits:    workflowInfo.CreditPoints,
		WorkflowID:      workflowInfo.WorkflowID,
		CalculationType: s.getCalculationType(toolID),
		ToolID:          toolID,
	}

	if toolID != "" {
		calcResult, err := s.calculateToolCredits(ctx, result, toolID, duration, resolution)
		if err != nil {
			return nil, err
		}
		s.applyExtraImageCredits(ctx, calcResult, imageCount)
		return calcResult, nil
	}

	calcResult, err := s.calculateWorkflowCredits(ctx, result, workflowInfo, duration, resolution, quality)
	if err != nil {
		return nil, err
	}
	s.applyExtraImageCredits(ctx, calcResult, imageCount)
	return calcResult, nil
}

// applyExtraImageCredits 计算多图额外积分
// 规则：每额外2张图片增加1积分，第1张不额外收费
func (s *PictureForgeService) applyExtraImageCredits(ctx context.Context, result *CreditCalculationResult, imageCount int) {
	if imageCount <= 1 {
		return
	}
	extraImageCredits := (imageCount - 1 + 1) / 2 // ceil((imageCount-1) / 2)
	result.ExtraImageCredits = extraImageCredits
	result.FinalCredits += extraImageCredits
	zlog.LogWithContext(ctx).Info("applied extra image credits",
		zap.Int("image_count", imageCount),
		zap.Int("extra_image_credits", extraImageCredits),
		zap.Int("final_credits", result.FinalCredits))
}

func (s *PictureForgeService) getCalculationType(toolID string) string {
	if toolID != "" {
		return "tool"
	}
	return "workflow"
}

func (s *PictureForgeService) calculateToolCredits(ctx context.Context, result *CreditCalculationResult, toolID, duration, resolution string) (*CreditCalculationResult, error) {
	toolCredits, isToolTask, err := s.CalculateToolTaskCredits(ctx, toolID, duration, resolution)
	if err != nil {
		return nil, fmt.Errorf("计算工具积分失败: %w", err)
	}

	if isToolTask && toolCredits > 0 {
		result.ToolCredits = toolCredits
		result.FinalCredits = toolCredits
		s.logCreditsCalculation(ctx, "tool", toolID, result.BaseCredits, toolCredits, result.FinalCredits)
	}

	return result, nil
}

func (s *PictureForgeService) calculateWorkflowCredits(ctx context.Context, result *CreditCalculationResult, workflowInfo *model.Workflow, duration, resolution, quality string) (*CreditCalculationResult, error) {
	extraCredits, hasExtra, err := s.CalculateWorkflowExtraCredits(ctx, duration, resolution, workflowInfo.SupportExtraParams)
	if err != nil {
		return nil, fmt.Errorf("计算工作流额外积分失败: %w", err)
	}

	if hasExtra && extraCredits > 0 {
		result.ExtraCredits = extraCredits
		result.FinalCredits = result.BaseCredits + extraCredits
		s.logCreditsCalculation(ctx, "workflow", workflowInfo.WorkflowID, result.BaseCredits, extraCredits, result.FinalCredits)
	}

	// 根据 quality 参数从 QualityCreditRules 查找额外积分
	if quality != "" && len(workflowInfo.QualityCreditRules) > 0 {
		if qualityExtra, ok := workflowInfo.QualityCreditRules[quality]; ok && qualityExtra > 0 {
			result.ExtraCredits += qualityExtra
			result.FinalCredits += qualityExtra
			zlog.LogWithContext(ctx).Info("applied quality credit rules",
				zap.String("quality", quality),
				zap.Int("quality_extra_credits", qualityExtra),
				zap.Int("final_credits", result.FinalCredits))
		}
	}

	return result, nil
}

func (s *PictureForgeService) logCreditsCalculation(ctx context.Context, calcType, id string, baseCredits, additionalCredits, finalCredits int) {
	zlog.LogWithContext(ctx).Info(fmt.Sprintf("applied %s credits", calcType),
		zap.String(calcType+"_id", id),
		zap.Int("base_credits", baseCredits),
		zap.Int(calcType+"_credits", additionalCredits),
		zap.Int("final_credits", finalCredits))
}

// GetWorkflowRecommend 获取工作流推荐列表
// 基于近7天使用率Top10的热门workflow进行随机推荐
// appVersion: 客户端版本号,用于版本兼容性过滤
// platform: 客户端平台(ios/android),用于选择对应的版本约束字段
func (s *PictureForgeService) GetWorkflowRecommend(ctx context.Context, limit int32, appVersion, platform string) ([]*vai.Workflow, error) {
	if limit <= 0 {
		limit = 10 // 默认返回10个
	}

	// 1. 获取近7天Top10热门workflow统计
	top10Stats, err := s.taskDao.GetTop10HotWorkflowsLast7Days(ctx, 10)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get top10 hot workflows")
	}

	// 2. 如果没有统计数据,返回空列表
	if len(top10Stats) == 0 {
		zlog.LogWithContext(ctx).Info("no hot workflows found in last 7 days")
		return []*vai.Workflow{}, nil
	}

	// 3. 随机选择workflow IDs
	selectedIDs := s.selectRandomWorkflowIDs(top10Stats, int(limit))
	if len(selectedIDs) == 0 {
		return []*vai.Workflow{}, nil
	}

	// 4. 批量查询workflow详细信息
	workflows, err := s.picForgeDao.ListWorkflowsByIDs(ctx, selectedIDs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get workflows by ids")
	}

	// 5. 收集需要的kind IDs并批量查询
	kindIDs := make([]string, 0, len(workflows))
	for _, wf := range workflows {
		if wf.KindID != "" {
			kindIDs = append(kindIDs, wf.KindID)
		}
	}

	kindInfoMap := make(map[string]*model.WorkflowKind)
	if len(kindIDs) > 0 {
		kindInfoMap, err = s.GetKindInfoMap(ctx, kindIDs)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("failed to get kind info map", zap.Error(err))
			// 不返回错误,继续处理
		}
	}

	// 5.5. 基于客户端版本过滤workflow
	beforeFilterCount := len(workflows)
	workflows = s.filterWorkflowsByKindVersion(ctx, workflows, kindInfoMap, appVersion, platform)
	zlog.LogWithContext(ctx).Info("workflow version filter applied",
		zap.Int("before_filter", beforeFilterCount),
		zap.Int("after_filter", len(workflows)),
		zap.String("app_version", appVersion),
		zap.String("platform", platform))

	// 6. 过滤不可用的workflow并转换为protobuf格式
	validWorkflows := make([]*vai.Workflow, 0, len(workflows))
	for _, wf := range workflows {
		if s.isValidWorkflowForRecommend(wf) {
			kindInfo := kindInfoMap[wf.KindID]
			pbWorkflow := s.convertWorkflowToProto(ctx, wf, kindInfo)
			if pbWorkflow != nil {
				validWorkflows = append(validWorkflows, pbWorkflow)
			}
		}
	}

	zlog.LogWithContext(ctx).Info("workflow recommend completed",
		zap.Int("top10_count", len(top10Stats)),
		zap.Int("selected_count", len(selectedIDs)),
		zap.Int("valid_count", len(validWorkflows)))

	return validWorkflows, nil
}

// selectRandomWorkflowIDs 从Top10中随机选择指定数量的workflow IDs
func (s *PictureForgeService) selectRandomWorkflowIDs(stats []dao.WorkflowUsageStats, limit int) []string {
	if len(stats) == 0 {
		return []string{}
	}

	result := make([]string, 0, limit)

	if len(stats) >= limit {
		// 场景1: Top10足够,随机选择limit个不重复
		indices := rand.Perm(len(stats))[:limit]
		for _, idx := range indices {
			result = append(result, stats[idx].WorkflowID)
		}
	} else {
		// 场景2: Top10不足,重复随机选择
		for i := 0; i < limit; i++ {
			idx := rand.Intn(len(stats))
			result = append(result, stats[idx].WorkflowID)
		}
	}

	return result
}

// isValidWorkflowForRecommend 检查workflow是否适合推荐
func (s *PictureForgeService) isValidWorkflowForRecommend(wf *model.Workflow) bool {
	if wf == nil {
		return false
	}

	// 检查是否下线
	if wf.Status != 1 { // status=1 表示正常
		return false
	}

	// 检查是否有示例图片
	if wf.ExampleJson == "" {
		return false
	}

	return true
}

// filterWorkflowsByKindVersion 基于客户端版本过滤workflow列表
// 根据workflow所属的WorkflowKind的版本约束,过滤掉不兼容的workflow
// appVersion: 客户端版本号,为空时跳过版本过滤
// platform: 客户端平台(ios/android),用于选择IOSupportVersions或AndroidSupportVersions字段
// 返回: 过滤后的workflow列表
func (s *PictureForgeService) filterWorkflowsByKindVersion(
	ctx context.Context,
	workflows []*model.Workflow,
	kindInfoMap map[string]*model.WorkflowKind,
	appVersion, platform string,
) []*model.Workflow {
	// 如果版本号为空,跳过版本过滤
	if appVersion == "" {
		zlog.LogWithContext(ctx).Info("appVersion is empty, skip version filter")
		return workflows
	}

	filtered := make([]*model.Workflow, 0, len(workflows))

	for _, wf := range workflows {
		// 获取workflow对应的kind信息
		kindInfo, exists := kindInfoMap[wf.KindID]
		if !exists {
			// kind信息缺失,采用宽松策略,保留该workflow
			zlog.LogWithContext(ctx).Warn("workflow kind not found, keep it",
				zap.String("workflow_id", wf.WorkflowID),
				zap.String("kind_id", wf.KindID))
			filtered = append(filtered, wf)
			continue
		}

		// 根据平台选择对应的版本约束字段
		supportVersions := ""
		switch platform {
		case constants.IOS:
			supportVersions = kindInfo.IOSupportVersions
		case constants.ANDROID:
			supportVersions = kindInfo.AndroidSupportVersions
		default:
			// 平台不明确,采用宽松策略,保留该workflow
			zlog.LogWithContext(ctx).Warn("unknown platform, keep workflow",
				zap.String("platform", platform),
				zap.String("workflow_id", wf.WorkflowID))
			filtered = append(filtered, wf)
			continue
		}

		// 版本约束为空,表示无版本限制,保留该workflow
		if supportVersions == "" {
			filtered = append(filtered, wf)
			continue
		}

		// 校验版本范围
		ok, err := utils.CheckVersionRange(appVersion, supportVersions)
		if err != nil {
			// 版本解析失败,记录警告并保留该workflow(宽松策略)
			zlog.LogWithContext(ctx).Warn("failed to check version range, keep workflow",
				zap.Error(err),
				zap.String("workflow_id", wf.WorkflowID),
				zap.String("app_version", appVersion),
				zap.String("support_versions", supportVersions))
			filtered = append(filtered, wf)
			continue
		}

		// 版本匹配成功,保留该workflow
		if ok {
			filtered = append(filtered, wf)
		} else {
			// 版本不匹配,过滤掉
			zlog.LogWithContext(ctx).Debug("workflow filtered by version",
				zap.String("workflow_id", wf.WorkflowID),
				zap.String("app_version", appVersion),
				zap.String("support_versions", supportVersions))
		}
	}

	return filtered
}

// convertWorkflowToProto 将model.Workflow转换为protobuf格式
func (s *PictureForgeService) convertWorkflowToProto(ctx context.Context, wf *model.Workflow, kindInfo *model.WorkflowKind) *vai.Workflow {
	if wf == nil {
		return nil
	}

	// 解析workflow的JSON字段(包括example_json和inputs_json)
	if err := ParseWorkflowJSONFields(wf); err != nil {
		zlog.LogWithContext(ctx).Error("failed to parse workflow json fields",
			zap.Error(err),
			zap.String("workflow_id", wf.WorkflowID))
		return nil
	}

	// 如果没有kindInfo,创建空对象避免panic
	if kindInfo == nil {
		kindInfo = &model.WorkflowKind{}
	}

	// 使用现有的转换工具函数
	cc, _ := s.workflowQueryService.getTaskChainCreditCost(ctx)
	params := s.workflowQueryService.getDefaultWorkflowExtraParams(ctx)

	return ConvertToProtoWorkflow(wf, kindInfo, "", cc, params)
}

// SaveCustomPromptHistory 工具任务提交成功后保存历史
// 通过哈希值快速检测重复，避免保存相同的 Prompt
func (s *PictureForgeService) SaveCustomPromptHistory(ctx context.Context, userID, toolID, toolType, prompt string) error {
	if userID == "" || toolID == "" || toolType == "" || prompt == "" {
		err := errors.New("invalid parameters: userID, toolID, toolType and prompt are required")
		zlog.LogWithContext(ctx).Warn("failed to save custom prompt history due to invalid parameters",
			zap.String("user_id", userID),
			zap.String("tool_id", toolID),
			zap.String("tool_type", toolType),
			zap.Bool("prompt_empty", prompt == ""),
			zap.Error(err))
		return err
	}

	// 生成 Prompt 哈希值用于去重
	promptHash := utils.GeneratePromptHash(prompt)

	// 检查该工具类型下是否已存在相同哈希的 Prompt
	exists, err := s.picForgeDao.CheckPromptHashExists(ctx, userID, toolType, promptHash)
	if err != nil {
		// 查询失败不影响主流程，记录日志后继续保存
		zlog.LogWithContext(ctx).Warn("failed to check prompt hash for deduplication, continue to save",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("tool_id", toolID),
			zap.String("tool_type", toolType))
	} else if exists {
		// 发现重复的 Prompt，跳过保存
		zlog.LogWithContext(ctx).Info("skip saving duplicate custom prompt",
			zap.String("user_id", userID),
			zap.String("tool_id", toolID),
			zap.String("tool_type", toolType),
			zap.String("prompt_hash", promptHash))
		return nil
	}

	// 不重复，保存新记录
	h := &model.CustomPromptHistory{
		HistoryID:    utils.GeneratePromptHistoryID(),
		UserID:       userID,
		ToolID:       toolID,
		ToolType:     toolType,
		CustomPrompt: prompt,
		PromptHash:   promptHash,
	}
	if err := s.picForgeDao.CreateCustomPromptHistory(ctx, h); err != nil {
		return LogAndWrapError(ctx, err, "failed to create custom prompt history record",
			zap.String("user_id", userID),
			zap.String("tool_id", toolID),
			zap.String("tool_type", toolType),
			zap.String("history_id", h.HistoryID))
	}
	zlog.LogWithContext(ctx).Info("successfully saved custom prompt history",
		zap.String("user_id", userID),
		zap.String("tool_id", toolID),
		zap.String("history_id", h.HistoryID),
		zap.String("prompt_hash", promptHash))
	return nil
}

// GetUserCustomPromptHistoryByToolType 查询用户某工具类型的自定义 Prompt 历史
func (s *PictureForgeService) GetUserCustomPromptHistoryByToolType(ctx context.Context, userID, toolID string, limit int) ([]*vai.CustomPromptHistoryItem, error) {
	if userID == "" || toolID == "" {
		err := errors.New("invalid parameters: userID and toolID are required")
		zlog.LogWithContext(ctx).Warn("failed to get custom prompt history by tool type due to invalid parameters",
			zap.String("user_id", userID),
			zap.String("tool_id", toolID),
			zap.Error(err))
		return nil, err
	}

	tool, err := s.GetToolByID(ctx, toolID)
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to get tool info for custom prompt history query by tool type",
			zap.String("user_id", userID),
			zap.String("tool_id", toolID),
			zap.Int("limit", limit))
	}
	if tool == nil {
		zlog.LogWithContext(ctx).Warn("tool not found when querying custom prompt history by tool type",
			zap.String("user_id", userID),
			zap.String("tool_id", toolID))
		return []*vai.CustomPromptHistoryItem{}, nil
	}
	if tool.ToolType == "" {
		zlog.LogWithContext(ctx).Warn("tool type is empty for custom prompt history query by tool type",
			zap.String("user_id", userID),
			zap.String("tool_id", toolID),
			zap.String("title", tool.Title))
		return []*vai.CustomPromptHistoryItem{}, nil
	}

	list, err := s.picForgeDao.ListCustomPromptHistoryByToolType(ctx, userID, tool.ToolType, limit)
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to list custom prompt history by tool type",
			zap.String("user_id", userID),
			zap.String("tool_id", toolID),
			zap.String("tool_type", tool.ToolType),
			zap.Int("limit", limit))
	}

	items := make([]*vai.CustomPromptHistoryItem, 0, len(list))
	for _, it := range list {
		items = append(items, &vai.CustomPromptHistoryItem{
			HistoryId:    it.HistoryID,
			CustomPrompt: it.CustomPrompt,
		})
	}

	zlog.LogWithContext(ctx).Info("successfully retrieved custom prompt history by tool type",
		zap.String("user_id", userID),
		zap.String("tool_id", toolID),
		zap.String("tool_type", tool.ToolType),
		zap.Int("count", len(items)))

	return items, nil
}

// DeleteCustomPromptHistory 删除用户的某条自定义 Prompt 历史
func (s *PictureForgeService) DeleteCustomPromptHistory(ctx context.Context, userID, historyID string) error {
	if userID == "" || historyID == "" {
		err := errors.New("invalid parameters: userID and historyID are required")
		zlog.LogWithContext(ctx).Warn("failed to delete custom prompt history due to invalid parameters",
			zap.String("user_id", userID),
			zap.String("history_id", historyID),
			zap.Error(err))
		return err
	}

	if err := s.picForgeDao.DeleteCustomPromptHistory(ctx, userID, historyID); err != nil {
		return LogAndWrapError(ctx, err, "failed to delete custom prompt history record",
			zap.String("user_id", userID),
			zap.String("history_id", historyID))
	}

	zlog.LogWithContext(ctx).Info("successfully deleted custom prompt history",
		zap.String("user_id", userID),
		zap.String("history_id", historyID))

	return nil
}

// ==================== Banner相关Service方法 ====================

// GetBannerList 获取Banner列表
func (s *PictureForgeService) GetBannerList(ctx context.Context) ([]*vai.BannerInfo, error) {
	// 从数据库获取Banner列表
	banners, err := s.picForgeDao.ListBanners(ctx)
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to list banners")
	}

	// 转换为proto格式
	result := make([]*vai.BannerInfo, 0, len(banners))
	for _, banner := range banners {
		if banner == nil {
			continue
		}

		// 从workflow相关信息中获取Title和KindType
		title, kindType := s.extractBannerTitleAndKindType(ctx, banner)

		result = append(result, &vai.BannerInfo{
			BannerKey:    banner.BannerKey,
			Title:        title,
			ImageUrl:     banner.ImageURL,
			ThumbnailUrl: banner.ThumbnailURL,
			LinkType:     vai.BannerLinkType(banner.LinkType),
			LinkAddr:     banner.LinkAddr,
			KindType:     kindType,
		})
	}

	zlog.LogWithContext(ctx).Info("successfully retrieved banner list",
		zap.Int("count", len(result)))

	return result, nil
}

// extractBannerTitleAndKindType 从workflow相关信息中提取Banner的Title和KindType
func (s *PictureForgeService) extractBannerTitleAndKindType(ctx context.Context, banner *model.BannerConfig) (string, vai.WorkflowKindType) {
	// 默认值
	title := banner.Title
	kindType := vai.WorkflowKindType_WORKFLOW_KIND_TYPE_UNKNOWN

	// 根据LinkType获取不同的信息源
	switch banner.LinkType {
	case 1: // KIND类型 - LinkAddr存储kindID
		if banner.LinkAddr != "" {
			kind, err := s.picForgeDao.GetWorkflowKind(ctx, banner.LinkAddr)
			if err != nil {
				zlog.LogWithContext(ctx).Warn("failed to get workflow kind for banner",
					zap.String("banner_key", banner.BannerKey),
					zap.String("kind_id", banner.LinkAddr),
					zap.Error(err))
			} else if kind != nil {
				title = kind.Label
				kindType = ConvertKindTypeEnum(kind.KindType)
			}
		}

	case 2: // TOOL类型 - LinkAddr存储toolID
		if banner.LinkAddr != "" {
			tool, err := s.GetToolByID(ctx, banner.LinkAddr)
			if err != nil {
				zlog.LogWithContext(ctx).Warn("failed to get tool for banner",
					zap.String("banner_key", banner.BannerKey),
					zap.String("tool_id", banner.LinkAddr),
					zap.Error(err))
			} else if tool != nil {
				title = tool.SimpleTitle
				// 通过tool的RefWorkflowID获取workflow的KindType
				if tool.RefWorkflowID != "" {
					workflow, err := s.GetWorkflowByID(ctx, tool.RefWorkflowID)
					if err != nil {
						zlog.LogWithContext(ctx).Warn("failed to get workflow for tool",
							zap.String("banner_key", banner.BannerKey),
							zap.String("workflow_id", tool.RefWorkflowID),
							zap.Error(err))
					} else if workflow != nil && workflow.KindID != "" {
						kind, err := s.picForgeDao.GetWorkflowKind(ctx, workflow.KindID)
						if err != nil {
							zlog.LogWithContext(ctx).Warn("failed to get kind for workflow",
								zap.String("banner_key", banner.BannerKey),
								zap.String("kind_id", workflow.KindID),
								zap.Error(err))
						} else if kind != nil {
							kindType = ConvertKindTypeEnum(kind.KindType)
						}
					}
				}
			}
		}

	default:
		// LinkType为0(不跳转)或其他值，使用默认值
		zlog.LogWithContext(ctx).Debug("banner has no link or unsupported link type",
			zap.String("banner_key", banner.BannerKey),
			zap.Int8("link_type", banner.LinkType))
	}

	return title, kindType
}

// Transaction 提供事务执行支持
func (s *PictureForgeService) Transaction(fc func(tx *gorm.DB) error) error {
	return s.picForgeDao.GetDB().Transaction(fc)
}

// ClickBanner 处理Banner点击事件
// 返回：是否展示弹窗，弹窗信息，错误
func (s *PictureForgeService) ClickBanner(ctx context.Context, userID, bannerKey string) (bool, *vai.PopupInfo, error) {
	// 1. 查询banner配置
	banner, err := s.picForgeDao.GetBannerByKey(ctx, bannerKey)
	if err != nil {
		return false, nil, LogAndWrapError(ctx, err, "failed to get banner by key")
	}

	// 检查banner是否有效（上架状态）
	if banner.Status != 1 {
		zlog.LogWithContext(ctx).Warn("banner is not active",
			zap.String("banner_key", bannerKey),
			zap.Int8("status", banner.Status))
		return false, nil, errors.New("banner is not active")
	}

	// 2. 检查用户是否已点击过
	clickRecord, err := s.picForgeDao.GetBannerClickRecord(ctx, userID, bannerKey)
	if err != nil {
		return false, nil, LogAndWrapError(ctx, err, "failed to get banner click record")
	}

	// 如果已经点击过，直接返回不需要弹窗
	if clickRecord != nil {
		zlog.LogWithContext(ctx).Info("user already clicked this banner",
			zap.String("user_id", userID),
			zap.String("banner_key", bannerKey))
		return false, nil, nil
	}

	// 3. 准备弹窗信息和点击记录
	shouldShowPopup := banner.EnablePopup
	pointsAwarded := 0

	// 如果开启了积分奖励，记录奖励积分数
	if banner.EnableReward {
		pointsAwarded = banner.RewardPoints
	}

	// 构建弹窗信息（如果需要弹窗）
	var popupInfo *vai.PopupInfo
	if shouldShowPopup {
		popupInfo = &vai.PopupInfo{
			PopupImageUrl:        banner.PopupImageUrl,
			PointsAwarded:        int32(pointsAwarded),
			PopupButtonText:      banner.PopupButtonText,
			PopupButtonColor:     banner.PopupButtonColor,
			PopupButtonTextColor: banner.PopupButtonTextColor,
		}
	}

	// 4. 创建点击记录（无论是否奖励都要记录点击）
	newRecord := &model.BannerClickRecord{
		UserID:        userID,
		BannerKey:     bannerKey,
		PointsAwarded: pointsAwarded,
		PopupShown:    shouldShowPopup,
		ClickTime:     time.Now(),
	}

	// 如果没有积分奖励，只需插入点击记录
	if !banner.EnableReward || pointsAwarded <= 0 {
		if err := s.picForgeDao.CreateBannerClickRecord(ctx, s.picForgeDao.GetDB(), newRecord); err != nil {
			zlog.LogWithContext(ctx).Error("failed to create banner click record",
				zap.String("user_id", userID),
				zap.String("banner_key", bannerKey),
				zap.Error(err))
			// 记录失败不影响返回结果，但记录日志
		}
		return shouldShowPopup, popupInfo, nil
	}

	// 5. 如果有积分奖励，需要在事务中执行（不在这里执行，由API层调用时处理）
	// 这里只返回需要弹窗的信息，实际的事务操作由调用方完成
	zlog.LogWithContext(ctx).Info("banner click requires reward",
		zap.String("user_id", userID),
		zap.String("banner_key", bannerKey),
		zap.Int("points", pointsAwarded))

	return shouldShowPopup, popupInfo, nil
}

// ClickBannerWithReward 处理带积分奖励的Banner点击（在事务中执行）
// 此方法用于API层在事务中调用
func (s *PictureForgeService) ClickBannerWithReward(ctx context.Context, tx *gorm.DB, userID, bannerKey string, pointsAwarded int, shouldShowPopup bool) error {
	// 创建点击记录
	newRecord := &model.BannerClickRecord{
		UserID:        userID,
		BannerKey:     bannerKey,
		PointsAwarded: pointsAwarded,
		PopupShown:    shouldShowPopup,
		ClickTime:     time.Now(),
	}

	if err := s.picForgeDao.CreateBannerClickRecord(ctx, tx, newRecord); err != nil {
		return LogAndWrapError(ctx, err, "failed to create banner click record in transaction")
	}

	return nil
}
