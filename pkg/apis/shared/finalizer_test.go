package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFinalizerString(t *testing.T) {
	var o FinalizerName = "test"

	assert.Equal(t, "test", o.String())
}
