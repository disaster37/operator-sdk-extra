package controller

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

// BasicRemoteDiff is the basic implementation of RemoteDiff interface
type BasicRemoteDiff[T any] struct {

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
func NewBasicRemoteDiff[T any]() RemoteDiff[T] {
	return &BasicRemoteDiff[T]{}
}

func (h *BasicRemoteDiff[T]) NeedCreate() bool {
	return h.needCreate
}

func (h *BasicRemoteDiff[T]) NeedUpdate() bool {
	return h.needUpdate
}

func (h *BasicRemoteDiff[T]) GetObjectToCreate() T {
	return h.createObject
}

func (h *BasicRemoteDiff[T]) SetObjectToCreate(object T) {
	h.createObject = object
	h.needCreate = true
}

func (h *BasicRemoteDiff[T]) GetObjectToUpdate() T {
	return h.updateObject
}

func (h *BasicRemoteDiff[T]) SetObjectToUpdate(object T) {
	h.updateObject = object
	h.needUpdate = true
}

func (h *BasicRemoteDiff[T]) AddDiff(diff string) {
	h.diff.WriteString(diff)
	h.diff.WriteString("\n")
}

func (h *BasicRemoteDiff[T]) Diff() string {
	return h.diff.String()
}

func (h *BasicRemoteDiff[T]) IsDiff() bool {
	return h.diff.Len() > 0
}
