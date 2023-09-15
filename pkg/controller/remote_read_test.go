package controller

import (
	"testing"

	"github.com/disaster37/go-kibana-rest/v8/kbapi"
	"github.com/stretchr/testify/assert"
)

func TestBasicRemoteReadCurrent(t *testing.T) {
	o := NewBasicRemoteRead[*kbapi.LogstashPipeline]()

	assert.Empty(t, o.GetCurrentObject())

	// When set object
	object := &kbapi.LogstashPipeline{}
	o.SetCurrentObject(object)
	assert.Equal(t, object, o.GetCurrentObject())
}

func TestBasicRemoteReadExpected(t *testing.T) {
	o := NewBasicRemoteRead[*kbapi.LogstashPipeline]()

	assert.Empty(t, o.GetExpectedObject())

	// When set object
	object := &kbapi.LogstashPipeline{}
	o.SetExpectedObject(object)
	assert.Equal(t, object, o.GetExpectedObject())
}
