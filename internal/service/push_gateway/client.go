// nolint:bodyclose
package push_gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/utils"
)

// Client push-gateway HTTP 客户端
type Client struct {
	enabled bool
	addr    string
	timeout time.Duration
	env     string
	log     *zap.Logger
}

// RegisterDeviceRequest 注册设备请求
type RegisterDeviceRequest struct {
	UserID   string `json:"user_id"`
	Platform string `json:"platform"` // "ios" | "android"
	Token    string `json:"token"`
	Env      string `json:"env"`    // "production" | "sandbox"
	AppID    string `json:"app_id"` // 应用标识
}

// RegisterDeviceResponse 注册设备响应
type RegisterDeviceResponse struct {
	Data      *DeviceData `json:"data,omitempty"`
	Error     *ErrorData  `json:"error,omitempty"`
	RequestID string      `json:"request_id"`
}

// DeviceData 设备数据
type DeviceData struct {
	ID         uint64 `json:"id"`
	UserID     string `json:"user_id"`
	Platform   string `json:"platform"`
	Token      string `json:"token"`
	Env        string `json:"env"`
	AppID      string `json:"app_id"`
	IsValid    bool   `json:"is_valid"`
	LastSeenAt string `json:"last_seen_at,omitempty"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// ErrorData 错误数据
type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// GetUserDevicesResponse 查询用户设备响应
type GetUserDevicesResponse struct {
	Data      []*DeviceData `json:"data,omitempty"`
	Error     *ErrorData    `json:"error,omitempty"`
	RequestID string        `json:"request_id"`
}

// MaxPushBatchSize 批量推送最大用户数
const MaxPushBatchSize = 100

// SendPushRequest 发送推送请求
type SendPushRequest struct {
	UserIDs []string               `json:"user_ids"`         // 用户ID列表，推送到这些用户所有有效设备，最大100个
	Title   string                 `json:"title,omitempty"`  // 通知标题 (title/body 至少填一个)
	Body    string                 `json:"body,omitempty"`   // 通知内容
	Sound   string                 `json:"sound,omitempty"`  // 提示音，如 default
	Badge   *int                   `json:"badge,omitempty"`  // 角标数字 (仅 iOS 有效)
	Data    map[string]interface{} `json:"data,omitempty"`   // 自定义数据
	BizID   string                 `json:"biz_id,omitempty"` // 业务幂等ID
}

// SendPushResponse 发送推送响应
type SendPushResponse struct {
	Data      *PushResultData `json:"data,omitempty"`
	Error     *ErrorData      `json:"error,omitempty"`
	RequestID string          `json:"request_id"`
}

// PushResultData 推送结果数据
type PushResultData struct {
	TotalDevices int                 `json:"TotalDevices"`
	Sent         int                 `json:"Sent"`
	Failed       int                 `json:"Failed"`
	Skipped      int                 `json:"Skipped"`
	Results      []*PushDeviceResult `json:"Results"`
}

// PushDeviceResult 单设备推送结果
type PushDeviceResult struct {
	DeviceID  uint64 `json:"DeviceID"`
	Platform  string `json:"Platform"`
	Success   bool   `json:"Success"`
	MessageID string `json:"MessageID,omitempty"`
	ErrorCode string `json:"ErrorCode,omitempty"`
	Skipped   bool   `json:"Skipped,omitempty"`
}

// NewClient 创建 push-gateway 客户端
func NewClient(enabled bool, addr string, timeout time.Duration, env string, log *zap.Logger) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if env == "" {
		env = "production"
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &Client{
		enabled: enabled,
		addr:    addr,
		timeout: timeout,
		env:     env,
		log:     log,
	}
}

// IsEnabled 返回客户端是否启用
func (c *Client) IsEnabled() bool {
	return c.enabled
}

// RegisterDevice 注册设备推送 token
func (c *Client) RegisterDevice(ctx context.Context, req *RegisterDeviceRequest) error {
	if req == nil {
		return errors.New("invalid request: req is nil")
	}

	if !c.enabled {
		c.log.Debug("push-gateway 未启用，跳过设备注册",
			zap.String("user_id", req.UserID),
			zap.String("platform", req.Platform))
		return nil
	}

	if req.UserID == "" || req.Token == "" || req.Platform == "" {
		return errors.New("invalid request: user_id, token and platform are required")
	}

	if req.Env == "" {
		req.Env = c.env
	}

	url := c.addr + "/v1/devices"

	c.log.Info("开始注册推送 token",
		zap.String("user_id", req.UserID),
		zap.String("platform", req.Platform),
		zap.String("app_id", req.AppID))

	httpReq := utils.NewHTTPRequest("POST", url).
		WithBody(req).
		WithOptions(utils.HTTPClientOptions{
			Timeout:          c.timeout,
			MaxRetries:       2,
			RetryInterval:    500 * time.Millisecond,
			RetryStatusCodes: []int{408, 429, 500, 502, 503, 504},
		})

	resp, body, err := httpReq.Do(ctx)
	if err != nil {
		c.log.Error("注册推送 token 请求失败",
			zap.String("user_id", req.UserID),
			zap.String("platform", req.Platform),
			zap.Error(err))
		return fmt.Errorf("register device request failed: %w", err)
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		c.log.Error("注册推送 token 返回错误状态码",
			zap.String("user_id", req.UserID),
			zap.String("platform", req.Platform),
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(body)))
		return fmt.Errorf("register device returned status %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var response RegisterDeviceResponse
	if err := json.Unmarshal(body, &response); err != nil {
		c.log.Error("解析响应失败",
			zap.String("user_id", req.UserID),
			zap.String("platform", req.Platform),
			zap.String("response", string(body)),
			zap.Error(err))
		return fmt.Errorf("parse response failed: %w", err)
	}

	// 检查业务错误
	if response.Error != nil {
		c.log.Error("注册推送 token 业务错误",
			zap.String("user_id", req.UserID),
			zap.String("platform", req.Platform),
			zap.String("error_code", response.Error.Code),
			zap.String("error_message", response.Error.Message))
		return fmt.Errorf("register device error: %s - %s", response.Error.Code, response.Error.Message)
	}

	if response.Data == nil {
		c.log.Error("注册推送 token 返回数据为空",
			zap.String("user_id", req.UserID),
			zap.String("platform", req.Platform))
		return errors.New("register device response data is nil")
	}

	c.log.Info("推送 token 注册成功",
		zap.String("user_id", req.UserID),
		zap.String("platform", req.Platform),
		zap.Uint64("device_id", response.Data.ID))

	return nil
}

// UnregisterDevice 注销设备推送 token
func (c *Client) UnregisterDevice(ctx context.Context, platform, env, token string) error {
	if !c.enabled {
		c.log.Debug("push-gateway 未启用，跳过设备注销")
		return nil
	}

	if platform == "" || token == "" {
		return errors.New("invalid request: platform and token are required")
	}

	url := c.addr + "/v1/devices"

	c.log.Info("开始注销推送 token",
		zap.String("platform", platform))

	reqBody := map[string]string{
		"platform": platform,
		"token":    token,
		"env":      env,
	}

	httpReq := utils.NewHTTPRequest("DELETE", url).
		WithBody(reqBody).
		WithOptions(utils.HTTPClientOptions{
			Timeout:          c.timeout,
			MaxRetries:       2,
			RetryInterval:    500 * time.Millisecond,
			RetryStatusCodes: []int{408, 429, 500, 502, 503, 504},
		})

	resp, body, err := httpReq.Do(ctx)
	if err != nil {
		c.log.Error("注销推送 token 请求失败",
			zap.String("platform", platform),
			zap.Error(err))
		return fmt.Errorf("unregister device request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.log.Error("注销推送 token 返回错误状态码",
			zap.String("platform", platform),
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(body)))
		return fmt.Errorf("unregister device returned status %d: %s", resp.StatusCode, string(body))
	}

	c.log.Info("推送 token 注销成功",
		zap.String("platform", platform))

	return nil
}

// GetUserDevices 获取用户所有有效设备列表
func (c *Client) GetUserDevices(ctx context.Context, userID string) ([]*DeviceData, error) {
	if !c.enabled {
		c.log.Debug("push-gateway 未启用，跳过查询用户设备",
			zap.String("user_id", userID))
		return nil, nil
	}

	if userID == "" {
		return nil, errors.New("invalid request: user_id is required")
	}

	url := fmt.Sprintf("%s/v1/users/%s/devices", c.addr, userID)

	c.log.Info("开始查询用户设备",
		zap.String("user_id", userID))

	httpReq := utils.NewHTTPRequest("GET", url).
		WithOptions(utils.HTTPClientOptions{
			Timeout:          c.timeout,
			MaxRetries:       2,
			RetryInterval:    500 * time.Millisecond,
			RetryStatusCodes: []int{408, 429, 500, 502, 503, 504},
		})

	resp, body, err := httpReq.Do(ctx)
	if err != nil {
		c.log.Error("查询用户设备请求失败",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, fmt.Errorf("get user devices request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.log.Error("查询用户设备返回错误状态码",
			zap.String("user_id", userID),
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(body)))
		return nil, fmt.Errorf("get user devices returned status %d: %s", resp.StatusCode, string(body))
	}

	var response GetUserDevicesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		c.log.Error("解析响应失败",
			zap.String("user_id", userID),
			zap.String("response", string(body)),
			zap.Error(err))
		return nil, fmt.Errorf("parse response failed: %w", err)
	}

	if response.Error != nil {
		c.log.Error("查询用户设备业务错误",
			zap.String("user_id", userID),
			zap.String("error_code", response.Error.Code),
			zap.String("error_message", response.Error.Message))
		return nil, fmt.Errorf("get user devices error: %s - %s", response.Error.Code, response.Error.Message)
	}

	c.log.Info("查询用户设备成功",
		zap.String("user_id", userID),
		zap.Int("device_count", len(response.Data)))

	return response.Data, nil
}

// SendPush 向用户所有有效设备发送推送通知
func (c *Client) SendPush(ctx context.Context, req *SendPushRequest) (*PushResultData, error) {
	if req == nil {
		return nil, errors.New("invalid request: req is nil")
	}

	if !c.enabled {
		c.log.Debug("push-gateway 未启用，跳过发送推送",
			zap.Strings("user_ids", req.UserIDs))
		return nil, nil
	}

	if len(req.UserIDs) == 0 {
		return nil, errors.New("invalid request: user_ids is required")
	}

	if len(req.UserIDs) > MaxPushBatchSize {
		return nil, fmt.Errorf("invalid request: user_ids exceeds max batch size %d", MaxPushBatchSize)
	}

	if req.Title == "" && req.Body == "" {
		return nil, errors.New("invalid request: title or body is required")
	}

	url := c.addr + "/v1/push"

	c.log.Info("开始发送推送通知",
		zap.Strings("user_ids", req.UserIDs),
		zap.Int("user_count", len(req.UserIDs)),
		zap.String("title", req.Title),
		zap.String("biz_id", req.BizID))

	httpReq := utils.NewHTTPRequest("POST", url).
		WithBody(req).
		WithOptions(utils.HTTPClientOptions{
			Timeout:          c.timeout,
			MaxRetries:       2,
			RetryInterval:    500 * time.Millisecond,
			RetryStatusCodes: []int{408, 429, 500, 502, 503, 504},
		})

	resp, body, err := httpReq.Do(ctx)
	if err != nil {
		c.log.Error("发送推送通知请求失败",
			zap.Strings("user_ids", req.UserIDs),
			zap.Error(err))
		return nil, fmt.Errorf("send push request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.log.Error("发送推送通知返回错误状态码",
			zap.Strings("user_ids", req.UserIDs),
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(body)))
		return nil, fmt.Errorf("send push returned status %d: %s", resp.StatusCode, string(body))
	}

	var response SendPushResponse
	if err := json.Unmarshal(body, &response); err != nil {
		c.log.Error("解析响应失败",
			zap.Strings("user_ids", req.UserIDs),
			zap.String("response", string(body)),
			zap.Error(err))
		return nil, fmt.Errorf("parse response failed: %w", err)
	}

	if response.Error != nil {
		c.log.Error("发送推送通知业务错误",
			zap.Strings("user_ids", req.UserIDs),
			zap.String("error_code", response.Error.Code),
			zap.String("error_message", response.Error.Message))
		return nil, fmt.Errorf("send push error: %s - %s", response.Error.Code, response.Error.Message)
	}

	if response.Data != nil {
		c.log.Info("推送通知发送完成",
			zap.Strings("user_ids", req.UserIDs),
			zap.Int("total_devices", response.Data.TotalDevices),
			zap.Int("sent", response.Data.Sent),
			zap.Int("failed", response.Data.Failed),
			zap.Int("skipped", response.Data.Skipped))
	}

	return response.Data, nil
}

// SendPushToUser 向单个用户发送推送（便捷方法）
func (c *Client) SendPushToUser(ctx context.Context, userID, title, body, bizID string) (*PushResultData, error) {
	return c.SendPush(ctx, &SendPushRequest{
		UserIDs: []string{userID},
		Title:   title,
		Body:    body,
		BizID:   bizID,
	})
}
