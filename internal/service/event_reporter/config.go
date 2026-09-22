package event_reporter

import (
	"os"
	"time"
)

// Config EventReporter 配置
type Config struct {
	// Addr gRPC 服务地址（必填）
	Addr string

	// QueueSize 队列容量（默认 10000）
	QueueSize int

	// BatchSize 批量大小阈值（默认 100）
	BatchSize int

	// FlushInterval 刷新间隔（默认 5s）
	FlushInterval time.Duration

	// ShutdownTimeout 关闭超时（默认 10s）
	ShutdownTimeout time.Duration

	// Source 事件来源标识（默认 "va_visionai_server"）
	Source string

	// ServerNode 服务节点标识（用于 ServerContext，默认取 hostname）
	ServerNode string

	// Enabled 是否启用（默认 true，设为 false 时 Track 为空操作）
	Enabled bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	hostname, _ := os.Hostname()
	return Config{
		QueueSize:       10000,
		BatchSize:       100,
		FlushInterval:   5 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		Source:          "va_visionai_server",
		ServerNode:      hostname,
		Enabled:         true,
	}
}

// WithDefaults 将配置与默认值合并
func (c Config) WithDefaults() Config {
	defaults := DefaultConfig()

	if c.QueueSize <= 0 {
		c.QueueSize = defaults.QueueSize
	}
	if c.BatchSize <= 0 {
		c.BatchSize = defaults.BatchSize
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = defaults.FlushInterval
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = defaults.ShutdownTimeout
	}
	if c.Source == "" {
		c.Source = defaults.Source
	}
	if c.ServerNode == "" {
		c.ServerNode = defaults.ServerNode
	}

	return c
}
