package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoteObjectIsSync(t *testing.T) {
	// With basic object
	o := DefaultRemoteObjectStatus{}

	assert.False(t, o.GetIsSync())

	// When is false
	o.SetIsSync(false)
	assert.False(t, o.GetIsSync())

	// When is true
	o.SetIsSync(true)
	assert.True(t, o.GetIsSync())
}

func TestLastAppliedConfiguration(t *testing.T) {
	// With basic object
	o := DefaultRemoteObjectStatus{}

	assert.Empty(t, o.GetLastAppliedConfiguration())

	// When set
	o.SetLastAppliedConfiguration("test")
	assert.Equal(t, "test", o.GetLastAppliedConfiguration())
}
