package uploader

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
	"gorm.io/gorm"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/task"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type voiceUploader struct {
	*BaseUploader
	Body     []byte
	FileType vai.FileType
	UserID   string
	Duration uint
	voiceDao *dao.VoiceDao
}

func NewVoiceUploader(ctx context.Context, body []byte, fileType vai.FileType, userID string, duration uint, voiceDao *dao.VoiceDao, uploadDao *dao.UploadDao) *voiceUploader {
	return &voiceUploader{
		BaseUploader: NewBaseUploader(ctx, uploadDao),
		Body:         body,
		FileType:     fileType,
		UserID:       userID,
		Duration:     duration,
		voiceDao:     voiceDao,
	}
}

func (h *voiceUploader) Process(ctx context.Context) (*vai.UploadResponse, error) {
	fileHash := utils.CalculateSHA256(h.Body)
	currentDate := time.Now().Format("200601")
	projectID := common.GetProjectID(ctx)
	filename := fmt.Sprintf("%s/upload/%s/voice/%s/%s.m4a", projectID, currentDate, h.UserID, fileHash)
	originalFileUrl, err := utils.UploadToOSS(filename, h.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to upload original file to OSS: %v", err)
	}
	voiceInfo, err := h.voiceDao.GetByFileHash(fileHash)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		zlog.Logger.Info("Get Voice Info By FileHash Error", zap.Error(err))
		return nil, err
	}
	//获取traceid
	var voiceTraceId string
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		traceIDList := md.Get(constants.CtxTraceID)
		if len(traceIDList) > 0 {
			voiceTraceId = traceIDList[0]
		}
	}
	// uploaderRole := common.CtxGetStrValue(ctx, constants.CtxUploaderRole)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := h.persistence(projectID, "m4a", "", originalFileUrl, fileHash, voiceTraceId); err != nil {
			zlog.Logger.Error("Voice Persistence Error", zap.Error(err))
			return nil, err
		}
	}
	if voiceInfo.Status != constants.VoiceSuccess {
		if err := task.Voice.Submit(fileHash); err != nil {
			zlog.Logger.Error("Voice Submit Error", zap.Error(err))
			return nil, err
		}
	}

	response := &vai.UploadResponse{
		FileUrl:  originalFileUrl,
		FileName: fileHash,
		FileMd5:  fileHash,
	}
	return response, nil
}

func (h *voiceUploader) persistence(projectID, fileType, content, ossAddr, hash string, voiceTraceId string) error {
	doc := model.Voice{
		ProjectID: projectID,
		FileType:  fileType,
		FileUrl:   ossAddr,
		Content:   content,
		FileHash:  hash,
		Status:    constants.VoiceWaiting,
		Duration:  h.Duration,
		UserID:    h.UserID,
		TraceId:   voiceTraceId,
	}
	return h.voiceDao.Create(&doc)
}
