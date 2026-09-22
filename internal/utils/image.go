package utils

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// ImageSourceType 定义图片来源类型
type ImageSourceType int

const (
	ImageSourceUnknown ImageSourceType = iota
	ImageSourceBase64
	ImageSourceURL
)

// getImageSourceType 判断图片来源类型
func getImageSourceType(source string) ImageSourceType {
	switch {
	case strings.HasPrefix(source, "data:image"):
		return ImageSourceBase64
	case strings.HasPrefix(source, "http://"), strings.HasPrefix(source, "https://"):
		if _, err := url.ParseRequestURI(source); err == nil {
			return ImageSourceURL
		}
	}
	return ImageSourceUnknown
}

// CalculateImageMD5 计算图片的MD5值，支持Base64编码和URL格式
// 参数 source 可以是Base64编码的图片数据或图片URL
// 返回MD5哈希值（十六进制字符串格式）和可能的错误
func CalculateImageMD5(source string) (string, error) {
	sourceType := getImageSourceType(source)

	switch sourceType {
	case ImageSourceBase64:
		return calculateBase64MD5(source)
	case ImageSourceURL:
		return calculateURLMD5(source)
	default:
		return "", ErrInvalidImageSource
	}
}

// calculateBase64MD5 计算Base64编码图片的MD5值
func calculateBase64MD5(source string) (string, error) {
	// 提取Base64实际数据部分
	base64Data := source[strings.IndexByte(source, ',')+1:]

	// 解码Base64数据
	imageData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", err
	}

	// 计算MD5
	hash := md5.Sum(imageData)
	return hex.EncodeToString(hash[:]), nil
}

func GetMD5FromBytes(imageBytes []byte) string {
	hash := md5.Sum(imageBytes)
	return hex.EncodeToString(hash[:])
}

// calculateURLMD5 计算URL图片的MD5值
func calculateURLMD5(source string) (string, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 发起HTTP请求
	resp, err := client.Get(source)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return "", ErrInvalidImageResponse
	}

	// 创建MD5哈希对象
	hash := md5.New()

	// 将响应体数据写入哈希对象
	if _, err := io.Copy(hash, resp.Body); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// URLToBase64 将URL图片转换为Base64编码
// 参数 imageURL 是图片的URL地址
// 返回Base64编码的图片数据和可能的错误
func URLToBase64(imageURL string) (string, error) {
	// 验证URL格式
	if getImageSourceType(imageURL) != ImageSourceURL {
		return "", ErrInvalidImageSource
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 发起HTTP请求
	resp, err := client.Get(imageURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return "", ErrInvalidImageResponse
	}

	// 读取响应内容
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return "", err
	}

	// 获取内容类型（MIME类型）
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg" // 默认使用jpeg类型
	}

	// 构建Base64编码
	base64Prefix := "data:" + contentType + ";base64,"
	base64Data := base64.StdEncoding.EncodeToString(buf.Bytes())

	return base64Prefix + base64Data, nil
}

// 定义错误类型
var (
	ErrInvalidImageSource   = errors.New("无效的图片来源格式")
	ErrInvalidImageResponse = errors.New("获取图片失败，服务器返回非200状态码")
)

// DownloadFile 从URL下载文件内容
func DownloadFile(url string) ([]byte, error) {
	// 如果URL已经是base64编码数据，直接解码返回
	if strings.HasPrefix(url, "data:image") && strings.Contains(url, ";base64,") {
		parts := strings.Split(url, ";base64,")
		if len(parts) != 2 {
			return nil, nil
		}
		return base64.StdEncoding.DecodeString(parts[1])
	}

	// 发起HTTP请求获取文件
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应体内容
	return io.ReadAll(resp.Body)
}

// ReadFile 从本地文件系统读取文件
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// EncodeImageContentForLLM 根据模型类型编码图片内容
func EncodeImageContentForLLM(imageBytes []byte, modelID vai.Model, userPrompt string) string {
	if len(imageBytes) == 0 {
		return ""
	}

	// 根据不同的模型，返回不同格式的图片编码内容
	switch modelID {
	case vai.Model_MODEL_GEMINI_2_0_FLASH:
		// Gemini模型使用base64编码
		base64Img := base64.StdEncoding.EncodeToString(imageBytes)
		return base64Img

	case vai.Model_MODEL_GPT4O, vai.Model_MODEL_GPT4O_MINI:
		// OpenAI的GPT-4V使用base64编码，可能格式略有不同
		base64Img := base64.StdEncoding.EncodeToString(imageBytes)
		return base64Img

	case vai.Model_MODEL_DOUBAO_VISION_PRO:
		// 豆包视觉模型编码
		base64Img := base64.StdEncoding.EncodeToString(imageBytes)
		return base64Img

	default:
		// 默认使用base64编码
		zlog.Logger.Info("未知模型图片编码", zap.String("model", modelID.String()))
		return base64.StdEncoding.EncodeToString(imageBytes)
	}
}
