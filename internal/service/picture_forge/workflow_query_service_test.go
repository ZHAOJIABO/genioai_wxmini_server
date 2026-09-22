package picture_forge

import (
	"testing"

	"va_visionai_server/internal/model"
)

// Test that toProtoWorkflows uses taskChainCreditCost from workflowTransformContext
func TestToProtoWorkflows_UsesContextCreditCost(t *testing.T) {
	svc := NewWorkflowQueryService(nil, nil, nil)

	kind := &model.WorkflowKind{
		KindID:   "k_image",
		KindType: "WORKFLOW_KIND_TYPE_IMAGE",
	}
	wf := &model.Workflow{
		WorkflowID:   "w1",
		KindID:       "k_image",
		CreditPoints: 10,
	}

	tctx := &workflowTransformContext{
		taskChainCreditCost: 100,
		defaultToolParams:   nil,
	}

	got := svc.toProtoWorkflows([]*model.Workflow{wf}, kind, "", tctx)
	if len(got) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(got))
	}
	if got[0].GetChainCreditCost() != int32(110) {
		t.Fatalf("expected ChainCreditCost=110, got %d", got[0].GetChainCreditCost())
	}
}
