package controller

import (
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseDiff is used to know if currents resources differ with expected
type MultiPhaseDiff interface {

	// NeedCreate is true when need to create K8s object
	NeedCreate() bool

	// NeedUpdate is true when need to update K8s object
	NeedUpdate() bool

	// NeedDelete is true when need to delete K8s object
	NeedDelete() bool

	// GetObjectsToCreate is the list of object to create on K8s
	GetObjectsToCreate() []client.Object

	// SetObjectsToCreate permit to set the list of object to create on K8s
	SetObjectsToCreate(objects []client.Object)

	// GetObjectsToUpdate is the list of object to update on K8s
	GetObjectsToUpdate() []client.Object

	// SetObjectsToUpdate permit to set the list of object to update on K8s
	SetObjectsToUpdate(objects []client.Object)

	// GetObjectsToDelete is the list of Object to delete on K8s
	GetObjectsToDelete() []client.Object

	// SetObjectsToDelete permit to set the list of object to delete
	SetObjectsToDelete(objects []client.Object)

	// AddDiff permit to add diff
	// It add return line at the end
	AddDiff(diff string)

	// Diff permit to print human diff
	Diff() string

	// IsDiff permit to know is there are current diff to print
	IsDiff() bool
}

// BasicMultiPhaseDiff is the basic implementation of MultiPhaseDiff interface
type BasicMultiPhaseDiff struct {

	// CreateObjects is the list of object to create on K8s
	createObjects []client.Object

	// UpdateObjects is the list of object to update on K8s
	updateObjects []client.Object

	// DeleteObjects is the list of object to delete on K8s
	deleteObjects []client.Object

	// Diff is the diff as string for human knowlegment
	diff strings.Builder
}

// NewBasicMultiPhaseDiff is the basic contructor of MultiPhaseDiff interface
func NewBasicMultiPhaseDiff() MultiPhaseDiff {
	return &BasicMultiPhaseDiff{}
}

func (h *BasicMultiPhaseDiff) NeedCreate() bool {
	return len(h.createObjects) > 0
}

func (h *BasicMultiPhaseDiff) NeedUpdate() bool {
	return len(h.updateObjects) > 0
}

func (h *BasicMultiPhaseDiff) NeedDelete() bool {
	return len(h.deleteObjects) > 0
}

func (h *BasicMultiPhaseDiff) GetObjectsToCreate() []client.Object {
	return h.createObjects
}

func (h *BasicMultiPhaseDiff) SetObjectsToCreate(objects []client.Object) {
	h.createObjects = objects
}

func (h *BasicMultiPhaseDiff) GetObjectsToUpdate() []client.Object {
	return h.updateObjects
}

func (h *BasicMultiPhaseDiff) SetObjectsToUpdate(objects []client.Object) {
	h.updateObjects = objects
}

func (h *BasicMultiPhaseDiff) GetObjectsToDelete() []client.Object {
	return h.deleteObjects
}

func (h *BasicMultiPhaseDiff) SetObjectsToDelete(objects []client.Object) {
	h.deleteObjects = objects
}

func (h *BasicMultiPhaseDiff) AddDiff(diff string) {
	h.diff.WriteString(diff)
	h.diff.WriteString("\n")
}

func (h *BasicMultiPhaseDiff) Diff() string {
	return h.diff.String()
}

func (h *BasicMultiPhaseDiff) IsDiff() bool {
	return h.diff.Len() > 0
}
