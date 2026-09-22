package uploader

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"code.sajari.com/docconv"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type docUploader struct {
	*BaseUploader
	Body     []byte
	FileType vai.FileType
	UserID   string
	fileName string
}

func NewDocUploader(ctx context.Context, body []byte, fileType vai.FileType, userID string, fileName string, uploadDao *dao.UploadDao) *docUploader {
	return &docUploader{
		BaseUploader: NewBaseUploader(ctx, uploadDao),
		Body:         body,
		FileType:     fileType,
		UserID:       userID,
		fileName:     fileName,
	}
}

func (h *docUploader) Process(ctx context.Context) (*vai.UploadResponse, error) {
	fileHash := utils.CalculateSHA256(h.Body)
	filetypeStr := convertFileType(h.FileType)
	currentDate := time.Now().Format("200601")
	filename := fmt.Sprintf("upload/%s/%s/%s.%s", currentDate, filetypeStr, fileHash, filetypeStr)
	fileContent, err := h.extractContent()
	if err != nil {
		zlog.Logger.Error("Extract Doc Content Error", zap.Error(err))
		return nil, err
	}
	ossAddr, err := utils.UploadToOSS(filename, h.Body)
	if err != nil {
		zlog.Logger.Error("Upload Doc To Oss Error", zap.Error(err))
		return nil, err
	}
	contentFilename := fmt.Sprintf("upload/%s/%s/%s_content.txt", currentDate, filetypeStr, fileHash)
	contentOssAddr, err := utils.UploadToOSS(contentFilename, []byte(fileContent))
	if err != nil {
		zlog.Logger.Error("Upload Content To Oss Error", zap.Error(err))
		return nil, err
	}
	//获取traceid
	var docTraceId string
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		traceIDList := md.Get(constants.CtxTraceID)
		if len(traceIDList) > 0 {
			docTraceId = traceIDList[0]
		}
	}
	uploaderRole := common.CtxGetStrValue(ctx, constants.CtxUploaderRole)
	if err := h.persistence(filetypeStr, contentOssAddr, ossAddr, fileHash, docTraceId, uploaderRole); err != nil {
		return nil, err
	}
	response := &vai.UploadResponse{
		FileName: fileHash,
		FileMd5:  fileHash,
		FileUrl:  ossAddr,
	}
	return response, nil
}

func (h *docUploader) persistence(fileType, contentOssAddr, ossAddr, hash, docTraceId, uploaderRole string) error {
	uploadFile := &model.UploadFile{
		ProjectID:    h.projectID,
		UploaderRole: uploaderRole,
		FileName:     h.fileName,
		FileType:     fileType,
		OSSAddr:      ossAddr,
		ContentURL:   contentOssAddr,
		Hash:         hash,
		UserID:       h.UserID,
		TraceId:      docTraceId,
	}
	return h.uploadDao.CreateUploadFile(h.ctx, nil, uploadFile, uploaderRole)
}

// todo: 需要结合模型进行进一步的处理，无意义的页脚 页眉信息，无意义的格式。使用大模型进行总结 节省空间 提升问答效率。
func (h *docUploader) extractContent() (string, error) {
	var content string
	var err error
	switch h.FileType {
	case vai.FileType_FT_PDF:
		content, _, err = docconv.ConvertPDF(bytes.NewReader(h.Body))
	case vai.FileType_FT_DOC:
		content, _, err = docconv.ConvertDoc(bytes.NewReader(h.Body))
	case vai.FileType_FT_TXT:
		content = string(h.Body)

	}
	return content, err
}
