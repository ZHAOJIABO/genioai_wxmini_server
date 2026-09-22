package task

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/zlog"
)

type ChatTaskProcessor struct {
	DataMap map[string]*time.Time
	Mux     *sync.Mutex
	chatDao *dao.ChatDao
}

var Chat ChatTaskProcessor

func (t *ChatTaskProcessor) Listen() {
	if t.chatDao == nil {
		t.chatDao = dao.NewChatDao(db.GetDB())
	}
	Chat.DataMap = make(map[string]*time.Time, 100)
	Chat.Mux = &sync.Mutex{}
	tk := time.NewTicker(15 * time.Second)
	go func() {
		for range tk.C {
			lockKey := constants.RedisKeyUpdateChatLastMsgTimeLock
			lockTimeout := 1 * time.Minute
			rdb := db.GetRedis()
			ok, err := rdb.SetNX(lockKey, time.Now().String(), lockTimeout).Result()
			if err != nil {
				zlog.Logger.Error("Failed to acquire lock", zap.Error(err))
				continue
			}
			if !ok {
				continue
			}
			t.Process()
			_, err = rdb.Del(lockKey).Result()
			if err != nil {
				zlog.Logger.Error("Failed to release lock", zap.Error(err))
			}
		}
	}()
}

func (t *ChatTaskProcessor) Process() {
	t.Mux.Lock()
	defer t.Mux.Unlock()
	for k, v := range t.DataMap {
		zlog.Logger.Debug("Task:Update Chat LastMsgTime")
		if err := t.chatDao.UpdateChatLastTime(k, v); err != nil {
			zlog.Logger.Error("Update Chat LastMsgTime Error", zap.Error(err), zap.String("ChatID", k))
		}
	}
	clear(t.DataMap)
}

func (t *ChatTaskProcessor) Submit(chatID string, lastMsgTime *time.Time) {
	t.Mux.Lock()
	defer t.Mux.Unlock()
	t.DataMap[chatID] = lastMsgTime
}

// Stop 停止聊天任务处理
func (t *ChatTaskProcessor) Stop() {
	t.Mux.Lock()
	defer t.Mux.Unlock()

	// 处理剩余的聊天任务
	t.Process()

	zlog.Logger.Info("聊天任务处理器已停止")
}
