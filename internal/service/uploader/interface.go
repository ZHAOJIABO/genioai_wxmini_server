package uploader

import (
	"context"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/dao"
	vai "va_visionai_server/internal/va_interface"
)

type Uploader interface {
	Process(ctx context.Context) (*vai.UploadResponse, error)
}

var (
	_ Uploader = (*imgUploader)(nil)
	_ Uploader = (*docUploader)(nil)
	_ Uploader = (*voiceUploader)(nil)
)

type BaseUploader struct {
	ctx          context.Context
	uploadDao    *dao.UploadDao
	projectID    string
	uploaderRole string
}

func NewBaseUploader(ctx context.Context, uploadDao *dao.UploadDao) *BaseUploader {
	return &BaseUploader{
		ctx:       ctx,
		uploadDao: uploadDao,
		projectID: common.GetProjectID(ctx),
	}
}
