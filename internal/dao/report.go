package dao

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/va_interface"
)

type ReportDao struct {
	db *gorm.DB
}

func NewReportDao() *ReportDao {
	return &ReportDao{
		db: db.GetDB(),
	}
}

func (d *ReportDao) ReportLLMMsgQuality(userID, chatID, msgID string, quality va_interface.LLMMsgQuality) error {
	msgReportInfo := model.MessageReportInfo{
		UserID:    userID,
		ChatID:    chatID,
		MessageID: msgID,
		Quality:   quality,
	}
	err := d.db.Model(&model.MessageReportInfo{}).Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "message_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"quality"}),
		},
	).Create(&msgReportInfo).Error

	return err
}

func (d *ReportDao) AddUserFeedback(projectID, userID, feedbackType, feedbackContent, headerInfo string, screenshotUrls []string, mail string) error {
	userFeedbackInfo := model.UserFeedbackRecord{
		ProjectID:       projectID,
		UserID:          userID,
		FeedbackType:    feedbackType,
		FeedbackContent: feedbackContent,
		ScreenshotUrls:  strings.Join(screenshotUrls, ","),
		HeaderInfo:      headerInfo,
		Mail:            mail,
	}
	err := d.db.Model(&model.UserFeedbackRecord{}).Create(&userFeedbackInfo).Error
	return err
}
