package aigc_core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"va_visionai_server/conf"
	"va_visionai_server/internal/zlog"
	v1 "va_visionai_server/internal/aibrain/v1"
)

type AIGCConfig struct {
	Addr string
}

type Client struct {
	conn       *grpc.ClientConn
	grpcClient v1.AIBrainServiceClient
	config     AIGCConfig
}

var (
	defaultClient *Client
	once          sync.Once
)

func NewClient(ctx context.Context, config AIGCConfig) (*Client, error) {
	conn, err := grpc.Dial(config.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		zlog.LogWithContext(ctx).Error("创建AIGC Core gRPC连接失败",
			zap.String("addr", config.Addr),
			zap.Error(err))
		return nil, err
	}

	grpcClient := v1.NewAIBrainServiceClient(conn)

	zlog.LogWithContext(ctx).Info("AIGC Core客户端初始化成功",
		zap.String("addr", config.Addr))

	return &Client{
		conn:       conn,
		grpcClient: grpcClient,
		config:     config,
	}, nil
}

func Default(ctx context.Context) (*Client, error) {
	var err error
	once.Do(func() {
		config := AIGCConfig{
			Addr: conf.GlobalConfig.AIBrainServer.Addr,
		}
		defaultClient, err = NewClient(ctx, config)
	})
	return defaultClient, err
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) SubmitAsyncTask(ctx context.Context, req *v1.SubmitAsyncTaskRequest) (*v1.SubmitAsyncTaskResponse, error) {
	zlog.LogWithContext(ctx).Debug("提交异步任务",
		zap.String("task_type", req.GetTaskType()))

	resp, err := c.grpcClient.SubmitAsyncTask(ctx, req)
	if err != nil {
		zlog.LogWithContext(ctx).Error("提交异步任务失败",
			zap.String("task_type", req.GetTaskType()),
			zap.Error(err))
		return nil, err
	}

	zlog.LogWithContext(ctx).Info("异步任务提交成功",
		zap.String("task_id", resp.GetTaskId()))

	return resp, nil
}

func (c *Client) GetAsyncTask(ctx context.Context, req *v1.GetAsyncTaskRequest) (*v1.GetAsyncTaskResponse, error) {
	zlog.LogWithContext(ctx).Debug("获取异步任务状态",
		zap.String("task_id", req.GetTaskId()))

	resp, err := c.grpcClient.GetAsyncTask(ctx, req)
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取异步任务状态失败",
			zap.String("task_id", req.GetTaskId()),
			zap.Error(err))
		return nil, err
	}

	return resp, nil
}

func (c *Client) HealthCheck(ctx context.Context, req *v1.HealthCheckRequest) (*v1.HealthCheckResponse, error) {
	zlog.LogWithContext(ctx).Debug("执行健康检查")

	resp, err := c.grpcClient.HealthCheck(ctx, req)
	if err != nil {
		zlog.LogWithContext(ctx).Error("健康检查失败", zap.Error(err))
		return nil, err
	}

	return resp, nil
}

func (c *Client) UploadComfyUIInputFile(ctx context.Context, req *v1.UploadComfyUIInputFileRequest) (*v1.UploadComfyUIInputFileResponse, error) {
	zlog.LogWithContext(ctx).Debug("上传ComfyUI输入文件",
		zap.String("file_name", req.GetFileName()),
		zap.String("url", req.GetUrl()))

	resp, err := c.grpcClient.UploadComfyUIInputFile(ctx, req)
	if err != nil {
		zlog.LogWithContext(ctx).Error("上传ComfyUI输入文件失败",
			zap.String("file_name", req.GetFileName()),
			zap.String("url", req.GetUrl()),
			zap.Error(err))
		return nil, err
	}

	zlog.LogWithContext(ctx).Info("ComfyUI输入文件上传成功",
		zap.String("file_name", resp.GetFileName()),
		zap.String("file_url", resp.GetFileUrl()))

	return resp, nil
}

func (c *Client) DownloadVideo(ctx context.Context, videoURL string) ([]byte, error) {
	zlog.LogWithContext(ctx).Info("开始下载 AIGC Core 视频", zap.String("url", videoURL))

	req, err := http.NewRequestWithContext(context.Background(), "GET", videoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建下载请求失败: %w", err)
	}

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载视频失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载视频返回错误状态码: %d", resp.StatusCode)
	}

	videoData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取视频数据失败: %w", err)
	}

	zlog.LogWithContext(ctx).Info("AIGC Core 视频下载完成",
		zap.String("url", videoURL),
		zap.Int("size", len(videoData)))

	return videoData, nil
}

func (c *Client) ChatStream(ctx context.Context, req *v1.ChatStreamRequest) (v1.AIBrainService_ChatStreamClient, error) {
	zlog.LogWithContext(ctx).Debug("创建聊天流",
		zap.String("model_name", req.GetModelName()))

	stream, err := c.grpcClient.ChatStream(ctx, req)
	if err != nil {
		zlog.LogWithContext(ctx).Error("创建聊天流失败",
			zap.String("model_name", req.GetModelName()),
			zap.Error(err))
		return nil, err
	}

	zlog.LogWithContext(ctx).Info("聊天流创建成功",
		zap.String("model_name", req.GetModelName()))

	return stream, nil
}
