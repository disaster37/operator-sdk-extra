package sentinel

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

// DefaultSentinelRead is the default implementation of SentinelRead
type DefaultSentinelRead struct {
	currentObjects  map[string][]client.Object
	expectedObjects map[string][]client.Object
}

// NewSentinelRead is the default implementation of SentinelRead interface
func NewSentinelRead() SentinelRead {
	return &DefaultSentinelRead{
		currentObjects:  map[string][]client.Object{},
		expectedObjects: map[string][]client.Object{},
	}
}

func (h *DefaultSentinelRead) GetAllCurrentObjects() map[string][]client.Object {
	return h.currentObjects
}

func (h *DefaultSentinelRead) GetCurrentObjects(objectType string) []client.Object {
	return h.currentObjects[objectType]
}

func (h *DefaultSentinelRead) SetCurrentObjects(objectType string, objects []client.Object) {
	h.currentObjects[objectType] = objects
}

func (h *DefaultSentinelRead) GetAllExpectedObjects() map[string][]client.Object {
	return h.expectedObjects
}

func (h *DefaultSentinelRead) GetExpectedObjects(objectType string) []client.Object {
	return h.expectedObjects[objectType]
}

func (h *DefaultSentinelRead) SetExpectedObjects(objectType string, objects []client.Object) {
	h.expectedObjects[objectType] = objects
}
