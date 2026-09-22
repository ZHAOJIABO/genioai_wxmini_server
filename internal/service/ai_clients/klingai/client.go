package klingai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/utils"
	"va_visionai_server/internal/zlog"
)

type KlingConfig struct {
	Endpoint string
	AppID    string
	Secret   string
}

type KlingClient struct {
	config     KlingConfig
	httpClient *http.Client
}

type Video struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Duration int    `json:"duration"`
}

type Response struct {
	Code      int64  `json:"code"`
	Data      Data   `json:"data"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type Data struct {
	CreatedAt     int64      `json:"created_at"`
	EndTime       int64      `json:"end_time"`
	StartTime     int64      `json:"start_time"`
	TaskID        int64      `json:"task_id"`
	TaskInfo      TaskInfo   `json:"task_info"`
	TaskResult    TaskResult `json:"task_result"`
	TaskStatus    string     `json:"task_status"`
	TaskStatusMsg string     `json:"task_status_msg"`
	TaskType      string     `json:"task_type"`
	UpdatedAt     int64      `json:"updated_at"`
}

type TaskInfo struct {
	ExternalTaskID string      `json:"external_task_id"`
	ParentVideo    ParentVideo `json:"parent_video"`
}

type ParentVideo struct {
	Duration int64  `json:"duration"`
	ID       string `json:"id"`
	URL      string `json:"url"`
}

type TaskResult struct {
	Videos []Video `json:"videos"`
}

const (
	KlingImage2VideoEndpoint      = "api/kling/videos/image2video"
	KlingMultiImage2VideoEndpoint = "api/kling/videos/multi-image2video"
	KlingGetTaskStatusEndpoint    = "api/kling/task/%s"
	KlingVideoEffectsEndpoint     = "api/kling/videos/effects"
)

func NewKlingClient(config KlingConfig) *KlingClient {
	httpClient := utils.NewHTTPClient(utils.HTTPClientOptions{
		Timeout: 5 * time.Minute,
	})
	return &KlingClient{
		config:     config,
		httpClient: httpClient,
	}
}

type JobResult struct {
	// kling 人物ID
	JobID string
	// 系统内部任务 ID
	ExternalTaskID string
	Progress       float64
	Done           bool
	VideoURL       string
	Error          string
}

// 新增结构化API错误类型
// KlingAPIError 用于携带HTTP状态码、业务码、消息和请求ID
type KlingAPIError struct {
	HTTPStatus int
	BizCode    int64
	Message    string
	RequestID  string
}

func (e *KlingAPIError) Error() string {
	return fmt.Sprintf("Kling API Error - HTTPStatus: %d, BizCode: %d, Message: %s, RequestID: %s", e.HTTPStatus, e.BizCode, e.Message, e.RequestID)
}

func (c *KlingClient) StartImageToVideo(ctx context.Context, params []byte) (*JobResult, error) {
	url, err := url.JoinPath(c.config.Endpoint, KlingImage2VideoEndpoint)
	if err != nil {
		return nil, fmt.Errorf("构建请求URL失败: %w", err)
	}

	request := utils.NewHTTPRequest("POST", url)
	request.Headers["Content-Type"] = "application/json"
	request.Headers["appid"] = c.config.AppID
	request.Headers["secret"] = c.config.Secret
	request.Body = params

	//nolint:bodyclose
	resp, body, err := request.Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("API请求失败: %w", err)
	}

	var response Response

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w, 响应: %s", err, string(body))
	}

	if response.Code != 0 {
		httpStatus := 0
		if resp != nil {
			httpStatus = resp.StatusCode
		}
		return nil, &KlingAPIError{
			HTTPStatus: httpStatus,
			BizCode:    response.Code,
			Message:    response.Message,
			RequestID:  response.RequestID,
		}
	}
	jobID := strconv.FormatInt(response.Data.TaskID, 10)
	return &JobResult{
		JobID:    jobID,
		Progress: 10,
		Done:     false,
	}, nil
}

func (c *KlingClient) StartMultiImageToVideo(ctx context.Context, params []byte) (*JobResult, error) {
	url, err := url.JoinPath(c.config.Endpoint, KlingMultiImage2VideoEndpoint)
	if err != nil {
		return nil, fmt.Errorf("构建请求URL失败: %w", err)
	}

	request := utils.NewHTTPRequest("POST", url)
	request.Headers["Content-Type"] = "application/json"
	request.Headers["appid"] = c.config.AppID
	request.Headers["secret"] = c.config.Secret
	request.Body = params

	//nolint:bodyclose
	resp, body, err := request.Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("API请求失败: %w", err)
	}

	var response Response

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w, 响应: %s", err, string(body))
	}

	if response.Code != 0 {
		httpStatus := 0
		if resp != nil {
			httpStatus = resp.StatusCode
		}
		return nil, &KlingAPIError{
			HTTPStatus: httpStatus,
			BizCode:    response.Code,
			Message:    response.Message,
			RequestID:  response.RequestID,
		}
	}
	jobID := strconv.FormatInt(response.Data.TaskID, 10)
	return &JobResult{
		JobID:    jobID,
		Progress: 10,
		Done:     false,
	}, nil
}

func (c *KlingClient) GetJobStatus(ctx context.Context, jobID string) (*JobResult, error) {
	url, err := url.JoinPath(c.config.Endpoint, fmt.Sprintf(KlingGetTaskStatusEndpoint, jobID))
	if err != nil {
		return nil, fmt.Errorf("构建请求URL失败: %w", err)
	}

	request := utils.NewHTTPRequest("GET", url)
	request.Headers["appid"] = c.config.AppID
	request.Headers["secret"] = c.config.Secret

	//nolint:bodyclose
	resp, body, err := request.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	var response Response

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w, 响应: %s", err, string(body))
	}

	if response.Code != 0 {
		httpStatus := 0
		if resp != nil {
			httpStatus = resp.StatusCode
		}
		return nil, &KlingAPIError{
			HTTPStatus: httpStatus,
			BizCode:    response.Code,
			Message:    response.Message,
			RequestID:  response.RequestID,
		}
	}

	if response.Data.TaskStatus == "submitted" {
		return &JobResult{
			JobID:    jobID,
			Progress: 15,
			Done:     false,
			VideoURL: "",
		}, nil
	}

	if response.Data.TaskStatus == "processing" {
		return &JobResult{
			JobID:    jobID,
			Progress: 50,
			Done:     false,
			VideoURL: "",
		}, nil
	}

	if response.Data.TaskStatus == "succeed" {
		return &JobResult{
			JobID:    jobID,
			Progress: 100,
			Done:     true,
			VideoURL: response.Data.TaskResult.Videos[0].URL,
			Error:    "",
		}, nil
	}

	if response.Data.TaskStatus == "failed" {
		return &JobResult{
				JobID:    jobID,
				Progress: 0,
				Done:     true,
				VideoURL: "",
				Error:    response.Data.TaskStatusMsg,
			}, &KlingAPIError{
				HTTPStatus: 0,
				BizCode:    response.Code,
				Message:    response.Data.TaskStatusMsg,
				RequestID:  response.RequestID,
			}
	}
	return nil, &KlingAPIError{
		HTTPStatus: 0,
		BizCode:    response.Code,
		Message:    "未知任务状态: " + response.Data.TaskStatus,
		RequestID:  response.RequestID,
	}
}

func (c *KlingClient) DownloadVideo(ctx context.Context, videoURL string) ([]byte, error) {
	log := zlog.LogWithContext(ctx)
	log.Info("开始下载视频", zap.String("url", videoURL))

	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		// 检查调用者是否已设置截止时间
		req := utils.NewHTTPRequest("GET", videoURL).WithClient(c.httpClient)
		req.Options = utils.DefaultHTTPOptions
		// nolint:bodyclose
		resp, body, err := req.Do(context.Background())
		if err == nil && resp.StatusCode == http.StatusOK {
			log.Info("视频下载完成",
				zap.String("url", videoURL),
				zap.Int("size", len(body)))
			return body, nil
		}

		// 构造错误信息
		if err != nil {
			lastErr = fmt.Errorf("下载视频失败: %w", err)
		} else {
			lastErr = fmt.Errorf("下载视频返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
		}

		// 如果不是最后一次尝试，记录重试日志
		if i < maxRetries-1 {
			log.Warn("下载视频失败，重试中",
				zap.String("url", videoURL),
				zap.Int("attempt", i+1),
				zap.Error(lastErr))
		}
	}

	return nil, errors.Wrapf(lastErr, "下载视频失败，已重试%d次", maxRetries)
}

// StartVideoEffects 调用可灵视频特效API启动任务
func (c *KlingClient) StartVideoEffects(ctx context.Context, params []byte) (*JobResult, error) {
	url, err := url.JoinPath(c.config.Endpoint, KlingVideoEffectsEndpoint)
	if err != nil {
		return nil, fmt.Errorf("构建请求URL失败: %w", err)
	}

	request := utils.NewHTTPRequest("POST", url)
	request.Headers["Content-Type"] = "application/json"
	request.Headers["appid"] = c.config.AppID
	request.Headers["secret"] = c.config.Secret
	request.Body = params

	//nolint:bodyclose
	newctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// nolint:bodyclose
	resp, body, err := request.Do(newctx)
	if err != nil {
		return nil, fmt.Errorf("API请求失败: %w", err)
	}

	var response Data
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w, 响应: %s", err, string(body))
	}

	if response.TaskStatus == "failed" {
		httpStatus := 0
		if resp != nil {
			httpStatus = resp.StatusCode
		}
		return nil, &KlingAPIError{
			HTTPStatus: httpStatus,
		}
	}
	jobID := strconv.FormatInt(response.TaskID, 10)
	return &JobResult{
		JobID:    jobID,
		Progress: 10,
		Done:     false,
	}, nil
}
