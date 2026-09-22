package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"va_visionai_server/conf"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service/llm"
)

//nolint:testifylint
func TestGenerateQuestionsFromImage(t *testing.T) {
	conf.ConfigInit("../../conf/server.yaml")

	// 读取本地图片文件并转为base64
	imageBytes, err := os.ReadFile("/Users/lxp/Downloads/QQ20250122-175221.jpeg")
	if err != nil {
		t.Fatalf("failed to read image file: %v", err)
	}
	base64Image := base64.StdEncoding.EncodeToString(imageBytes)
	imageURL := "data:image/png;base64," + base64Image
	httpImgUrl := "https://va-visionai-server.oss-cn-hangzhou.aliyuncs.com/upload/20250225/1714281719666666666.jpeg"
	// 创建 LLMFactoryImpl 实例
	connectionInfo := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8&autocommit=true&parseTime=true&loc=%s&multiStatements=true",
		conf.GlobalConfig.Mysql.User,
		conf.GlobalConfig.Mysql.Password,
		conf.GlobalConfig.Mysql.Addr,
		conf.GlobalConfig.Mysql.Port,
		conf.GlobalConfig.Mysql.Db,
		"Asia%2FShanghai")
	fmt.Println(connectionInfo)
	mysqldb, err := gorm.Open(mysql.Open(connectionInfo), &gorm.Config{
		PrepareStmt: true,
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "va_",
			SingularTable: true,
		},
	})
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	modelDao := dao.NewModelDao(mysqldb)
	voiceDao := dao.NewVoiceDAO(mysqldb)
	uploadDao := dao.NewUploadDao(mysqldb)
	llmFactory := llm.NewLLMFactoryImpl(modelDao, voiceDao, uploadDao)
	promptDao := dao.NewPromptDao(mysqldb)

	// 创建 UploadService 实例
	service := NewUploadService(llmFactory, promptDao)

	// 测试用例
	testCases := []struct {
		name      string
		imageURL  string
		language  string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "Generate questions in Chinese",
			imageURL:  httpImgUrl,
			language:  "ZH",
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:      "Generate questions in Chinese",
			imageURL:  imageURL,
			language:  "ZH",
			wantCount: 3,
			wantErr:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 调用 GenerateQuestionsAndReview 方法，现在它能够正确地调用imgUploader的导出方法
			ctx := context.Background()
			questions, review, err := service.GenerateQuestionsAndReview(ctx, tc.imageURL, tc.language)

			fmt.Println(review)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, questions, tc.wantCount)
				for _, q := range questions {
					assert.NotEmpty(t, q)
				}
			}
		})
	}
}
