package biz

import "errors"

var (
	// ErrRemoteClose 远程连接关闭错误
	ErrRemoteClose = errors.New("remote close conn")
)
