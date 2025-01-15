package remote

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

// DefaultRemoteRead is the default implementation of RemoteRead
type DefaultRemoteRead[T any] struct {
	currentObject  T
	expectedObject T
}

// NewRemoteRead is the default implementation of RemoteRead interface
func NewRemoteRead[T any]() RemoteRead[T] {
	return &DefaultRemoteRead[T]{}
}

func (h *DefaultRemoteRead[T]) GetCurrentObject() T {
	return h.currentObject
}

func (h *DefaultRemoteRead[T]) SetCurrentObject(object T) {
	h.currentObject = object
}

func (h *DefaultRemoteRead[T]) GetExpectedObject() T {
	return h.expectedObject
}

func (h *DefaultRemoteRead[T]) SetExpectedObject(object T) {
	h.expectedObject = object
}
