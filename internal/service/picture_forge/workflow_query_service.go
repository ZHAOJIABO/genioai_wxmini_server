package picture_forge

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/cache"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type WorkflowQueryService struct {
	picForgeDao   *dao.PictureForgeDao
	configService *service.ConfigService
	cacheService  *cache.CacheService
}

// workflowTransformContext 在单次请求处理周期内复用公用数据，避免重复获取
type workflowTransformContext struct {
	taskChainCreditCost int32
	defaultToolParams   []*model.WorkflowToolParam
}

func NewWorkflowQueryService(picForgeDao *dao.PictureForgeDao, configService *service.ConfigService, cacheService *cache.CacheService) *WorkflowQueryService {
	return &WorkflowQueryService{
		picForgeDao:   picForgeDao,
		configService: configService,
		cacheService:  cacheService,
	}
}

func (s *WorkflowQueryService) buildWorkflowFilters(theme vai.WorkflowTheme, kindType vai.WorkflowKindType, page, pageSize int32) (themeFilter, kindTypeFilter string, offset, limit int) {
	offset = 0
	limit = int(pageSize)
	if pageSize != -1 {
		offset = int((page - 1) * pageSize)
	}

	if theme != vai.WorkflowTheme_WORKFLOW_THEME_UNKNOWN {
		themeFilter = theme.String()
	}

	if kindType != vai.WorkflowKindType_WORKFLOW_KIND_TYPE_UNKNOWN {
		kindTypeFilter = kindType.String()
	}

	return
}

func (s *WorkflowQueryService) extractUniqueKindIDs(workflows []*model.Workflow) []string {
	seen := make(map[string]struct{})
	uniqueKindIDs := make([]string, 0, len(workflows))

	for _, w := range workflows {
		if _, ok := seen[w.KindID]; !ok {
			seen[w.KindID] = struct{}{}
			uniqueKindIDs = append(uniqueKindIDs, w.KindID)
		}
	}

	return uniqueKindIDs
}

func (s *WorkflowQueryService) FetchWorkflowsWithKinds(ctx context.Context, kindID, themeFilter, kindTypeFilter string, offset, limit int, tagProcessor func(context.Context, []*model.Workflow) error) ([]*model.Workflow, map[string]*model.WorkflowKind, int64, error) {
	workflows, total, err := s.picForgeDao.ListWorkflows(ctx, kindID, themeFilter, kindTypeFilter, offset, limit)
	if err != nil {
		return nil, nil, 0, LogAndWrapError(ctx, err, "failed to list workflows",
			zap.String("kindID", kindID),
			zap.String("theme", themeFilter),
			zap.String("kindType", kindTypeFilter))
	}

	if len(workflows) == 0 {
		return workflows, make(map[string]*model.WorkflowKind), total, nil
	}

	if err := tagProcessor(ctx, workflows); err != nil {
		zlog.LogWithContext(ctx).Error("failed to apply workflow tags", zap.Error(err))
	}

	uniqueKindIDs := s.extractUniqueKindIDs(workflows)
	kindInfos, err := s.picForgeDao.ListWorkflowKindsByIDs(ctx, uniqueKindIDs)
	if err != nil {
		return nil, nil, 0, LogAndWrapError(ctx, err, "get workflow kinds info")
	}

	kindInfoMap := make(map[string]*model.WorkflowKind, len(kindInfos))
	for _, ki := range kindInfos {
		kindInfoMap[ki.KindID] = ki
	}

	return workflows, kindInfoMap, total, nil
}

func (s *WorkflowQueryService) processWorkflowsForResponse(ctx context.Context, workflows []*model.Workflow, kindInfoMap map[string]*model.WorkflowKind, themeFilter string, tctx *workflowTransformContext) []*vai.Workflow {
	result := make([]*vai.Workflow, 0, len(workflows))
	// 优先使用单请求上下文中的值
	taskChainCreditCost := tctx.taskChainCreditCost
	defaultToolParams := tctx.defaultToolParams

	for _, w := range workflows {
		kindInfo, ok := kindInfoMap[w.KindID]
		if !ok {
			zlog.LogWithContext(ctx).Warn("workflow has no corresponding kind info",
				zap.String("workflowID", w.WorkflowID),
				zap.String("kindID", w.KindID))
			continue
		}

		if err := ParseWorkflowJSONFields(w); err != nil {
			zlog.LogWithContext(ctx).Error("failed to parse workflow JSON fields",
				zap.String("workflowID", w.WorkflowID),
				zap.Error(err))
			continue
		}

		protoWorkflow := ConvertToProtoWorkflow(w, kindInfo, themeFilter, taskChainCreditCost, defaultToolParams)
		result = append(result, protoWorkflow)
	}

	return result
}

func (s *WorkflowQueryService) ListWorkflowsPage(ctx context.Context, kindID string, theme vai.WorkflowTheme, kindType vai.WorkflowKindType, page, pageSize int32, tagProcessor func(context.Context, []*model.Workflow) error) ([]*vai.Workflow, int64, error) {
	themeFilter, kindTypeFilter, offset, limit := s.buildWorkflowFilters(theme, kindType, page, pageSize)

	workflows, kindInfoMap, total, err := s.FetchWorkflowsWithKinds(ctx, kindID, themeFilter, kindTypeFilter, offset, limit, tagProcessor)
	if err != nil {
		return nil, 0, err
	}

	if len(workflows) == 0 {
		return []*vai.Workflow{}, 0, nil
	}
	// 构建单请求级上下文，避免多次读取
	cc, err := s.getTaskChainCreditCost(ctx)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get task chain credit cost", zap.Error(err))
	}
	params := s.getDefaultWorkflowExtraParams(ctx)
	tctx := &workflowTransformContext{taskChainCreditCost: cc, defaultToolParams: params}

	result := s.processWorkflowsForResponse(ctx, workflows, kindInfoMap, themeFilter, tctx)
	return result, total, nil
}

func (s *WorkflowQueryService) processWorkflowList(ctx context.Context, workflows []*model.Workflow, kindInfo *model.WorkflowKind, tctx *workflowTransformContext) []*vai.Workflow {
	result := make([]*vai.Workflow, 0, len(workflows))
	// 优先使用单请求上下文中的值
	taskChainCreditCost := tctx.taskChainCreditCost
	defaultToolParams := tctx.defaultToolParams

	for _, w := range workflows {
		if err := ParseWorkflowJSONFields(w); err != nil {
			zlog.LogWithContext(ctx).Error("failed to parse workflow JSON fields",
				zap.String("workflowID", w.WorkflowID),
				zap.Error(err))
			continue
		}

		protoWorkflow := ConvertToProtoWorkflow(w, kindInfo, "", taskChainCreditCost, defaultToolParams)
		result = append(result, protoWorkflow)
	}

	return result
}

func (s *WorkflowQueryService) loadAndGroupWorkflows(ctx context.Context, kinds []*model.WorkflowKind, tagProcessor func(context.Context, []*model.Workflow) error) error {
	if len(kinds) == 0 {
		return nil
	}

	kindIDs := make([]string, len(kinds))
	kindMap := make(map[string]*model.WorkflowKind, len(kinds))
	for i, k := range kinds {
		kindIDs[i] = k.KindID
		kindMap[k.KindID] = k
	}

	workflows, err := s.picForgeDao.ListAllWorkflows(ctx, kindIDs)
	if err != nil {
		return LogAndWrapError(ctx, err, "failed to list all workflows")
	}

	for _, w := range workflows {
		if err := ParseWorkflowJSONFields(w); err != nil {
			zlog.LogWithContext(ctx).Error("failed to parse workflow JSON fields",
				zap.String("workflowID", w.WorkflowID),
				zap.Error(err))
			continue
		}
		kindMap[w.KindID].Workflows = append(kindMap[w.KindID].Workflows, w)
	}

	if err := tagProcessor(ctx, workflows); err != nil {
		zlog.LogWithContext(ctx).Error("failed to apply workflow tags", zap.Error(err))
	}

	return nil
}

func (s *WorkflowQueryService) toProtoWorkflowKinds(kinds []*model.WorkflowKind, limit int, theme string, workflowProcessor func([]*model.Workflow, *model.WorkflowKind, string, *workflowTransformContext) []*vai.Workflow, sortingRules func([]*vai.Workflow) []*vai.Workflow, tctx *workflowTransformContext) []*vai.WorkflowKind {
	result := make([]*vai.WorkflowKind, 0, len(kinds))
	for _, k := range kinds {
		ws := k.Workflows
		protoWorkflows := workflowProcessor(ws, k, theme, tctx)
		sortedWorkflows := sortingRules(protoWorkflows)
		if limit > 0 && len(sortedWorkflows) > limit {
			sortedWorkflows = sortedWorkflows[:limit]
		}
		protoKind := ConvertToProtoWorkflowKind(k, sortedWorkflows, theme)
		result = append(result, protoKind)
	}
	return result
}

func (s *WorkflowQueryService) toProtoWorkflows(workflows []*model.Workflow, kind *model.WorkflowKind, theme string, tctx *workflowTransformContext) []*vai.Workflow {
	protoWorkflows := make([]*vai.Workflow, 0, len(workflows))
	// 使用请求上下文传递的值，避免重复获取
	taskChainCreditCost := tctx.taskChainCreditCost
	defaultToolParams := tctx.defaultToolParams

	for _, w := range workflows {
		protoWorkflow := ConvertToProtoWorkflow(w, kind, theme, taskChainCreditCost, defaultToolParams)
		protoWorkflows = append(protoWorkflows, protoWorkflow)
	}
	return protoWorkflows
}

// getTaskChainCreditCost 使用缓存获取任务链积分成本，失败时降级到DB
func (s *WorkflowQueryService) getTaskChainCreditCost(ctx context.Context) (int32, error) {
	var credit int32
	// 15分钟TTL，平衡一致性与性能
	err := s.cacheService.GetOrSet(ctx, constants.ConfigKeyPictureToVideoGuideWorkflowID, &credit, 15*time.Minute, func() (interface{}, error) {
		return s.fetchTaskChainCreditCostFromDB(ctx)
	})
	if err != nil {
		zlog.LogWithContext(ctx).Warn("getTaskChainCreditCost fallback to DB", zap.Error(err))
		return s.fetchTaskChainCreditCostFromDB(ctx)
	}
	return credit, nil
}

// fetchTaskChainCreditCostFromDB 直接从DB读取任务链积分成本
func (s *WorkflowQueryService) fetchTaskChainCreditCostFromDB(ctx context.Context) (int32, error) {
	taskChainID, err := s.configService.GetConfValue(constants.ConfigKeyPictureToVideoGuideWorkflowID)
	if err != nil {
		return 0, errors.Wrap(err, "get built-in workflow id failed")
	}

	taskChain, err := s.picForgeDao.GetWorkflow(ctx, db.GetDB(), taskChainID)
	if err != nil {
		return 0, errors.Wrap(err, "get built-in workflow failed")
	}

	return int32(taskChain.CreditPoints), nil
}

// InvalidateTaskChainCreditCostCache 主动失效任务链积分成本缓存
func (s *WorkflowQueryService) InvalidateTaskChainCreditCostCache(ctx context.Context) error {
	return s.cacheService.Del(ctx, "workflow:task_chain_credit_cost")
}

func (s *WorkflowQueryService) ListKindWithWorkflows(ctx context.Context, limit int, theme vai.WorkflowTheme, tagProcessor func(context.Context, []*model.Workflow) error, sortingRules func([]*vai.Workflow) []*vai.Workflow) ([]*vai.WorkflowKind, error) {
	lang := common.GetLang(ctx)

	themeFilter := ""
	if theme != vai.WorkflowTheme_WORKFLOW_THEME_UNKNOWN {
		themeFilter = theme.String()
	}

	kinds, err := s.picForgeDao.ListWorkflowKinds(ctx, lang, "", themeFilter)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to list workflow kinds", zap.Error(err))
		return nil, errors.Wrap(err, "list workflow kinds")
	}

	if len(kinds) == 0 && lang != constants.EN {
		kinds, err = s.picForgeDao.ListWorkflowKinds(ctx, constants.EN, "", themeFilter)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to list workflow kinds with fallback language", zap.Error(err))
			return nil, errors.Wrap(err, "list workflow kinds with fallback language")
		}
	}

	if err := s.loadAndGroupWorkflows(ctx, kinds, tagProcessor); err != nil {
		return nil, errors.Wrap(err, "load and group workflows")
	}
	// 构建单请求级上下文，避免多次读取
	cc, err := s.getTaskChainCreditCost(ctx)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get task chain credit cost", zap.Error(err))
	}
	params := s.getDefaultWorkflowExtraParams(ctx)
	tctx := &workflowTransformContext{taskChainCreditCost: cc, defaultToolParams: params}

	return s.toProtoWorkflowKinds(kinds, limit, themeFilter, s.toProtoWorkflows, sortingRules, tctx), nil
}

func (s *WorkflowQueryService) ListBannerKindWithWorkflows(ctx context.Context, tagProcessor func(context.Context, []*model.Workflow) error, sortingRules func([]*vai.Workflow) []*vai.Workflow) (*vai.WorkflowKind, *vai.WorkflowKind, error) {
	lang := common.GetLang(ctx)

	kinds, err := s.picForgeDao.ListBannerWorkflowKinds(ctx, lang)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to list banner workflow kinds", zap.Error(err))
		return nil, nil, errors.Wrap(err, "list banner workflow kinds")
	}

	if len(kinds) == 0 && lang != constants.EN {
		kinds, err = s.picForgeDao.ListBannerWorkflowKinds(ctx, constants.EN)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to list banner workflow kinds with fallback language", zap.Error(err))
			return nil, nil, errors.Wrap(err, "list banner workflow kinds with fallback language")
		}
	}

	if err := s.loadAndGroupWorkflows(ctx, kinds, tagProcessor); err != nil {
		return nil, nil, errors.Wrap(err, "load and group workflows")
	}
	// 构建单请求级上下文，避免多次读取
	cc, err := s.getTaskChainCreditCost(ctx)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get task chain credit cost", zap.Error(err))
	}
	params := s.getDefaultWorkflowExtraParams(ctx)
	tctx := &workflowTransformContext{taskChainCreditCost: cc, defaultToolParams: params}
	protoKinds := s.toProtoWorkflowKinds(kinds, 0, "", s.toProtoWorkflows, sortingRules, tctx)
	var person, pet *vai.WorkflowKind
	for _, k := range protoKinds {
		if k.GetKindId() == constants.WorkflowBannerKindIDPerson {
			person = k
		} else if k.GetKindId() == constants.WorkflowBannerKindIDPet {
			pet = k
		}
	}
	return person, pet, nil
}

func (s *WorkflowQueryService) GetWorkflowByID(ctx context.Context, workflowID string) (*model.Workflow, error) {
	workflow, err := s.picForgeDao.GetWorkflow(ctx, db.GetDB(), workflowID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("[(s *WorkflowQueryService) GetWorkflowByID] failed to get workflow", zap.Error(err))
		return nil, errors.Wrap(err, "get workflow by id")
	}
	return workflow, nil
}

func (s *WorkflowQueryService) GetKindInfoMap(ctx context.Context, kindIDs []string) (map[string]*model.WorkflowKind, error) {
	if len(kindIDs) == 0 {
		return make(map[string]*model.WorkflowKind), nil
	}

	uniqueKindIDs := make([]string, 0, len(kindIDs))
	seen := make(map[string]bool)
	for _, kindID := range kindIDs {
		if !seen[kindID] {
			uniqueKindIDs = append(uniqueKindIDs, kindID)
			seen[kindID] = true
		}
	}

	kinds, err := s.picForgeDao.ListWorkflowKindsByIDs(ctx, uniqueKindIDs)
	if err != nil {
		return nil, LogAndWrapError(ctx, err, "failed to get workflow kinds by IDs")
	}

	kindInfoMap := make(map[string]*model.WorkflowKind, len(kinds))
	for _, kind := range kinds {
		kindInfoMap[kind.KindID] = kind
	}

	return kindInfoMap, nil
}

// getDefaultWorkflowExtraParams 获取默认工具参数，使用缓存服务
func (s *WorkflowQueryService) getDefaultWorkflowExtraParams(ctx context.Context) []*model.WorkflowToolParam {
	var params []*model.WorkflowToolParam

	err := s.cacheService.GetOrSet(ctx, constants.ConfigKeyDefaultWorkflowExtraParams, &params, 30*time.Minute, func() (interface{}, error) {
		return s.loadDefaultWorkflowExtraParamsFromConfig(ctx)
	})

	if err != nil {
		zlog.LogWithContext(ctx).Warn("failed to get default workflow extra params from cache", zap.Error(err))
		return nil
	}

	return params
}

// loadDefaultWorkflowExtraParamsFromConfig 从配置表加载默认工具参数
func (s *WorkflowQueryService) loadDefaultWorkflowExtraParamsFromConfig(ctx context.Context) ([]*model.WorkflowToolParam, error) {
	configStr, err := s.configService.GetConfValue(constants.ConfigKeyDefaultWorkflowExtraParams)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("failed to get default workflow extra params from config", zap.Error(err))
		return nil, err
	}

	if configStr == "" {
		return []*model.WorkflowToolParam{}, nil
	}

	var params []*model.WorkflowToolParam
	if err := json.Unmarshal([]byte(configStr), &params); err != nil {
		zlog.LogWithContext(ctx).Error("failed to unmarshal default workflow extra params",
			zap.Error(err),
			zap.String("config_value", configStr))
		return nil, err
	}

	return params, nil
}
