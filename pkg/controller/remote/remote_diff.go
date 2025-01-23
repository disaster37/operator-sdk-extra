package remote

import (
	"strings"
)

type RemoteDiff[apiObject comparable] interface {
	// NeedCreate is true when need to create K8s object
	NeedCreate() bool

	// NeedUpdate is true when need to update K8s object
	NeedUpdate() bool

	// GetObjectsToCreate is the list of object to create on K8s
	GetObjectToCreate() apiObject

	// SetObjectsToCreate permit to set the list of object to create on K8s
	SetObjectToCreate(object apiObject)

	// GetObjectsToUpdate is the list of object to update on K8s
	GetObjectToUpdate() apiObject

	// SetObjectsToUpdate permit to set the list of object to update on K8s
	SetObjectToUpdate(object apiObject)

	// AddDiff permit to add diff
	// It add return line at the end
	AddDiff(diff string)

	// Diff permit to print human diff
	Diff() string

	// IsDiff permit to know is there are current diff to print
	IsDiff() bool
}

// DefaultRemoteDiff is the default implementation of RemoteDiff interface
type DefaultRemoteDiff[apiObject comparable] struct {
	// CreateObject is the  object to create
	createObject apiObject

	// UpdateObject is the object to update
	updateObject apiObject

	needCreate bool

	needUpdate bool

	// Diff is the diff as string for human knowlegment
	diff strings.Builder
}

// NewBasicRemoteDiff is the basic contructor of RemoteDiff interface
func NewRemoteDiff[apiObject comparable]() RemoteDiff[apiObject] {
	return &DefaultRemoteDiff[apiObject]{}
}

func (h *DefaultRemoteDiff[apiObject]) NeedCreate() bool {
	return h.needCreate
}

func (h *DefaultRemoteDiff[apiObject]) NeedUpdate() bool {
	return h.needUpdate
}

func (h *DefaultRemoteDiff[apiObject]) GetObjectToCreate() apiObject {
	return h.createObject
}

func (h *DefaultRemoteDiff[apiObject]) SetObjectToCreate(object apiObject) {
	h.createObject = object
	h.needCreate = true
}

func (h *DefaultRemoteDiff[apiObject]) GetObjectToUpdate() apiObject {
	return h.updateObject
}

func (h *DefaultRemoteDiff[apiObject]) SetObjectToUpdate(object apiObject) {
	h.updateObject = object
	h.needUpdate = true
}

func (h *DefaultRemoteDiff[apiObject]) AddDiff(diff string) {
	h.diff.WriteString(diff)
	h.diff.WriteString("\n")
}

func (h *DefaultRemoteDiff[apiObject]) Diff() string {
	return h.diff.String()
}

func (h *DefaultRemoteDiff[apiObject]) IsDiff() bool {
	return h.diff.Len() > 0
}
