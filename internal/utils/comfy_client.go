package utils

import (
	"context"
	"fmt"
	"image"
	"sync"
	"time"

	"github.com/dundunHa/comfy2go/client"
	"github.com/dundunHa/comfy2go/graphapi"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/zlog"
)

// ComfyUIClient ComfyUI客户端封装
type ComfyUIClient struct {
	client     *client.ComfyClient
	host       string
	port       int
	timeout    time.Duration
	retries    int
	mu         sync.Mutex
	callbacks  *client.ComfyClientCallbacks
	isRunning  bool
	errorCount int
}

// ClientConfig ComfyUI客户端配置
type ClientConfig struct {
	Host    string
	Port    int
	Timeout time.Duration
	Retries int
}

// NewComfyUIClient 创建新的ComfyUI客户端
func NewComfyUIClient(config ClientConfig) *ComfyUIClient {
	callbacks := &client.ComfyClientCallbacks{
		ClientQueueCountChanged: func(c *client.ComfyClient, queuecount int) {
			zlog.Logger.Info("Queue count changed",
				zap.String("clientID", c.ClientID()),
				zap.Int("queueCount", queuecount))
		},
		// ClientConnected: func(c *client.ComfyClient) {
		// 	zlog.Logger.Info("Client connected", zap.String("clientID", c.ClientID()))
		// },
		// ClientDisconnected: func(c *client.ComfyClient) {
		// 	zlog.Logger.Info("Client disconnected", zap.String("clientID", c.ClientID()))
		// },
	}

	return &ComfyUIClient{
		host:      config.Host,
		port:      config.Port,
		timeout:   config.Timeout,
		retries:   config.Retries,
		callbacks: callbacks,
	}
}

// ensureClient 确保客户端连接可用
func (c *ComfyUIClient) ensureClient() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果客户端已存在且已初始化，直接返回
	if c.client != nil && c.client.IsInitialized() && c.isRunning {
		return nil
	}

	if err := c.client.CheckConnection(); err != nil {
		// 创建新客户端
		c.client = client.NewComfyClientWithTimeout(c.host, c.port, c.callbacks, int(c.timeout), c.retries)
		// 初始化客户端
		if err := c.client.Init(); err != nil {
			c.client = nil
			return errors.Wrap(err, "failed to initialize client")
		}
		c.isRunning = true
		c.errorCount = 0
	}

	return nil
}

// LoadWorkflow 加载工作流
func (c *ComfyUIClient) LoadWorkflow(workflowPath string) (*graphapi.Graph, error) {
	if err := c.ensureClient(); err != nil {
		return nil, err
	}

	graph, _, err := c.client.NewGraphFromJsonFile(workflowPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load workflow")
	}

	return graph, nil
}

// UploadImage 上传图片
func (c *ComfyUIClient) UploadImage(ctx context.Context, img image.Image, filename string) error {
	if err := c.ensureClient(); err != nil {
		return err
	}

	_, err := c.client.UploadImage(img, filename, false, client.InputImageType, "", nil)
	if err != nil {
		c.errorCount++
		if c.errorCount >= 3 {
			// 重置客户端
			c.client = nil
			c.isRunning = false
		}
		return errors.Wrap(err, "failed to upload image")
	}

	return nil
}

// ExecuteWorkflow 执行工作流
func (c *ComfyUIClient) ExecuteWorkflow(ctx context.Context, graph *graphapi.Graph) ([]byte, error) {
	if err := c.ensureClient(); err != nil {
		return nil, err
	}

	// 提交工作流
	item, err := c.client.QueuePrompt(graph)
	if err != nil {
		return nil, errors.Wrap(err, "failed to queue prompt")
	}

	var resultImage []byte
	timeout := time.After(5 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-timeout:
			return nil, errors.New("workflow execution timed out")

		case msg, ok := <-item.Messages:
			if !ok {
				return nil, errors.New("message channel closed unexpectedly")
			}

			switch msg.Type {
			case "executing":
				qm := msg.ToPromptMessageExecuting()
				zlog.Logger.Debug("Executing node",
					zap.Int("nodeID", qm.NodeID),
					zap.String("title", qm.Title))

			case "progress":
				qm := msg.ToPromptMessageProgress()
				zlog.Logger.Debug("Progress",
					zap.Int("value", qm.Value),
					zap.Int("max", qm.Max))

			case "stopped":
				qm := msg.ToPromptMessageStopped()
				if qm.Exception != nil {
					return nil, fmt.Errorf("workflow stopped with exception: %v", qm.Exception)
				}

			case "data":
				qm := msg.ToPromptMessageData()
				for k, v := range qm.Data {
					if k == "images" {
						for _, output := range v {
							imgData, err := c.client.GetImage(output)
							if err != nil {
								continue
							}
							resultImage = *imgData
							return resultImage, nil
						}
					}
				}
			}
		}
	}
}

// Close 关闭客户端
func (c *ComfyUIClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		c.client = nil
		c.isRunning = false
	}
}
