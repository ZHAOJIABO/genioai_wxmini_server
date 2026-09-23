package db

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"va_visionai_server/conf"
	adminmodel "va_visionai_server/internal/admin/model"
	"va_visionai_server/internal/model"
)

var mysqldb *gorm.DB

func GetDB() *gorm.DB {
	return mysqldb
}

func InitMysql() error {
	connectionInfo := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&autocommit=true&parseTime=true&loc=%s&multiStatements=true",
		conf.GlobalConfig.Mysql.User,
		conf.GlobalConfig.Mysql.Password,
		conf.GlobalConfig.Mysql.Addr,
		conf.GlobalConfig.Mysql.Port,
		conf.GlobalConfig.Mysql.Db,
		"Asia%2FShanghai")
	var err error
	mysqldb, err = gorm.Open(mysql.Open(connectionInfo), &gorm.Config{
		PrepareStmt: true,
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "va_",
			SingularTable: true,
		},
	})
	if err != nil {
		return err
	}
	sqldb, err := mysqldb.DB()
	if err != nil {
		return err
	}
	sqldb.SetMaxIdleConns(3)
	sqldb.SetMaxOpenConns(30)
	sqldb.SetConnMaxLifetime(time.Hour)
	mysqldb.Logger.LogMode(logger.Error)
	if !conf.IsProd() {
		mysqldb = mysqldb.Debug()
	}

	// 先执行 AutoMigrate 创建表结构
	if err := mysqldb.AutoMigrate(
		&model.UploadFile{},
		&model.UserRecord{},
		&model.Chat{},
		//&model.Voice{},
		&model.Config{},
		&model.UserDeviceInfo{},
		&model.Prompt{},
		&model.PromptKind{},
		&model.MessageReportInfo{},
		//&model.AttributionEvent{},
		&model.UserMapping{},
		&model.Workflow{},
		&model.WorkflowKind{},
		&model.PictureTask{},
		&model.UserEvent{},
		&model.Project{},
		&model.Model{},
		&model.TrackingEvent{},
		&model.UploadFileInfo{},
		//&model.AsaEvent{},
		&model.UserProfile{},
		//	&model.CustomProfile{},
		&model.ChatParticipant{},
		&model.UserAmount{},
		&model.CreditTransaction{},
		//&model.PictureTools{},
		//&model.PictureToolCapability{},
		//&model.TaskChain{},
		&model.UserFeedbackRecord{},
		&model.InspirationPrompt{},
		&model.InspirationApplication{},
		&model.CustomPromptHistory{},
		//&model.BannerConfig{},
		//&model.BannerClickRecord{},
		//&model.ToolGroup{},
		//&model.InviteCode{},
		//&model.InviteRecord{},
		&model.ImageGenerationModel{},
		&model.PopupNotification{},
		&adminmodel.AdminUser{},
		&adminmodel.AdminAuditLog{},
	); err != nil {
		return err
	}

	// 暂时跳过 SQL 数据迁移，避免与 AutoMigrate 冲突
	// TODO: 需要时可以手动导入初始数据
	// if err := MigrateDB(sqldb, conf.GlobalConfig.Mysql.Db); err != nil {
	// 	fmt.Printf("migrate db failed: %v", err)
	// 	return err
	// }

	log.Println("DB Init Success")
	return err
}
