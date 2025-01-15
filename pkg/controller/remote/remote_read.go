package remote

// RemoteRead is the interface to store the result of read on remote reconciler
type RemoteRead[apiObject comparable] interface {

	// GetCurrentObject permit to get the current object
	GetCurrentObject() apiObject

	// SetCurrentObject permit to set the current object
	SetCurrentObject(object apiObject)

	// GetExpectedObject permit to get the  expected object
	GetExpectedObject() apiObject

	// SetExpectedObject permit to set the expected object
	SetExpectedObject(object apiObject)
}

// DefaultRemoteRead is the default implementation of RemoteRead
type DefaultRemoteRead[T any] struct {
	currentObject  T
	expectedObject T
}

// NewRemoteRead is the default implementation of RemoteRead interface
func NewRemoteRead[apiObject comparable]() RemoteRead[apiObject] {
	return &DefaultRemoteRead[apiObject]{}
}

func (h *DefaultRemoteRead[apiObject]) GetCurrentObject() apiObject {
	return h.currentObject
}

func (h *DefaultRemoteRead[apiObject]) SetCurrentObject(object apiObject) {
	h.currentObject = object
}

func (h *DefaultRemoteRead[apiObject]) GetExpectedObject() apiObject {
	return h.expectedObject
}

func (h *DefaultRemoteRead[apiObject]) SetExpectedObject(object apiObject) {
	h.expectedObject = object
}
