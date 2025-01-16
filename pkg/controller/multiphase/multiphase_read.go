package multiphase

import "sigs.k8s.io/controller-runtime/pkg/client"

// MultiPhaseRead is the interface to store the result of read step on multi phase reconciler
type MultiPhaseRead[k8sObject client.Object] interface {

	// GetCurrentObjects permit to get the list of current objects
	GetCurrentObjects() []k8sObject

	// SetCurrentObjects permit to set the list of current objects
	SetCurrentObjects(objects []k8sObject)

	// AddCurrentObject will add object on the list of current objects
	AddCurrentObject(o k8sObject)

	// GetExpectedObjects permit to get the list of expected objects
	GetExpectedObjects() []k8sObject

	// SetExpectedObjects permit to set the list of expected objects
	SetExpectedObjects(objects []k8sObject)

	// AddExpectedObject will add object on the list of expected objects
	AddExpectedObject(o k8sObject)
}

// DefaultMultiPhaseRead is the default implementation if MultiPhaseRead
type DefaultMultiPhaseRead[k8sObject client.Object] struct {
	currentObjects  []k8sObject
	expectedObjects []k8sObject
}

// NewMultiPhaseRead is the default implementation of MultiPhaseRead interface
func NewMultiPhaseRead[k8sObject client.Object]() MultiPhaseRead[k8sObject] {
	return &DefaultMultiPhaseRead[k8sObject]{
		currentObjects:  make([]k8sObject, 0, 1),
		expectedObjects: make([]k8sObject, 0, 1),
	}
}

func (h *DefaultMultiPhaseRead[k8sObject]) GetCurrentObjects() []k8sObject {
	return h.currentObjects
}

func (h *DefaultMultiPhaseRead[k8sObject]) SetCurrentObjects(objects []k8sObject) {
	if len(objects) == 0 {
		return
	}
	if len(h.currentObjects) == 0 {
		h.currentObjects = objects
	} else {
		h.currentObjects = append(h.currentObjects, objects...)
	}

}

func (h *DefaultMultiPhaseRead[k8sObject]) AddCurrentObject(o k8sObject) {
	h.currentObjects = append(h.currentObjects, o)
}

func (h *DefaultMultiPhaseRead[k8sObject]) GetExpectedObjects() []k8sObject {
	return h.expectedObjects
}

func (h *DefaultMultiPhaseRead[k8sObject]) SetExpectedObjects(objects []k8sObject) {

	if len(objects) == 0 {
		return
	}
	if len(h.expectedObjects) == 0 {
		h.expectedObjects = objects
	} else {
		h.expectedObjects = append(h.expectedObjects, objects...)
	}
}

func (h *DefaultMultiPhaseRead[k8sObject]) AddExpectedObject(o k8sObject) {
	h.expectedObjects = append(h.expectedObjects, o)
}
