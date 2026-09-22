package gpt4o

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/disintegration/imaging"
	"go.uber.org/zap"

	"va_visionai_server/internal/utils"
	"va_visionai_server/internal/zlog"
)

type Gpt4oConfig struct {
	Endpoint string
	ApiKey   string
}

type Gpt4oClient struct {
	config     Gpt4oConfig
	httpClient *http.Client
}
type ErrorResponse struct {
	Error struct {
		Message string  `json:"message"`
		Type    string  `json:"type"`
		Param   *string `json:"param"`
		Code    string  `json:"code"`
	} `json:"error"`
}

func NewGpt4oClient(config Gpt4oConfig) *Gpt4oClient {
	httpClient := utils.NewHTTPClient(utils.HTTPClientOptions{
		Timeout: 5 * time.Minute,
	})
	return &Gpt4oClient{
		config:     config,
		httpClient: httpClient,
	}
}

func (e *Gpt4oClient) StartImageToImage(ctx context.Context, taskID string, prompt string, localImagePath, localMaskPath string) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// 打开并添加图片文件
	imageFile, err := os.Open(localImagePath)
	if err != nil {
		zlog.LogWithContext(ctx).Error("打开并添加图片失败", zap.Error(err))
		return nil, err
	}
	defer imageFile.Close()

	if err = utils.CreateFormFileWithMIME(writer, "image", localImagePath, imageFile); err != nil {
		zlog.LogWithContext(ctx).Error("CreateFormFileWithMIME失败", zap.Error(err))
		return nil, err
	}

	// 打开并添加蒙版文件
	maskFile, err := os.Open(localMaskPath)
	if err != nil {
		zlog.LogWithContext(ctx).Error("打开蒙版图片失败", zap.Error(err))
		return nil, err
	}
	defer maskFile.Close()

	if err = utils.CreateFormFileWithMIME(writer, "mask", localMaskPath, maskFile); err != nil {
		zlog.LogWithContext(ctx).Error("CreateFormFileWithMIME蒙版图片失败", zap.Error(err))
		return nil, err
	}
	// 添加提示词
	err = writer.WriteField("prompt", prompt)
	if err != nil {
		zlog.LogWithContext(ctx).Error("添加提示词失败", zap.Error(err))
		return nil, err
	}

	// 读取原始图片尺寸
	origImg, err := imaging.Open(localImagePath)
	if err != nil {
		zlog.LogWithContext(ctx).Error("读取原始图片尺寸失败", zap.Error(err))
		return nil, err
	}

	widthImg := origImg.Bounds().Dx()
	heightImg := origImg.Bounds().Dy()
	aspectRatioImg := float64(widthImg) / float64(heightImg)

	// 根据宽高比决定输出尺寸
	var size string
	if aspectRatioImg > 1.0 { // 横向图片
		size = "1536x1024"
	} else if aspectRatioImg < 1.0 { // 纵向图片 (1/1.5 ≈ 0.67)
		size = "1024x1536"
	} else { // 接近正方形的图片
		size = "1024x1024"
	}

	err = writer.WriteField("size", size)
	if err != nil {
		zlog.LogWithContext(ctx).Error("设置图片尺寸失败", zap.Error(err))
		return nil, err
	}
	err = writer.WriteField("quality", "high")
	if err != nil {
		zlog.LogWithContext(ctx).Error("设置图片生成质量失败", zap.Error(err))
		return nil, err
	}
	writer.Close()

	req, err := http.NewRequest("POST", e.config.Endpoint, &body)
	if err != nil {
		zlog.LogWithContext(ctx).Error("构建请求失败", zap.Error(err))
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.config.ApiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// 为请求添加超时控制
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	resp, err := e.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			zlog.LogWithContext(ctx).Error("请求超时", zap.Error(err))
		} else {
			zlog.LogWithContext(ctx).Error("请求失败", zap.Error(err))
		}
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	parseError, err := e.parseError(bodyBytes)
	if parseError != nil {
		if len(parseError.Error.Message) != 0 {
			zlog.LogWithContext(ctx).Error("请求失败", zap.Error(err))
			return nil, errors.New(parseError.Error.Message)
		}
	}

	if err != nil {
		zlog.LogWithContext(ctx).Error("读取响应失败", zap.Error(err))
		return nil, err
	}
	// 再解析 JSON
	var result struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		zlog.LogWithContext(ctx).Error("解析响应json失败", zap.Error(err))
		return nil, err
	}

	if len(result.Data) == 0 {
		zlog.LogWithContext(ctx).Error("生成图片失败", zap.Error(err))
		return nil, err
	}

	// 解码 base64 图片数据
	imgData, err := base64.StdEncoding.DecodeString(result.Data[0].B64JSON)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to decode base64 image", zap.Error(err))
		return nil, err
	}

	return imgData, nil
}

// StartTextToImage 文生图方法
func (e *Gpt4oClient) StartTextToImage(ctx context.Context, taskID string, prompt string, size string) ([]byte, error) {
	reqBody := map[string]interface{}{
		"prompt":  prompt,
		"n":       1,
		"size":    size,
		"quality": "high",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		zlog.LogWithContext(ctx).Error("序列化请求参数失败", zap.Error(err))
		return nil, err
	}

	req, err := http.NewRequest("POST", e.config.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		zlog.LogWithContext(ctx).Error("构建请求失败", zap.Error(err))
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+e.config.ApiKey)
	req.Header.Set("Content-Type", "application/json")

	// 为请求添加超时控制
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	resp, err := e.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			zlog.LogWithContext(ctx).Error("请求超时", zap.Error(err))
		} else {
			zlog.LogWithContext(ctx).Error("请求失败", zap.Error(err))
		}
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		zlog.LogWithContext(ctx).Error("读取响应失败", zap.Error(err))
		return nil, err
	}

	// 检查错误响应
	parseError, err := e.parseError(bodyBytes)
	if parseError != nil {
		if len(parseError.Error.Message) != 0 {
			zlog.LogWithContext(ctx).Error("API返回错误", zap.String("message", parseError.Error.Message))
			return nil, errors.New(parseError.Error.Message)
		}
	}

	// 解析 JSON 响应
	var result struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		zlog.LogWithContext(ctx).Error("解析响应json失败", zap.Error(err))
		return nil, err
	}

	if len(result.Data) == 0 {
		zlog.LogWithContext(ctx).Error("生成图片失败：返回数据为空")
		return nil, errors.New("生成图片失败：返回数据为空")
	}

	// 解码 base64 图片数据
	imgData, err := base64.StdEncoding.DecodeString(result.Data[0].B64JSON)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to decode base64 image", zap.Error(err))
		return nil, err
	}

	return imgData, nil
}

func (e *Gpt4oClient) parseError(response []byte) (*ErrorResponse, error) {
	var errResp ErrorResponse
	if err := json.Unmarshal(response, &errResp); err != nil {
		return nil, err
	}
	return &errResp, nil
}
