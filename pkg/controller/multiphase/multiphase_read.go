package multiphase

import (
	"reflect"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/helper"
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

// As converts this read container to expose a different client.Object subtype D.
// The conversion is identity-based (all client.Object subtypes share the same
// underlying interface); it is provided as a method for ergonomics so callers
// don't need to construct the wrapper type manually.
func (h *DefaultMultiPhaseRead[S]) As[D client.Object]() MultiPhaseRead[D] {
	if h == nil {
		return nil
	}
	return &objectMultiPhaseRead[S, D]{in: h}
}

// objectMultiPhaseRead wraps MultiPhaseRead[Src] as MultiPhaseRead[Dst].
type objectMultiPhaseRead[Src, Dst client.Object] struct {
	in MultiPhaseRead[Src]
}

// Deprecated: Use DefaultMultiPhaseRead.As[D]() instead.
type ObjectMultiPhaseRead[Src, Dst client.Object] = objectMultiPhaseRead[Src, Dst]

// Deprecated: Use a concrete read's .As[D]() method instead.
func NewObjectMultiphaseRead[Src, Dst client.Object](in MultiPhaseRead[Src]) MultiPhaseRead[Dst] {
	return &objectMultiPhaseRead[Src, Dst]{in: in}
}

func (h *objectMultiPhaseRead[Src, Dst]) GetCurrentObjects() []Dst {
	return helper.ToSliceOfObject[Src, Dst](h.in.GetCurrentObjects())
}

func (h *objectMultiPhaseRead[Src, Dst]) SetCurrentObjects(objects []Dst) {
	h.in.SetCurrentObjects(helper.ToSliceOfObject[Dst, Src](objects))
}

func (h *objectMultiPhaseRead[Src, Dst]) AddCurrentObject(o Dst) {
	h.in.AddCurrentObject(helper.ToObject[Dst, Src](o))
}

func (h *objectMultiPhaseRead[Src, Dst]) GetExpectedObjects() []Dst {
	return helper.ToSliceOfObject[Src, Dst](h.in.GetExpectedObjects())
}

func (h *objectMultiPhaseRead[Src, Dst]) SetExpectedObjects(objects []Dst) {
	h.in.SetExpectedObjects(helper.ToSliceOfObject[Dst, Src](objects))
}

func (h *objectMultiPhaseRead[Src, Dst]) AddExpectedObject(o Dst) {
	h.in.AddExpectedObject(helper.ToObject[Dst, Src](o))
}
