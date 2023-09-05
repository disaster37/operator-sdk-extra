package apis

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMultiPhaseObjectGetStatus(t *testing.T) {
	status := BasicMultiPhaseObjectStatus{
		PhaseName: "test",
	}
	o := &BasicMultiPhaseObject{
		Status: status,
	}

	assert.Equal(t, &status, o.GetStatus())
}

func TestBasicMultiPhaseObjectStatusPhase(t *testing.T) {
	// With default object
	o := &BasicMultiPhaseObjectStatus{}

	assert.Empty(t, o.GetPhaseName())

	// When phase is set
	o.SetPhaseName("test")
	assert.Equal(t, "test", o.GetPhaseName().String())

	o.SetLastErrorMessage("test error")
	assert.Equal(t, "test error", o.GetLastErrorMessage())

}
