package service

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type ReportService struct {
	dao  *dao.ReportDao
	cdao *dao.ChatDao
	udao *dao.UserDao
}

func NewReportService(repos *dao.Repositories) *ReportService {
	return &ReportService{
		dao:  dao.NewReportDao(),
		cdao: repos.Chat,
		udao: repos.User,
	}
}

func (s *ReportService) ReportLLMMsgQuality(ctx context.Context, userID, chatID, msgID string, quality vai.LLMMsgQuality) error {
	if userID == "" || chatID == "" || msgID == "" {
		return constants.ERR_INVALID_PARAM
	}
	err := s.dao.ReportLLMMsgQuality(userID, chatID, msgID, quality)
	if err != nil {
		zlog.LogWithContext(ctx).Error("ReportLLMMsgQuality Upsert Error", zap.Error(err))
		return err
	}
	projectID := common.GetProjectID(ctx)
	chatInfo, err := s.cdao.GetByID(projectID, chatID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("ReportLLMMsg Get Chat Error", zap.Error(err))
		return err
	}
	createDate := time.Unix(chatInfo.CreateTime, 0).Format(constants.DateOnly)
	coll := db.GetCollection(createDate)
	filter := bson.M{"messageId": msgID}
	update := bson.M{
		"$set": bson.M{
			"quality": quality,
		},
	}
	// 执行更新操作
	_, err = coll.UpdateOne(context.TODO(), filter, update)
	if err != nil {
		zlog.LogWithContext(ctx).Error("ReportLLMMsgQuality Update Mongo Error", zap.Error(err))
	}

	return nil
}

func (s *ReportService) UserFeedback(ctx context.Context, projectID, userID, feedbackType, feedbackContent, headerInfo string, screenshotUrls []string, mail string) error {
	if userID == "" || feedbackType == "" {
		return constants.ERR_INVALID_PARAM
	}
	err := s.dao.AddUserFeedback(projectID, userID, feedbackType, feedbackContent, headerInfo, screenshotUrls, mail)
	if err != nil {
		zlog.LogWithContext(ctx).Error("UserFeedback Add Error", zap.Error(err))
		return err
	}

	// 如果邮箱合法且反馈信息保存成功,更新用户邮箱
	if mail != "" && common.IsValidEmail(mail) {
		err = s.udao.UpdateUserEmail(projectID, userID, mail)
		if err != nil {
			// 邮箱更新失败记录日志,但不影响反馈提交的成功
			zlog.LogWithContext(ctx).Error("UserFeedback Update Email Error",
				zap.String("userID", userID),
				zap.String("email", mail),
				zap.Error(err))
		} else {
			zlog.LogWithContext(ctx).Info("UserFeedback Update Email Success",
				zap.String("userID", userID),
				zap.String("email", mail))
		}
	}

	return nil
}
