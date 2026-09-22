package utils

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/tencentyun/cos-go-sdk-v5"

	"va_visionai_server/conf"
)

// Helper function to upload file to OSS
func UploadToOSS(fileName string, data []byte) (string, error) {
	region := conf.GlobalConfig.VisionAiServerConfig.Region
	if region == "cn" {
		return UploadToAliOSS(fileName, data)
	} else if region == "r2" {
		return UploadToR2(fileName, data)
	}
	return UploadToTencentOSS(fileName, data)

}
func UploadToAliOSS(fileName string, data []byte) (string, error) {
	ossConfig := conf.GlobalConfig.OssConfig
	accessKeyID := ossConfig.AccessKeyID
	accessKeySecret := ossConfig.AccessKeySecret
	endpoint := ossConfig.Endpoint
	bucketName := ossConfig.Bucket

	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return "", fmt.Errorf("Error creating OSS client: %v", err)
	}

	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return "", fmt.Errorf("Error getting bucket: %v", err)
	}
	err = bucket.PutObject(fileName, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("Error uploading file: %v", err)
	}
	fileURL := fmt.Sprintf("%s/%s", conf.GlobalConfig.OssConfig.UgcAddr, fileName)
	return fileURL, nil
}

// Helper function to upload file to COS
func UploadToTencentOSS(fileName string, data []byte) (string, error) {
	cosConfig := conf.GlobalConfig.CosConfig
	accessKeyID := cosConfig.AccessKeyID
	accessKeySecret := cosConfig.AccessKeySecret
	endpoint := cosConfig.Endpoint
	// 解析 endpoint URL
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("Error parsing endpoint URL: %v", err)
	}
	b := &cos.BaseURL{BucketURL: u}

	// 创建 COS 客户端
	client := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  accessKeyID,
			SecretKey: accessKeySecret,
		},
	})
	// 上传文件
	_, err = client.Object.Put(context.Background(), fileName, bytes.NewReader(data), nil)
	if err != nil {
		return "", fmt.Errorf("Error uploading file: %v", err)
	}

	// 返回文件访问 URL
	fileURL := fmt.Sprintf("%s/%s", conf.GlobalConfig.CosConfig.UgcAddr, fileName)
	return fileURL, nil
}

// Helper function to upload file to Cloudflare R2
// R2 is S3-compatible, so we use AWS SDK v2
func UploadToR2(fileName string, data []byte) (string, error) {
	r2Config := conf.GlobalConfig.R2Config
	accessKeyID := r2Config.AccessKeyID
	accessKeySecret := r2Config.AccessKeySecret
	endpoint := r2Config.Endpoint
	bucketName := r2Config.Bucket

	// 创建 AWS S3 配置（R2 兼容 S3 API）
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, accessKeySecret, ""),
		),
		config.WithRegion("auto"), // R2 使用 "auto" 作为区域
	)
	if err != nil {
		return "", fmt.Errorf("Error creating R2 config: %v", err)
	}

	// 创建 S3 客户端并指定 R2 endpoint
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		// R2 需要使用 path-style addressing
		o.UsePathStyle = true
	})

	// 上传文件到 R2
	_, err = client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(fileName),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return "", fmt.Errorf("Error uploading file to R2: %v", err)
	}

	// 返回文件访问 URL
	fileURL := fmt.Sprintf("%s/%s", conf.GlobalConfig.R2Config.UgcAddr, fileName)
	return fileURL, nil
}
