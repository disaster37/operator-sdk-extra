package controller

// RemoteRead is the interface to store the result of read on remote reconciler
type RemoteRead[T any] interface {

	// GetCurrentObject permit to get the current object
	GetCurrentObject() T

	// SetCurrentObject permit to set the current object
	SetCurrentObject(object T)

	// GetExpectedObject permit to get the  expected object
	GetExpectedObject() T

	// SetExpectedObject permit to set the expected object
	SetExpectedObject(object T)
}

// BasicMultiPhaseRead is the basic implementation if MultiPhaseRead
type BasicRemoteRead[T any] struct {
	currentObject  T
	expectedObject T
}

// NewBasicRemoteRead is the basic constructor of RemoteRead interface
func NewBasicRemoteRead[T any]() RemoteRead[T] {
	return &BasicRemoteRead[T]{}
}

func (h *BasicRemoteRead[T]) GetCurrentObject() T {
	return h.currentObject
}

func (h *BasicRemoteRead[T]) SetCurrentObject(object T) {
	h.currentObject = object
}

func (h *BasicRemoteRead[T]) GetExpectedObject() T {
	return h.expectedObject
}

func (h *BasicRemoteRead[T]) SetExpectedObject(object T) {
	h.expectedObject = object
}
