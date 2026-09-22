package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// UnixSocketClient Unix套接字客户端
type UnixSocketClient struct {
	socketPath string
	timeout    time.Duration
}

// NewUnixSocketClient 创建一个新的Unix套接字客户端
func NewUnixSocketClient(socketPath string, timeout time.Duration) *UnixSocketClient {
	if timeout == 0 {
		timeout = 30 * time.Second // 默认30秒超时
	}
	return &UnixSocketClient{
		socketPath: socketPath,
		timeout:    timeout,
	}
}

// Request 发送请求并接收响应
func (c *UnixSocketClient) Request(ctx context.Context, path string, method string, requestData interface{}, responseData interface{}) error {
	// 创建一个基于Unix套接字的传输
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: c.timeout}
			return dialer.DialContext(ctx, "unix", c.socketPath)
		},
	}

	// 创建HTTP客户端
	client := &http.Client{
		Transport: transport,
		Timeout:   c.timeout,
	}

	// 准备请求数据
	var reqBody io.Reader
	if requestData != nil {
		jsonData, err := json.Marshal(requestData)
		if err != nil {
			return fmt.Errorf("序列化请求数据失败: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	// 构建URL
	reqURL := &url.URL{
		Scheme: "http",
		Host:   "localhost", // 这个实际上不会被使用，只是为了格式
		Path:   path,
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), reqBody)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// URL强制重写为Unix域套接字格式
	req.URL.Scheme = "http"
	req.URL.Host = "unix"
	req.URL.Path = path

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("请求失败，状态码: %d, 响应内容: %s", resp.StatusCode, string(body))
	}

	// 解析响应数据
	if responseData != nil {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("读取响应内容失败: %w", err)
		}

		err = json.Unmarshal(respBody, responseData)
		if err != nil {
			return fmt.Errorf("解析响应数据失败: %w", err)
		}
	}

	return nil
}

// Get 发送GET请求
func (c *UnixSocketClient) Get(ctx context.Context, path string, responseData interface{}) error {
	return c.Request(ctx, path, http.MethodGet, nil, responseData)
}

// Post 发送POST请求
func (c *UnixSocketClient) Post(ctx context.Context, path string, requestData interface{}, responseData interface{}) error {
	return c.Request(ctx, path, http.MethodPost, requestData, responseData)
}

// Put 发送PUT请求
func (c *UnixSocketClient) Put(ctx context.Context, path string, requestData interface{}, responseData interface{}) error {
	return c.Request(ctx, path, http.MethodPut, requestData, responseData)
}

// Delete 发送DELETE请求
func (c *UnixSocketClient) Delete(ctx context.Context, path string, responseData interface{}) error {
	return c.Request(ctx, path, http.MethodDelete, nil, responseData)
}
