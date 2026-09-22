package picture_forge

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type ThemeManagementService struct {
	picForgeDao   *dao.PictureForgeDao
	configService *service.ConfigService
}

func NewThemeManagementService(picForgeDao *dao.PictureForgeDao, configService *service.ConfigService) *ThemeManagementService {
	return &ThemeManagementService{
		picForgeDao:   picForgeDao,
		configService: configService,
	}
}

func (s *ThemeManagementService) loadThemeMetadata(ctx context.Context) (map[string]struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Icon         string `json:"icon"`
	SelectedIcon string `json:"selected_icon"`
	Order        int    `json:"order"`
}, error) {
	var themeMetadata map[string]struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		Icon         string `json:"icon"`
		SelectedIcon string `json:"selected_icon"`
		Order        int    `json:"order"`
	}
	projectID := common.GetProjectID(ctx)
	if err := s.configService.GetJSONConfig(constants.GetConfigKey(projectID, constants.BaseConfigKeyThemeMetadata), &themeMetadata); err != nil {
		return nil, errors.Wrap(err, "get theme metadata config")
	}

	return themeMetadata, nil
}

func (s *ThemeManagementService) buildFlatThemeWorkflows(kinds []*model.WorkflowKind, themeMetadata map[string]struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Icon         string `json:"icon"`
	SelectedIcon string `json:"selected_icon"`
	Order        int    `json:"order"`
}, limit int, workflowProcessor func([]*model.Workflow, *model.WorkflowKind, string) []*vai.Workflow, sortingRules func([]*vai.Workflow) []*vai.Workflow) []*vai.WorkflowThemeInfoWithWorkflowKind {
	groupedWorkflowsByTheme := make(map[string][]*vai.Workflow)
	themeOrderMap := make(map[string]struct{})

	for _, kind := range kinds {
		themes := kind.GetThemes()
		if len(themes) == 0 {
			continue
		}
		protoWorkflows := workflowProcessor(kind.Workflows, kind, "")
		for _, theme := range themes {
			themeOrderMap[theme] = struct{}{}
			groupedWorkflowsByTheme[theme] = append(groupedWorkflowsByTheme[theme], protoWorkflows...)
		}
	}

	result := make([]*vai.WorkflowThemeInfoWithWorkflowKind, 0, len(groupedWorkflowsByTheme))
	for themeStr := range themeOrderMap {
		meta, ok := themeMetadata[themeStr]
		if !ok {
			continue
		}

		workflows := groupedWorkflowsByTheme[themeStr]
		workflows = sortingRules(workflows)

		if limit > 0 && len(workflows) > limit {
			workflows = workflows[:limit]
		}

		themeInfo := &vai.WorkflowThemeInfoWithWorkflowKind{
			ThemeInfo: &vai.WorkflowThemeInfo{
				Theme:        themeStr,
				Name:         meta.Name,
				Description:  meta.Description,
				Icon:         meta.Icon,
				SelectedIcon: meta.SelectedIcon,
			},
			Workflow: workflows,
		}
		result = append(result, themeInfo)
	}
	return result
}

func (s *ThemeManagementService) buildHierarchicalThemeWorkflows(kinds []*model.WorkflowKind, themeMetadata map[string]struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Icon         string `json:"icon"`
	SelectedIcon string `json:"selected_icon"`
	Order        int    `json:"order"`
}, limit int, protoKindsConverter func([]*model.WorkflowKind, int, string) []*vai.WorkflowKind) []*vai.WorkflowThemeInfoWithWorkflowKind {
	groupedByTheme := make(map[string][]*model.WorkflowKind)
	themeOrder := make([]string, 0)
	themeOrderMap := make(map[string]struct{})

	for _, kind := range kinds {
		themes := kind.GetThemes()
		for _, theme := range themes {
			if _, exists := themeOrderMap[theme]; !exists {
				groupedByTheme[theme] = make([]*model.WorkflowKind, 0)
				themeOrder = append(themeOrder, theme)
				themeOrderMap[theme] = struct{}{}
			}
			groupedByTheme[theme] = append(groupedByTheme[theme], kind)
		}
	}

	result := make([]*vai.WorkflowThemeInfoWithWorkflowKind, 0, len(groupedByTheme))
	for _, theme := range themeOrder {
		kindsInTheme := groupedByTheme[theme]
		meta, ok := themeMetadata[theme]
		if !ok {
			continue
		}
		themeInfo := &vai.WorkflowThemeInfoWithWorkflowKind{
			ThemeInfo: &vai.WorkflowThemeInfo{
				Theme:        theme,
				Name:         meta.Name,
				Description:  meta.Description,
				Icon:         meta.Icon,
				SelectedIcon: meta.SelectedIcon,
			},
			WorkflowKinds: protoKindsConverter(kindsInTheme, limit, theme),
		}
		result = append(result, themeInfo)
	}
	return result
}

func (s *ThemeManagementService) ListThemeWithWorkflows(ctx context.Context, lang string, limit int, kindType vai.WorkflowKindType, withoutKind bool, workflowLoader func(context.Context, []*model.WorkflowKind) error, workflowProcessor func([]*model.Workflow, *model.WorkflowKind, string) []*vai.Workflow, sortingRules func([]*vai.Workflow) []*vai.Workflow, protoKindsConverter func([]*model.WorkflowKind, int, string) []*vai.WorkflowKind) ([]*vai.WorkflowThemeInfoWithWorkflowKind, error) {
	kindTypeFilter := ""
	if kindType != vai.WorkflowKindType_WORKFLOW_KIND_TYPE_UNKNOWN {
		kindTypeFilter = kindType.String()
	}

	kinds, err := s.picForgeDao.ListWorkflowKinds(ctx, lang, kindTypeFilter, "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to list all active workflow kinds")
	}
	if len(kinds) == 0 && lang != constants.EN {
		kinds, err = s.picForgeDao.ListWorkflowKinds(ctx, constants.EN, kindTypeFilter, "")
		if err != nil {
			return nil, errors.Wrap(err, "failed to list all active workflow kinds with fallback language")
		}
	}
	if len(kinds) == 0 {
		return []*vai.WorkflowThemeInfoWithWorkflowKind{}, nil
	}

	filteredKinds := make([]*model.WorkflowKind, 0)
	appVersion := common.GetAppVersion(ctx)
	osName := common.GetOSName(ctx)

	for _, kind := range kinds {
		supportVersions := ""
		switch osName {
		case constants.IOS:
			supportVersions = kind.IOSupportVersions
		case constants.ANDROID:
			supportVersions = kind.AndroidSupportVersions
		}
		if supportVersions == "" {
			filteredKinds = append(filteredKinds, kind)
			continue
		}
		ok, err := utils.CheckVersionRange(appVersion, supportVersions)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to check version range", zap.Error(err))
			continue
		}
		if !ok {
			continue
		}
		filteredKinds = append(filteredKinds, kind)
	}

	if err := workflowLoader(ctx, filteredKinds); err != nil {
		return nil, errors.Wrap(err, "failed to load and group workflows")
	}

	themeMetadata, err := s.loadThemeMetadata(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load theme metadata")
	}
	var result []*vai.WorkflowThemeInfoWithWorkflowKind
	if withoutKind {
		result = s.buildFlatThemeWorkflows(filteredKinds, themeMetadata, limit, workflowProcessor, sortingRules)
	} else {
		result = s.buildHierarchicalThemeWorkflows(filteredKinds, themeMetadata, limit, protoKindsConverter)
	}

	sort.Slice(result, func(i, j int) bool {
		metaI, okI := themeMetadata[result[i].GetThemeInfo().GetTheme()]
		metaJ, okJ := themeMetadata[result[j].GetThemeInfo().GetTheme()]
		if !okI {
			return false
		}
		if !okJ {
			return true
		}
		return metaI.Order < metaJ.Order
	})

	return result, nil
}

func (s *ThemeManagementService) ListAllThemeInfo(ctx context.Context) ([]*vai.WorkflowThemeInfo, error) {
	themeMetadata, err := s.loadThemeMetadata(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load theme metadata")
	}

	themes := make([]*vai.WorkflowThemeInfo, 0, len(themeMetadata))
	for theme, meta := range themeMetadata {
		themes = append(themes, &vai.WorkflowThemeInfo{
			Theme:        theme,
			Name:         meta.Name,
			Description:  meta.Description,
			Icon:         meta.Icon,
			SelectedIcon: meta.SelectedIcon,
		})
	}

	sort.Slice(themes, func(i, j int) bool {
		metaI, okI := themeMetadata[themes[i].GetTheme()]
		metaJ, okJ := themeMetadata[themes[j].GetTheme()]
		if !okI {
			return false
		}
		if !okJ {
			return true
		}
		return metaI.Order < metaJ.Order
	})

	return themes, nil
}
