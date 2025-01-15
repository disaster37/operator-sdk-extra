package remote

import (
	"strings"
)

type RemoteDiff[T any] interface {

	// NeedCreate is true when need to create K8s object
	NeedCreate() bool

	// NeedUpdate is true when need to update K8s object
	NeedUpdate() bool

	// GetObjectsToCreate is the list of object to create on K8s
	GetObjectToCreate() T

	// SetObjectsToCreate permit to set the list of object to create on K8s
	SetObjectToCreate(object T)

	// GetObjectsToUpdate is the list of object to update on K8s
	GetObjectToUpdate() T

	// SetObjectsToUpdate permit to set the list of object to update on K8s
	SetObjectToUpdate(object T)

	// AddDiff permit to add diff
	// It add return line at the end
	AddDiff(diff string)

	// Diff permit to print human diff
	Diff() string

	// IsDiff permit to know is there are current diff to print
	IsDiff() bool
}

// DefaultRemoteDiff is the default implementation of RemoteDiff interface
type DefaultRemoteDiff[T any] struct {

	// CreateObject is the  object to create
	createObject T

	// UpdateObject is the object to update
	updateObject T

	needCreate bool

	needUpdate bool

	// Diff is the diff as string for human knowlegment
	diff strings.Builder
}

// NewBasicRemoteDiff is the basic contructor of RemoteDiff interface
func NewRemoteDiff[T any]() RemoteDiff[T] {
	return &DefaultRemoteDiff[T]{}
}

func (h *DefaultRemoteDiff[T]) NeedCreate() bool {
	return h.needCreate
}

func (h *DefaultRemoteDiff[T]) NeedUpdate() bool {
	return h.needUpdate
}

func (h *DefaultRemoteDiff[T]) GetObjectToCreate() T {
	return h.createObject
}

func (h *DefaultRemoteDiff[T]) SetObjectToCreate(object T) {
	h.createObject = object
	h.needCreate = true
}

func (h *DefaultRemoteDiff[T]) GetObjectToUpdate() T {
	return h.updateObject
}

func (h *DefaultRemoteDiff[T]) SetObjectToUpdate(object T) {
	h.updateObject = object
	h.needUpdate = true
}

func (h *DefaultRemoteDiff[T]) AddDiff(diff string) {
	h.diff.WriteString(diff)
	h.diff.WriteString("\n")
}

func (h *DefaultRemoteDiff[T]) Diff() string {
	return h.diff.String()
}

func (h *DefaultRemoteDiff[T]) IsDiff() bool {
	return h.diff.Len() > 0
}
