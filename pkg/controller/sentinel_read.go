package controller

import "sigs.k8s.io/controller-runtime/pkg/client"

// SentinelRead is the interface to store the result of read from sentinel reconciler
// objectType is the key if you need to handle some type of object to not shuffle them
type SentinelRead interface {

	// GetAllCurrentObjects premit to get the map of current objects, mapped by type
	GetAllCurrentObjects() map[string][]client.Object

	// GetCurrentObjects permit to get the list of current objects
	GetCurrentObjects(objectType string) []client.Object

	// SetCurrentObjects permit to set the list of current objects
	SetCurrentObjects(objectType string, objects []client.Object)

	// GetAllExpectedObjects premit to get the map of expected objects, mapped by type
	GetAllExpectedObjects() map[string][]client.Object

	// GetExpectedObjects permit to get the list of expected objects
	GetExpectedObjects(objectType string) []client.Object

	// SetExpectedObjects permit to set the list of expected objects
	SetExpectedObjects(objectType string, objects []client.Object)
}

// BasicSentinelRead is the basic implementation of SentinelRead
type BasicSentinelRead struct {
	currentObjects  map[string][]client.Object
	expectedObjects map[string][]client.Object
}

// NewBasicSentinelRead is the basic constructor of SentinelRead interface
func NewBasicSentinelRead() SentinelRead {
	return &BasicSentinelRead{
		currentObjects:  map[string][]client.Object{},
		expectedObjects: map[string][]client.Object{},
	}
}

func (h *BasicSentinelRead) GetAllCurrentObjects() map[string][]client.Object {
	return h.currentObjects
}

func (h *BasicSentinelRead) GetCurrentObjects(objectType string) []client.Object {
	return h.currentObjects[objectType]
}

func (h *BasicSentinelRead) SetCurrentObjects(objectType string, objects []client.Object) {
	h.currentObjects[objectType] = objects
}

func (h *BasicSentinelRead) GetAllExpectedObjects() map[string][]client.Object {
	return h.expectedObjects
}

func (h *BasicSentinelRead) GetExpectedObjects(objectType string) []client.Object {
	return h.expectedObjects[objectType]
}

func (h *BasicSentinelRead) SetExpectedObjects(objectType string, objects []client.Object) {
	h.expectedObjects[objectType] = objects
}
