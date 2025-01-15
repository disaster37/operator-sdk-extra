package multiphase

import "sigs.k8s.io/controller-runtime/pkg/client"

// MultiPhaseRead is the interface to store the result of read step on multi phase reconciler
type MultiPhaseRead interface {

	// GetCurrentObjects permit to get the list of current objects
	GetCurrentObjects() []client.Object

	// SetCurrentObjects permit to set the list of current objects
	SetCurrentObjects(objects []client.Object)

	// GetExpectedObjects permit to get the list of expected objects
	GetExpectedObjects() []client.Object

	// SetExpectedObjects permit to set the list of expected objects
	SetExpectedObjects(objects []client.Object)
}

// DefaultMultiPhaseRead is the default implementation if MultiPhaseRead
type DefaultMultiPhaseRead struct {
	currentObjects  []client.Object
	expectedObjects []client.Object
}

// NewMultiPhaseRead is the default implementation of MultiPhaseRead interface
func NewMultiPhaseRead() MultiPhaseRead {
	return &DefaultMultiPhaseRead{}
}

func (h *DefaultMultiPhaseRead) GetCurrentObjects() []client.Object {
	return h.currentObjects
}

func (h *DefaultMultiPhaseRead) SetCurrentObjects(objects []client.Object) {
	h.currentObjects = objects
}

func (h *DefaultMultiPhaseRead) GetExpectedObjects() []client.Object {
	return h.expectedObjects
}

func (h *DefaultMultiPhaseRead) SetExpectedObjects(objects []client.Object) {
	h.expectedObjects = objects
}
