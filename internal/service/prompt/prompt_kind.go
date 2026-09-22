package prompt

import (
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type PromptKindService struct {
	dao *dao.PromptKindDao
}

func NewPromptKindService() *PromptKindService {
	return &PromptKindService{
		dao: dao.NewPromptKindDao(),
	}
}

func (p *PromptKindService) ListPromptKind(lang string) ([]*model.PromptKind, error) {
	if lang == "" {
		lang = "zh"
	}
	data, err := p.dao.ListPromptKinds(lang)
	if err != nil {
		zlog.Logger.Error("ListPromptKind Error", zap.Error(err))
		return nil, constants.ERR_REQUEST_FAILED
	}
	return data, nil
}

func (p *PromptKindService) ListPromptKindResp(lang string) []*vai.PromptKind {
	data, err := p.ListPromptKind(lang)
	result := []*vai.PromptKind{}
	if err != nil {
		zlog.Logger.Error("ListPromptKindResp Error", zap.Error(err))
		return result
	}
	for _, v := range data {
		result = append(result, &vai.PromptKind{
			KindId:  v.Kind,
			Label:   v.Label,
			IconUrl: v.IconURL,
		})
	}
	return result
}

func (p *PromptKindService) GroupPromptByKind(promptList []*vai.Prompt, promptKindList []*vai.PromptKind) []*vai.Prompt {
	result := make(map[string]*vai.PromptKind)
	for _, kind := range promptKindList {
		result[kind.GetKindId()] = kind
	}
	for _, prompt := range promptList {
		if v, ok := result[prompt.GetKindId()]; ok {
			v.Prompts = append(v.Prompts, prompt)
		}
	}
	return promptList
}

//
//func (p *PromptKindService) UpsertPromptKind(info *vai.PromptKind) (*model.PromptKind, error) {
//
//	promptInfo := &model.PromptKind{
//		Kind:     info.Kind,
//		Label:    info.Label,
//		Language: info.Language,
//		Display:  info.Display,
//		Score:    info.Score,
//	}
//	if err := p.dao.SavePromptKind(promptInfo); err != nil {
//		zlog.Logger.Error("UpsertPromptKind Error", zap.Error(err))
//		return nil, constants.ERR_REQUEST_FAILED
//	}
//	return promptInfo, nil
//}
