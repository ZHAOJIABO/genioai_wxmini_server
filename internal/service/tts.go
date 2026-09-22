package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/conf"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/text_to_speech"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type TTSService struct {
	dao *dao.TTSDao
}

type TTSTask struct {
	Ctx        context.Context
	Cancel     func()
	Input      []string
	Output     chan []byte
	Err        chan error
	Speaker    string
	TTSEncoder vai.TTSEncoder
}

var taskChannel chan TTSTask

func NewTTSService() *TTSService {
	taskChannel = make(chan TTSTask, 10)
	return &TTSService{
		dao: dao.NewTTSDao(),
	}
}

func (s *TTSService) getTTSClient() (*websocket.Conn, error) {
	ttsConfig := conf.GlobalConfig.LlmConfig.VolcEngineTTS
	appkey := ttsConfig.AppKey
	accessKey := ttsConfig.AccessKey
	connID := uuid.New().String()
	client, err := text_to_speech.NewClient(context.TODO(), connID, connID, appkey, accessKey)
	return client, err
}

func (s *TTSService) Submit(ctx context.Context, task TTSTask) {
	taskChannel <- task
	zlog.LogWithContext(ctx).Info("Submit TTS Task")
}

func (s *TTSService) GetMsgAudio(ctx context.Context, chatID, msgID string) *model.TTS {
	r, err := s.dao.FindAudio(chatID, msgID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		zlog.LogWithContext(ctx).Error("GetMsgAudio Error", zap.Error(err))
	}
	return r
}

func (s *TTSService) StartProcess() {
	for i := 0; i < 5; i++ {
		go s.processTask()
	}
}

func (s *TTSService) processTask() {
	defer func() {
		zlog.Logger.Info("TTS Process Stop")
	}()

	for t := range taskChannel {
		client, err := s.getTTSClient()
		if err != nil {
			zlog.LogWithContext(t.Ctx).Error("Create TTS Client Error", zap.Error(err))
		}
		text_to_speech.Process(t.Ctx, client, t.Input, t.Output, t.Speaker, t.Cancel, t.TTSEncoder)
		client.Close()
	}
}

func (s *TTSService) LoopReceiveAudio(ctx context.Context, task TTSTask, streamServer vai.TTSService_GetMsgAudioServer, msgID string) ([]byte, error) {
	var audio []byte
	var taskErr error
loopReceive:
	for {
		select {
		case v := <-task.Output:
			if v == nil {
				continue
			}
			if err := s.SendAudioSuccess(streamServer, msgID, v, false); err != nil {
				zlog.LogWithContext(ctx).Error("Send Audio Msg Error", zap.Error(err))
				task.Cancel()
				return nil, err
			}
			audio = append(audio, v...)
		case taskErr = <-task.Err:
			zlog.Logger.Error("TTS Task Error", zap.Error(taskErr))
			break loopReceive
		case <-task.Ctx.Done():
			break loopReceive
		}
	}
	if taskErr != nil {
		return nil, taskErr
	}

	return audio, nil
}

func (s *TTSService) sendAudioRsp(streamServer vai.TTSService_GetMsgAudioServer, rsp *vai.TTSStreamResponse) error {
	ctx := streamServer.Context()

	if err := streamServer.Send(rsp); err != nil {
		zlog.LogWithContext(ctx).Error("Send TTSStreamResponse Msg Error", zap.Error(err))
		return err
	}
	return nil
}

func (s *TTSService) BuildHeader(code vai.StatusCode, msg string) *vai.ResponseHeader {
	return &vai.ResponseHeader{
		Code: code,
		Msg:  msg,
	}
}
func (s *TTSService) SendAudioSuccess(streamServer vai.TTSService_GetMsgAudioServer, msgID string, audioByte []byte, isEnd bool) error {
	rsp := &vai.TTSStreamResponse{
		ResponseHeader: s.BuildHeader(vai.StatusCode_SUCCESS, constants.ErrMsgSuccess),
		Audio:          audioByte,
		MessageId:      msgID,
		IsEnd:          isEnd,
	}
	return s.sendAudioRsp(streamServer, rsp)
}

func (s *TTSService) SendAduioError(streamServer vai.TTSService_GetMsgAudioServer, code vai.StatusCode, msgID string) error {
	rsp := &vai.TTSStreamResponse{
		ResponseHeader: s.BuildHeader(code, constants.CodeMsg(code)),
		MessageId:      msgID,
		IsEnd:          true,
	}
	return s.sendAudioRsp(streamServer, rsp)
}
