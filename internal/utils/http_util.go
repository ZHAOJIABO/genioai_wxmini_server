// nolint:bodyclose
package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

var (
	// DefaultHTTPClient 默认的HTTP客户端
	DefaultHTTPClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	// ErrMaxRetriesReached 达到最大重试次数
	ErrMaxRetriesReached = errors.New("reached maximum number of retries")
	// ErrRequestFailed 请求失败
	ErrRequestFailed = errors.New("request failed")
)

// HTTPClientOptions 配置HTTP客户端的选项
type HTTPClientOptions struct {
	Timeout          time.Duration // 超时时间
	MaxRetries       int           // 最大重试次数
	RetryInterval    time.Duration // 重试间隔
	RetryStatusCodes []int         // 需要重试的状态码
}

// DefaultHTTPOptions 默认的HTTP选项
var DefaultHTTPOptions = HTTPClientOptions{
	Timeout:          30 * time.Second,
	MaxRetries:       3,
	RetryInterval:    1 * time.Second,
	RetryStatusCodes: []int{408, 429, 500, 502, 503, 504},
}

// CommonHeaders 常用的HTTP头部
var CommonHeaders = map[string]string{
	"Content-Type": "application/json",
	"User-Agent":   "Go-HTTP-Client/1.0",
	"Accept":       "application/json",
}

// NewHTTPClient 创建一个新的HTTP客户端
func NewHTTPClient(options HTTPClientOptions) *http.Client {
	return &http.Client{
		Timeout: options.Timeout,
	}
}

// HTTPRequest 表示一个HTTP请求
type HTTPRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    interface{}
	Options HTTPClientOptions
	Client  *http.Client
}

// NewHTTPRequest 创建一个新的HTTP请求
func NewHTTPRequest(method, url string) *HTTPRequest {
	// 默认使用常用头部
	headers := make(map[string]string)
	for k, v := range CommonHeaders {
		headers[k] = v
	}

	return &HTTPRequest{
		Method:  method,
		URL:     url,
		Headers: headers,
		Options: DefaultHTTPOptions,
		Client:  DefaultHTTPClient,
	}
}

// WithHeaders 设置请求头
func (r *HTTPRequest) WithHeaders(headers map[string]string) *HTTPRequest {
	for k, v := range headers {
		r.Headers[k] = v
	}
	return r
}

// WithHeader 设置单个请求头
func (r *HTTPRequest) WithHeader(key, value string) *HTTPRequest {
	r.Headers[key] = value
	return r
}

// WithBody 设置请求体
func (r *HTTPRequest) WithBody(body interface{}) *HTTPRequest {
	r.Body = body
	return r
}

// WithOptions 设置请求选项
func (r *HTTPRequest) WithOptions(options HTTPClientOptions) *HTTPRequest {
	r.Options = options
	return r
}

// WithClient 设置HTTP客户端
func (r *HTTPRequest) WithClient(client *http.Client) *HTTPRequest {
	r.Client = client
	return r
}

// shouldRetry 判断是否应该重试请求
func shouldRetry(statusCode int, retryStatusCodes []int) bool {
	for _, code := range retryStatusCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

// checkTimeoutError 内部函数：检查错误是否为超时错误
func checkTimeoutError(err error) bool {
	if err, ok := err.(interface{ Timeout() bool }); ok && err.Timeout() {
		return true
	}
	return false
}

// Do 执行HTTP请求并处理重试逻辑
func (r *HTTPRequest) Do(ctx context.Context) (*http.Response, []byte, error) {
	var requestBody io.Reader
	var err error

	// 准备请求体
	if r.Body != nil {
		switch body := r.Body.(type) {
		case string:
			requestBody = bytes.NewBufferString(body)
		case []byte:
			requestBody = bytes.NewBuffer(body)
		default:
			jsonBody, err := json.Marshal(body)
			if err != nil {
				return nil, nil, err
			}
			requestBody = bytes.NewBuffer(jsonBody)
		}
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, r.Method, r.URL, requestBody)
	if err != nil {
		return nil, nil, err
	}

	// 设置请求头
	for key, value := range r.Headers {
		req.Header.Set(key, value)
	}

	// 执行带重试的请求
	var response *http.Response
	var responseBody []byte
	var lastErr error

	for attempt := 0; attempt <= r.Options.MaxRetries; attempt++ {
		// 如果不是第一次尝试，等待重试间隔
		if attempt > 0 {
			select {
			case <-time.After(r.Options.RetryInterval):
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}

		response, err = r.Client.Do(req)
		if err != nil {
			// 保存最后的错误
			lastErr = err
			// 如果是超时错误或上下文取消，不继续重试
			if checkTimeoutError(err) || errors.Is(err, context.DeadlineExceeded) {
				return nil, nil, err
			}
			// 连接错误，继续重试
			continue
		}

		// 读取响应体
		responseBody, err = io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			// 读取错误，继续重试
			lastErr = err
			continue
		}

		// 检查状态码，决定是否重试
		if !shouldRetry(response.StatusCode, r.Options.RetryStatusCodes) {
			return response, responseBody, nil
		}

		// 需要重试，准备下一次请求
		if requestBody != nil {
			// 重置请求体
			switch body := r.Body.(type) {
			case string:
				requestBody = bytes.NewBufferString(body)
			case []byte:
				requestBody = bytes.NewBuffer(body)
			default:
				jsonBody, err := json.Marshal(body)
				if err != nil {
					return nil, nil, err
				}
				requestBody = bytes.NewBuffer(jsonBody)
			}
			req, err = http.NewRequestWithContext(ctx, r.Method, r.URL, requestBody)
			if err != nil {
				return nil, nil, err
			}
			// 重新设置请求头
			for key, value := range r.Headers {
				req.Header.Set(key, value)
			}
		}
	}

	// 达到最大重试次数
	if response != nil {
		return response, responseBody, ErrMaxRetriesReached
	}

	// 如果有具体错误，返回该错误
	if lastErr != nil {
		return nil, nil, lastErr
	}

	return nil, nil, ErrRequestFailed
}

// Get 发送GET请求
func Get(ctx context.Context, url string, headers ...map[string]string) (*http.Response, []byte, error) {
	req := NewHTTPRequest("GET", url)
	if len(headers) > 0 {
		req.WithHeaders(headers[0])
	}
	return req.Do(ctx)
}

// Post 发送POST请求
func Post(ctx context.Context, url string, body interface{}, headers ...map[string]string) (*http.Response, []byte, error) {
	req := NewHTTPRequest("POST", url).WithBody(body)
	if len(headers) > 0 {
		req.WithHeaders(headers[0])
	}
	return req.Do(ctx)
}

// Put 发送PUT请求
func Put(ctx context.Context, url string, body interface{}, headers ...map[string]string) (*http.Response, []byte, error) {
	req := NewHTTPRequest("PUT", url).WithBody(body)
	if len(headers) > 0 {
		req.WithHeaders(headers[0])
	}
	return req.Do(ctx)
}

// Delete 发送DELETE请求
func Delete(ctx context.Context, url string, headers ...map[string]string) (*http.Response, []byte, error) {
	req := NewHTTPRequest("DELETE", url)
	if len(headers) > 0 {
		req.WithHeaders(headers[0])
	}
	return req.Do(ctx)
}

// Patch 发送PATCH请求
func Patch(ctx context.Context, url string, body interface{}, headers ...map[string]string) (*http.Response, []byte, error) {
	req := NewHTTPRequest("PATCH", url).WithBody(body)
	if len(headers) > 0 {
		req.WithHeaders(headers[0])
	}
	return req.Do(ctx)
}

// GetJSON 发送GET请求并将响应解析为JSON
func GetJSON(ctx context.Context, url string, result interface{}, headers ...map[string]string) error {
	_, body, err := Get(ctx, url, headers...)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, result)
}

// PostJSON 发送POST请求并将响应解析为JSON
func PostJSON(ctx context.Context, url string, requestBody interface{}, result interface{}, headers ...map[string]string) error {
	_, body, err := Post(ctx, url, requestBody, headers...)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, result)
}

// PutJSON 发送PUT请求并将响应解析为JSON
func PutJSON(ctx context.Context, url string, requestBody interface{}, result interface{}, headers ...map[string]string) error {
	_, body, err := Put(ctx, url, requestBody, headers...)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, result)
}

// DeleteJSON 发送DELETE请求并将响应解析为JSON
func DeleteJSON(ctx context.Context, url string, result interface{}, headers ...map[string]string) error {
	_, body, err := Delete(ctx, url, headers...)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, result)
}

// PatchJSON 发送PATCH请求并将响应解析为JSON
func PatchJSON(ctx context.Context, url string, requestBody interface{}, result interface{}, headers ...map[string]string) error {
	_, body, err := Patch(ctx, url, requestBody, headers...)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, result)
}

// BuildMultipartForm 构建multipart表单数据
func BuildMultipartForm(fileData []byte, fileName string, extraFields map[string]string) ([]byte, string, error) {
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)

	// 添加文件字段
	part, err := writer.CreateFormFile("image", fileName)
	if err != nil {
		return nil, "", errors.New("创建文件字段失败: " + err.Error())
	}
	if _, err := part.Write(fileData); err != nil {
		return nil, "", errors.New("写入文件内容失败: " + err.Error())
	}

	// 添加额外字段
	for key, value := range extraFields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", errors.New("写入字段失败: " + err.Error())
		}
	}

	contentType := writer.FormDataContentType()
	if err := writer.Close(); err != nil {
		return nil, "", errors.New("关闭writer失败: " + err.Error())
	}

	return buf.Bytes(), contentType, nil
}
