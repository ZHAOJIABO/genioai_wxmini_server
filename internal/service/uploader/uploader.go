package uploader

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/service/llm"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

func GetUploader(ctx context.Context, req *vai.UploadRequest, uploadDao *dao.UploadDao, llmFactory *llm.LLMFactoryImpl) (Uploader, error) {
	if req == nil || req.GetRequestHeader() == nil {
		return nil, errors.New("invalid request")
	}

	fileType := req.GetFileType()
	data := req.GetData()
	filename := req.GetFileName()
	userID := req.GetRequestHeader().GetUserId()
	// 打印部分data 如果过长就截断
	truncateIndex := 50
	if len(data) < truncateIndex {
		truncateIndex = len(data)
	}

	zlog.LogWithContext(ctx).Info("[Uploader GetUploader]upload file",
		zap.String("filename", filename),
		zap.String("userID", userID),
		zap.Any("fileType", fileType),
		zap.Any("dataSize", len(data)),
		zap.Any("data", data[:truncateIndex]),
	)
	switch fileType {
	case vai.FileType_FT_DOC, vai.FileType_FT_PDF, vai.FileType_FT_TXT:
		return NewDocUploader(ctx, data, fileType, userID, filename, uploadDao), nil
	case vai.FileType_FT_IMAGE:
		return NewImgUploader(ctx, data, fileType, userID, filename, uploadDao, llmFactory, req.GetAskQuestion()), nil
	case vai.FileType_FT_AUDIO:
		voiceDao := dao.NewVoiceDAO(db.GetDB())
		return NewVoiceUploader(ctx, data, fileType, userID, uint(req.GetDuration()), voiceDao, uploadDao), nil
	}

	return nil, fmt.Errorf("unsupported file type: %v", fileType)
}

func convertFileType(fileType vai.FileType) string {
	switch fileType {
	case vai.FileType_FT_DOC:
		return "doc"
	case vai.FileType_FT_PDF:
		return "pdf"
	case vai.FileType_FT_TXT:
		return "txt"
	case vai.FileType_FT_IMAGE:
		return "jpg"
	}
	return ""
}
