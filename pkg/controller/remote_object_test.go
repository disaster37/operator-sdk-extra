package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestRemoteObjectGetStatus(t *testing.T) {
	status := BasicRemoteObjectStatus{
		IsSync: ptr.To[bool](true),
	}
	o := &BasicRemoteObject{
		Status: status,
	}

	assert.Equal(t, &status, o.GetStatus())
}

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
