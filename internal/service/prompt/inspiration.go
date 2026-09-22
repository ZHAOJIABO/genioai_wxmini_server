package prompt

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/zlog"
)

// InspirationService 灵感提示词服务
type InspirationService struct {
	repo InspirationRepository
}

// NewInspirationService 创建灵感提示词服务
// 使用Repository接口依赖注入，遵循依赖倒置原则
func NewInspirationService(repo InspirationRepository) *InspirationService {
	return &InspirationService{
		repo: repo,
	}
}

// NewInspirationServiceWithDB 使用数据库创建服务的便捷方法
// 用于生产环境快速初始化
func NewInspirationServiceWithDB(db *gorm.DB) *InspirationService {
	return NewInspirationService(dao.NewInspirationPromptDao(db))
}

// PromptWithCount 灵感及其应用数
type PromptWithCount struct {
	Prompt *model.InspirationPrompt
	Count  int64
}

// GetInspirationPrompts 获取推荐灵感列表(支持游标分页)
// 规则:
// 1. 根据工具类型筛选（不筛选语言，返回所有语言版本）
// 2. 统计每个灵感近3天的应用数
// 3. 按确定性规则排序:应用数降序 → sort_weight降序 → prompt_id字典序
// 4. 基于游标返回分页结果,到达末尾自动循环回开头
//
// 参数:
//   - cursor: 上次返回的最后一个prompt_id,空字符串表示从头开始
//   - limit: 每页返回数量
//
// 返回:
//   - prompts: 当前页的灵感列表
//   - err: 错误信息
//
// 客户端使用方式:
//  1. 首次请求传空cursor,获取第一页数据
//  2. 从返回的prompts中取最后一条的prompt_id作为cursor
//  3. 传递该cursor获取下一页,服务端自动循环
func (s *InspirationService) GetInspirationPrompts(
	ctx context.Context,
	toolType string,
	cursor string,
	limit int32,
) (
	prompts []*model.InspirationPrompt,
	err error,
) {
	if limit <= 0 {
		limit = 5 // 默认返回3条
	}

	// 1. 获取该工具类型的所有启用灵感
	allPrompts, err := s.repo.GetPromptsByToolType(toolType)
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取灵感列表失败",
			zap.String("tool_type", toolType),
			zap.Error(err))
		return nil, fmt.Errorf("获取灵感列表失败: %w", err)
	}

	if len(allPrompts) == 0 {
		zlog.LogWithContext(ctx).Info("该工具类型暂无可用灵感",
			zap.String("tool_type", toolType))
		return []*model.InspirationPrompt{}, nil
	}

	// 2. 统计每个灵感近3天的应用数
	threeDaysAgo := time.Now().Add(-72 * time.Hour)

	// 提取所有 promptID
	promptIDs := make([]string, 0, len(allPrompts))
	for _, p := range allPrompts {
		promptIDs = append(promptIDs, p.PromptID)
	}

	// 批量统计应用数
	countMap, err := s.repo.CountApplicationsBatch(promptIDs, threeDaysAgo)
	if err != nil {
		zlog.LogWithContext(ctx).Error("统计灵感应用数失败",
			zap.Strings("prompt_ids", promptIDs),
			zap.Error(err))
		return nil, fmt.Errorf("统计灵感应用数失败: %w", err)
	}

	// 3. 组装 PromptWithCount
	promptsWithCount := make([]PromptWithCount, 0, len(allPrompts))
	for _, prompt := range allPrompts {
		count := countMap[prompt.PromptID]
		promptsWithCount = append(promptsWithCount, PromptWithCount{
			Prompt: prompt,
			Count:  count,
		})
	}

	// 4. 确定性多级排序(确保结果可重现)
	// 第一优先级: 应用数降序
	// 第二优先级: sort_weight降序
	// 第三优先级: prompt_id字典序
	sort.SliceStable(promptsWithCount, func(i, j int) bool {
		// 优先级1: 应用数降序
		if promptsWithCount[i].Count != promptsWithCount[j].Count {
			return promptsWithCount[i].Count > promptsWithCount[j].Count
		}
		// 优先级2: sort_weight降序
		if promptsWithCount[i].Prompt.SortWeight != promptsWithCount[j].Prompt.SortWeight {
			return promptsWithCount[i].Prompt.SortWeight > promptsWithCount[j].Prompt.SortWeight
		}
		// 优先级3: prompt_id字典序(保证完全确定性)
		return promptsWithCount[i].Prompt.PromptID < promptsWithCount[j].Prompt.PromptID
	})

	// 5. 提取排序后的灵感列表
	sortedPrompts := make([]*model.InspirationPrompt, 0, len(promptsWithCount))
	for _, pwc := range promptsWithCount {
		sortedPrompts = append(sortedPrompts, pwc.Prompt)
	}

	// 6. 基于游标分页(自动循环)
	result := paginateWithCursor(sortedPrompts, cursor, int(limit))

	// 记录无效cursor警告
	if cursor != "" && findCursorPosition(sortedPrompts, cursor) == -1 {
		zlog.LogWithContext(ctx).Warn("无效的cursor,从头开始",
			zap.String("cursor", cursor),
			zap.String("tool_type", toolType))
	}

	zlog.LogWithContext(ctx).Info("成功获取灵感推荐",
		zap.String("tool_type", toolType),
		zap.String("cursor", cursor),
		zap.Int("total_count", len(allPrompts)),
		zap.Int("returned_count", len(result)))

	return result, nil
}

// findCursorPosition 查找cursor在列表中的位置
// 返回索引,如果未找到返回-1
func findCursorPosition(prompts []*model.InspirationPrompt, cursor string) int {
	if cursor == "" {
		return -1
	}

	for i, p := range prompts {
		if p.PromptID == cursor {
			return i
		}
	}

	return -1
}

// paginateWithCursor 基于游标分页,支持自动循环
// 参数:
//   - prompts: 已排序的灵感列表
//   - cursor: 上次返回的最后一个prompt_id
//   - limit: 每页数量
//
// 返回:
//   - result: 当前页的灵感列表
//
// 说明:
//   - 到达末尾时自动循环回到开头
//   - 客户端从返回结果中取最后一条的prompt_id作为下次请求的cursor
func paginateWithCursor(
	prompts []*model.InspirationPrompt,
	cursor string,
	limit int,
) []*model.InspirationPrompt {
	total := len(prompts)
	if total == 0 {
		return []*model.InspirationPrompt{}
	}

	if limit <= 0 {
		limit = 5
	}

	// 查找起始位置
	startIdx := findCursorPosition(prompts, cursor)
	if startIdx == -1 {
		// 无cursor或cursor无效,从头开始
		startIdx = 0
	} else {
		// 从cursor的下一个位置开始
		startIdx = startIdx + 1
	}

	// 循环逻辑:如果已经到末尾,循环回到开头
	if startIdx >= total {
		startIdx = 0
	}

	// 计算结束位置
	endIdx := startIdx + limit
	if endIdx > total {
		endIdx = total
	}

	// 切片获取结果
	return prompts[startIdx:endIdx]
}

// ApplyInspirationPrompt 记录用户应用灵感
func (s *InspirationService) ApplyInspirationPrompt(ctx context.Context, promptID, userID, toolID, toolType string) error {
	application := &model.InspirationApplication{
		PromptID:  promptID,
		UserID:    userID,
		ToolID:    toolID,
		ToolType:  toolType,
		AppliedAt: time.Now(),
	}

	if err := s.repo.RecordApplication(application); err != nil {
		zlog.LogWithContext(ctx).Error("记录灵感应用失败",
			zap.String("prompt_id", promptID),
			zap.String("user_id", userID),
			zap.String("tool_id", toolID),
			zap.Error(err))
		return fmt.Errorf("记录灵感应用失败: %w", err)
	}

	zlog.LogWithContext(ctx).Info("成功记录灵感应用",
		zap.String("prompt_id", promptID),
		zap.String("user_id", userID),
		zap.String("tool_id", toolID),
		zap.String("tool_type", toolType))

	return nil
}

// GetPromptByID 根据 prompt_id 获取灵感详情
func (s *InspirationService) GetPromptByID(ctx context.Context, promptID string) (*model.InspirationPrompt, error) {
	prompt, err := s.repo.GetPromptByID(promptID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			zlog.LogWithContext(ctx).Warn("灵感不存在",
				zap.String("prompt_id", promptID))
			return nil, fmt.Errorf("灵感不存在: %s", promptID)
		}
		zlog.LogWithContext(ctx).Error("查询灵感失败",
			zap.String("prompt_id", promptID),
			zap.Error(err))
		return nil, fmt.Errorf("查询灵感失败: %w", err)
	}

	return prompt, nil
}

// GetApplicationCount 获取灵感的应用数统计
func (s *InspirationService) GetApplicationCount(ctx context.Context, promptID string, days int) (int64, error) {
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	count, err := s.repo.CountApplications(promptID, since)
	if err != nil {
		zlog.LogWithContext(ctx).Error("统计灵感应用数失败",
			zap.String("prompt_id", promptID),
			zap.Int("days", days),
			zap.Error(err))
		return 0, fmt.Errorf("统计灵感应用数失败: %w", err)
	}

	return count, nil
}
