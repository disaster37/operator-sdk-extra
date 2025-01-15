package multiphase

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultMultiPhaseObjectStatusPhase(t *testing.T) {
	// With default object
	o := &DefaultMultiPhaseObjectStatus{}

	assert.Empty(t, o.GetPhaseName())

	// When phase is set
	o.SetPhaseName("test")
	assert.Equal(t, "test", o.GetPhaseName().String())

}
