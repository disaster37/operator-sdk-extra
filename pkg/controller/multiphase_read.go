package controller

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

// BasicMultiPhaseRead is the basic implementation if MultiPhaseRead
type BasicMultiPhaseRead struct {
	currentObjects  []client.Object
	expectedObjects []client.Object
}

func NewBasicMultiPhaseRead() MultiPhaseRead {
	return &BasicMultiPhaseRead{}
}

func (h *BasicMultiPhaseRead) GetCurrentObjects() []client.Object {
	return h.currentObjects
}

func (h *BasicMultiPhaseRead) SetCurrentObjects(objects []client.Object) {
	h.currentObjects = objects
}

func (h *BasicMultiPhaseRead) GetExpectedObjects() []client.Object {
	return h.expectedObjects
}

func (h *BasicMultiPhaseRead) SetExpectedObjects(objects []client.Object) {
	h.expectedObjects = objects
}
