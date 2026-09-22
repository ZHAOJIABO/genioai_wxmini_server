package picture_forge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/model"
)

// ============ Test Cases ============

func TestFilterWorkflowsByKindVersion(t *testing.T) {
	ctx := context.Background()
	svc := &PictureForgeService{}

	tests := []struct {
		name          string
		workflows     []*model.Workflow
		kindInfoMap   map[string]*model.WorkflowKind
		appVersion    string
		platform      string
		expectedCount int
		expectedIDs   []string
		description   string
	}{
		{
			name: "正常过滤_iOS版本符合要求",
			workflows: []*model.Workflow{
				{WorkflowID: "wf1", KindID: "kind1", Status: 1},
				{WorkflowID: "wf2", KindID: "kind2", Status: 1},
			},
			kindInfoMap: map[string]*model.WorkflowKind{
				"kind1": {KindID: "kind1", IOSupportVersions: ">=1.0.0,<2.0.0"},
				"kind2": {KindID: "kind2", IOSupportVersions: ">=2.0.0"},
			},
			appVersion:    "1.5.0",
			platform:      constants.IOS,
			expectedCount: 1,
			expectedIDs:   []string{"wf1"},
			description:   "iOS 1.5.0只匹配 >=1.0.0,<2.0.0 的kind1",
		},
		{
			name: "正常过滤_Android版本符合要求",
			workflows: []*model.Workflow{
				{WorkflowID: "wf1", KindID: "kind1", Status: 1},
				{WorkflowID: "wf2", KindID: "kind2", Status: 1},
			},
			kindInfoMap: map[string]*model.WorkflowKind{
				"kind1": {KindID: "kind1", AndroidSupportVersions: "<3.0.0"},
				"kind2": {KindID: "kind2", AndroidSupportVersions: ">=3.0.0"},
			},
			appVersion:    "2.5.0",
			platform:      constants.ANDROID,
			expectedCount: 1,
			expectedIDs:   []string{"wf1"},
			description:   "Android 2.5.0只匹配 <3.0.0 的kind1",
		},
		{
			name: "无版本限制_保留所有workflow",
			workflows: []*model.Workflow{
				{WorkflowID: "wf1", KindID: "kind1", Status: 1},
				{WorkflowID: "wf2", KindID: "kind2", Status: 1},
			},
			kindInfoMap: map[string]*model.WorkflowKind{
				"kind1": {KindID: "kind1", IOSupportVersions: ""},
				"kind2": {KindID: "kind2", IOSupportVersions: ""},
			},
			appVersion:    "1.0.0",
			platform:      constants.IOS,
			expectedCount: 2,
			expectedIDs:   []string{"wf1", "wf2"},
			description:   "版本字段为空,保留所有workflow",
		},
		{
			name: "Kind缺失_宽松策略保留workflow",
			workflows: []*model.Workflow{
				{WorkflowID: "wf1", KindID: "kind_missing", Status: 1},
				{WorkflowID: "wf2", KindID: "kind2", Status: 1},
			},
			kindInfoMap: map[string]*model.WorkflowKind{
				"kind2": {KindID: "kind2", IOSupportVersions: ">=1.0.0"},
			},
			appVersion:    "1.5.0",
			platform:      constants.IOS,
			expectedCount: 2,
			expectedIDs:   []string{"wf1", "wf2"},
			description:   "kind1缺失时采用宽松策略,保留workflow",
		},
		{
			name: "空版本参数_跳过所有过滤",
			workflows: []*model.Workflow{
				{WorkflowID: "wf1", KindID: "kind1", Status: 1},
				{WorkflowID: "wf2", KindID: "kind2", Status: 1},
			},
			kindInfoMap: map[string]*model.WorkflowKind{
				"kind1": {KindID: "kind1", IOSupportVersions: ">=2.0.0"},
				"kind2": {KindID: "kind2", IOSupportVersions: ">=3.0.0"},
			},
			appVersion:    "", // 空版本
			platform:      constants.IOS,
			expectedCount: 2,
			expectedIDs:   []string{"wf1", "wf2"},
			description:   "appVersion为空时,跳过版本过滤",
		},
		{
			name: "版本完全不符合_过滤所有",
			workflows: []*model.Workflow{
				{WorkflowID: "wf1", KindID: "kind1", Status: 1},
				{WorkflowID: "wf2", KindID: "kind2", Status: 1},
			},
			kindInfoMap: map[string]*model.WorkflowKind{
				"kind1": {KindID: "kind1", IOSupportVersions: ">=5.0.0"},
				"kind2": {KindID: "kind2", IOSupportVersions: ">=6.0.0"},
			},
			appVersion:    "1.0.0",
			platform:      constants.IOS,
			expectedCount: 0,
			expectedIDs:   []string{},
			description:   "所有kind要求的版本都高于客户端版本,全部过滤",
		},
		{
			name: "平台不明确_宽松策略保留",
			workflows: []*model.Workflow{
				{WorkflowID: "wf1", KindID: "kind1", Status: 1},
			},
			kindInfoMap: map[string]*model.WorkflowKind{
				"kind1": {KindID: "kind1", IOSupportVersions: ">=5.0.0"},
			},
			appVersion:    "1.0.0",
			platform:      "unknown", // 未知平台
			expectedCount: 1,
			expectedIDs:   []string{"wf1"},
			description:   "平台不明确时采用宽松策略",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.filterWorkflowsByKindVersion(ctx, tt.workflows, tt.kindInfoMap, tt.appVersion, tt.platform)

			assert.Len(t, result, tt.expectedCount, tt.description)

			// 验证返回的workflow ID
			actualIDs := make([]string, len(result))
			for i, wf := range result {
				actualIDs[i] = wf.WorkflowID
			}
			assert.ElementsMatch(t, tt.expectedIDs, actualIDs, "返回的workflow ID应该匹配")
		})
	}
}
