package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhaseString(t *testing.T) {
	var o PhaseName = "test"
	assert.Equal(t, "test", o.String())
}
