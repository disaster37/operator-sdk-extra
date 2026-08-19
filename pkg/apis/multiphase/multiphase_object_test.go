package multiphase

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultMultiPhaseObjectStatusDeepCopy(t *testing.T) {
	o := &DefaultMultiPhaseObjectStatus{
		PhaseName: "phase-1",
	}
	o.SetIsOnError(true)
	o.SetConditions([]metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}})
	o.SetLastErrorMessage("test error")
	o.SetObservedGeneration(42)

	copy := o.DeepCopy()
	assert.NotNil(t, copy)
	assert.Equal(t, o.PhaseName, copy.PhaseName)
	assert.Equal(t, o.GetIsOnError(), copy.GetIsOnError())
	assert.Equal(t, o.Conditions[0].Type, copy.Conditions[0].Type)
	assert.Equal(t, o.LastErrorMessage, copy.LastErrorMessage)
	assert.Equal(t, o.ObservedGeneration, copy.ObservedGeneration)

	// Modify original to verify deep copy is independent
	o.PhaseName = "phase-2"
	o.SetIsOnError(false)
	o.Conditions[0].Status = metav1.ConditionUnknown
	o.LastErrorMessage = "modified"
	o.SetObservedGeneration(99)
	assert.NotEqual(t, o.PhaseName, copy.PhaseName)
	assert.NotEqual(t, o.GetIsOnError(), copy.GetIsOnError())
	assert.NotEqual(t, o.Conditions[0].Status, copy.Conditions[0].Status)
	assert.NotEqual(t, o.LastErrorMessage, copy.LastErrorMessage)
	assert.NotEqual(t, o.ObservedGeneration, copy.ObservedGeneration)
}

func TestDefaultMultiPhaseObjectStatusDeepCopyNilReceiver(t *testing.T) {
	var m *DefaultMultiPhaseObjectStatus = nil
	assert.Nil(t, m.DeepCopy())
}

func TestDefaultMultiPhaseObjectStatusDeepCopyWithNilPointer(t *testing.T) {
	o := &DefaultMultiPhaseObjectStatus{
		PhaseName: "phase-1",
}

	copy := o.DeepCopy()
	assert.NotNil(t, copy)
	assert.Equal(t, o.PhaseName, copy.PhaseName)
}

func TestDefaultMultiPhaseObjectStatusMethods(t *testing.T) {
	o := &DefaultMultiPhaseObjectStatus{}

	// Test inherited methods from DefaultObjectStatus
	assert.False(t, o.GetIsOnError())

	o.SetIsOnError(true)
	assert.True(t, o.GetIsOnError())

	o.SetIsOnError(false)
	assert.False(t, o.GetIsOnError())

	// Test with true value
	o.SetIsOnError(true)
	assert.True(t, o.GetIsOnError())

	// LastErrorMessage
	assert.Empty(t, o.GetLastErrorMessage())
	o.SetLastErrorMessage("some error")
	assert.Equal(t, "some error", o.GetLastErrorMessage())
	
	// Test observed generation
	assert.Equal(t, int64(0), o.GetObservedGeneration())
	o.SetObservedGeneration(42)
	assert.Equal(t, int64(42), o.GetObservedGeneration())
	
	// Test phase name methods
	var emptyPhase shared.PhaseName = ""
	assert.Equal(t, emptyPhase, o.GetPhaseName())
	o.SetPhaseName("new-phase")
	assert.Equal(t, shared.PhaseName("new-phase"), o.GetPhaseName())
}
