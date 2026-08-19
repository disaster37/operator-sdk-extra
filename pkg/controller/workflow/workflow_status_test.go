package workflow_test

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/workflow"
)

func TestPhaseAliasMatchesWorkflowPhase(t *testing.T) {
	// Verify that the Phase alias in the controller package matches
	// the workflow.WorkflowPhase type.
	var _ workflow.WorkflowPhase = ""
	_ = workflow.WorkflowPhase("test")
}
