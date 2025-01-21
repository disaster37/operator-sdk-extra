package multiphase

import (
	"reflect"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/helper"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseRead is the interface to store the result of read step on multi phase reconciler
type MultiPhaseRead[k8sStepObject client.Object] interface {

	// GetCurrentObjects permit to get the list of current objects
	GetCurrentObjects() []k8sStepObject

	// SetCurrentObjects permit to set the list of current objects
	SetCurrentObjects(objects []k8sStepObject)

	// AddCurrentObject will add object on the list of current objects
	AddCurrentObject(o k8sStepObject)

	// GetExpectedObjects permit to get the list of expected objects
	GetExpectedObjects() []k8sStepObject

	// SetExpectedObjects permit to set the list of expected objects
	SetExpectedObjects(objects []k8sStepObject)

	// AddExpectedObject will add object on the list of expected objects
	AddExpectedObject(o k8sStepObject)
}

// DefaultMultiPhaseRead is the default implementation if MultiPhaseRead
type DefaultMultiPhaseRead[k8sStepObject client.Object] struct {
	currentObjects  []k8sStepObject
	expectedObjects []k8sStepObject
}

// NewMultiPhaseRead is the default implementation of MultiPhaseRead interface
func NewMultiPhaseRead[k8sStepObject client.Object]() MultiPhaseRead[k8sStepObject] {
	return &DefaultMultiPhaseRead[k8sStepObject]{
		currentObjects:  make([]k8sStepObject, 0, 1),
		expectedObjects: make([]k8sStepObject, 0, 1),
	}
}

func (h *DefaultMultiPhaseRead[k8sStepObject]) GetCurrentObjects() []k8sStepObject {
	return h.currentObjects
}

func (h *DefaultMultiPhaseRead[k8sStepObject]) SetCurrentObjects(objects []k8sStepObject) {
	for _, object := range objects {
		h.AddCurrentObject(object)
	}
}

func (h *DefaultMultiPhaseRead[k8sStepObject]) AddCurrentObject(o k8sStepObject) {
	if reflect.ValueOf(o).IsNil() {
		return
	}
	h.currentObjects = append(h.currentObjects, o)
}

func (h *DefaultMultiPhaseRead[k8sStepObject]) GetExpectedObjects() []k8sStepObject {
	return h.expectedObjects
}

func (h *DefaultMultiPhaseRead[k8sStepObject]) SetExpectedObjects(objects []k8sStepObject) {
	for _, object := range objects {
		h.AddExpectedObject(object)
	}
}

func (h *DefaultMultiPhaseRead[k8sStepObject]) AddExpectedObject(o k8sStepObject) {
	if reflect.ValueOf(o).IsNil() {
		return
	}

	h.expectedObjects = append(h.expectedObjects, o)
}

// ObjectMultiPhaseRead is the implementation of MultiPhaseRead for a specific client.Object type needed by multiphase reconciler
// It's kind of wrapper to conver MultiPhaseRead[k8sStepObject] to MultiPhaseRead[client.Object]
type ObjectMultiPhaseRead[k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object] struct {
	in MultiPhaseRead[k8sStepObjectSrc]
}

func NewObjectMultiphaseRead[k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object](in MultiPhaseRead[k8sStepObjectSrc]) MultiPhaseRead[k8sStepObjectDst] {
	return &ObjectMultiPhaseRead[k8sStepObjectSrc, k8sStepObjectDst]{
		in: in,
	}
}

func (h *ObjectMultiPhaseRead[k8sStepObjectSrc, k8sStepObjectDst]) GetCurrentObjects() []k8sStepObjectDst {
	return helper.ToSliceOfObject[k8sStepObjectSrc, k8sStepObjectDst](h.in.GetCurrentObjects())
}

func (h *ObjectMultiPhaseRead[k8sStepObjectSrc, k8sStepObjectDst]) SetCurrentObjects(objects []k8sStepObjectDst) {
	h.in.SetCurrentObjects(helper.ToSliceOfObject[k8sStepObjectDst, k8sStepObjectSrc](objects))
}

func (h *ObjectMultiPhaseRead[k8sStepObjectSrc, k8sStepObjectDst]) AddCurrentObject(o k8sStepObjectDst) {
	h.in.AddCurrentObject(helper.ToObject[k8sStepObjectDst, k8sStepObjectSrc](o))
}

func (h *ObjectMultiPhaseRead[k8sStepObjectSrc, k8sStepObjectDst]) GetExpectedObjects() []k8sStepObjectDst {
	return helper.ToSliceOfObject[k8sStepObjectSrc, k8sStepObjectDst](h.in.GetExpectedObjects())
}

func (h *ObjectMultiPhaseRead[k8sStepObjectSrc, k8sStepObjectDst]) SetExpectedObjects(objects []k8sStepObjectDst) {
	h.in.SetExpectedObjects(helper.ToSliceOfObject[k8sStepObjectDst, k8sStepObjectSrc](objects))
}

func (h *ObjectMultiPhaseRead[k8sStepObjectSrc, k8sStepObjectDst]) AddExpectedObject(o k8sStepObjectDst) {
	h.in.AddExpectedObject(helper.ToObject[k8sStepObjectDst, k8sStepObjectSrc](o))
}
