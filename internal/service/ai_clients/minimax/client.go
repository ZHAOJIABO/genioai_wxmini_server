package minimax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/utils"
	"va_visionai_server/internal/zlog"
)

type MinimaxConfig struct {
	Endpoint string
	AppID    string
	Secret   string
}

type MinimaxClient struct {
	config     MinimaxConfig
	httpClient *http.Client
}

// VideoGenerationRequest 视频生成请求结构
type VideoGenerationRequest struct {
	Model           string `json:"model"`
	Prompt          string `json:"prompt"`
	FirstFrameImage string `json:"first_frame_image,omitempty"`
}

// TaskQueryAPIResponse represents the API response for a task query.
type TaskQueryAPIResponse struct {
	Code    int64         `json:"code"`
	Message string        `json:"message"`
	Data    TaskQueryData `json:"data"`
}

// TaskQueryData contains the detailed data of a task query response.
type TaskQueryData struct {
	ID              int64                    `json:"id"`
	TaskID          int64                    `json:"task_id"`
	TaskType        string                   `json:"task_type"`
	Status          string                   `json:"status"`
	TaskStatusMsg   string                   `json:"task_status_msg"`
	Video           TaskQueryVideo           `json:"video"`
	VideoGeneration TaskQueryVideoGeneration `json:"video_generation"`
	CreateTime      int64                    `json:"create_time"`
	StartTime       int64                    `json:"start_time"`
	EndTime         int64                    `json:"end_time"`
	UpdateTime      int64                    `json:"update_time"`
}

// TaskQueryVideo contains video information from a task query response.
type TaskQueryVideo struct {
	ID        int64  `json:"id"`
	Height    int64  `json:"height"`
	Width     int64  `json:"width"`
	OriginURL string `json:"origin_url"`
	OSSURL    string `json:"oss_url"`
}

// TaskQueryVideoGeneration contains video generation parameters from a task query response.
type TaskQueryVideoGeneration struct {
	Model            string      `json:"model"`
	Prompt           string      `json:"prompt"`
	Resolution       string      `json:"resolution"`
	Duration         int64       `json:"duration"`
	PromptOptimizer  bool        `json:"prompt_optimizer"`
	FirstFrameImage  string      `json:"first_frame_image"`
	SubjectReference interface{} `json:"subject_reference"`
}

// Response 通用API响应结构

type Response struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Data    Data   `json:"data"`
}

type Data struct {
	ID              int64           `json:"id"`
	TaskID          int64           `json:"task_id"`
	TaskType        string          `json:"task_type"`
	Status          string          `json:"status"`
	VideoGeneration VideoGeneration `json:"video_generation"`
	CreateTime      int64           `json:"create_time"`
	StartTime       int64           `json:"start_time"`
}

type VideoGeneration struct {
	Model            string      `json:"model"`
	Prompt           string      `json:"prompt"`
	Resolution       string      `json:"resolution"`
	Duration         int64       `json:"duration"`
	PromptOptimizer  bool        `json:"prompt_optimizer"`
	FirstFrameImage  string      `json:"first_frame_image"`
	SubjectReference interface{} `json:"subject_reference"`
}

type StatusMsg struct {
	TaskID      string            `json:"task_id"`
	Status      string            `json:"status"`
	FileID      string            `json:"file_id"`
	VideoWidth  int64             `json:"video_width"`
	VideoHeight int64             `json:"video_height"`
	BaseResp    StatusMsgBaseResp `json:"base_resp"`
}

type StatusMsgBaseResp struct {
	StatusCode uint16 `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

const (
	MinimaxVideoGenerationEndpoint = "/v1/video_generation"
	MinimaxTaskQueryEndpoint       = "/v1/query/video_generation"
)

func NewMinimaxClient(config MinimaxConfig) *MinimaxClient {
	httpClient := utils.NewHTTPClient(utils.HTTPClientOptions{
		Timeout: 5 * time.Minute,
	})
	return &MinimaxClient{
		config:     config,
		httpClient: httpClient,
	}
}

type JobResult struct {
	// Minimax 任务ID
	JobID string
	// 系统内部任务 ID
	ExternalTaskID   string
	Progress         float64
	Done             bool
	VideoURL         string
	Error            string
	ServerOriginCode uint16
	ServerOriginMsg  string
}

// MinimaxAPIError 用于携带HTTP状态码、业务码、消息和请求ID
type MinimaxAPIError struct {
	HTTPStatus int
	BizCode    int
	Message    string
	TraceID    string
}

func (e *MinimaxAPIError) Error() string {
	return fmt.Sprintf("Minimax API Error - HTTPStatus: %d, BizCode: %d, Message: %s, TraceID: %s", e.HTTPStatus, e.BizCode, e.Message, e.TraceID)
}

func (c *MinimaxClient) StartVideoGeneration(ctx context.Context, params []byte) (*JobResult, error) {
	url, err := url.JoinPath(c.config.Endpoint, MinimaxVideoGenerationEndpoint)
	if err != nil {
		return nil, fmt.Errorf("构建请求URL失败: %w", err)
	}

	request := utils.NewHTTPRequest("POST", url)
	request.Headers["Content-Type"] = "application/json"
	request.Headers["appid"] = c.config.AppID
	request.Headers["secret"] = c.config.Secret
	request.Body = params
	//nolint:bodyclose
	resp, body, err := request.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("API请求失败: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return nil, &MinimaxAPIError{
			HTTPStatus: resp.StatusCode,
			BizCode:    0,
			Message:    fmt.Sprintf("HTTP error: %d", resp.StatusCode),
			TraceID:    "",
		}
	}

	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w, 响应: %s", err, string(body))
	}

	// 检查是否有task_id
	if response.Data.TaskID == 0 {
		return nil, fmt.Errorf("API响应中缺少task_id, 响应: %s", string(body))
	}

	return &JobResult{
		JobID:    strconv.FormatInt(response.Data.TaskID, 10),
		Progress: 10,
		Done:     false,
	}, nil
}

func (c *MinimaxClient) GetJobStatus(ctx context.Context, jobID string) (*JobResult, error) {
	baseURL, err := url.JoinPath(c.config.Endpoint, MinimaxTaskQueryEndpoint)
	if err != nil {
		return nil, fmt.Errorf("构建请求URL失败: %w", err)
	}

	// 构建带查询参数的URL - 使用task_id参数
	fullURL := fmt.Sprintf("%s?task_id=%s", baseURL, jobID)

	request := utils.NewHTTPRequest("GET", fullURL)
	request.Headers["appid"] = c.config.AppID
	request.Headers["secret"] = c.config.Secret

	//nolint:bodyclose
	resp, body, err := request.Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("查询任务状态失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &MinimaxAPIError{
			HTTPStatus: resp.StatusCode,
			BizCode:    0,
			Message:    fmt.Sprintf("HTTP error: %d", resp.StatusCode),
			TraceID:    "",
		}
	}

	var response TaskQueryAPIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析状态查询响应失败: %w, 响应: %s", err, string(body))
	}

	if response.Code != 0 {
		return nil, &MinimaxAPIError{
			HTTPStatus: resp.StatusCode,
			BizCode:    int(response.Code),
			Message:    response.Message,
		}
	}

	switch response.Data.Status {
	case "TS_PENDING":
		return &JobResult{
			JobID:    jobID,
			Progress: 20,
			Done:     false,
			VideoURL: "",
		}, nil
	case "TS_PROCESSING":
		return &JobResult{
			JobID:    jobID,
			Progress: 30,
			Done:     false,
			VideoURL: "",
		}, nil
	case "TS_SUCCEED":
		return &JobResult{
			JobID:    jobID,
			Progress: 70,
			Done:     true,
			VideoURL: response.Data.Video.OSSURL,
		}, nil
	case "TS_FAILED":
		errMsg := "Task failed at provider"
		if response.Message != "" {
			errMsg = response.Message
		}
		if response.Data.TaskStatusMsg != "" {
			errMsg = response.Data.TaskStatusMsg
		}
		var statusMsg StatusMsg
		json.Unmarshal([]byte(response.Data.TaskStatusMsg), &statusMsg)
		if statusMsg.BaseResp.StatusCode != 0 {
			errMsg = statusMsg.BaseResp.StatusMsg
		}
		return &JobResult{
			JobID:            jobID,
			Progress:         0,
			Done:             true,
			VideoURL:         "",
			Error:            errMsg,
			ServerOriginCode: statusMsg.BaseResp.StatusCode,
			ServerOriginMsg:  statusMsg.BaseResp.StatusMsg,
		}, errors.New(errMsg)
	default:
		return &JobResult{
			JobID:    jobID,
			Progress: 0,
			Done:     false,
			VideoURL: "",
		}, nil
	}
}

func (c *MinimaxClient) DownloadVideo(ctx context.Context, videoURL string) ([]byte, error) {
	zlog.LogWithContext(ctx).Info("开始下载 Minimax 视频", zap.String("url", videoURL))

	request := utils.NewHTTPRequest("GET", videoURL)
	//nolint:bodyclose
	resp, body, err := request.Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("下载视频失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载视频失败，状态码: %d", resp.StatusCode)
	}

	zlog.LogWithContext(ctx).Info("成功下载 Minimax 视频",
		zap.String("url", videoURL),
		zap.Int("size", len(body)))
	return body, nil
}
