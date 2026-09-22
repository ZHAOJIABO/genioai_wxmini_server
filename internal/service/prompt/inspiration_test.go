package prompt

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"va_visionai_server/internal/model"
)

// ============ Fake Repository Implementation ============
// 手写fake实现，遵循Go最佳实践，无需Mock框架

type fakeInspirationRepo struct {
	// 数据存储
	prompts      map[string]*model.InspirationPrompt
	applications []*model.InspirationApplication

	// 错误注入（用于测试错误场景）
	getPromptsByToolTypeErr error
	getPromptByIDErr        error
	countApplicationsErr    error
	recordApplicationErr    error
}

func newFakeInspirationRepo() *fakeInspirationRepo {
	return &fakeInspirationRepo{
		prompts:      make(map[string]*model.InspirationPrompt),
		applications: make([]*model.InspirationApplication, 0),
	}
}

func (f *fakeInspirationRepo) GetPromptsByToolType(toolType string) ([]*model.InspirationPrompt, error) {
	if f.getPromptsByToolTypeErr != nil {
		return nil, f.getPromptsByToolTypeErr
	}

	result := make([]*model.InspirationPrompt, 0)
	for _, p := range f.prompts {
		if p.ToolType == toolType && p.Status == 1 {
			result = append(result, p)
		}
	}
	return result, nil
}

func (f *fakeInspirationRepo) GetPromptByID(promptID string) (*model.InspirationPrompt, error) {
	if f.getPromptByIDErr != nil {
		return nil, f.getPromptByIDErr
	}

	if p, ok := f.prompts[promptID]; ok {
		return p, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeInspirationRepo) CountApplicationsBatch(promptIDs []string, since time.Time) (map[string]int64, error) {
	if f.countApplicationsErr != nil {
		return nil, f.countApplicationsErr
	}

	countMap := make(map[string]int64)
	for _, app := range f.applications {
		if app.AppliedAt.After(since) || app.AppliedAt.Equal(since) {
			for _, pid := range promptIDs {
				if app.PromptID == pid {
					countMap[pid]++
				}
			}
		}
	}

	// 填充零值
	for _, pid := range promptIDs {
		if _, exists := countMap[pid]; !exists {
			countMap[pid] = 0
		}
	}

	return countMap, nil
}

func (f *fakeInspirationRepo) CountApplications(promptID string, since time.Time) (int64, error) {
	if f.countApplicationsErr != nil {
		return 0, f.countApplicationsErr
	}

	var count int64
	for _, app := range f.applications {
		if app.PromptID == promptID && (app.AppliedAt.After(since) || app.AppliedAt.Equal(since)) {
			count++
		}
	}
	return count, nil
}

func (f *fakeInspirationRepo) RecordApplication(app *model.InspirationApplication) error {
	if f.recordApplicationErr != nil {
		return f.recordApplicationErr
	}

	f.applications = append(f.applications, app)
	return nil
}

// ============ Test Helpers ============

func setupTestPrompts() map[string]*model.InspirationPrompt {
	return map[string]*model.InspirationPrompt{
		"p1": {PromptID: "p1", Title: "Prompt 1", Status: 1, ToolType: "text2img", Language: "zh"},
		"p2": {PromptID: "p2", Title: "Prompt 2", Status: 1, ToolType: "text2img", Language: "zh"},
		"p3": {PromptID: "p3", Title: "Prompt 3", Status: 1, ToolType: "text2img", Language: "zh"},
	}
}

// ============ GetInspirationPrompts Tests ============

func TestGetInspirationPrompts_Success(t *testing.T) {
	fake := newFakeInspirationRepo()
	fake.prompts = map[string]*model.InspirationPrompt{
		"p1": {PromptID: "p1", Title: "Prompt 1", Status: 1, ToolType: "text2img", Language: "zh", SortWeight: 100},
		"p2": {PromptID: "p2", Title: "Prompt 2", Status: 1, ToolType: "text2img", Language: "zh", SortWeight: 90},
		"p3": {PromptID: "p3", Title: "Prompt 3", Status: 1, ToolType: "text2img", Language: "zh", SortWeight: 80},
	}

	// 设置不同的应用数
	now := time.Now()
	fake.applications = []*model.InspirationApplication{
		{PromptID: "p1", AppliedAt: now.Add(-1 * time.Hour)},
		{PromptID: "p1", AppliedAt: now.Add(-2 * time.Hour)},
		{PromptID: "p1", AppliedAt: now.Add(-3 * time.Hour)},
		{PromptID: "p1", AppliedAt: now.Add(-4 * time.Hour)},
		{PromptID: "p1", AppliedAt: now.Add(-5 * time.Hour)},
		{PromptID: "p1", AppliedAt: now.Add(-6 * time.Hour)},
		{PromptID: "p1", AppliedAt: now.Add(-7 * time.Hour)},
		{PromptID: "p1", AppliedAt: now.Add(-8 * time.Hour)},
		{PromptID: "p1", AppliedAt: now.Add(-9 * time.Hour)},
		{PromptID: "p1", AppliedAt: now.Add(-10 * time.Hour)}, // 10个应用
		{PromptID: "p2", AppliedAt: now.Add(-1 * time.Hour)},
		{PromptID: "p2", AppliedAt: now.Add(-2 * time.Hour)},
		{PromptID: "p2", AppliedAt: now.Add(-3 * time.Hour)},
		{PromptID: "p2", AppliedAt: now.Add(-4 * time.Hour)},
		{PromptID: "p2", AppliedAt: now.Add(-5 * time.Hour)}, // 5个应用
		{PromptID: "p3", AppliedAt: now.Add(-1 * time.Hour)},
		{PromptID: "p3", AppliedAt: now.Add(-2 * time.Hour)},
		{PromptID: "p3", AppliedAt: now.Add(-3 * time.Hour)}, // 3个应用
	}

	svc := NewInspirationService(fake)
	ctx := context.Background()

	result, err := svc.GetInspirationPrompts(ctx, "text2img", "", 3)

	require.NoError(t, err)
	assert.Len(t, result, 3)
	// 验证按应用数降序排序
	assert.Equal(t, "p1", result[0].PromptID)
	assert.Equal(t, "p2", result[1].PromptID)
	assert.Equal(t, "p3", result[2].PromptID)
}

// TestGetInspirationPrompts_StableSorting 测试稳定排序(多级排序规则)
func TestGetInspirationPrompts_StableSorting(t *testing.T) {
	fake := newFakeInspirationRepo()
	// p1和p2有相同的应用数,但sort_weight不同
	// p3和p4有相同的应用数和sort_weight,但prompt_id不同
	fake.prompts = map[string]*model.InspirationPrompt{
		"p1": {PromptID: "p1", Title: "Prompt 1", Status: 1, ToolType: "text2img", Language: "zh", SortWeight: 100},
		"p2": {PromptID: "p2", Title: "Prompt 2", Status: 1, ToolType: "text2img", Language: "zh", SortWeight: 80},
		"p3": {PromptID: "p3", Title: "Prompt 3", Status: 1, ToolType: "text2img", Language: "zh", SortWeight: 50},
		"p4": {PromptID: "p4", Title: "Prompt 4", Status: 1, ToolType: "text2img", Language: "zh", SortWeight: 50},
	}

	now := time.Now()
	// p1和p2都有5个应用
	for i := 0; i < 5; i++ {
		fake.applications = append(fake.applications, &model.InspirationApplication{
			PromptID:  "p1",
			AppliedAt: now.Add(-time.Duration(i) * time.Hour),
		})
		fake.applications = append(fake.applications, &model.InspirationApplication{
			PromptID:  "p2",
			AppliedAt: now.Add(-time.Duration(i) * time.Hour),
		})
	}
	// p3和p4都有2个应用
	for i := 0; i < 2; i++ {
		fake.applications = append(fake.applications, &model.InspirationApplication{
			PromptID:  "p3",
			AppliedAt: now.Add(-time.Duration(i) * time.Hour),
		})
		fake.applications = append(fake.applications, &model.InspirationApplication{
			PromptID:  "p4",
			AppliedAt: now.Add(-time.Duration(i) * time.Hour),
		})
	}

	svc := NewInspirationService(fake)
	ctx := context.Background()

	// 多次调用验证排序稳定性
	for i := 0; i < 5; i++ {
		result, err := svc.GetInspirationPrompts(ctx, "text2img", "", 10)

		require.NoError(t, err)
		assert.Len(t, result, 4)

		// 验证多级排序规则
		// 第一级:应用数降序,p1和p2(5个应用)排在前面
		assert.Contains(t, []string{"p1", "p2"}, result[0].PromptID)
		assert.Contains(t, []string{"p1", "p2"}, result[1].PromptID)

		// 第二级:相同应用数时,按sort_weight降序
		// p1(weight=100) 应该在 p2(weight=80) 前面
		assert.Equal(t, "p1", result[0].PromptID)
		assert.Equal(t, "p2", result[1].PromptID)

		// 第三级:相同应用数和weight时,按prompt_id字典序
		// p3(weight=50) 和 p4(weight=50),应该按prompt_id排序
		assert.Equal(t, "p3", result[2].PromptID)
		assert.Equal(t, "p4", result[3].PromptID)
	}
}

// TestGetInspirationPrompts_WithCursorPagination 测试游标分页
func TestGetInspirationPrompts_WithCursorPagination(t *testing.T) {
	fake := newFakeInspirationRepo()
	// 创建8个灵感
	for i := 1; i <= 8; i++ {
		promptID := fmt.Sprintf("p%d", i)
		fake.prompts[promptID] = &model.InspirationPrompt{
			PromptID:   promptID,
			Title:      fmt.Sprintf("Prompt %d", i),
			Status:     1,
			ToolType:   "text2img",
			Language:   "zh",
			SortWeight: 100 - i, // 确保有明确的排序
		}
	}

	svc := NewInspirationService(fake)
	ctx := context.Background()
	limit := int32(3)

	// 第一页:不传cursor
	result1, err := svc.GetInspirationPrompts(ctx, "text2img", "", limit)
	require.NoError(t, err)
	assert.Len(t, result1, 3)
	assert.Equal(t, "p1", result1[0].PromptID)
	assert.Equal(t, "p2", result1[1].PromptID)
	assert.Equal(t, "p3", result1[2].PromptID)

	// 客户端取最后一条的prompt_id作为cursor
	cursor1 := result1[len(result1)-1].PromptID

	// 第二页:使用第一页的最后一条prompt_id
	result2, err := svc.GetInspirationPrompts(ctx, "text2img", cursor1, limit)
	require.NoError(t, err)
	assert.Len(t, result2, 3)
	assert.Equal(t, "p4", result2[0].PromptID)
	assert.Equal(t, "p5", result2[1].PromptID)
	assert.Equal(t, "p6", result2[2].PromptID)

	// 客户端取最后一条的prompt_id作为cursor
	cursor2 := result2[len(result2)-1].PromptID

	// 第三页:最后一页(只剩2条)
	result3, err := svc.GetInspirationPrompts(ctx, "text2img", cursor2, limit)
	require.NoError(t, err)
	assert.Len(t, result3, 2) // 只剩2条
	assert.Equal(t, "p7", result3[0].PromptID)
	assert.Equal(t, "p8", result3[1].PromptID)

	// 客户端取最后一条的prompt_id作为cursor
	cursor3 := result3[len(result3)-1].PromptID

	// 第四页:循环回到开头
	result4, err := svc.GetInspirationPrompts(ctx, "text2img", cursor3, limit)
	require.NoError(t, err)
	assert.Len(t, result4, 3)
	assert.Equal(t, "p1", result4[0].PromptID) // 循环回到开头
	assert.Equal(t, "p2", result4[1].PromptID)
	assert.Equal(t, "p3", result4[2].PromptID)
}

// TestGetInspirationPrompts_InvalidCursor 测试无效cursor的容错
func TestGetInspirationPrompts_InvalidCursor(t *testing.T) {
	fake := newFakeInspirationRepo()
	fake.prompts = map[string]*model.InspirationPrompt{
		"p1": {PromptID: "p1", Title: "Prompt 1", Status: 1, ToolType: "text2img", Language: "zh", SortWeight: 100},
		"p2": {PromptID: "p2", Title: "Prompt 2", Status: 1, ToolType: "text2img", Language: "zh", SortWeight: 90},
		"p3": {PromptID: "p3", Title: "Prompt 3", Status: 1, ToolType: "text2img", Language: "zh", SortWeight: 80},
	}

	svc := NewInspirationService(fake)
	ctx := context.Background()

	// 使用不存在的cursor,应该从头开始
	result, err := svc.GetInspirationPrompts(ctx, "text2img", "p999", 2)

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "p1", result[0].PromptID) // 从头开始
	assert.Equal(t, "p2", result[1].PromptID)
}

// TestGetInspirationPrompts_EmptyList 测试空列表
func TestGetInspirationPrompts_EmptyList(t *testing.T) {
	fake := newFakeInspirationRepo()
	svc := NewInspirationService(fake)

	result, err := svc.GetInspirationPrompts(context.Background(), "text2img", "", 3)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestGetInspirationPrompts_Empty(t *testing.T) {
	fake := newFakeInspirationRepo()
	svc := NewInspirationService(fake)

	result, err := svc.GetInspirationPrompts(context.Background(), "text2img", "", 3)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestGetInspirationPrompts_LimitZero(t *testing.T) {
	fake := newFakeInspirationRepo()
	fake.prompts = setupTestPrompts()

	svc := NewInspirationService(fake)
	result, err := svc.GetInspirationPrompts(context.Background(), "text2img", "", 0)

	require.NoError(t, err)
	assert.Len(t, result, 3, "limit=0时应该返回默认5条（但只有3个可用数据）")
}

func TestGetInspirationPrompts_LimitGreaterThanTotal(t *testing.T) {
	fake := newFakeInspirationRepo()
	fake.prompts = map[string]*model.InspirationPrompt{
		"p1": {PromptID: "p1", Title: "Prompt 1", Status: 1, ToolType: "text2img", Language: "zh"},
		"p2": {PromptID: "p2", Title: "Prompt 2", Status: 1, ToolType: "text2img", Language: "zh"},
	}

	svc := NewInspirationService(fake)
	result, err := svc.GetInspirationPrompts(context.Background(), "text2img", "", 10)

	require.NoError(t, err)
	assert.Len(t, result, 2, "limit大于总数时应该返回所有结果")
}

func TestGetInspirationPrompts_GetPromptsError(t *testing.T) {
	fake := newFakeInspirationRepo()
	fake.getPromptsByToolTypeErr = errors.New("database error")

	svc := NewInspirationService(fake)
	result, err := svc.GetInspirationPrompts(context.Background(), "text2img", "", 3)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "获取灵感列表失败")
}

func TestGetInspirationPrompts_CountError(t *testing.T) {
	fake := newFakeInspirationRepo()
	fake.prompts = map[string]*model.InspirationPrompt{
		"p1": {PromptID: "p1", Title: "Prompt 1", Status: 1, ToolType: "text2img", Language: "zh"},
	}
	fake.countApplicationsErr = errors.New("count error")

	svc := NewInspirationService(fake)
	result, err := svc.GetInspirationPrompts(context.Background(), "text2img", "", 3)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "统计灵感应用数失败")
}

func TestGetInspirationPrompts_ZeroCount(t *testing.T) {
	fake := newFakeInspirationRepo()
	fake.prompts = setupTestPrompts()
	// 没有应用记录

	svc := NewInspirationService(fake)
	result, err := svc.GetInspirationPrompts(context.Background(), "text2img", "", 2)

	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// ============ ApplyInspirationPrompt Tests ============

func TestApplyInspirationPrompt_Success(t *testing.T) {
	fake := newFakeInspirationRepo()
	fake.prompts = map[string]*model.InspirationPrompt{
		"p1": {PromptID: "p1", Title: "Test", Status: 1, ToolType: "text2img", Language: "zh"},
	}

	svc := NewInspirationService(fake)
	err := svc.ApplyInspirationPrompt(context.Background(), "p1", "user1", "tool1", "text2img")

	require.NoError(t, err)
	assert.Len(t, fake.applications, 1)
	assert.Equal(t, "p1", fake.applications[0].PromptID)
	assert.Equal(t, "user1", fake.applications[0].UserID)
}

func TestApplyInspirationPrompt_RecordError(t *testing.T) {
	fake := newFakeInspirationRepo()
	fake.prompts = map[string]*model.InspirationPrompt{
		"p1": {PromptID: "p1", Title: "Test", Status: 1, ToolType: "text2img", Language: "zh"},
	}
	fake.recordApplicationErr = errors.New("record error")

	svc := NewInspirationService(fake)
	err := svc.ApplyInspirationPrompt(context.Background(), "p1", "user1", "tool1", "text2img")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "记录灵感应用失败")
}

// ============ GetPromptByID Tests ============

func TestGetPromptByID_Success(t *testing.T) {
	fake := newFakeInspirationRepo()
	expectedPrompt := &model.InspirationPrompt{
		PromptID: "p1",
		Title:    "Test Prompt",
		Content:  "Test Content",
		Status:   1,
	}
	fake.prompts["p1"] = expectedPrompt

	svc := NewInspirationService(fake)
	result, err := svc.GetPromptByID(context.Background(), "p1")

	require.NoError(t, err)
	assert.Equal(t, expectedPrompt, result)
}

func TestGetPromptByID_NotFound(t *testing.T) {
	fake := newFakeInspirationRepo()
	svc := NewInspirationService(fake)

	result, err := svc.GetPromptByID(context.Background(), "p999")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "灵感不存在")
}

func TestGetPromptByID_GetError(t *testing.T) {
	fake := newFakeInspirationRepo()
	fake.getPromptByIDErr = errors.New("database error")

	svc := NewInspirationService(fake)
	result, err := svc.GetPromptByID(context.Background(), "p1")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "查询灵感失败")
}

// ============ GetApplicationCount Tests ============

func TestGetApplicationCount_Success(t *testing.T) {
	fake := newFakeInspirationRepo()
	now := time.Now()
	// 添加15条7天内的应用记录
	for i := 0; i < 15; i++ {
		fake.applications = append(fake.applications, &model.InspirationApplication{
			PromptID:  "p1",
			AppliedAt: now.Add(-time.Duration(i) * time.Hour),
		})
	}

	svc := NewInspirationService(fake)
	count, err := svc.GetApplicationCount(context.Background(), "p1", 7)

	require.NoError(t, err)
	assert.Equal(t, int64(15), count)
}

func TestGetApplicationCount_ZeroCount(t *testing.T) {
	fake := newFakeInspirationRepo()
	svc := NewInspirationService(fake)

	count, err := svc.GetApplicationCount(context.Background(), "p1", 3)

	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestGetApplicationCount_CountError(t *testing.T) {
	fake := newFakeInspirationRepo()
	fake.countApplicationsErr = errors.New("database error")

	svc := NewInspirationService(fake)
	count, err := svc.GetApplicationCount(context.Background(), "p1", 7)

	require.Error(t, err)
	assert.Equal(t, int64(0), count)
	assert.Contains(t, err.Error(), "统计灵感应用数失败")
}
