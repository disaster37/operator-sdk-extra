package workflow_test

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestWorkflowStatusDeepCopyNil(t *testing.T) {
	assert.Nil(t, (*workflow.WorkflowStatus)(nil).DeepCopy())
}

func TestWorkflowStatusDeepCopyEmpty(t *testing.T) {
	ws := &workflow.WorkflowStatus{}
	cp := ws.DeepCopy()
	require.NotNil(t, cp)
	assert.Equal(t, ws, cp)
	assert.Nil(t, cp.PhaseConditions)
}

func TestWorkflowStatusDeepCopyPopulated(t *testing.T) {
	lastTransition := metav1.Now()
	ws := &workflow.WorkflowStatus{
		CurrentPhase: "Rotate",
		PhaseConditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				Reason:             "Success",
				Message:            "Ready",
				LastTransitionTime: lastTransition,
			},
		},
	}

	cp := ws.DeepCopy()
	require.NotNil(t, cp)
	assert.Equal(t, ws, cp)

	// Deep isolation: mutating the copy must not affect the original.
	cp.PhaseConditions[0].Type = "Mutated"
	assert.Equal(t, "Ready", ws.PhaseConditions[0].Type)

	// Independent slice header: appending to the copy must not grow the original.
	cp.PhaseConditions = append(cp.PhaseConditions, metav1.Condition{Type: "Extra", Status: metav1.ConditionFalse})
	assert.Len(t, ws.PhaseConditions, 1)
	assert.Len(t, cp.PhaseConditions, 2)
}

func TestWorkflowStatusDeepCopyInto(t *testing.T) {
	ws := &workflow.WorkflowStatus{
		CurrentPhase: "Converge",
		PhaseConditions: []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue},
		},
	}

	out := &workflow.WorkflowStatus{}
	ws.DeepCopyInto(out)
	assert.Equal(t, ws, out)

	// Deep isolation for PhaseConditions.
	out.PhaseConditions[0].Type = "Mutated"
	assert.Equal(t, "Ready", ws.PhaseConditions[0].Type)
}
