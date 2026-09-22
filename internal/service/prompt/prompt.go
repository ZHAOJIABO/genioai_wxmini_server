package prompt

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type HeatTracker struct {
	ttl        time.Duration
	buffer     chan string
	bufferSize int
	staticData map[string]bool // 用于标记静态数据
	mu         sync.Mutex
}

type PromptService struct {
	rdb         *redis.Client
	heatTracker *HeatTracker
	db          *gorm.DB
}

func NewPromptService(rdb *redis.Client, tracker *HeatTracker, db *gorm.DB) *PromptService {
	return &PromptService{
		rdb:         rdb,
		heatTracker: tracker,
		db:          db,
	}
}

var Prompt PromptService

// ListPrompt 获取 Prompt 的所有历史记录
func (s *PromptService) ListPrompt(kind, language string) ([]*model.Prompt, error) {
	pdao := dao.NewPromptDao(s.db)
	return pdao.ListPrompt(kind, language)
}

func (s *PromptService) GetPromptByPromptID(promptID, language string) (*model.Prompt, error) {
	pdao := dao.NewPromptDao(s.db)
	prompt, err := pdao.GetPrompt(promptID, language)
	if err != nil {
		return nil, err
	}
	return prompt, nil
}

func (s *PromptService) ListPromptResp(ctx context.Context, kind, language string) []*vai.Prompt {
	data, err := s.ListPrompt(kind, language)
	rsp := make([]*vai.Prompt, 0, len(data))
	if err != nil {
		zlog.Logger.Error("Fetch prompt list failed", zap.Error(err))
		return rsp
	}

	quickPromptIDs, err := s.GetTopN(context.TODO(), 20)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Fetch prompt list failed", zap.Error(err))

	}
	pdao := dao.NewPromptDao(s.db)
	var quickPrompts []*model.Prompt
	if len(quickPromptIDs) > 0 {
		quickPrompts, err = pdao.ListQuickPrompt(quickPromptIDs, language)
		if err != nil {
			zlog.Logger.Error("Fetch Hot prompt list failed", zap.Error(err))
		}
		for _, prompt := range quickPrompts {
			prompt.Kind = "quick"
		}
	}
	quickMap := make(map[string]struct{})
	if len(quickPrompts) < 5 {
		for _, d := range quickPromptIDs {
			quickMap[d] = struct{}{}
		}
	}

	for _, v := range data {
		if len(quickPrompts) >= 5 {
			break
		}
		if v.Kind == "Popular" {
			continue
		}
		if _, ok := quickMap[v.PromptID]; !ok {
			quickMap[v.PromptID] = struct{}{}
			cp := v.DeepCopy()
			cp.Kind = "quick"
			quickPrompts = append(quickPrompts, cp)
		}
	}
	data = append(data, quickPrompts...)
	for _, v := range data {
		var photoQuestion []string
		if err := json.Unmarshal([]byte(v.PhotoQuestion), &photoQuestion); err != nil {
			zlog.LogWithContext(ctx).Error("Unmarshal PhotoQuestion Error", zap.Error(err), zap.String(constants.CtxPromptID, v.PromptID))
		}
		kindID := v.Kind
		rsp = append(rsp, &vai.Prompt{
			PromptId:      v.PromptID,
			Title:         v.Title,
			Description:   v.Describe,
			LeadTitle:     v.LeadTitle,
			Lead:          v.Lead,
			Icon:          v.Icon,
			KindId:        kindID,
			PhotoQuestion: photoQuestion,
			EnableCamera:  v.EnableCamera,
		})
	}
	return rsp
}

// 创建热度追踪器实例
func NewHeatTracker() *HeatTracker {
	return &HeatTracker{
		ttl:        72 * time.Hour,
		buffer:     make(chan string, 100),
		bufferSize: 100,
		staticData: make(map[string]bool),
	}
}

// 启动热度追踪任务
func (s *PromptService) Start(ctx context.Context) {
	go func() {
		for dataID := range s.heatTracker.buffer {
			s.processData(ctx, dataID)
		}
	}()

	ticker := time.NewTicker(time.Hour)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.cleanOldRecords(ctx)
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

func (s *PromptService) processData(ctx context.Context, promptID string) {
	now := time.Now().Unix()
	lockKey := "lock:processData:" + promptID
	lockAcquired, err := s.rdb.SetNX(lockKey, 1, time.Second*5).Result()
	if err != nil || !lockAcquired {
		zlog.LogWithContext(ctx).Error("Failed to acquire lock for processData", zap.String(constants.CtxPromptID, promptID), zap.Any(constants.ServiceEvent, constants.EventHeatTracker))
		return
	}
	defer s.rdb.Del(lockKey)

	s.rdb.ZIncrBy("heat_counts", 1, promptID)
	s.rdb.ZAdd("time_window:"+promptID, redis.Z{Score: float64(now), Member: now})
	s.rdb.Expire("time_window:"+promptID, s.heatTracker.ttl)
	s.rdb.HDel("static_data", promptID)

	zlog.LogWithContext(ctx).Info("Processed prompt data", zap.String(constants.CtxPromptID, promptID), zap.Any(constants.ServiceEvent, constants.EventHeatTracker))
}

func (h *HeatTracker) Track(ctx context.Context, dataID string) {
	select {
	case h.buffer <- dataID:
	default:
		zlog.LogWithContext(ctx).Error("Buffer full, dropping data", zap.String("data", dataID))
	}
}

func (s *PromptService) cleanOldRecords(ctx context.Context) {
	threshold := time.Now().Add(-s.heatTracker.ttl).Unix()

	lockKey := "lock:cleanOldRecords"
	lockAcquired, err := s.rdb.SetNX(lockKey, 1, time.Minute).Result()
	if err != nil || !lockAcquired {
		//zlog.LogWithContext(ctx).Warn("Failed to acquire lock for cleanOldRecords")
		return
	}
	defer s.rdb.Del(lockKey)

	dataIDs, _ := s.rdb.ZRange("heat_counts", 0, -1).Result()
	for _, dataID := range dataIDs {
		s.rdb.ZRemRangeByScore("time_window:"+dataID, "0", strconv.FormatInt(threshold, 10))

		newHeat, _ := s.rdb.ZCard("time_window:" + dataID).Result()
		if newHeat > 0 {
			s.rdb.ZAdd("heat_counts", redis.Z{Score: float64(newHeat), Member: dataID})
		} else {
			s.rdb.HSet("static_data", dataID, 1)
		}
	}
}

func (s *PromptService) GetTopN(ctx context.Context, n int64) ([]string, error) {
	res, err := s.rdb.ZRevRangeWithScores("heat_counts", 0, n-1).Result()
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to fetch top N prompts", zap.Error(err))
		return nil, err
	}
	promptIDs := make([]string, 0, len(res))
	for _, item := range res {
		if v, ok := item.Member.(string); ok {
			promptIDs = append(promptIDs, v)
		}
	}
	return promptIDs, nil
}

func (s *PromptService) BuildReqSystemPrompt(ctx context.Context, req *vai.ChatMessageSendRequest) error {
	if req.GetMessage().GetSystemPrompt() != "" {
		return nil
	}

	promptID := req.GetMessage().GetPromptId()
	if req.GetMessage().GetMessageType() == vai.MessageType_MT_VIDEO {
		req.GetMessage().PromptId = constants.SystemPromptVideo
		promptID = constants.SystemPromptVideo
	}

	lang := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	var promptInfo *model.Prompt
	var err error
	if promptID != "" {
		promptInfo, err = s.GetPromptByPromptID(promptID, lang)
		if err != nil {
			zlog.LogWithContext(ctx).Error("Get Prompt By PromptID Error", zap.Error(err), zap.String(constants.CtxPromptID, promptID))
		}
	}

	basePromptInfo, err := s.GetPromptByPromptID(constants.SystemPromptBase, "zh")
	if err != nil {
		zlog.LogWithContext(ctx).Error("Get Prompt By PromptID Error", zap.Error(err), zap.String(constants.CtxPromptID, constants.SystemPromptBase))
		return constants.ERR_INVALID_PARAM
	}

	promptContent := strings.ReplaceAll(basePromptInfo.Content, "{visionai-replace-lang}", lang)
	if promptInfo != nil && promptInfo.Content != "" {
		promptContent = strings.ReplaceAll(promptContent, "{visionai-replace-role}", promptInfo.Content)
	} else {
		promptContent = strings.ReplaceAll(promptContent, "{visionai-replace-role}", "")
	}

	req.GetMessage().SystemPrompt = promptContent
	return nil
}

// ListCameraPrompt 获取指定场景下的相机场景 Prompt
func (s *PromptService) ListCameraPrompt(ctx context.Context, kindID, language string) ([]*vai.Prompt, error) {
	pdao := dao.NewPromptDao(s.db)
	data, err := pdao.ListCameraPrompt(kindID, language)
	if err != nil {
		zlog.LogWithContext(ctx).Error("List Camera Prompt Error", zap.Error(err))
		return nil, err
	}

	rsp := make([]*vai.Prompt, 0, len(data))
	for _, v := range data {
		var photoQuestion []string
		if err := json.Unmarshal([]byte(v.PhotoQuestion), &photoQuestion); err != nil {
			zlog.LogWithContext(ctx).Error("Unmarshal PhotoQuestion Error", zap.Error(err), zap.String(constants.CtxPromptID, v.PromptID))
		}
		rsp = append(rsp, &vai.Prompt{
			PromptId:      v.PromptID,
			Title:         v.Title,
			Description:   v.Describe,
			LeadTitle:     v.LeadTitle,
			Lead:          v.Lead,
			Icon:          v.Icon,
			KindId:        v.Kind,
			PhotoQuestion: photoQuestion,
			EnableCamera:  v.EnableCamera,
			CameraShow:    v.CameraShow,
		})
	}
	return rsp, nil
}
