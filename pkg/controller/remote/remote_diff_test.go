package remote

import (
	"testing"

	"github.com/disaster37/go-kibana-rest/v8/kbapi"
	"github.com/stretchr/testify/assert"
)

func TestBasicRemoteDiffDiff(t *testing.T) {
	o := NewRemoteDiff[*kbapi.LogstashPipeline]()

	assert.False(t, o.IsDiff())
	assert.Empty(t, o.Diff())

	// When diff
	o.AddDiff("test")
	assert.True(t, o.IsDiff())
	assert.Contains(t, o.Diff(), "test")
}

func TestBasicRemoteDiffCreate(t *testing.T) {
	o := NewRemoteDiff[*kbapi.LogstashPipeline]()

	assert.False(t, o.NeedCreate())
	assert.Nil(t, o.GetObjectToCreate())

	object := &kbapi.LogstashPipeline{}

	// When need to create object
	o.SetObjectToCreate(object)

	assert.True(t, o.NeedCreate())
	assert.Equal(t, object, o.GetObjectToCreate())
}

func TestBasicRemoteDiffUpdate(t *testing.T) {
	o := NewRemoteDiff[*kbapi.LogstashPipeline]()

	assert.False(t, o.NeedUpdate())
	assert.Nil(t, o.GetObjectToUpdate())

	object := &kbapi.LogstashPipeline{}

	// When need to update object
	o.SetObjectToUpdate(object)

	assert.True(t, o.NeedUpdate())
	assert.Equal(t, object, o.GetObjectToUpdate())
}
