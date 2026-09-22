package utils

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPRequest_BasicRequest(t *testing.T) {
	// 创建一个测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求头
		if r.Header.Get("X-Test-Header") != "test-value" {
			t.Errorf("Expected header X-Test-Header=test-value, got %s", r.Header.Get("X-Test-Header"))
		}

		// 验证Content-Type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type=application/json, got %s", r.Header.Get("Content-Type"))
		}

		// 验证方法
		if r.Method != "POST" {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		// 读取并验证请求体
		var requestBody map[string]string
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&requestBody)
		if err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}
		defer r.Body.Close()

		if requestBody["key"] != "value" {
			t.Errorf("Expected body with key=value, got %s", requestBody["key"])
		}

		// 返回响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer server.Close()

	// 创建HTTP请求
	headers := map[string]string{
		"X-Test-Header": "test-value",
	}
	body := map[string]string{
		"key": "value",
	}

	// 执行请求
	// nolint:bodyclose
	resp, respBody, err := Post(context.Background(), server.URL, body, headers)

	// 验证结果
	if err != nil {
		t.Fatalf("Post request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}

	// 验证响应体
	var responseBody map[string]string
	err = json.Unmarshal(respBody, &responseBody)
	if err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}

	if responseBody["status"] != "success" {
		t.Errorf("Expected response status=success, got %s", responseBody["status"])
	}
}

func TestHTTPRequest_NoHeaders(t *testing.T) {
	// 创建一个测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证默认Content-Type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type=application/json, got %s", r.Header.Get("Content-Type"))
		}

		// 验证默认User-Agent
		if r.Header.Get("User-Agent") != "Go-HTTP-Client/1.0" {
			t.Errorf("Expected User-Agent=Go-HTTP-Client/1.0, got %s", r.Header.Get("User-Agent"))
		}

		// 返回响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer server.Close()

	// 执行没有自定义头部的请求
	// nolint:bodyclose
	resp, respBody, err := Get(context.Background(), server.URL)

	// 验证结果
	if err != nil {
		t.Fatalf("Get request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}

	// 验证响应体
	var responseBody map[string]string
	err = json.Unmarshal(respBody, &responseBody)
	if err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}

	if responseBody["status"] != "success" {
		t.Errorf("Expected response status=success, got %s", responseBody["status"])
	}
}

func TestHTTPRequest_Retry(t *testing.T) {
	attempts := 0
	maxAttempts := 3

	// 创建一个测试服务器，前两次请求返回500，第三次返回200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < maxAttempts {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success after retry"}`))
	}))
	defer server.Close()

	// 创建HTTP请求，设置快速重试
	options := HTTPClientOptions{
		Timeout:          15 * time.Second,
		MaxRetries:       3,
		RetryInterval:    100 * time.Millisecond,
		RetryStatusCodes: []int{500},
	}

	request := NewHTTPRequest("GET", server.URL).WithOptions(options)

	// 执行请求
	// nolint:bodyclose
	resp, respBody, err := request.Do(context.Background())

	// 验证结果
	if err != nil {
		t.Fatalf("Request with retry failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}

	// 验证尝试次数
	if attempts != maxAttempts {
		t.Errorf("Expected %d attempts, got %d", maxAttempts, attempts)
	}

	// 验证响应体
	var responseBody map[string]string
	err = json.Unmarshal(respBody, &responseBody)
	if err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}

	if responseBody["status"] != "success after retry" {
		t.Errorf("Expected response status='success after retry', got %s", responseBody["status"])
	}
}

func TestHTTPRequest_Timeout(t *testing.T) {
	// 创建一个会延迟响应的测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 延迟2秒
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 创建一个设置了短超时的HTTP请求
	options := HTTPClientOptions{
		Timeout:    500 * time.Millisecond, // 500毫秒超时
		MaxRetries: 0,
	}

	request := NewHTTPRequest("GET", server.URL).WithOptions(options)
	client := NewHTTPClient(options)
	request = request.WithClient(client)

	// 执行请求
	// nolint:bodyclose
	_, _, err := request.Do(context.Background())

	// 验证超时错误
	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}

	// 错误应该包含超时信息
	if !isTimeoutError(err) {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

// isTimeoutError 检查错误是否为超时错误
func isTimeoutError(err error) bool {
	if err, ok := err.(interface{ Timeout() bool }); ok && err.Timeout() {
		return true
	}
	return false
}

func TestHTTPRequest_MaxRetriesReached(t *testing.T) {
	// 创建一个总是返回错误的测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// 创建HTTP请求，设置重试
	options := HTTPClientOptions{
		Timeout:          15 * time.Second,
		MaxRetries:       2,
		RetryInterval:    100 * time.Millisecond,
		RetryStatusCodes: []int{500},
	}

	request := NewHTTPRequest("GET", server.URL).WithOptions(options)

	// 执行请求
	// nolint:bodyclose
	resp, _, err := request.Do(context.Background())

	// 验证结果
	if err != ErrMaxRetriesReached {
		t.Fatalf("Expected ErrMaxRetriesReached, got: %v", err)
	}

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %v", resp.Status)
	}
}

func TestHTTPRequest_ContextCancellation(t *testing.T) {
	// 创建一个会延迟响应的测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 延迟1秒
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 创建带有超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// 创建HTTP请求
	request := NewHTTPRequest("GET", server.URL)

	// 执行请求
	// nolint:bodyclose
	_, _, err := request.Do(ctx)

	// 验证上下文超时错误
	if err == nil {
		t.Fatal("Expected context timeout error, got nil")
	}

	// 使用字符串包含检查，而不是直接比较错误
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("Expected error containing 'context deadline exceeded', got: %v", err)
	}
}

func TestWithHeader(t *testing.T) {
	// 创建请求并设置单个头部
	req := NewHTTPRequest("GET", "https://example.com").
		WithHeader("X-Custom-Header", "custom-value")

	// 验证头部已设置
	if req.Headers["X-Custom-Header"] != "custom-value" {
		t.Errorf("Expected header X-Custom-Header=custom-value, got %s", req.Headers["X-Custom-Header"])
	}
}

func TestGetJSON(t *testing.T) {
	// 创建一个测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 返回响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"test","value":123}`))
	}))
	defer server.Close()

	// 准备结果结构
	var result struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	// 执行GetJSON请求
	err := GetJSON(context.Background(), server.URL, &result)

	// 验证结果
	if err != nil {
		t.Fatalf("GetJSON request failed: %v", err)
	}

	if result.Name != "test" || result.Value != 123 {
		t.Errorf("Expected name=test and value=123, got name=%s and value=%d", result.Name, result.Value)
	}
}

func TestPostJSON(t *testing.T) {
	// 创建一个测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证方法
		if r.Method != "POST" {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		// 读取请求体
		var requestBody map[string]interface{}
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&requestBody)
		if err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}
		defer r.Body.Close()

		// 验证请求体
		if requestBody["id"] != float64(1) || requestBody["name"] != "test" {
			t.Errorf("Unexpected request body: %v", requestBody)
		}

		// 返回响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","id":1}`))
	}))
	defer server.Close()

	// 准备请求和结果结构
	requestData := map[string]interface{}{
		"id":   1,
		"name": "test",
	}

	var result struct {
		Status string `json:"status"`
		ID     int    `json:"id"`
	}

	// 执行PostJSON请求
	err := PostJSON(context.Background(), server.URL, requestData, &result)

	// 验证结果
	if err != nil {
		t.Fatalf("PostJSON request failed: %v", err)
	}

	if result.Status != "success" || result.ID != 1 {
		t.Errorf("Expected status=success and id=1, got status=%s and id=%d", result.Status, result.ID)
	}
}
