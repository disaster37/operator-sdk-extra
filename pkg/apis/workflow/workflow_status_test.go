package workflow_test

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/workflow"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkflowPhaseString(t *testing.T) {
	phase := workflow.WorkflowPhase("test-phase")
	assert.Equal(t, "test-phase", phase.String())
}

func TestWorkflowStatusCurrent(t *testing.T) {
	ws := &workflow.WorkflowStatus{}
	assert.Equal(t, workflow.WorkflowPhase(""), ws.Current())

	ws.Advance("phase-1")
	assert.Equal(t, workflow.WorkflowPhase("phase-1"), ws.Current())
}

func TestWorkflowStatusAdvance(t *testing.T) {
	ws := &workflow.WorkflowStatus{}
	ws.Advance("phase-1")
	assert.Equal(t, workflow.WorkflowPhase("phase-1"), ws.CurrentPhase)

	ws.Advance("phase-2")
	assert.Equal(t, workflow.WorkflowPhase("phase-2"), ws.CurrentPhase)
}

func TestWorkflowStatusIsPhase(t *testing.T) {
	ws := &workflow.WorkflowStatus{}
	assert.False(t, ws.IsPhase("phase-1"))

	ws.Advance("phase-1")
	assert.True(t, ws.IsPhase("phase-1"))
	assert.False(t, ws.IsPhase("phase-2"))
}

func TestWorkflowStatusIsPhaseEmpty(t *testing.T) {
	ws := &workflow.WorkflowStatus{}
	assert.True(t, ws.IsPhaseEmpty())

	ws.Advance("phase-1")
	assert.False(t, ws.IsPhaseEmpty())
}

func TestWorkflowStatusPhaseConditions(t *testing.T) {
	ws := &workflow.WorkflowStatus{}
	assert.Empty(t, ws.PhaseConditions)

	ws.PhaseConditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
	}
	assert.Len(t, ws.PhaseConditions, 1)
	assert.Equal(t, metav1.ConditionTrue, ws.PhaseConditions[0].Status)
}
