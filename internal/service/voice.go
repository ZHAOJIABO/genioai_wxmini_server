package service

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type VoiceService struct {
	voiceDao *dao.VoiceDao
}

func NewVoiceService() *VoiceService {
	return &VoiceService{
		voiceDao: dao.NewVoiceDAO(db.GetDB()),
	}
}

func (s *VoiceService) GetVoiceInfo(ctx context.Context, fileHash string) (*model.Voice, error) {
	voiceInfo, err := s.voiceDao.GetByFileHash(fileHash)
	if err != nil {
		return nil, err
	}
	return &voiceInfo, nil
}

func (s *VoiceService) GetVoiceInfoByID(ctx context.Context, id int64) (*model.Voice, error) {
	return s.voiceDao.GetByID(id)
}

func (s *VoiceService) WaitAndGetVoice(fileHash string) (model.Voice, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var voiceInfo model.Voice
	resultError := make(chan error, 1)
	result := make(chan model.Voice, 1)
	go func() {
		info, err := s.waitVoiceAnalyze(fileHash)
		if err != nil {
			resultError <- err
		} else {
			result <- info
		}
	}()

	select {
	case <-ctx.Done():
		zlog.Logger.Error("Wait Voice Analyze timeout", zap.String("fileHash", fileHash))
		return voiceInfo, errors.New("Voice Analyze Timeout,Please Retry")
	case err := <-resultError:
		zlog.Logger.Error("Voice Analyze Error", zap.String("fileHash", fileHash), zap.Error(err))
		return voiceInfo, err
	case info := <-result:
		if info.Content == "" {
			zlog.Logger.Error("Voice Analyze Success But Content Empty", zap.String("fileHash", fileHash))
			return voiceInfo, errors.New("Voice Content Empty")
		}
		return info, nil
	}
}

func (s *VoiceService) waitVoiceAnalyze(fileHash string) (model.Voice, error) {
	for {
		voiceInfo, err := s.voiceDao.GetByFileHash(fileHash)
		if err != nil {
			return voiceInfo, err
		}
		if voiceInfo.Status == constants.VoiceSuccess {
			return voiceInfo, nil
		}
		if voiceInfo.Status == constants.VoiceError {
			return voiceInfo, errors.New("Voice Extract Error")
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (s *VoiceService) FillMsgVoiceInfo(msgs []*vai.Message) error {
	voiceHash := []string{}
	for _, msg := range msgs {
		voiceMd5 := msg.GetVoiceInfo().GetMd5()
		if voiceMd5 != "" {
			voiceHash = append(voiceHash, voiceMd5)
		}
	}
	if len(voiceHash) == 0 {
		return nil
	}

	voiceInfos, err := s.voiceDao.GetBatchByFileHash(voiceHash)
	if err != nil {
		zlog.Logger.Error("Get Voice Info Batch Error", zap.Error(err))
		return err
	}
	voiceMap := map[string]model.Voice{}
	for _, info := range voiceInfos {
		voiceMap[info.FileHash] = info
	}
	for _, msg := range msgs {
		voiceMd5 := msg.GetVoiceInfo().GetMd5()
		if voiceMd5 != "" {
			info := voiceMap[voiceMd5]
			msg.VoiceInfo = &vai.VoiceInfo{
				Md5:      voiceMd5,
				Url:      info.FileUrl,
				Duration: uint32(info.Duration),
			}

		}
	}
	return nil
}
