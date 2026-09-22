package picture_forge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"va_visionai_server/internal/model"
)

// ============ filterToolsForHome 测试 ============

func TestFilterToolsForHome(t *testing.T) {
	ctx := context.Background()
	svc := &PictureToolsService{}

	tests := []struct {
		name        string
		tools       []*model.PictureTools
		expectedLen int
		expectedIDs []string
		description string
	}{
		{
			name: "全部工具可见",
			tools: []*model.PictureTools{
				{ToolID: "tool1", HideOnHome: false},
				{ToolID: "tool2", HideOnHome: false},
				{ToolID: "tool3", HideOnHome: false},
			},
			expectedLen: 3,
			expectedIDs: []string{"tool1", "tool2", "tool3"},
			description: "所有工具 HideOnHome=false，全部返回",
		},
		{
			name: "部分工具隐藏",
			tools: []*model.PictureTools{
				{ToolID: "tool1", HideOnHome: false},
				{ToolID: "tool2", HideOnHome: true},
				{ToolID: "tool3", HideOnHome: false},
			},
			expectedLen: 2,
			expectedIDs: []string{"tool1", "tool3"},
			description: "tool2 HideOnHome=true 被过滤",
		},
		{
			name: "全部工具隐藏",
			tools: []*model.PictureTools{
				{ToolID: "tool1", HideOnHome: true},
				{ToolID: "tool2", HideOnHome: true},
			},
			expectedLen: 0,
			expectedIDs: []string{},
			description: "所有工具都隐藏，返回空列表",
		},
		{
			name:        "空列表",
			tools:       []*model.PictureTools{},
			expectedLen: 0,
			expectedIDs: []string{},
			description: "输入空列表，返回空列表",
		},
		{
			name:        "nil列表",
			tools:       nil,
			expectedLen: 0,
			expectedIDs: []string{},
			description: "输入nil，返回空列表",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.filterToolsForHome(ctx, tt.tools)
			assert.Len(t, result, tt.expectedLen, tt.description)

			// 验证返回的工具ID
			resultIDs := make([]string, 0, len(result))
			for _, tool := range result {
				resultIDs = append(resultIDs, tool.ToolID)
			}
			assert.Equal(t, tt.expectedIDs, resultIDs, "返回的工具ID应匹配")
		})
	}
}
