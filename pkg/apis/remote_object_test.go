package apis

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoteObjectIsSync(t *testing.T) {
	// With basic object
	o := BasicRemoteObjectStatus{}

	assert.False(t, o.GetIsSync())

	// When is false
	o.SetIsSync(false)
	assert.False(t, o.GetIsSync())

	// When is true
	o.SetIsSync(true)
	assert.True(t, o.GetIsSync())
}
