package multiphase

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/k8s-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/helper"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/object"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	condition "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	k8sstrings "k8s.io/utils/strings"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseStepReconcilerAction is the interface that use by reconciler step to reconcile your intermediate K8s resources
type MultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] interface {
	controller.ReconcilerAction

	// Configure permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, o k8sObject, logger *logrus.Entry) (res ctrl.Result, err error)

	// Read permit to read kubernetes resources
	Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (read MultiPhaseRead[k8sStepObject], res ctrl.Result, err error)

	// Create permit to create resources on kubernetes
	Create(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObject, logger *logrus.Entry) (res ctrl.Result, err error)

	// Update permit to update resources on kubernetes
	Update(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObject, logger *logrus.Entry) (res ctrl.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObject, logger *logrus.Entry) (res ctrl.Result, err error)

	// OnError is call when error is throwing on current phase
	// It the right way to set status condition when error
	OnError(ctx context.Context, o k8sObject, data map[string]any, currentErr error, logger *logrus.Entry) (res ctrl.Result, err error)

	// OnSuccess is call at the end of current phase, if not error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObject], logger *logrus.Entry) (res ctrl.Result, err error)

	// Diff permit to compare the actual state and the expected state
	Diff(ctx context.Context, o k8sObject, read MultiPhaseRead[k8sStepObject], data map[string]any, logger *logrus.Entry, ignoreDiff ...patch.CalculateOption) (diff MultiPhaseDiff[k8sStepObject], res ctrl.Result, err error)

	// GetPhaseName permit to get the phase name
	GetPhaseName() shared.PhaseName

	GetIgnoresDiff() []patch.CalculateOption
}

// DefaultMultiPhaseStepReconcilerAction is the default implementation of MultiPhaseStepReconcilerAction
type DefaultMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] struct {
	controller.ReconcilerAction
	phaseName shared.PhaseName
}

// NewMultiPhaseStepReconcilerAction is the default implementation of MultiPhaseStepReconcilerAction interface
func NewMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object](client client.Client, phaseName shared.PhaseName, conditionName shared.ConditionName, recorder record.EventRecorder) (multiPhaseStepReconciler MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) {

	return &DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]{
		ReconcilerAction: controller.NewReconcilerAction(
			client,
			recorder,
			conditionName,
		),
		phaseName: phaseName,
	}
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) GetIgnoresDiff() []patch.CalculateOption {
	return make([]patch.CalculateOption, 0)
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) Configure(ctx context.Context, req ctrl.Request, o k8sObject, logger *logrus.Entry) (res ctrl.Result, err error) {
	conditions := o.GetStatus().GetConditions()

	// Init condition
	if condition.FindStatusCondition(conditions, h.Condition().String()) == nil {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:   h.Condition().String(),
			Status: metav1.ConditionFalse,
			Reason: "Initialize",
		})
	}

	// Init phase
	o.GetStatus().SetPhaseName(h.GetPhaseName())

	return res, nil
}
func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (read MultiPhaseRead[k8sStepObject], res ctrl.Result, err error) {
	panic("You need implement it")
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) Create(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObject, logger *logrus.Entry) (res ctrl.Result, err error) {

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

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) Update(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObject, logger *logrus.Entry) (res ctrl.Result, err error) {

	for _, oChild := range objects {
		if err = h.Client().Update(ctx, oChild); err != nil {
			return res, errors.Wrapf(err, "Error when update object '%s'", oChild.GetName())
		}
		logger.Debugf("Update object '%s' successfully", oChild.GetName())
		h.Recorder().Eventf(o, corev1.EventTypeNormal, "UpdateCompleted", "Object '%s' successfully updated", oChild.GetName())
	}

	return res, nil
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) Delete(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObject, logger *logrus.Entry) (res ctrl.Result, err error) {

	for _, oChild := range objects {
		if err = h.Client().Delete(ctx, oChild); err != nil {
			return res, errors.Wrapf(err, "Error when delete object '%s'", oChild.GetName())
		}
		logger.Debugf("Delete object '%s' successfully", oChild.GetName())
		h.Recorder().Eventf(o, corev1.EventTypeNormal, "DeleteCompleted", "Object '%s' successfully deleted", oChild.GetName())
	}

	return res, nil
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) OnError(ctx context.Context, o k8sObject, data map[string]any, currentErr error, logger *logrus.Entry) (res ctrl.Result, err error) {
	conditions := o.GetStatus().GetConditions()

	condition.SetStatusCondition(&conditions, metav1.Condition{
		Type:    h.Condition().String(),
		Status:  metav1.ConditionFalse,
		Reason:  "Failed",
		Message: k8sstrings.ShortenString(currentErr.Error(), controller.ShortenError),
	})

	h.Recorder().Event(o, corev1.EventTypeWarning, "ReconcilerStepActionError", k8sstrings.ShortenString(currentErr.Error(), controller.ShortenError))
	return res, currentErr

}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) OnSuccess(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObject], logger *logrus.Entry) (res ctrl.Result, err error) {
	conditions := o.GetStatus().GetConditions()

	// Update condition status if needed
	if !condition.IsStatusConditionPresentAndEqual(conditions, h.Condition().String(), metav1.ConditionTrue) {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:    h.Condition().String(),
			Reason:  "Success",
			Status:  metav1.ConditionTrue,
			Message: "Ready",
		})
	}

	return res, nil
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) Diff(ctx context.Context, o k8sObject, read MultiPhaseRead[k8sStepObject], data map[string]any, logger *logrus.Entry, ignoreDiff ...patch.CalculateOption) (diff MultiPhaseDiff[k8sStepObject], res ctrl.Result, err error) {

	tmpCurrentObjects := make([]k8sStepObject, len(read.GetCurrentObjects()))
	copy(tmpCurrentObjects, read.GetCurrentObjects())

	diff = NewMultiPhaseDiff[k8sStepObject]()

	patchOptions := []patch.CalculateOption{
		patch.CleanMetadata(),
		patch.IgnoreStatusFields(),
	}
	patchOptions = append(patchOptions, ignoreDiff...)

	for _, expectedObject := range read.GetExpectedObjects() {
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
					updatedObject := patchResult.Patched.(k8sStepObject)
					diff.AddDiff(fmt.Sprintf("diff %s: %s", updatedObject.GetName(), string(patchResult.Patch)))
					diff.AddObjectToUpdate(updatedObject)
					logger.Debugf("Need update object '%s'", updatedObject.GetName())
				}

				// Remove items found
				tmpCurrentObjects = helper.DeleteItemFromSlice(tmpCurrentObjects, i).([]k8sStepObject)

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

	// Need delete
	if len(tmpCurrentObjects) > 0 {
		diff.SetObjectsToDelete(tmpCurrentObjects)
		for _, object := range tmpCurrentObjects {
			diff.AddDiff(fmt.Sprintf("Need delete object '%s'", object.GetName()))
		}
	}

	return diff, res, nil
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) GetPhaseName() shared.PhaseName {
	return h.phaseName
}
