package service

// Generate by GPT4o
import (
	"context"

	"github.com/pkg/errors"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	llm "va_visionai_server/internal/service/llm"
	"va_visionai_server/internal/service/uploader"
	vai "va_visionai_server/internal/va_interface"

	_ "image/png"
)

type UploadService struct {
	uploadDao  *dao.UploadDao
	llmFactory *llm.LLMFactoryImpl
	promptDao  *dao.PromptDao
	configDao  *dao.ConfigDao
}

func NewUploadService(llmFactory *llm.LLMFactoryImpl, promptDao *dao.PromptDao) *UploadService {
	return &UploadService{
		uploadDao:  dao.NewUploadDao(db.GetDB()),
		llmFactory: llmFactory,
		promptDao:  promptDao,
		configDao:  dao.NewConfigDao(),
	}
}

func (s *UploadService) Upload(ctx context.Context, req *vai.UploadRequest) (*vai.UploadResponse, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Data == nil {
		return nil, errors.New("file data is empty")
	}
	// 直接调用uploader处理，传递llmFactory
	upProcessor, err := uploader.GetUploader(ctx, req, s.uploadDao, s.llmFactory)
	if err != nil {
		return nil, err
	}
	ctx = common.CtxSetStrValue(ctx, constants.CtxPromptID, req.GetPromptId())
	return upProcessor.Process(ctx)
}

func (s *UploadService) SysUpload(ctx context.Context, req *vai.UploadRequest) (*vai.UploadResponse, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Data == nil {
		return nil, errors.New("file data is empty")
	}
	ctx = common.CtxSetStrValue(ctx, constants.CtxUploaderRole, vai.UploaderRole_UPLOADER_ROLE_SYSTEM.String())

	// 直接调用uploader处理，传递llmFactory
	upProcessor, err := uploader.GetUploader(ctx, req, s.uploadDao, s.llmFactory)
	if err != nil {
		return nil, err
	}
	return upProcessor.Process(ctx)
}

// 用户最近上传
func (s *UploadService) RecentUpload(ctx context.Context, userID, fileType string) ([]model.UploadFile, error) {
	projectID := common.GetProjectID(ctx)
	uploadInfos, err := s.uploadDao.ListUserUpload(ctx, projectID, userID, fileType, vai.UploaderRole_UPLOADER_ROLE_USER)
	if err != nil {
		return nil, err
	}
	return uploadInfos, nil
}

// ClearUserUploadPicture 清除用户最近上传的图片
func (s *UploadService) ClearUserUploadPicture(ctx context.Context, userID string) error {
	projectID := common.GetProjectID(ctx)
	return s.uploadDao.DeleteUserUploadFiles(ctx, projectID, userID, vai.UploaderRole_UPLOADER_ROLE_USER)
}

// GenerateQuestionsAndReview 生成问题和道德审查结果
// 这是一个包装方法，用于调用imgUploader的GenerateQuestionsAndReview方法
func (s *UploadService) GenerateQuestionsAndReview(ctx context.Context, imageURL string, language string) ([]string, float64, error) {
	// 创建一个imgUploader实例
	imgUploader := uploader.NewImgUploader(ctx, []byte{}, vai.FileType_FT_IMAGE, "", "", s.uploadDao, s.llmFactory, true)

	// 调用imgUploader的导出方法GenerateQuestionsAndReview
	return imgUploader.GenerateQuestionsAndReview(ctx, imageURL, language)
}

func (s *UploadService) DeleteUserUploadPicture(ctx context.Context, userID, fileID string) error {
	projectID := common.GetProjectID(ctx)
	return s.uploadDao.DeleteUserUploadFileByFileID(ctx, projectID, userID, fileID, vai.UploaderRole_UPLOADER_ROLE_USER)
}
