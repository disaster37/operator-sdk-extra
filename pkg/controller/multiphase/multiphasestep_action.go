package multiphase

import (
	"context"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/helper"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	condition "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MultiPhaseStepReconcilerAction is the interface that use by reconciler step to reconcile your intermediate K8s resources
// It uses Server-Side Apply (SSA) to manage child resources.
type MultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] interface {
	controller.ReconcilerAction

	// Configure permit to init condition on status
	Configure(ctx context.Context, req reconcile.Request, o k8sObject, logger *logrus.Entry) (res reconcile.Result, err error)

	// Read permit to read kubernetes resources
	Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (read MultiPhaseRead[k8sStepObject], res reconcile.Result, err error)

	// Apply permit to apply resources on kubernetes using Server-Side Apply
	Apply(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObject, logger *logrus.Entry) (res reconcile.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObject, logger *logrus.Entry) (res reconcile.Result, err error)

	// OnError is call when error is throwing on current phase
	// It the right way to set status condition when error
	OnError(ctx context.Context, o k8sObject, data map[string]any, currentErr error, logger *logrus.Entry) (res reconcile.Result, err error)

	// OnSuccess is call at the end of current phase, if not error
	// It's the right way to set status condition when everything is good
	OnSuccess(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObject], logger *logrus.Entry) (res reconcile.Result, err error)

	// OnDiff is called between Diff and Apply, after the diff has been computed.
	// When dry-run diff detection is disabled, the default implementation returns ErrDiffDisabled.
	// Controllers that need pre-apply tasks (e.g. drain before StatefulSet update) can check
	// diff.NeedUpdate() / diff.GetObjectsToUpdate() here.
	OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObject], logger *logrus.Entry) (res reconcile.Result, err error)

	// Diff permit to compare the actual state and the expected state
	// When dryRun is enabled, uses SSA dry-run to classify objects into create/update/delete.
	// When dryRun is disabled, all expected objects go to the update list (legacy always-apply).
	Diff(ctx context.Context, o k8sObject, read MultiPhaseRead[k8sStepObject], data map[string]any, logger *logrus.Entry) (diff MultiPhaseDiff[k8sStepObject], res reconcile.Result, err error)

	// GetPhaseName permit to get the phase name
	GetPhaseName() shared.PhaseName
}

// DefaultMultiPhaseStepReconcilerAction is the default implementation of MultiPhaseStepReconcilerAction
type DefaultMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] struct {
	controller.ReconcilerAction
	phaseName    shared.PhaseName
	fieldManager string
	dryRun       bool
}

// NewMultiPhaseStepReconcilerAction creates a new step reconciler action using Server-Side Apply.
// The fieldManager parameter identifies this controller as the owner of the fields it manages.
// The dryRun parameter enables SSA dry-run diff detection (one extra API call per existing object).
func NewMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object](client client.Client, phaseName shared.PhaseName, conditionName shared.ConditionName, recorder record.EventRecorder, fieldManager string, dryRun bool) (multiPhaseStepReconciler MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) {
	return &DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]{
		ReconcilerAction: controller.NewReconcilerAction(
			client,
			recorder,
			conditionName,
		),
		phaseName:    phaseName,
		fieldManager: fieldManager,
		dryRun:       dryRun,
	}
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) Configure(ctx context.Context, req reconcile.Request, o k8sObject, logger *logrus.Entry) (res reconcile.Result, err error) {
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

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (read MultiPhaseRead[k8sStepObject], res reconcile.Result, err error) {
	panic("You need implement it")
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) Apply(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObject, logger *logrus.Entry) (res reconcile.Result, err error) {
	// Note: Apply mutates diff objects in-place (clears managedFields and resourceVersion).
	// Callers must not reuse objects from GetObjectsToApply() after calling Apply.
	for _, oChild := range objects {
		// Set owner reference
		if err = ctrl.SetControllerReference(o, oChild, h.Client().Scheme()); err != nil {
			return res, errors.Wrapf(err, "Error when set owner reference on object '%s'", oChild.GetName())
		}

		// Clear fields that are not compatible with SSA apply objects
		oChild.SetManagedFields(nil)
		oChild.SetResourceVersion("")

		if err = h.Client().Patch(ctx, oChild, client.Apply, client.FieldOwner(h.fieldManager), client.ForceOwnership); err != nil {
			return res, errors.Wrapf(err, "Error when apply object '%s'", oChild.GetName())
		}
		logger.Debugf("Apply object '%s' successfully", oChild.GetName())
		h.Recorder().Eventf(o, corev1.EventTypeNormal, "ApplyCompleted", "Object '%s' successfully applied", oChild.GetName())
	}

	return res, nil
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) Delete(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObject, logger *logrus.Entry) (res reconcile.Result, err error) {
	for _, oChild := range objects {
		if err = h.Client().Delete(ctx, oChild); err != nil {
			return res, errors.Wrapf(err, "Error when delete object '%s'", oChild.GetName())
		}
		logger.Debugf("Delete object '%s' successfully", oChild.GetName())
		h.Recorder().Eventf(o, corev1.EventTypeNormal, "DeleteCompleted", "Object '%s' successfully deleted", oChild.GetName())
	}

	return res, nil
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) OnError(ctx context.Context, o k8sObject, data map[string]any, currentErr error, logger *logrus.Entry) (res reconcile.Result, err error) {
	conditions := o.GetStatus().GetConditions()

	condition.SetStatusCondition(&conditions, metav1.Condition{
		Type:    h.Condition().String(),
		Status:  metav1.ConditionFalse,
		Reason:  "Failed",
		Message: controller.UserFacingError(currentErr, controller.MaxConditionMessage),
	})

	h.Recorder().Event(o, corev1.EventTypeWarning, "ReconcilerStepActionError", controller.UserFacingError(currentErr, controller.MaxEventMessage))
	return res, currentErr
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) OnSuccess(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObject], logger *logrus.Entry) (res reconcile.Result, err error) {
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

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObject], logger *logrus.Entry) (res reconcile.Result, err error) {
	if !h.dryRun {
		return res, controller.ErrDiffDisabled
	}
	return res, nil
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) Diff(ctx context.Context, o k8sObject, read MultiPhaseRead[k8sStepObject], data map[string]any, logger *logrus.Entry) (diff MultiPhaseDiff[k8sStepObject], res reconcile.Result, err error) {
	diff = NewMultiPhaseDiff[k8sStepObject]()

	creates, updates, deletes, diffStrs, err := ClassifyObjects(ctx, h.Client(), read.GetExpectedObjects(), read.GetCurrentObjects(), h.fieldManager, h.dryRun)
	if err != nil {
		return diff, res, err
	}
	PopulateDiff(diff, creates, updates, deletes, diffStrs)

	return diff, res, nil
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) GetPhaseName() shared.PhaseName {
	return h.phaseName
}

// ObjectMultiPhaseStepReconcilerAction is the implementation of MultiPhaseStepReconcilerAction for a specific client.Object type needed by multiphase reconciler
// It's kind of wrapper to convert MultiPhaseStepReconcilerAction[k8sStepObject] to MultiPhaseStepReconcilerAction[client.Object]
type ObjectMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object] struct {
	controller.ReconcilerAction
	in MultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc]
}

func NewObjectMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object](in MultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc]) MultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectDst] {
	return &ObjectMultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc, k8sStepObjectDst]{
		in: in,
		ReconcilerAction: controller.NewReconcilerAction(
			in.Client(),
			in.Recorder(),
			in.Condition(),
		),
	}
}

func (h *ObjectMultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc, k8sStepObjectDst]) Configure(ctx context.Context, req reconcile.Request, o k8sObject, logger *logrus.Entry) (res reconcile.Result, err error) {
	return h.in.Configure(ctx, req, o, logger)
}

func (h *ObjectMultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc, k8sStepObjectDst]) Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (read MultiPhaseRead[k8sStepObjectDst], res reconcile.Result, err error) {
	readTmp, res, err := h.in.Read(ctx, o, data, logger)
	return NewObjectMultiphaseRead[k8sStepObjectSrc, k8sStepObjectDst](readTmp), res, err
}

func (h *ObjectMultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc, k8sStepObjectDst]) Apply(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObjectDst, logger *logrus.Entry) (res reconcile.Result, err error) {
	return h.in.Apply(ctx, o, data, helper.ToSliceOfObject[k8sStepObjectDst, k8sStepObjectSrc](objects), logger)
}

func (h *ObjectMultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc, k8sStepObjectDst]) Delete(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObjectDst, logger *logrus.Entry) (res reconcile.Result, err error) {
	return h.in.Delete(ctx, o, data, helper.ToSliceOfObject[k8sStepObjectDst, k8sStepObjectSrc](objects), logger)
}

func (h *ObjectMultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc, k8sStepObjectDst]) OnError(ctx context.Context, o k8sObject, data map[string]any, currentErr error, logger *logrus.Entry) (res reconcile.Result, err error) {
	return h.in.OnError(ctx, o, data, currentErr, logger)
}

func (h *ObjectMultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc, k8sStepObjectDst]) OnSuccess(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObjectDst], logger *logrus.Entry) (res reconcile.Result, err error) {
	return h.in.OnSuccess(ctx, o, data, NewObjectMultiphaseDiff[k8sStepObjectDst, k8sStepObjectSrc](diff), logger)
}

func (h *ObjectMultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc, k8sStepObjectDst]) OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObjectDst], logger *logrus.Entry) (res reconcile.Result, err error) {
	return h.in.OnDiff(ctx, o, data, NewObjectMultiphaseDiff[k8sStepObjectDst, k8sStepObjectSrc](diff), logger)
}

func (h *ObjectMultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc, k8sStepObjectDst]) Diff(ctx context.Context, o k8sObject, read MultiPhaseRead[k8sStepObjectDst], data map[string]any, logger *logrus.Entry) (diff MultiPhaseDiff[k8sStepObjectDst], res reconcile.Result, err error) {
	diffTmp, res, err := h.in.Diff(ctx, o, NewObjectMultiphaseRead[k8sStepObjectDst, k8sStepObjectSrc](read), data, logger)
	return NewObjectMultiphaseDiff[k8sStepObjectSrc, k8sStepObjectDst](diffTmp), res, err
}

func (h *ObjectMultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc, k8sStepObjectDst]) GetPhaseName() shared.PhaseName {
	return h.in.GetPhaseName()
}
