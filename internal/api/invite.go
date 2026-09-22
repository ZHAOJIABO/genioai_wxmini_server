package api

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/invite"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// InviteServer 邀请码服务
type InviteServer struct {
	vai.UnimplementedInviteServiceServer

	inviteService *invite.InviteService
	configService *service.ConfigService
}

// NewInviteServer 创建邀请码服务
func NewInviteServer(inviteService *invite.InviteService, configService *service.ConfigService) *InviteServer {
	return &InviteServer{
		inviteService: inviteService,
		configService: configService,
	}
}

// GetInviteInfo 获取当前用户邀请信息
func (s *InviteServer) GetInviteInfo(ctx context.Context, req *vai.GetInviteInfoRequest) (*vai.GetInviteInfoResponse, error) {
	userID := req.GetRequestHeader().GetUserId()
	projectID := common.GetProjectID(ctx)
	var err error
	var rsp *vai.GetInviteInfoResponse

	defer func() {
		zlog.LogWithContext(ctx).Info("GetInviteInfo",
			zap.String("user_id", userID),
			zap.String("project_id", projectID),
			zap.Error(err))
	}()

	// 获取邀请信息
	info, err := s.inviteService.GetInviteInfo(ctx, projectID, userID)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.GetInviteInfoResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	// 获取邀请链接基础URL
	baseUrl, _ := s.configService.GetStringConfig(constants.ConfigKeyInviteBaseUrl)
	inviteUrl := ""
	if baseUrl != "" {
		inviteUrl = fmt.Sprintf("%s?user_id=%s&invite_code=%s", baseUrl, userID, info.InviteCode)
	}

	rsp, err = BuildSuccessResponse(&vai.GetInviteInfoResponse{
		InviteInfo: &vai.InviteInfo{
			InviteCode:     info.InviteCode,
			InvitedCount:   info.InvitedCount,
			TotalCredits:   info.TotalCredits,
			UsedInviteCode: info.UsedInviteCode,
			InviteUrl:      inviteUrl,
		},
	})
	return rsp, nil
}

// UseInviteCode 使用邀请码
func (s *InviteServer) UseInviteCode(ctx context.Context, req *vai.UseInviteCodeRequest) (*vai.UseInviteCodeResponse, error) {
	userID := req.GetRequestHeader().GetUserId()
	projectID := common.GetProjectID(ctx)
	inviteCode := req.GetInviteCode()
	var err error
	var rsp *vai.UseInviteCodeResponse

	defer func() {
		zlog.LogWithContext(ctx).Info("UseInviteCode",
			zap.String("user_id", userID),
			zap.String("project_id", projectID),
			zap.String("invite_code", inviteCode),
			zap.Error(err))
	}()

	// 使用邀请码
	err = s.inviteService.UseInviteCode(ctx, projectID, userID, inviteCode)
	if err != nil {
		statusCode := vai.StatusCode_REQUEST_FAILED
		// 根据错误类型映射状态码
		if errors.Is(err, invite.ErrInviteCodeNotFound) || errors.Is(err, invite.ErrCannotInviteSelf) {
			statusCode = vai.StatusCode_INVALID_PARAM
		} else if errors.Is(err, invite.ErrAlreadyUsedInviteCode) {
			statusCode = vai.StatusCode_INVALID_REQUEST
		}
		rsp, err = BuildErrorResponse[vai.UseInviteCodeResponse](ctx, statusCode, err, err.Error())
		rsp.GetResponseHeader().Msg = constants.ErrMsgInvalidInviteCode
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.UseInviteCodeResponse{
		CreditAmount: s.inviteService.GetInviteCreditAmount(),
	})
	return rsp, nil
}

// GetInviteRecords 获取邀请记录列表
func (s *InviteServer) GetInviteRecords(ctx context.Context, req *vai.GetInviteRecordsRequest) (*vai.GetInviteRecordsResponse, error) {
	userID := req.GetRequestHeader().GetUserId()
	projectID := common.GetProjectID(ctx)
	timezone := req.GetRequestHeader().GetDevice().GetTimezone()
	var err error
	var rsp *vai.GetInviteRecordsResponse

	defer func() {
		zlog.LogWithContext(ctx).Info("GetInviteRecords",
			zap.String("user_id", userID),
			zap.String("project_id", projectID),
			zap.String("timezone", timezone),
			zap.Error(err))
	}()

	// 获取邀请记录（获取全部记录）
	records, _, err := s.inviteService.GetInviteRecords(ctx, projectID, userID, 1000, 0)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.GetInviteRecordsResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	// 转换为响应结构
	recordItems := make([]*vai.InviteRecordItem, 0, len(records))
	for _, r := range records {
		recordItems = append(recordItems, &vai.InviteRecordItem{
			InviteUserId: r.InviteUserID,
			InviteTime:   formatTimeWithTimezone(r.InviteTime, timezone),
			Credits:      "+" + strconv.FormatInt(r.Credits, 10),
		})
	}

	rsp, err = BuildSuccessResponse(&vai.GetInviteRecordsResponse{
		Records: recordItems,
	})
	return rsp, nil
}

// formatTimeWithTimezone 根据时区格式化时间
func formatTimeWithTimezone(t time.Time, timezone string) string {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("Jan 02,2006")
}
