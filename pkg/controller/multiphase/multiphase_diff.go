package multiphase

import (
	"strings"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/helper"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseDiff is used to know if current resources differ with expected.
// In SSA mode, it holds the list of objects to apply (via Server-Side Apply) and orphans to delete.
type MultiPhaseDiff[k8sStepObject client.Object] interface {
	// NeedApply is true when need to apply K8s objects via SSA
	NeedApply() bool

	// NeedDelete is true when need to delete K8s objects (orphans)
	NeedDelete() bool

	// GetObjectsToApply is the list of objects to apply via SSA on K8s
	GetObjectsToApply() []k8sStepObject

	// SetObjectsToApply permit to set the list of objects to apply via SSA
	SetObjectsToApply(objects []k8sStepObject)

	// AddObjectToApply permit to add object on apply list
	AddObjectToApply(o k8sStepObject)

	// GetObjectsToDelete is the list of objects to delete on K8s (orphans)
	GetObjectsToDelete() []k8sStepObject

	// SetObjectsToDelete permit to set the list of objects to delete
	SetObjectsToDelete(objects []k8sStepObject)

	// AddObjectToDelete permit to add object on delete list
	AddObjectToDelete(o k8sStepObject)

	// AddDiff permit to add diff
	// It adds a return line at the end
	AddDiff(diff string)

	// Diff permit to print human diff
	Diff() string

	// IsDiff permit to know if there are current diff to print
	IsDiff() bool
}

// DefaultMultiPhaseDiff is the default implementation of MultiPhaseDiff interface
type DefaultMultiPhaseDiff[k8sStepObject client.Object] struct {
	// ApplyObjects is the list of objects to apply via SSA on K8s
	applyObjects []k8sStepObject

	// DeleteObjects is the list of objects to delete on K8s
	deleteObjects []k8sStepObject

	// Diff is the diff as string for human knowledge
	diff strings.Builder
}

// NewMultiPhaseDiff is the default implementation of MultiPhaseDiff interface
func NewMultiPhaseDiff[k8sStepObject client.Object]() MultiPhaseDiff[k8sStepObject] {
	return &DefaultMultiPhaseDiff[k8sStepObject]{
		applyObjects:  make([]k8sStepObject, 0),
		deleteObjects: make([]k8sStepObject, 0),
	}
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) NeedApply() bool {
	return len(h.applyObjects) > 0
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) NeedDelete() bool {
	return len(h.deleteObjects) > 0
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) GetObjectsToApply() []k8sStepObject {
	return h.applyObjects
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) SetObjectsToApply(objects []k8sStepObject) {
	if len(objects) == 0 {
		return
	}

	if len(h.applyObjects) == 0 {
		h.applyObjects = objects
	} else {
		h.applyObjects = append(h.applyObjects, objects...)
	}
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) AddObjectToApply(o k8sStepObject) {
	h.applyObjects = append(h.applyObjects, o)
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

// ObjectMultiPhaseDiff is the implementation of MultiPhaseDiff for a specific client.Object type needed by multiphase reconciler
// It's kind of wrapper to convert MultiPhaseDiff[k8sStepObject] to MultiPhaseDiff[client.Object]
type ObjectMultiPhaseDiff[k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object] struct {
	in MultiPhaseDiff[k8sStepObjectSrc]
}

func NewObjectMultiphaseDiff[k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object](in MultiPhaseDiff[k8sStepObjectSrc]) MultiPhaseDiff[k8sStepObjectDst] {
	return &ObjectMultiPhaseDiff[k8sStepObjectSrc, k8sStepObjectDst]{
		in: in,
	}
}

func (h *ObjectMultiPhaseDiff[k8sStepObjectSrc, k8sStepObjectDst]) NeedApply() bool {
	return h.in.NeedApply()
}

func (h *ObjectMultiPhaseDiff[k8sStepObjectSrc, k8sStepObjectDst]) NeedDelete() bool {
	return h.in.NeedDelete()
}

func (h *ObjectMultiPhaseDiff[k8sStepObjectSrc, k8sStepObjectDst]) GetObjectsToApply() []k8sStepObjectDst {
	return helper.ToSliceOfObject[k8sStepObjectSrc, k8sStepObjectDst](h.in.GetObjectsToApply())
}

func (h *ObjectMultiPhaseDiff[k8sStepObjectSrc, k8sStepObjectDst]) SetObjectsToApply(objects []k8sStepObjectDst) {
	h.in.SetObjectsToApply(helper.ToSliceOfObject[k8sStepObjectDst, k8sStepObjectSrc](objects))
}

func (h *ObjectMultiPhaseDiff[k8sStepObjectSrc, k8sStepObjectDst]) AddObjectToApply(o k8sStepObjectDst) {
	h.in.AddObjectToApply(helper.ToObject[k8sStepObjectDst, k8sStepObjectSrc](o))
}

func (h *ObjectMultiPhaseDiff[k8sStepObjectSrc, k8sStepObjectDst]) GetObjectsToDelete() []k8sStepObjectDst {
	return helper.ToSliceOfObject[k8sStepObjectSrc, k8sStepObjectDst](h.in.GetObjectsToDelete())
}

func (h *ObjectMultiPhaseDiff[k8sStepObjectSrc, k8sStepObjectDst]) SetObjectsToDelete(objects []k8sStepObjectDst) {
	h.in.SetObjectsToDelete(helper.ToSliceOfObject[k8sStepObjectDst, k8sStepObjectSrc](objects))
}

func (h *ObjectMultiPhaseDiff[k8sStepObjectSrc, k8sStepObjectDst]) AddObjectToDelete(o k8sStepObjectDst) {
	h.in.AddObjectToDelete(helper.ToObject[k8sStepObjectDst, k8sStepObjectSrc](o))
}

func (h *ObjectMultiPhaseDiff[k8sStepObjectSrc, k8sStepObjectDst]) AddDiff(diff string) {
	h.in.AddDiff(diff)
}

func (h *ObjectMultiPhaseDiff[k8sStepObjectSrc, k8sStepObjectDst]) Diff() string {
	return h.in.Diff()
}

func (h *ObjectMultiPhaseDiff[k8sStepObjectSrc, k8sStepObjectDst]) IsDiff() bool {
	return h.in.IsDiff()
}
