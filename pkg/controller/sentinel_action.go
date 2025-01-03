package controller

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/k8s-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/pkg/helper"
	"github.com/sirupsen/logrus"
	"github.com/thoas/go-funk"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	k8sstrings "k8s.io/utils/strings"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SentinelAction is the interface that use by sentinel reconciler
// Put logger param on each function, permit to set contextual fields like namespace and object name, object type
type SentinelReconcilerAction interface {
	BaseReconciler

	// Confirgure permit to init external provider driver (API client REST)
	// It can also permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, o client.Object, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error)

	// Read permit to read the actual resource state from provider and set it on data map
	Read(ctx context.Context, o client.Object, data map[string]any, logger *logrus.Entry) (read SentinelRead, res ctrl.Result, err error)

	// Create permit to create resource on provider
	// It only call if diff.NeeCreated is true
	Create(ctx context.Context, o client.Object, data map[string]any, objects []client.Object, logger *logrus.Entry) (res ctrl.Result, err error)

	// Update permit to update resource on provider
	// It only call if diff.NeedUpdated is true
	Update(ctx context.Context, o client.Object, data map[string]any, objects []client.Object, logger *logrus.Entry) (res ctrl.Result, err error)

	// Delete permit to delete resource on provider
	// It only call if you have specified finalizer name when you create reconciler and if resource as marked to be deleted
	Delete(ctx context.Context, o client.Object, data map[string]any, objects []client.Object, logger *logrus.Entry) (err error)

	// OnError is call when error is throwing
	// It the right way to set status condition when error
	OnError(ctx context.Context, o client.Object, data map[string]any, currentErr error, logger *logrus.Entry) (res ctrl.Result, err error)

	// OnSuccess is call at the end if no error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, o client.Object, data map[string]any, diff SentinelDiff, logger *logrus.Entry) (res ctrl.Result, err error)

	// Diff permit to compare the actual state and the expected state
	Diff(ctx context.Context, o client.Object, read SentinelRead, data map[string]any, logger *logrus.Entry, ignoreDiff ...patch.CalculateOption) (diff SentinelDiff, res ctrl.Result, err error)

	GetIgnoresDiff() []patch.CalculateOption
}

// BasicSentinelAction is the basic implementation of SentinelAction
type BasicSentinelAction struct {
	BasicReconcilerAction
}

// NewRemoteReconcilerAction is the basic constructor of RemoteReconcilerAction interface
func NewBasicSentinelAction(client client.Client, recorder record.EventRecorder) (sentinelReconciler SentinelReconcilerAction) {
	return &BasicSentinelAction{
		BasicReconcilerAction: NewBasicReconcilerAction(client, recorder, ReadyCondition),
	}
}

func (h *BasicSentinelAction) Configure(ctx context.Context, req ctrl.Request, o client.Object, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error) {
	return res, nil
}

func (h *BasicSentinelAction) Read(ctx context.Context, o client.Object, data map[string]any, logger *logrus.Entry) (read SentinelRead, res ctrl.Result, err error) {
	panic("You need implement it")
}

func (h *BasicSentinelAction) Create(ctx context.Context, o client.Object, data map[string]any, objects []client.Object, logger *logrus.Entry) (res ctrl.Result, err error) {

	for _, oChild := range objects {

		// Set owner
		err = ctrl.SetControllerReference(o, oChild, h.Client().Scheme())
		if err != nil {
			return res, errors.Wrapf(err, "Error when set owner reference on object '%s'", oChild.GetName())
		}

		// Set diff 3-way annotations
		if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(oChild); err != nil {
			return res, errors.Wrapf(err, "Error when set annotation for 3-way diff on  object '%s'", oChild.GetName())
		}

		if err = h.Client().Create(ctx, oChild); err != nil {
			return res, errors.Wrapf(err, "Error when create object '%s'", oChild.GetName())
		}
		logger.Debugf("Create object '%s' successfully", oChild.GetName())
		h.Recorder().Eventf(o, corev1.EventTypeNormal, "CreateCompleted", "Object '%s' successfully created", oChild.GetName())
	}

	return res, nil
}

// Update can be call on your own version
// It only add some log / events
func (h *BasicSentinelAction) Update(ctx context.Context, o client.Object, data map[string]any, objects []client.Object, logger *logrus.Entry) (res ctrl.Result, err error) {

	for _, oChild := range objects {
		if err = h.Client().Update(ctx, oChild); err != nil {
			return res, errors.Wrapf(err, "Error when update object '%s'", oChild.GetName())
		}
		logger.Debugf("Update object '%s' successfully", oChild.GetName())
		h.Recorder().Eventf(o, corev1.EventTypeNormal, "UpdateCompleted", "Object '%s' successfully updated", oChild.GetName())
	}

	return res, nil
}

// Delete delete objects
func (h *BasicSentinelAction) Delete(ctx context.Context, o client.Object, data map[string]any, objects []client.Object, logger *logrus.Entry) (err error) {

	for _, oChild := range objects {
		if err = h.Client().Delete(ctx, oChild); err != nil {
			return errors.Wrapf(err, "Error when delete object '%s'", oChild.GetName())
		}
		logger.Debugf("Delete object '%s' successfully", oChild.GetName())
		h.Recorder().Eventf(o, corev1.EventTypeNormal, "DeleteCompleted", "Object '%s' successfully deleted", oChild.GetName())
	}

	return nil
}

func (h *BasicSentinelAction) OnError(ctx context.Context, o client.Object, data map[string]any, currentErr error, logger *logrus.Entry) (res ctrl.Result, err error) {
	h.Recorder().Event(o, corev1.EventTypeWarning, "SentinelActionError", k8sstrings.ShortenString(currentErr.Error(), ShortenError))
	return res, currentErr
}

func (h *BasicSentinelAction) OnSuccess(ctx context.Context, o client.Object, data map[string]any, diff SentinelDiff, logger *logrus.Entry) (res ctrl.Result, err error) {
	return res, nil
}

func (h *BasicSentinelAction) Diff(ctx context.Context, o client.Object, read SentinelRead, data map[string]any, logger *logrus.Entry, ignoreDiff ...patch.CalculateOption) (diff SentinelDiff, res ctrl.Result, err error) {

	diff = NewBasicSentinelDiff()

	patchOptions := []patch.CalculateOption{
		patch.CleanMetadata(),
		patch.IgnoreStatusFields(),
	}
	patchOptions = append(patchOptions, ignoreDiff...)

	toUpdate := make([]client.Object, 0)
	toCreate := make([]client.Object, 0)
	toDelete := make([]client.Object, 0)

	// Compare the expected and current objects type
	objectTypes := funk.Uniq(funk.Union(funk.Keys(read.GetAllCurrentObjects()), funk.Keys(read.GetAllExpectedObjects())).([]string)).([]string)
	for _, objectType := range objectTypes {
		logger.Debugf("Start process object type '%s'", objectType)
		tmpCurrentObjects := make([]client.Object, len(read.GetCurrentObjects(objectType)))
		copy(tmpCurrentObjects, read.GetCurrentObjects(objectType))

		for _, expectedObject := range read.GetExpectedObjects(objectType) {
			isFound := false
			for i, currentObject := range tmpCurrentObjects {
				// Need compare same object
				if currentObject.GetName() == expectedObject.GetName() {
					isFound = true

					// Copy TypeMeta to work with some ignore rules like IgnorePDBSelector()
					mustInjectTypeMeta(currentObject, expectedObject)
					patchResult, err := patch.DefaultPatchMaker.Calculate(currentObject, expectedObject, patchOptions...)
					if err != nil {
						return diff, res, errors.Wrapf(err, "Error when diffing object '%s'", currentObject.GetName())
					}
					if !patchResult.IsEmpty() {
						updatedObject := patchResult.Patched.(client.Object)
						diff.AddDiff(fmt.Sprintf("diff %s: %s", updatedObject.GetName(), string(patchResult.Patch)))
						toUpdate = append(toUpdate, updatedObject)
						logger.Debugf("Need update object '%s'", updatedObject.GetName())
					}

					// Remove items found
					tmpCurrentObjects = helper.DeleteItemFromSlice(tmpCurrentObjects, i).([]client.Object)

					break
				}
			}

			if !isFound {
				// Need create object
				diff.AddDiff(fmt.Sprintf("Need Create object '%s'", expectedObject.GetName()))

				toCreate = append(toCreate, expectedObject)

				logger.Debugf("Need create object '%s'", expectedObject.GetName())
			}
		}

		if len(tmpCurrentObjects) > 0 {
			for _, object := range tmpCurrentObjects {
				diff.AddDiff(fmt.Sprintf("Need delete object '%s'", object.GetName()))
			}
		}

		toDelete = append(toDelete, tmpCurrentObjects...)

	}

	diff.SetObjectsToCreate(toCreate)
	diff.SetObjectsToUpdate(toUpdate)
	diff.SetObjectsToDelete(toDelete)

	return diff, res, nil
}

func (h *BasicSentinelAction) GetIgnoresDiff() []patch.CalculateOption {
	return make([]patch.CalculateOption, 0)
}
