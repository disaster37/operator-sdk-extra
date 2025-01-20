package sentinel

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/k8s-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/multiphase"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/helper"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	k8sstrings "k8s.io/utils/strings"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SentinelReconcilerAction is the interface that use by sentinel reconciler
// Put logger param on each function, permit to set contextual fields like namespace and object name, object type
type SentinelReconcilerAction[k8sObject client.Object] interface {
	controller.ReconcilerAction

	// Confirgure permit to init external provider driver (API client REST)
	// It can also permit to init condition on status
	Configure(ctx context.Context, req reconcile.Request, o k8sObject, data map[string]any, logger *logrus.Entry) (res reconcile.Result, err error)

	// Read permit to read the actual resource state from provider and set it on data map
	Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (read SentinelRead, res reconcile.Result, err error)

	// Create permit to create resource on provider
	// It only call if diff.NeeCreated is true
	Create(ctx context.Context, o k8sObject, data map[string]any, objects []client.Object, logger *logrus.Entry) (res reconcile.Result, err error)

	// Update permit to update resource on provider
	// It only call if diff.NeedUpdated is true
	Update(ctx context.Context, o k8sObject, data map[string]any, objects []client.Object, logger *logrus.Entry) (res reconcile.Result, err error)

	// Delete permit to delete resource on provider
	// It only call if you have specified finalizer name when you create reconciler and if resource as marked to be deleted
	Delete(ctx context.Context, o k8sObject, data map[string]any, objects []client.Object, logger *logrus.Entry) (err error)

	// OnError is call when error is throwing
	// It the right way to set status condition when error
	OnError(ctx context.Context, o k8sObject, data map[string]any, currentErr error, logger *logrus.Entry) (res reconcile.Result, err error)

	// OnSuccess is call at the end if no error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, o k8sObject, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (res reconcile.Result, err error)

	// Diff permit to compare the actual state and the expected state
	Diff(ctx context.Context, o k8sObject, read SentinelRead, data map[string]any, logger *logrus.Entry, ignoreDiff ...patch.CalculateOption) (diff multiphase.MultiPhaseDiff[client.Object], res reconcile.Result, err error)

	GetIgnoresDiff() []patch.CalculateOption
}

// DefaultSentinelAction is the default implementation of SentinelAction
type DefaultSentinelAction[k8sObject client.Object] struct {
	controller.ReconcilerAction
}

// NewSentinelAction is the default implementation of SentinelReconcilerAction interface
func NewSentinelAction[k8sObject client.Object](client client.Client, recorder record.EventRecorder) (sentinelReconciler SentinelReconcilerAction[k8sObject]) {
	return &DefaultSentinelAction[k8sObject]{
		ReconcilerAction: controller.NewReconcilerAction(client, recorder, controller.ReadyCondition),
	}
}

func (h *DefaultSentinelAction[k8sObject]) Configure(ctx context.Context, req reconcile.Request, o k8sObject, data map[string]any, logger *logrus.Entry) (res reconcile.Result, err error) {
	return res, nil
}

func (h *DefaultSentinelAction[k8sObject]) Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (read SentinelRead, res reconcile.Result, err error) {
	panic("You need implement it")
}

func (h *DefaultSentinelAction[k8sObject]) Create(ctx context.Context, o k8sObject, data map[string]any, objects []client.Object, logger *logrus.Entry) (res reconcile.Result, err error) {

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
func (h *DefaultSentinelAction[k8sObject]) Update(ctx context.Context, o k8sObject, data map[string]any, objects []client.Object, logger *logrus.Entry) (res reconcile.Result, err error) {

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
func (h *DefaultSentinelAction[k8sObject]) Delete(ctx context.Context, o k8sObject, data map[string]any, objects []client.Object, logger *logrus.Entry) (err error) {

	for _, oChild := range objects {
		if err = h.Client().Delete(ctx, oChild); err != nil {
			return errors.Wrapf(err, "Error when delete object '%s'", oChild.GetName())
		}
		logger.Debugf("Delete object '%s' successfully", oChild.GetName())
		h.Recorder().Eventf(o, corev1.EventTypeNormal, "DeleteCompleted", "Object '%s' successfully deleted", oChild.GetName())
	}

	return nil
}

func (h *DefaultSentinelAction[k8sObject]) OnError(ctx context.Context, o k8sObject, data map[string]any, currentErr error, logger *logrus.Entry) (res reconcile.Result, err error) {
	h.Recorder().Event(o, corev1.EventTypeWarning, "SentinelActionError", k8sstrings.ShortenString(currentErr.Error(), controller.ShortenError))
	return res, currentErr
}

func (h *DefaultSentinelAction[k8sObject]) OnSuccess(ctx context.Context, o k8sObject, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (res reconcile.Result, err error) {
	return res, nil
}

func (h *DefaultSentinelAction[k8sObject]) Diff(ctx context.Context, o k8sObject, read SentinelRead, data map[string]any, logger *logrus.Entry, ignoreDiff ...patch.CalculateOption) (diff multiphase.MultiPhaseDiff[client.Object], res reconcile.Result, err error) {

	diff = multiphase.NewMultiPhaseDiff[client.Object]()

	patchOptions := []patch.CalculateOption{
		patch.CleanMetadata(),
		patch.IgnoreStatusFields(),
	}
	patchOptions = append(patchOptions, ignoreDiff...)

	// Compare the expected and current objects type
	for objectType, reader := range read.GetReads() {
		logger.Debugf("Start process object type '%s'", objectType)

		tmpCurrentObjects := make([]client.Object, len(reader.GetCurrentObjects()))
		copy(tmpCurrentObjects, reader.GetCurrentObjects())

		for _, expectedObject := range reader.GetExpectedObjects() {
			isFound := false
			for i, currentObject := range tmpCurrentObjects {
				// Need compare same object
				if currentObject.GetName() == expectedObject.GetName() {
					isFound = true

					// Copy TypeMeta to work with some ignore rules like IgnorePDBSelector()
					controller.MustInjectTypeMeta(currentObject, expectedObject)
					patchResult, err := patch.DefaultPatchMaker.Calculate(currentObject, expectedObject, patchOptions...)
					if err != nil {
						return diff, res, errors.Wrapf(err, "Error when diffing object '%s'", currentObject.GetName())
					}
					if !patchResult.IsEmpty() {
						updatedObject := patchResult.Patched.(client.Object)
						diff.AddDiff(fmt.Sprintf("diff %s: %s", updatedObject.GetName(), string(patchResult.Patch)))
						diff.AddObjectToUpdate(updatedObject)
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
				diff.AddObjectToCreate(expectedObject)

				logger.Debugf("Need create object '%s'", expectedObject.GetName())
			}
		}

		if len(tmpCurrentObjects) > 0 {
			diff.SetObjectsToDelete(tmpCurrentObjects)
			for _, object := range tmpCurrentObjects {
				diff.AddDiff(fmt.Sprintf("Need delete object '%s'", object.GetName()))
			}
		}
	}

	return diff, res, nil
}

func (h *DefaultSentinelAction[k8sObject]) GetIgnoresDiff() []patch.CalculateOption {
	return make([]patch.CalculateOption, 0)
}
