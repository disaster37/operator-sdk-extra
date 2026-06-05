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

func TestRemoteObjectDeepCopy(t *testing.T) {
	o := &DefaultRemoteObjectStatus{}
	o.SetIsSync(true)
	o.SetLastAppliedConfiguration("config-v1")
	o.SetObservedGeneration(5)

	copy := o.DeepCopy()
	assert.NotNil(t, copy)
	assert.Equal(t, o.GetIsSync(), copy.GetIsSync())
	assert.Equal(t, o.GetLastAppliedConfiguration(), copy.GetLastAppliedConfiguration())
	assert.Equal(t, o.GetObservedGeneration(), copy.GetObservedGeneration())

	// Verify independence
	o.SetIsSync(false)
	o.SetLastAppliedConfiguration("config-v2")
	o.SetObservedGeneration(10)
	assert.True(t, copy.GetIsSync())
	assert.Equal(t, "config-v1", copy.GetLastAppliedConfiguration())
	assert.Equal(t, int64(5), copy.GetObservedGeneration())
}

func TestRemoteObjectDeepCopyNilReceiver(t *testing.T) {
	var r *DefaultRemoteObjectStatus = nil
	assert.Nil(t, r.DeepCopy())
}

func TestRemoteObjectDeepCopyWithBothPointers(t *testing.T) {
	trueVal := true
	o := &DefaultRemoteObjectStatus{
		IsSync: &trueVal,
	}
	o.SetIsSync(true)
	o.SetLastAppliedConfiguration("applied-config")
	o.SetLastErrorMessage("err-msg")
	o.SetObservedGeneration(7)

	copy := o.DeepCopy()
	assert.NotNil(t, copy)
	assert.True(t, copy.GetIsSync())
	assert.Equal(t, "applied-config", copy.LastAppliedConfiguration)
	assert.Equal(t, "err-msg", copy.GetLastErrorMessage())
	assert.Equal(t, int64(7), copy.GetObservedGeneration())
}
