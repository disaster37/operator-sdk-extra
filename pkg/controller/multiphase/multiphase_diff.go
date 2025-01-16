package multiphase

import (
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseDiff is used to know if currents resources differ with expected
type MultiPhaseDiff[k8sStepObject client.Object] interface {

	// NeedCreate is true when need to create K8s object
	NeedCreate() bool

	// NeedUpdate is true when need to update K8s object
	NeedUpdate() bool

	// NeedDelete is true when need to delete K8s object
	NeedDelete() bool

	// GetObjectsToCreate is the list of object to create on K8s
	GetObjectsToCreate() []k8sStepObject

	// SetObjectsToCreate permit to set the list of object to create on K8s
	SetObjectsToCreate(objects []k8sStepObject)

	// AddObjectToCreate permit to add object on create list
	AddObjectToCreate(o k8sStepObject)

	// GetObjectsToUpdate is the list of object to update on K8s
	GetObjectsToUpdate() []k8sStepObject

	// SetObjectsToUpdate permit to set the list of object to update on K8s
	SetObjectsToUpdate(objects []k8sStepObject)

	// AddObjectToUpdate permit to add object on update list
	AddObjectToUpdate(o k8sStepObject)

	// GetObjectsToDelete is the list of Object to delete on K8s
	GetObjectsToDelete() []k8sStepObject

	// SetObjectsToDelete permit to set the list of object to delete
	SetObjectsToDelete(objects []k8sStepObject)

	// AddObjectToDelete permit to add object on delete list
	AddObjectToDelete(o k8sStepObject)

	// AddDiff permit to add diff
	// It add return line at the end
	AddDiff(diff string)

	// Diff permit to print human diff
	Diff() string

	// IsDiff permit to know is there are current diff to print
	IsDiff() bool
}

// DefaultMultiPhaseDiff is the default implementation of MultiPhaseDiff interface
type DefaultMultiPhaseDiff[k8sStepObject client.Object] struct {

	// CreateObjects is the list of object to create on K8s
	createObjects []k8sStepObject

	// UpdateObjects is the list of object to update on K8s
	updateObjects []k8sStepObject

	// DeleteObjects is the list of object to delete on K8s
	deleteObjects []k8sStepObject

	// Diff is the diff as string for human knowlegment
	diff strings.Builder
}

// NewMultiPhaseDiff is the default implementation of MultiPhaseDiff interface
func NewMultiPhaseDiff[k8sStepObject client.Object]() MultiPhaseDiff[k8sStepObject] {
	return &DefaultMultiPhaseDiff[k8sStepObject]{
		createObjects: make([]k8sStepObject, 0),
		updateObjects: make([]k8sStepObject, 0),
		deleteObjects: make([]k8sStepObject, 0),
	}
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) NeedCreate() bool {
	return len(h.createObjects) > 0
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) NeedUpdate() bool {
	return len(h.updateObjects) > 0
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) NeedDelete() bool {
	return len(h.deleteObjects) > 0
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) GetObjectsToCreate() []k8sStepObject {
	return h.createObjects
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) SetObjectsToCreate(objects []k8sStepObject) {
	if len(objects) == 0 {
		return
	}

	if len(h.createObjects) == 0 {
		h.createObjects = objects
	} else {
		h.createObjects = append(h.createObjects, objects...)
	}

}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) AddObjectToCreate(o k8sStepObject) {
	h.createObjects = append(h.createObjects, o)
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) GetObjectsToUpdate() []k8sStepObject {
	return h.updateObjects
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) SetObjectsToUpdate(objects []k8sStepObject) {
	if len(objects) == 0 {
		return
	}

	if len(h.updateObjects) == 0 {
		h.updateObjects = objects
	} else {
		h.updateObjects = append(h.updateObjects, objects...)
	}

}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) AddObjectToUpdate(o k8sStepObject) {
	h.updateObjects = append(h.updateObjects, o)
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) GetObjectsToDelete() []k8sStepObject {
	return h.deleteObjects
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) SetObjectsToDelete(objects []k8sStepObject) {
	if len(objects) == 0 {
		return
	}

	if len(h.deleteObjects) == 0 {
		h.deleteObjects = objects
	} else {
		h.deleteObjects = append(h.deleteObjects, objects...)
	}

}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) AddObjectToDelete(o k8sStepObject) {
	h.deleteObjects = append(h.deleteObjects, o)
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) AddDiff(diff string) {
	h.diff.WriteString(diff)
	h.diff.WriteString("\n")
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) Diff() string {
	return h.diff.String()
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) IsDiff() bool {
	return h.diff.Len() > 0
}
