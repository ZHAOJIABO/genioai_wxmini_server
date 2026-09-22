package api

import (
	"context"

	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/service"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type MediaServer struct {
	uploadService     *service.UploadService
	userService       *service.UserService
	faceDetectService *service.FaceDetectService
	vai.UnimplementedMediaServiceServer
}

func NewMediaServer(
	uploadService *service.UploadService,
	userService *service.UserService,
	faceDetectService *service.FaceDetectService,
) *MediaServer {
	return &MediaServer{
		uploadService:     uploadService,
		userService:       userService,
		faceDetectService: faceDetectService,
	}
}

func (s *MediaServer) UploadImage(ctx context.Context, req *vai.UploadRequest) (*vai.UploadResponse, error) {
	reqHeader := req.GetRequestHeader()
	var err error
	var rsp *vai.UploadResponse
	ctx = common.CtxSetStrValue(ctx, constants.CtxUploaderRole, vai.UploaderRole_UPLOADER_ROLE_USER.String())
	projectID := common.GetProjectID(ctx)

	entryLogFields := []zap.Field{
		zap.String(constants.CtxUserID, reqHeader.GetUserId()),
		zap.Any(constants.ServiceEvent, constants.EventUploadFile),
	}
	zlog.LogWithContext(ctx).Info("UploadFile", entryLogFields...)

	defer func() {
		exitLogFields := []zap.Field{
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
			zap.String("FileType", req.GetFileType().String()),
			zap.String("OssUrl", rsp.GetFileUrl()),
			zap.String("OssUrlLowQuality", rsp.GetLowQualityUrl()),
			zap.String("FileHash", rsp.GetFileMd5()),
			zap.Any(constants.ServiceEvent, constants.EventUploadFileComplete),
			zap.Error(err),
		}
		zlog.LogWithContext(ctx).Info("UploadFile",
			exitLogFields...,
		)
	}()

	code, err := isVerifyAccessToken(s.userService, reqHeader)
	if err != nil {
		rsp = &vai.UploadResponse{}
		rsp, err = BuildErrorResponse[vai.UploadResponse](ctx, code, err, constants.CodeMsg(code))
		return rsp, nil
	}
	// 普通上传开启道德审查和问题生成
	req.AskQuestion = true
	if projectID == constants.ProjectIdPicFlow || projectID == constants.ProjectIdPicLib {
		req.AskQuestion = false
	}

	rsp, err = s.uploadService.Upload(ctx, req)
	if err != nil {
		zlog.LogWithContext(ctx).Error("UploadImage: upload failed", zap.Error(err))
		rsp, _ = BuildErrorResponse[vai.UploadResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	if rsp.GetResponseHeader().GetResponseStatusCode() == vai.ResponseStatusCode_RESPONSE_STATUS_CODE_UPLOAD_CONTENT_SENSITIVE {
		return rsp, nil
	}

	if req.GetFileType() == vai.FileType_FT_IMAGE && req.GetFaceDetection() {
		faceCount, err := s.faceDetectService.FaceDetect(req.GetData())
		if err != nil {
			// 人脸检测失败不影响上传结果，只记录日志
			zlog.LogWithContext(ctx).Error("UploadImage: face detection failed", zap.Error(err))
		} else {
			rsp.FaceCount = int32(faceCount)
		}
	}

	rsp, err = BuildSuccessResponse(rsp)
	return rsp, nil
}

func (s *MediaServer) SysUploadFile(ctx context.Context, req *vai.UploadRequest) (*vai.UploadResponse, error) {
	reqHeader := req.GetRequestHeader()
	var err error
	var rsp *vai.UploadResponse
	ctx = common.CtxSetStrValue(ctx, constants.CtxUploaderRole, vai.UploaderRole_UPLOADER_ROLE_SYSTEM.String())

	entryLogFields := []zap.Field{
		zap.String(constants.CtxUserID, reqHeader.GetUserId()),
		zap.Any(constants.ServiceEvent, constants.EventSysUploadFile),
	}
	zlog.LogWithContext(ctx).Info("SysUploadFile", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String("FileType", req.GetFileType().String()),
				zap.String("OssUrl", rsp.GetFileUrl()),
				zap.String("OssUrlLowQuality", rsp.GetLowQualityUrl()),
				zap.String("FileHash", rsp.GetFileMd5()),
				zap.Any(constants.ServiceEvent, constants.EventSysUploadFileComplete),
				zap.Error(err),
			}
			zlog.LogWithContext(ctx).Info("SysUploadFile", exitLogFields...)
		}
	}()

	// 验证 access token
	code, err := isVerifyAccessToken(s.userService, reqHeader)
	if err != nil {
		zlog.LogWithContext(ctx).Error("SysUploadFile: token verification failed", zap.Error(err))
		rsp, _ = BuildErrorResponse[vai.UploadResponse](ctx, code, err, constants.CodeMsg(code))
		return rsp, nil
	}

	rsp, err = s.uploadService.SysUpload(ctx, req)
	if err != nil {
		zlog.LogWithContext(ctx).Error("SysUploadFile: upload failed", zap.Error(err))
		rsp, _ = BuildErrorResponse[vai.UploadResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	if req.GetFileType() == vai.FileType_FT_IMAGE && req.GetFaceDetection() {
		faceCount, err := s.faceDetectService.FaceDetect(req.GetData())
		if err != nil {
			// 人脸检测失败不影响上传结果，只记录日志
			zlog.LogWithContext(ctx).Error("SysUploadFile: face detection failed", zap.Error(err))
		} else {
			rsp.FaceCount = int32(faceCount)
		}
	}

	rsp, err = BuildSuccessResponse(rsp)
	return rsp, nil
}

// 暂时先不启用这个函数
// func (s *MediaServer) UploadWithOpts(ctx context.Context, req *vai.UploadWithOptsRequest) (*vai.UploadWithOptsResponse, error) {
// 	reqHeader := req.GetRequestHeader()
// 	var err error
// 	var rsp *vai.UploadWithOptsResponse
// 	ctx = common.CtxSetStrValue(ctx, constants.CtxUploaderRole, vai.UploaderRole_UPLOADER_ROLE_USER.String())

// 	entryLogFields := []zap.Field{
// 		zap.String(constants.CtxUserID, reqHeader.GetUserId()),
// 		zap.Any(constants.ServiceEvent, constants.EventUploadFile),
// 	}
// 	zlog.LogWithContext(ctx).Info("UploadWithOpts", entryLogFields...)

// 	defer func() {
// 		exitLogFields := []zap.Field{
// 			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
// 			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
// 			zap.String("FileType", req.GetFileType().String()),
// 			zap.String("OssUrl", rsp.GetFileUrl()),
// 			zap.Any(constants.ServiceEvent, constants.EventUploadFileComplete),
// 			zap.Error(err),
// 		}
// 		zlog.LogWithContext(ctx).Info("UploadWithOpts",
// 			exitLogFields...,
// 		)
// 	}()

// 	// 验证token
// 	code, err := isVerifyAccessToken(s.userService, reqHeader)
// 	if err != nil {
// 		rsp = &vai.UploadWithOptsResponse{}
// 		rsp, err = BuildErrorResponse[vai.UploadWithOptsResponse](ctx, code, err, constants.CodeMsg(code))
// 		return rsp, nil
// 	}

// 	// 上传文件
// 	uploadReq := &vai.UploadRequest{
// 		RequestHeader: req.GetRequestHeader(),
// 		FileName:      req.GetFileName(),
// 		FileType:      req.GetFileType(),
// 		FileSize:      req.GetFileSize(),
// 		Data:          req.GetData(),
// 	}
// 	uploadRsp, err := s.uploadService.Upload(ctx, uploadReq)
// 	if err != nil {
// 		rsp = &vai.UploadWithOptsResponse{}
// 		rsp, err = BuildErrorResponse[vai.UploadWithOptsResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
// 		return rsp, nil
// 	}

// 	// 构造基础响应
// 	rsp = &vai.UploadWithOptsResponse{
// 		FileUrl: uploadRsp.GetFileUrl(),
// 		Info: &vai.UploadWithOptsResponse_ImageInfo{
// 			ImageInfo: &vai.ImageInfo{
// 				Url:          uploadRsp.GetFileUrl(),
// 				ThumbnailUrl: uploadRsp.GetLowQualityUrl(),
// 				Width:        uploadRsp.GetImageWidth(),
// 				Height:       uploadRsp.GetImageHeight(),
// 			},
// 		},
// 	}

// 	if req.GetFileType() == vai.FileType_FT_IMAGE {
// 		if imageOpts := req.GetImageOpts(); imageOpts != nil && imageOpts.GetFaceDetection() {
// 			faceCount, err := s.faceDetectService.FaceDetect(req.GetData())
// 			if err != nil {
// 				zlog.LogWithContext(ctx).Error("face detect failed", zap.Error(err))
// 			} else {
// 				rsp.GetImageInfo().FaceCount = int32(faceCount)
// 			}
// 		}
// 	}

// 	rsp, err = BuildSuccessResponse(rsp)
// 	return rsp, nil
// }
