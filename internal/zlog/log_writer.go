package zlog

import (
	"bufio"
	"log"
	"os"
	"sync"
	"time"

	"va_visionai_server/conf"
)

type AccessLogWriter struct {
	currentFd       *os.File
	writer          *bufio.Writer
	currentFileName string
	mu              sync.RWMutex
}

var (
	logWriter     = &AccessLogWriter{}
	timerOfWriter = time.NewTicker(time.Minute) // 定期刷新间隔调整为一分钟
)

func GetLogWriter() *AccessLogWriter {
	return logWriter
}

// Flush 方法用于刷新AccessLogWriter中的日志数据到目标存储介质。
// 它确保所有之前写入的日志消息都被立即写入并同步到存储介质，例如磁盘。
// 这对于确保日志的实时性和不丢失任何日志条目非常重要。
// 特别是在应用程序退出或发生错误时，调用Flush可以确保所有日志数据都被正确地记录下来。
func (w *AccessLogWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writer != nil && w.currentFd != nil {
		err := w.writer.Flush()
		if err != nil {
			log.Printf("failed to flush data, %v", err)
		}
		err = w.currentFd.Close()
		if err != nil {
			log.Printf("failed to close file, %v", err)
		}
	}
}

func (w *AccessLogWriter) Write(logInfo string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.checkWriter()
	_, err := w.writer.Write(append([]byte(logInfo), '\n'))
	if err != nil {
		log.Fatalf("failed to write data, %v", err)
	}
	// 定期刷新缓存，避免日志长时间无法落盘
	select {
	case <-timerOfWriter.C:
		err := w.writer.Flush()
		if err != nil {
			log.Printf("failed to flush data, %v", err)
		}
	default:
	}
}

func (w *AccessLogWriter) checkWriter() {
	current := w.getLogName()
	if w.currentFileName == "" || w.currentFileName != current {
		if w.currentFd != nil {
			w.currentFd.Close()
		}
		var err error
		w.currentFd, err = os.OpenFile(current, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o664)
		if err != nil {
			log.Fatalf("failed to open access log file, %v", err)
		}
		w.writer = bufio.NewWriter(w.currentFd)
		w.currentFileName = current
	}
}

func (w *AccessLogWriter) getLogName() string {
	return conf.GlobalConfig.LogConfig.LogPath + "." + time.Now().Format("2006010215")
}

func init() {
	go func() {
		for range timerOfWriter.C {
			GetLogWriter().Flush()
		}
	}()
}
