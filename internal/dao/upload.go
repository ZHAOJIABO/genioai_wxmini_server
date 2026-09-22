package dao

import (
	"context"

	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"va_visionai_server/internal/model"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
)

type UploadDao struct {
	DB *gorm.DB
}

func NewUploadDao(db *gorm.DB) *UploadDao {
	return &UploadDao{DB: db}
}

func (dao *UploadDao) GetByFileUrl(ctx context.Context, tx *gorm.DB, projectID, fileUrl string, uploaderRole vai.UploaderRole) (*model.UploadFile, error) {
	var uploadFile model.UploadFile
	err := tx.WithContext(ctx).
		Where("project_id = ? AND content_url = ? AND uploader_role = ?",
			projectID, fileUrl, uploaderRole.String()).
		First(&uploadFile).Error
	if err != nil {
		return nil, errors.Wrap(err, "get upload file")
	}
	return &uploadFile, nil
}

type extendedUploadFile struct {
	model.UploadFile
	PhotoWidth  int `gorm:"column:photo_width"`
	PhotoHeight int `gorm:"column:photo_height"`
}

func (e *extendedUploadFile) toStandard() *model.UploadFile {
	e.UploadFile.PhotoHeight = int32(e.PhotoHeight)
	e.UploadFile.PhotoWidth = int32(e.PhotoWidth)
	return &e.UploadFile
}

func (dao *UploadDao) GetImgUrlWithout(ctx context.Context, tx *gorm.DB, fileUrl string) (*model.UploadFile, error) {

	var extendedFile extendedUploadFile

	// 使用Map定义查询条件
	conditions := map[string]interface{}{
		"va_upload_file.oss_addr": fileUrl,
	}

	db := tx
	if db == nil {
		db = dao.DB
	}

	err := db.WithContext(ctx).
		Select("va_upload_file.*, COALESCE(va_upload_file_info.photo_width, 0) as photo_width, COALESCE(va_upload_file_info.photo_height, 0) as photo_height").
		Table("va_upload_file").
		Joins("LEFT JOIN va_upload_file_info ON va_upload_file.hash = va_upload_file_info.hash").
		Where(conditions).
		First(&extendedFile).Error

	if err != nil {
		return nil, errors.Wrap(err, "get upload file")
	}

	return extendedFile.toStandard(), nil
}

// GetExtendedByHash 通过文件 hash 获取上传记录（含附加信息宽高）
// 场景：oss_addr 未能精确匹配，但 URL 中包含 32 位 MD5，可用 hash 补偿查询
func (dao *UploadDao) GetExtendedByHash(ctx context.Context, tx *gorm.DB, hash string) (*model.UploadFile, error) {
	var extendedFile extendedUploadFile

	db := tx
	if db == nil {
		db = dao.DB
	}

	// 优先从 upload_file LEFT JOIN info 获取
	err := db.WithContext(ctx).
		Select("va_upload_file.*, COALESCE(va_upload_file_info.photo_width, 0) as photo_width, COALESCE(va_upload_file_info.photo_height, 0) as photo_height").
		Table("va_upload_file").
		Joins("LEFT JOIN va_upload_file_info ON va_upload_file.hash = va_upload_file_info.hash").
		Where("va_upload_file.hash = ?", hash).
		First(&extendedFile).Error
	if err == nil {
		return extendedFile.toStandard(), nil
	}

	// 若 upload_file 不存在，但 info 存在，则退化为仅返回宽高信息
	if errors.Is(err, gorm.ErrRecordNotFound) {
		info, ierr := dao.GetFileInfoByHash(ctx, hash)
		if ierr != nil {
			return nil, errors.Wrap(ierr, "get upload file info by hash")
		}
		if info != nil {
			fallback := &model.UploadFile{}
			fallback.Hash = hash
			fallback.PhotoWidth = info.PhotoWidth
			fallback.PhotoHeight = info.PhotoHeight
			return fallback, nil
		}
	}

	return nil, errors.Wrap(err, "get upload file by hash")
}

func (dao *UploadDao) ListUserUpload(ctx context.Context, projectID, userID, fileType string, uploaderRole vai.UploaderRole) ([]model.UploadFile, error) {
	var extendedFiles []extendedUploadFile

	conditions := map[string]interface{}{
		"va_upload_file.project_id":    projectID,
		"va_upload_file.user_id":       userID,
		"va_upload_file.file_type":     fileType,
		"va_upload_file.uploader_role": uploaderRole.String(),
	}

	err := dao.DB.WithContext(ctx).
		Table("va_upload_file").
		Select("va_upload_file.*, COALESCE(va_upload_file_info.photo_width, 0) AS photo_width, COALESCE(va_upload_file_info.photo_height, 0) AS photo_height").
		Joins("LEFT JOIN va_upload_file_info ON va_upload_file.hash = va_upload_file_info.hash").
		Where(conditions).
		Order("va_upload_file.created_at desc").
		Limit(20).
		Find(&extendedFiles).Error

	uploadFiles := make([]model.UploadFile, len(extendedFiles))
	for i, file := range extendedFiles {
		uploadFiles[i] = *file.toStandard()
	}

	return uploadFiles, errors.Wrap(err, "list user upload")
}

func (dao *UploadDao) CreateUploadFile(ctx context.Context, tx *gorm.DB, file *model.UploadFile, uploaderRole string) error {
	file.UploaderRole = uploaderRole
	db := tx
	if db == nil {
		db = dao.DB
	}
	return errors.Wrap(
		db.WithContext(ctx).Create(file).Error,
		"create upload file",
	)
}

// DeleteUserUploadFiles 删除用户上传的文件
func (dao *UploadDao) DeleteUserUploadFiles(ctx context.Context, projectID, userID string, uploaderRole vai.UploaderRole) error {
	err := dao.DB.WithContext(ctx).
		Where("project_id = ? AND user_id = ? AND uploader_role = ?",
			projectID, userID, uploaderRole.String()).
		Delete(&model.UploadFile{}).Error
	return errors.Wrap(err, "delete user upload files")
}

// DeleteUserUploadFileByDataID 删除用户上传的文件
func (dao *UploadDao) DeleteUserUploadFileByFileID(ctx context.Context, projectID, userID, fileID string, uploaderRole vai.UploaderRole) error {
	err := dao.DB.WithContext(ctx).
		Where("project_id = ? AND user_id = ? AND id = ? AND uploader_role = ?",
			projectID, userID, fileID, uploaderRole.String()).
		Delete(&model.UploadFile{}).Error
	return errors.Wrap(err, "delete user upload file by data id")
}

// UpsertFileInfo 更新或插入文件信息，使用hash作为唯一标识
// 只有非零值和非空字符串字段会被更新，避免覆盖已有值
func (dao *UploadDao) UpsertFileInfo(ctx context.Context, fileInfo *model.UploadFileInfo) error {
	// 使用clause.OnConflict实现Upsert,同时保持选择性更新特性
	db := dao.DB.WithContext(ctx)

	// var updateColumns []clause.Column
	var assignmentExprs []string

	if fileInfo.PhotoWidth > 0 {
		assignmentExprs = append(assignmentExprs, "photo_width")
		// updateColumns = append(updateColumns, clause.Column{Name: "photo_width"})
	}
	if fileInfo.PhotoHeight > 0 {
		assignmentExprs = append(assignmentExprs, "photo_height")
		// updateColumns = append(updateColumns, clause.Column{Name: "photo_height"})
	}
	if fileInfo.PhotoQuestion != "" {
		assignmentExprs = append(assignmentExprs, "photo_question")
		// updateColumns = append(updateColumns, clause.Column{Name: "photo_question"})
	}
	if fileInfo.MoralReview != "" {
		assignmentExprs = append(assignmentExprs, "moral_review")
		// updateColumns = append(updateColumns, clause.Column{Name: "moral_review"})
	}
	if fileInfo.ImageInfo != "" {
		assignmentExprs = append(assignmentExprs, "image_info")
	}

	// 使用自定义更新策略
	conflict := clause.OnConflict{
		Columns: []clause.Column{{Name: "hash"}},
	}

	// 如果有需要更新的字段,则添加更新表达式
	if len(assignmentExprs) > 0 {
		conflict.DoUpdates = clause.AssignmentColumns(assignmentExprs)
	} else {
		// 如果没有需要更新的字段,则不执行更新
		conflict.DoNothing = true
	}

	// 执行带冲突处理的插入操作
	err := db.Clauses(conflict).Create(fileInfo).Error
	return errors.Wrap(err, "upsert file info with selective updates")
}

// GetFileInfoByHash 根据hash查询文件信息
func (dao *UploadDao) GetFileInfoByHash(ctx context.Context, hash string) (*model.UploadFileInfo, error) {
	var fileInfo model.UploadFileInfo
	err := dao.DB.WithContext(ctx).
		Where("hash = ?", hash).
		First(&fileInfo).Error
	if err != nil {
		return nil, errors.Wrap(err, "get file info by hash")
	}
	return &fileInfo, nil
}

// GetImageContentByURL 通过URL获取图片内容
func (dao *UploadDao) GetImageContentByURL(ctx context.Context, url string) ([]byte, error) {
	// 清理URL
	cleanURL := url

	// 查询文件信息
	uploadFile, err := dao.GetImgUrlWithout(ctx, nil, cleanURL)
	if err != nil {
		// 尝试直接从URL下载
		return utils.DownloadFile(cleanURL)
	}

	// 判断是否有文件路径或直接使用URL
	// 注意：这里使用OSSAddr字段，因为UploadFile结构中没有LocalPath字段
	if uploadFile.OSSAddr != "" {
		// 尝试从本地获取，如果本地不存在，则从OSS地址下载
		return utils.DownloadFile(uploadFile.OSSAddr)
	}

	// 否则尝试使用内容URL
	if uploadFile.ContentURL != "" {
		return utils.DownloadFile(uploadFile.ContentURL)
	}

	// 如果都不存在，则使用原始URL
	return utils.DownloadFile(cleanURL)
}
