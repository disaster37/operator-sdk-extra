package multiphase

import (
	"context"
	"fmt"
	"strings"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/helper"
	ssadiff "github.com/disaster37/operator-sdk-extra/v3/pkg/helper/ssa"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseDiff is used to know if current resources differ with expected.
// Objects are classified into create, update, and delete lists.
type MultiPhaseDiff[k8sStepObject client.Object] interface {
	NeedCreate() bool
	NeedUpdate() bool
	NeedDelete() bool

	GetObjectsToCreate() []k8sStepObject
	SetObjectsToCreate(objects []k8sStepObject)
	AddObjectToCreate(o k8sStepObject)

	GetObjectsToUpdate() []k8sStepObject
	SetObjectsToUpdate(objects []k8sStepObject)
	AddObjectToUpdate(o k8sStepObject)

	GetObjectsToDelete() []k8sStepObject
	SetObjectsToDelete(objects []k8sStepObject)
	AddObjectToDelete(o k8sStepObject)

	// GetObjectsToApply returns the union of create and update lists for SSA apply.
	GetObjectsToApply() []k8sStepObject

	AddDiff(diff string)
	Diff() string
	IsDiff() bool
}

// DefaultMultiPhaseDiff is the default implementation of MultiPhaseDiff interface
type DefaultMultiPhaseDiff[k8sStepObject client.Object] struct {
	createObjects []k8sStepObject
	updateObjects []k8sStepObject
	deleteObjects []k8sStepObject
	diff          strings.Builder
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
	h.createObjects = append(h.createObjects, objects...)
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) AddObjectToCreate(o k8sStepObject) {
	h.createObjects = append(h.createObjects, o)
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) GetObjectsToUpdate() []k8sStepObject {
	return h.updateObjects
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) SetObjectsToUpdate(objects []k8sStepObject) {
	h.updateObjects = append(h.updateObjects, objects...)
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) AddObjectToUpdate(o k8sStepObject) {
	h.updateObjects = append(h.updateObjects, o)
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) GetObjectsToDelete() []k8sStepObject {
	return h.deleteObjects
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) SetObjectsToDelete(objects []k8sStepObject) {
	h.deleteObjects = append(h.deleteObjects, objects...)
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) AddObjectToDelete(o k8sStepObject) {
	h.deleteObjects = append(h.deleteObjects, o)
}

func (h *DefaultMultiPhaseDiff[k8sStepObject]) GetObjectsToApply() []k8sStepObject {
	all := make([]k8sStepObject, 0, len(h.createObjects)+len(h.updateObjects))
	all = append(all, h.createObjects...)
	all = append(all, h.updateObjects...)
	return all
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

// As converts this diff container to expose a different client.Object subtype D.
func (h *DefaultMultiPhaseDiff[S]) As[D client.Object]() MultiPhaseDiff[D] {
	if h == nil {
		return nil
	}
	return &objectMultiPhaseDiff[S, D]{in: h}
}

// objectMultiPhaseDiff wraps MultiPhaseDiff[Src] as MultiPhaseDiff[Dst].
type objectMultiPhaseDiff[Src, Dst client.Object] struct {
	in MultiPhaseDiff[Src]
}

// Deprecated: Use DefaultMultiPhaseDiff.As[D]() instead.
type ObjectMultiPhaseDiff[Src, Dst client.Object] = objectMultiPhaseDiff[Src, Dst]

// Deprecated: Use a concrete diff's .As[D]() method instead.
func NewObjectMultiphaseDiff[Src, Dst client.Object](in MultiPhaseDiff[Src]) MultiPhaseDiff[Dst] {
	return &objectMultiPhaseDiff[Src, Dst]{in: in}
}

func (h *objectMultiPhaseDiff[Src, Dst]) NeedCreate() bool          { return h.in.NeedCreate() }
func (h *objectMultiPhaseDiff[Src, Dst]) NeedUpdate() bool          { return h.in.NeedUpdate() }
func (h *objectMultiPhaseDiff[Src, Dst]) NeedDelete() bool          { return h.in.NeedDelete() }
func (h *objectMultiPhaseDiff[Src, Dst]) AddDiff(diff string)       { h.in.AddDiff(diff) }
func (h *objectMultiPhaseDiff[Src, Dst]) Diff() string              { return h.in.Diff() }
func (h *objectMultiPhaseDiff[Src, Dst]) IsDiff() bool              { return h.in.IsDiff() }

func (h *objectMultiPhaseDiff[Src, Dst]) GetObjectsToCreate() []Dst {
	return helper.ToSliceOfObject[Src, Dst](h.in.GetObjectsToCreate())
}

func (h *objectMultiPhaseDiff[Src, Dst]) SetObjectsToCreate(objects []Dst) {
	h.in.SetObjectsToCreate(helper.ToSliceOfObject[Dst, Src](objects))
}

func (h *objectMultiPhaseDiff[Src, Dst]) AddObjectToCreate(o Dst) {
	h.in.AddObjectToCreate(helper.ToObject[Dst, Src](o))
}

func (h *objectMultiPhaseDiff[Src, Dst]) GetObjectsToUpdate() []Dst {
	return helper.ToSliceOfObject[Src, Dst](h.in.GetObjectsToUpdate())
}

func (h *objectMultiPhaseDiff[Src, Dst]) SetObjectsToUpdate(objects []Dst) {
	h.in.SetObjectsToUpdate(helper.ToSliceOfObject[Dst, Src](objects))
}

func (h *objectMultiPhaseDiff[Src, Dst]) AddObjectToUpdate(o Dst) {
	h.in.AddObjectToUpdate(helper.ToObject[Dst, Src](o))
}

func (h *objectMultiPhaseDiff[Src, Dst]) GetObjectsToDelete() []Dst {
	return helper.ToSliceOfObject[Src, Dst](h.in.GetObjectsToDelete())
}

func (h *objectMultiPhaseDiff[Src, Dst]) SetObjectsToDelete(objects []Dst) {
	h.in.SetObjectsToDelete(helper.ToSliceOfObject[Dst, Src](objects))
}

func (h *objectMultiPhaseDiff[Src, Dst]) AddObjectToDelete(o Dst) {
	h.in.AddObjectToDelete(helper.ToObject[Dst, Src](o))
}

func (h *objectMultiPhaseDiff[Src, Dst]) GetObjectsToApply() []Dst {
	return helper.ToSliceOfObject[Src, Dst](h.in.GetObjectsToApply())
}

// ClassifyObjects compares expected and current objects, classifying them into
// create, update, and orphan (delete) lists. When dryRun is enabled, SSA dry-run
// is used to detect actual changes; unchanged objects are silently skipped.
//
// The algorithm is all-or-nothing: if any dry-run or diff computation fails,
// all preceding results are discarded and the error is returned. This prevents
// partial diffs that would cause misapplied resources.
func ClassifyObjects[T client.Object](
	ctx context.Context,
	c client.Client,
	expectedObjects []T,
	currentObjects []T,
	fieldManager string,
	dryRun bool,
) (creates []T, updates []T, deletes []T, diffStrs []string, err error) {
	// Build a set of current object names for orphan detection.
	// Items are removed from this set as they are matched against expected objects;
	// what remains after the loop are orphans to delete.
	unmatchedCurrent := make(map[string]T)
	for _, o := range currentObjects {
		unmatchedCurrent[o.GetName()] = o
	}

	for _, expected := range expectedObjects {
		current, exists := unmatchedCurrent[expected.GetName()]
		delete(unmatchedCurrent, expected.GetName())

		if !exists {
			creates = append(creates, expected)
			diffStrs = append(diffStrs, fmt.Sprintf("Create object '%s'", expected.GetName()))
			continue
		}

		if !dryRun {
			updates = append(updates, expected)
			diffStrs = append(diffStrs, fmt.Sprintf("Apply object '%s'", expected.GetName()))
			continue
		}

		predicted, dryErr := ssadiff.DryRunApply(ctx, c, expected, fieldManager)
		if dryErr != nil {
			return nil, nil, nil, nil, errors.Wrapf(dryErr, "Error when dry-run apply object '%s'", expected.GetName())
		}

		changed, human, dErr := ssadiff.IsObjectDiff(current, predicted)
		if dErr != nil {
			return nil, nil, nil, nil, errors.Wrapf(dErr, "Error when computing diff for object '%s'", expected.GetName())
		}

		if changed {
			updates = append(updates, expected)
			diffStrs = append(diffStrs, fmt.Sprintf("Update object '%s':\n%s", expected.GetName(), human))
		}
	}

	for _, o := range unmatchedCurrent {
		deletes = append(deletes, o)
		diffStrs = append(diffStrs, fmt.Sprintf("Need delete object '%s'", o.GetName()))
	}

	return creates, updates, deletes, diffStrs, nil
}

// PopulateDiff fills a MultiPhaseDiff from ClassifyObjects results.
func PopulateDiff[T client.Object](diff MultiPhaseDiff[T], creates, updates, deletes []T, diffStrs []string) {
	for _, o := range creates {
		diff.AddObjectToCreate(o)
	}
	for _, o := range updates {
		diff.AddObjectToUpdate(o)
	}
	for _, o := range deletes {
		diff.AddObjectToDelete(o)
	}
	for _, s := range diffStrs {
		diff.AddDiff(s)
	}
}
