package api

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/service"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type TTSServer struct {
	vai.UnimplementedTTSServiceServer

	TTSService    *service.TTSService
	MsgService    *service.MessageService
	ConfigService *service.ConfigService
}

func NewTTSServer(ttsService *service.TTSService, msgService *service.MessageService, config *service.ConfigService) *TTSServer {
	return &TTSServer{
		TTSService:    ttsService,
		MsgService:    msgService,
		ConfigService: config,
	}
}

func (s *TTSServer) GetMsgAudio(req *vai.TTSRequest, streamServer vai.TTSService_GetMsgAudioServer) error {
	words := []string{req.GetMsg()}
	output := make(chan []byte, 20)
	errPut := make(chan error)
	ctx, cancel := context.WithCancel(streamServer.Context())
	ttsEncoder := req.GetTtsEncoder()

	task := service.TTSTask{
		Input:      words,
		Output:     output,
		Err:        errPut,
		Speaker:    req.GetSpeaker(),
		Cancel:     cancel,
		Ctx:        ctx,
		TTSEncoder: ttsEncoder,
	}

	s.TTSService.Submit(streamServer.Context(), task)
	_, taskErr := s.TTSService.LoopReceiveAudio(streamServer.Context(), task, streamServer, req.GetMessageId())
	if taskErr != nil {
		if err := s.TTSService.SendAduioError(streamServer, vai.StatusCode_REQUEST_FAILED, req.GetMessageId()); err != nil {
			zlog.LogWithContext(streamServer.Context()).Error("Send Audio Error Msg Error", zap.Error(err))
		}
		return taskErr
	}

	zlog.LogWithContext(streamServer.Context()).Info("Send Audio Stream Success")
	if err := s.TTSService.SendAudioSuccess(streamServer, req.GetMessageId(), nil, true); err != nil {
		zlog.LogWithContext(ctx).Error("Send AudioMsg Error", zap.Error(err))
		return err
	}

	return taskErr
}

func (s *TTSServer) ListSpeaker(ctx context.Context, req *vai.ListSpeakerRequest) (*vai.ListSpeakerResponse, error) {
	rsp := &vai.ListSpeakerResponse{
		ResponseHeader: &vai.ResponseHeader{
			Code: vai.StatusCode_REQUEST_FAILED,
			Msg:  constants.ErrMsgRequestFailed,
		},
	}
	var speakerList []*vai.SpeakerInfo
	lang := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	if lang != "zh" {
		lang = "en"
	}
	confStr, err := s.ConfigService.GetConfValue("speaker_config_" + lang)
	if err != nil {
		return rsp, err
	}
	if err := json.Unmarshal([]byte(confStr), &speakerList); err != nil {
		return rsp, err
	}
	rsp.GetResponseHeader().Code = vai.StatusCode_SUCCESS
	rsp.GetResponseHeader().Msg = constants.ErrMsgSuccess
	rsp.SpeakerList = speakerList

	return rsp, nil
}
