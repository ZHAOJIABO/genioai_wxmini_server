package picture_forge

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

func ParseWorkflowJSONFields(workflow *model.Workflow) error {
	if workflow.InputsJSON == "" {
		workflow.Inputs = []*model.WorkflowInput{}
	} else {
		var inputs []*model.WorkflowInput
		if err := json.Unmarshal([]byte(workflow.InputsJSON), &inputs); err != nil {
			return errors.Wrap(err, "unmarshal workflow inputs")
		}
		workflow.Inputs = inputs
	}

	if workflow.ExampleJson == "" {
		workflow.ExampleImages = []*model.WorkflowExampleImage{}
	} else {
		var exampleImages []*model.WorkflowExampleImage
		if err := json.Unmarshal([]byte(workflow.ExampleJson), &exampleImages); err != nil {
			return errors.Wrap(err, "unmarshal workflow example images")
		}
		workflow.ExampleImages = exampleImages
	}

	return nil
}

func ConvertToProtoWorkflow(w *model.Workflow, kindInfo *model.WorkflowKind, theme string, taskChainCreditCost int32, defaultToolParams []*model.WorkflowToolParam) *vai.Workflow {
	isNew, isHot := ParseWorkflowTags(w.Tags)

	if theme == "" {
		theme = kindInfo.GetPrimaryTheme()
	}

	workflow := &vai.Workflow{
		WorkflowId:    w.WorkflowID,
		Title:         w.Title,
		Description:   w.Description,
		KindId:        w.KindID,
		Icon:          w.Icon,
		KindType:      ConvertKindTypeEnum(kindInfo.KindType),
		Theme:         ConvertThemeEnum(theme),
		Inputs:        ConvertToProtoInputs(w.Inputs),
		FaceCount:     w.FaceCount,
		IsFree:        w.CreditPoints == 0,
		IsNew:         isNew,
		IsHot:         isHot,
		WorkflowTheme: theme,
		CreditCost:    int32(w.CreditPoints),
	}

	// 只有视频类工作流才展示SupportExtraParams
	if workflow.GetKindType() == vai.WorkflowKindType_WORKFLOW_KIND_TYPE_VIDEO {
		workflow.SupportExtraParams = ConvertToProtoToolParams(w.SupportExtraParams, defaultToolParams)
	}
	if workflow.GetKindType() == vai.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE {
		workflow.ChainCreditCost = taskChainCreditCost + int32(w.CreditPoints)
	}

	if len(w.ExampleImages) > 0 {
		workflow.ExampleImage = &vai.WorkflowExampleImage{
			OriContent:         w.ExampleImages[0].OriContent,
			ResultContent:      w.ExampleImages[0].ResultContent,
			AspectRatio:        w.ExampleImages[0].AspectRatio,
			OriContentCount:    int32(len(w.ExampleImages[0].OriContent)),
			ResultThumbnailUrl: w.ExampleImages[0].ResultThumbnailUrl,
		}
	}

	return workflow
}

func ConvertToProtoInputs(inputs []*model.WorkflowInput) []*vai.WorkflowInput {
	protoInputs := make([]*vai.WorkflowInput, 0, len(inputs))
	for _, input := range inputs {
		protoInputs = append(protoInputs, &vai.WorkflowInput{
			InputName:    input.InputName,
			InputType:    vai.MessageType(input.InputType),
			InputContent: input.InputContent,
		})
	}
	return protoInputs
}

func ConvertToProtoWorkflowKind(kind *model.WorkflowKind, workflows []*vai.Workflow, theme string) *vai.WorkflowKind {
	if theme == "" {
		theme = kind.GetPrimaryTheme()
	}

	protoKind := &vai.WorkflowKind{
		KindId:       kind.KindID,
		Label:        kind.Label,
		DisplayStyle: ConvertDisplayStyleEnum(kind.DisplayStyle),
		KindType:     ConvertKindTypeEnum(kind.KindType),
		Theme:        ConvertThemeEnum(theme),
		Workflows:    workflows,
		Description:  kind.Description,
	}

	if kind.CoverPicturesJson != "" {
		var coverPictures []*vai.PictureInfo
		if err := json.Unmarshal([]byte(kind.CoverPicturesJson), &coverPictures); err == nil {
			protoKind.CoverPictures = coverPictures
		}
	}

	return protoKind
}

func ConvertKindTypeEnum(kindType string) vai.WorkflowKindType {
	if kindType == "" {
		return vai.WorkflowKindType_WORKFLOW_KIND_TYPE_UNKNOWN
	}
	return vai.WorkflowKindType(vai.WorkflowKindType_value[kindType])
}

func ConvertThemeEnum(theme string) vai.WorkflowTheme {
	if theme == "" {
		return vai.WorkflowTheme_WORKFLOW_THEME_UNKNOWN
	}
	return vai.WorkflowTheme(vai.WorkflowTheme_value[theme])
}

func ConvertDisplayStyleEnum(displayStyle string) vai.WorkflowKindDisplayStyle {
	if displayStyle == "" {
		return vai.WorkflowKindDisplayStyle_WORKFLOW_KIND_DISPLAY_STYLE_UNSPECIFIED
	}
	return vai.WorkflowKindDisplayStyle(vai.WorkflowKindDisplayStyle_value[displayStyle])
}

func ParseWorkflowTags(tags []string) (isNew, isHot bool) {
	for _, tag := range tags {
		switch tag {
		case "New":
			isNew = true
		case "Hot":
			isHot = true
		}
	}
	return
}

func LogAndWrapError(ctx context.Context, err error, message string, fields ...zap.Field) error {
	if err == nil {
		return nil
	}
	zlog.LogWithContext(ctx).Error(message, append(fields, zap.Error(err))...)
	return errors.Wrap(err, message)
}

func FilterWorkflowsByRestrictionRules(ctx context.Context, workflows []*model.Workflow, kindInfoMap map[string]*model.WorkflowKind, rules *service.CreationRestrictionRules) []*model.Workflow {
	if rules == nil {
		return workflows
	}

	filtered := make([]*model.Workflow, 0, len(workflows))

	for _, workflow := range workflows {
		kindInfo, ok := kindInfoMap[workflow.KindID]
		if !ok {
			continue
		}

		// 按配置的 KindID 过滤（如果提供）
		if len(rules.AllowedKindIDs) > 0 {
			if !contains(rules.AllowedKindIDs, workflow.KindID) {
				continue
			}
		}

		if len(rules.AllowedTypes) > 0 {
			if !contains(rules.AllowedTypes, kindInfo.KindType) {
				continue
			}
		}

		if len(rules.AllowedCategories) > 0 {
			themes := kindInfo.GetThemes()
			allowed := false
			for _, theme := range themes {
				if contains(rules.AllowedCategories, theme) {
					allowed = true
					break
				}
			}
			if !allowed {
				primaryTheme := kindInfo.GetPrimaryTheme()
				if primaryTheme != "" {
					allowed = contains(rules.AllowedCategories, primaryTheme)
				}
			}
			if !allowed {
				continue
			}
		}

		if len(rules.AllowedChannels) > 0 {
			if !contains(rules.AllowedChannels, workflow.Provider) {
				continue
			}
		}

		filtered = append(filtered, workflow)
	}

	zlog.LogWithContext(ctx).Debug("Filtered workflows based on restriction rules",
		zap.Int("original_count", len(workflows)),
		zap.Int("filtered_count", len(filtered)),
		zap.Strings("allowed_types", rules.AllowedTypes),
		zap.Strings("allowed_categories", rules.AllowedCategories),
		zap.Strings("allowed_channels", rules.AllowedChannels),
		zap.Strings("allowed_kind_ids", rules.AllowedKindIDs))

	return filtered
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

// ConvertToProtoToolParams 转换工具参数到proto格式
func ConvertToProtoToolParams(params []*model.WorkflowToolParam, defaultParams []*model.WorkflowToolParam) []*vai.ToolParamDetail {
	if params != nil {
		return convertWorkflowToolParamsToProto(params)
	}

	if len(defaultParams) > 0 {
		return convertWorkflowToolParamsToProto(defaultParams)
	}

	return []*vai.ToolParamDetail{}
}

func convertWorkflowToolParamsToProto(params []*model.WorkflowToolParam) []*vai.ToolParamDetail {
	result := make([]*vai.ToolParamDetail, 0, len(params))
	for _, param := range params {
		if param != nil {
			result = append(result, &vai.ToolParamDetail{
				Label:  param.Label,
				Value:  param.Value,
				Credit: param.Credit,
			})
		}
	}
	return result
}
