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
// This is the simple variant: it has no OnDiff hook and Diff always uses
// always-apply classification (all expected objects with current counterparts are applied).
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

	// Diff permit to compare the actual state and the expected state.
	// The simple variant always uses always-apply classification: all expected objects
	// with current counterparts go to the update list (legacy always-apply).
	Diff(ctx context.Context, o k8sObject, read MultiPhaseRead[k8sStepObject], data map[string]any, logger *logrus.Entry) (diff MultiPhaseDiff[k8sStepObject], res reconcile.Result, err error)

	// GetPhaseName permit to get the phase name
	GetPhaseName() shared.PhaseName
}

// MultiPhaseStepReconcilerActionWithDiff is the diff variant of MultiPhaseStepReconcilerAction.
// It embeds the base interface and adds OnDiff. Diff always uses SSA dry-run classification,
// so diff.NeedUpdate() / diff.GetObjectsToUpdate() are reliable in OnDiff.
type MultiPhaseStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] interface {
	MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]

	// OnDiff is called between Diff and Apply, after the diff has been computed.
	// The diff variant always uses SSA dry-run classification. The default implementation
	// returns nil (safe no-op); override it to run pre-apply tasks (e.g. drain before
	// StatefulSet update).
	OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObject], logger *logrus.Entry) (res reconcile.Result, err error)
}

// DefaultMultiPhaseStepReconcilerAction is the default implementation of MultiPhaseStepReconcilerAction
// (simple variant, always-apply classification).
type DefaultMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] struct {
	controller.ReconcilerAction
	phaseName    shared.PhaseName
	fieldManager string
}

// NewMultiPhaseStepReconcilerAction creates a new step reconciler action using Server-Side Apply.
// The fieldManager parameter identifies this controller as the owner of the fields it manages.
// This is the simple variant: Diff always uses always-apply classification and no OnDiff hook is available.
func NewMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object](client client.Client, phaseName shared.PhaseName, conditionName shared.ConditionName, recorder record.EventRecorder, fieldManager string) (multiPhaseStepReconciler MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) {
	return &DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]{
		ReconcilerAction: controller.NewReconcilerAction(
			client,
			recorder,
			conditionName,
		),
		phaseName:    phaseName,
		fieldManager: fieldManager,
	}
}

// DefaultMultiPhaseStepReconcilerActionWithDiff is the default implementation of MultiPhaseStepReconcilerActionWithDiff.
// It embeds the simple default action, overrides Diff to use SSA dry-run classification,
// and adds an OnDiff hook that returns nil by default.
type DefaultMultiPhaseStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] struct {
	*DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]
}

// NewMultiPhaseStepReconcilerActionWithDiff creates a new step reconciler action using Server-Side Apply.
// The fieldManager parameter identifies this controller as the owner of the fields it manages.
// This is the diff variant: Diff always uses SSA dry-run classification and OnDiff defaults to a safe no-op.
func NewMultiPhaseStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObject client.Object](client client.Client, phaseName shared.PhaseName, conditionName shared.ConditionName, recorder record.EventRecorder, fieldManager string) (multiPhaseStepReconciler MultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]) {
	return &DefaultMultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]{
		DefaultMultiPhaseStepReconcilerAction: NewMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject](
			client,
			phaseName,
			conditionName,
			recorder,
			fieldManager,
		).(*DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]),
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

		//nolint:staticcheck
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

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) Diff(ctx context.Context, o k8sObject, read MultiPhaseRead[k8sStepObject], data map[string]any, logger *logrus.Entry) (diff MultiPhaseDiff[k8sStepObject], res reconcile.Result, err error) {
	return h.classifyDiff(ctx, read, logger, false)
}

// classifyDiff classifies the read objects into create/update/delete lists using SSA.
// When dryRun is true the classification uses an SSA dry-run apply; otherwise it uses
// legacy always-apply classification.
func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) classifyDiff(ctx context.Context, read MultiPhaseRead[k8sStepObject], logger *logrus.Entry, dryRun bool) (diff MultiPhaseDiff[k8sStepObject], res reconcile.Result, err error) {
	diff = NewMultiPhaseDiff[k8sStepObject]()

	creates, updates, deletes, diffStrs, err := ClassifyObjects(ctx, h.Client(), read.GetExpectedObjects(), read.GetCurrentObjects(), h.fieldManager, dryRun)
	if err != nil {
		return diff, res, err
	}
	PopulateDiff(diff, creates, updates, deletes, diffStrs)

	return diff, res, nil
}

func (h *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]) GetPhaseName() shared.PhaseName {
	return h.phaseName
}

// As converts this step action to operate on a different client.Object subtype D.
func (h *DefaultMultiPhaseStepReconcilerAction[K, S]) As[D client.Object]() MultiPhaseStepReconcilerAction[K, D] {
	if h == nil {
		return nil
	}
	return &objectMultiPhaseStepReconcilerAction[K, S, D]{in: h, ReconcilerAction: h.ReconcilerAction}
}

func (h *DefaultMultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]) OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObject], logger *logrus.Entry) (res reconcile.Result, err error) {
	return res, nil
}

func (h *DefaultMultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]) Diff(ctx context.Context, o k8sObject, read MultiPhaseRead[k8sStepObject], data map[string]any, logger *logrus.Entry) (diff MultiPhaseDiff[k8sStepObject], res reconcile.Result, err error) {
	return h.classifyDiff(ctx, read, logger, true)
}

// AsWithDiff converts this diff-variant step action to operate on a different client.Object subtype D.
func (h *DefaultMultiPhaseStepReconcilerActionWithDiff[K, S]) AsWithDiff[D client.Object]() MultiPhaseStepReconcilerActionWithDiff[K, D] {
	if h == nil {
		return nil
	}
	return &objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]{in: h, ReconcilerAction: h.ReconcilerAction}
}

// objectMultiPhaseStepReconcilerAction wraps MultiPhaseStepReconcilerAction[K, S] as MultiPhaseStepReconcilerAction[K, D].
type objectMultiPhaseStepReconcilerAction[K object.MultiPhaseObject, S, D client.Object] struct {
	controller.ReconcilerAction
	in MultiPhaseStepReconcilerAction[K, S]
}

// As converts any MultiPhaseStepReconcilerAction[K, S] to operate on client.Object subtype D.
// It prefers the efficient .As[D]() generic method when the underlying type is
// *DefaultMultiPhaseStepReconcilerAction, falling back to a wrapper for custom implementations.
//
// This is the non-deprecated equivalent of NewObjectMultiPhaseStepReconcilerAction.
func As[K object.MultiPhaseObject, S, D client.Object](in MultiPhaseStepReconcilerAction[K, S]) MultiPhaseStepReconcilerAction[K, D] {
	if a, ok := in.(*DefaultMultiPhaseStepReconcilerAction[K, S]); ok {
		return a.As[D]()
	}
	if in == nil {
		return nil
	}
	return &objectMultiPhaseStepReconcilerAction[K, S, D]{
		in:               in,
		ReconcilerAction: controller.NewReconcilerAction(in.Client(), in.Recorder(), in.Condition()),
	}
}

// Deprecated: Use DefaultMultiPhaseStepReconcilerAction.As[D]() instead.
type ObjectMultiPhaseStepReconcilerAction[K object.MultiPhaseObject, S, D client.Object] = objectMultiPhaseStepReconcilerAction[K, S, D]

// Deprecated: Use As[K, S, D](in) instead.
func NewObjectMultiPhaseStepReconcilerAction[K object.MultiPhaseObject, S, D client.Object](in MultiPhaseStepReconcilerAction[K, S]) MultiPhaseStepReconcilerAction[K, D] {
	return As[K, S, D](in)
}

func (h *objectMultiPhaseStepReconcilerAction[K, S, D]) Configure(ctx context.Context, req reconcile.Request, o K, logger *logrus.Entry) (reconcile.Result, error) {
	return h.in.Configure(ctx, req, o, logger)
}

func (h *objectMultiPhaseStepReconcilerAction[K, S, D]) OnError(ctx context.Context, o K, data map[string]any, currentErr error, logger *logrus.Entry) (reconcile.Result, error) {
	return h.in.OnError(ctx, o, data, currentErr, logger)
}

func (h *objectMultiPhaseStepReconcilerAction[K, S, D]) GetPhaseName() shared.PhaseName {
	return h.in.GetPhaseName()
}

func (h *objectMultiPhaseStepReconcilerAction[K, S, D]) Read(ctx context.Context, o K, data map[string]any, logger *logrus.Entry) (MultiPhaseRead[D], reconcile.Result, error) {
	readTmp, res, err := h.in.Read(ctx, o, data, logger)
	return NewObjectMultiphaseRead[S, D](readTmp), res, err
}

func (h *objectMultiPhaseStepReconcilerAction[K, S, D]) Apply(ctx context.Context, o K, data map[string]any, objects []D, logger *logrus.Entry) (reconcile.Result, error) {
	return h.in.Apply(ctx, o, data, helper.ToSliceOfObject[D, S](objects), logger)
}

func (h *objectMultiPhaseStepReconcilerAction[K, S, D]) Delete(ctx context.Context, o K, data map[string]any, objects []D, logger *logrus.Entry) (reconcile.Result, error) {
	return h.in.Delete(ctx, o, data, helper.ToSliceOfObject[D, S](objects), logger)
}

func (h *objectMultiPhaseStepReconcilerAction[K, S, D]) OnSuccess(ctx context.Context, o K, data map[string]any, diff MultiPhaseDiff[D], logger *logrus.Entry) (reconcile.Result, error) {
	return h.in.OnSuccess(ctx, o, data, NewObjectMultiphaseDiff[D, S](diff), logger)
}

func (h *objectMultiPhaseStepReconcilerAction[K, S, D]) Diff(ctx context.Context, o K, read MultiPhaseRead[D], data map[string]any, logger *logrus.Entry) (MultiPhaseDiff[D], reconcile.Result, error) {
	diffTmp, res, err := h.in.Diff(ctx, o, NewObjectMultiphaseRead[D, S](read), data, logger)
	return NewObjectMultiphaseDiff[S, D](diffTmp), res, err
}

// objectMultiPhaseStepReconcilerActionWithDiff wraps MultiPhaseStepReconcilerActionWithDiff[K, S] as MultiPhaseStepReconcilerActionWithDiff[K, D].
type objectMultiPhaseStepReconcilerActionWithDiff[K object.MultiPhaseObject, S, D client.Object] struct {
	controller.ReconcilerAction
	in MultiPhaseStepReconcilerActionWithDiff[K, S]
}

// Deprecated: Use DefaultMultiPhaseStepReconcilerActionWithDiff.AsWithDiff[D]() instead.
type ObjectMultiPhaseStepReconcilerActionWithDiff[K object.MultiPhaseObject, S, D client.Object] = objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]

// AsWithDiff converts any MultiPhaseStepReconcilerActionWithDiff[K, S] to operate on
// client.Object subtype D.
// It prefers the efficient .AsWithDiff[D]() generic method when the underlying type is
// *DefaultMultiPhaseStepReconcilerActionWithDiff, falling back to a wrapper for custom
// implementations.
//
// This is the non-deprecated equivalent of NewObjectMultiPhaseStepReconcilerActionWithDiff.
func AsWithDiff[K object.MultiPhaseObject, S, D client.Object](in MultiPhaseStepReconcilerActionWithDiff[K, S]) MultiPhaseStepReconcilerActionWithDiff[K, D] {
	if a, ok := in.(*DefaultMultiPhaseStepReconcilerActionWithDiff[K, S]); ok {
		return a.AsWithDiff[D]()
	}
	if in == nil {
		return nil
	}
	return &objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]{
		in:               in,
		ReconcilerAction: controller.NewReconcilerAction(in.Client(), in.Recorder(), in.Condition()),
	}
}

// Deprecated: Use AsWithDiff[K, S, D](in) instead.
func NewObjectMultiPhaseStepReconcilerActionWithDiff[K object.MultiPhaseObject, S, D client.Object](in MultiPhaseStepReconcilerActionWithDiff[K, S]) MultiPhaseStepReconcilerActionWithDiff[K, D] {
	return AsWithDiff[K, S, D](in)
}

func (h *objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]) Configure(ctx context.Context, req reconcile.Request, o K, logger *logrus.Entry) (reconcile.Result, error) {
	return h.in.Configure(ctx, req, o, logger)
}

func (h *objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]) OnError(ctx context.Context, o K, data map[string]any, currentErr error, logger *logrus.Entry) (reconcile.Result, error) {
	return h.in.OnError(ctx, o, data, currentErr, logger)
}

func (h *objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]) GetPhaseName() shared.PhaseName {
	return h.in.GetPhaseName()
}

func (h *objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]) Read(ctx context.Context, o K, data map[string]any, logger *logrus.Entry) (MultiPhaseRead[D], reconcile.Result, error) {
	readTmp, res, err := h.in.Read(ctx, o, data, logger)
	return NewObjectMultiphaseRead[S, D](readTmp), res, err
}

func (h *objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]) Apply(ctx context.Context, o K, data map[string]any, objects []D, logger *logrus.Entry) (reconcile.Result, error) {
	return h.in.Apply(ctx, o, data, helper.ToSliceOfObject[D, S](objects), logger)
}

func (h *objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]) Delete(ctx context.Context, o K, data map[string]any, objects []D, logger *logrus.Entry) (reconcile.Result, error) {
	return h.in.Delete(ctx, o, data, helper.ToSliceOfObject[D, S](objects), logger)
}

func (h *objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]) OnSuccess(ctx context.Context, o K, data map[string]any, diff MultiPhaseDiff[D], logger *logrus.Entry) (reconcile.Result, error) {
	return h.in.OnSuccess(ctx, o, data, NewObjectMultiphaseDiff[D, S](diff), logger)
}

func (h *objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]) OnDiff(ctx context.Context, o K, data map[string]any, diff MultiPhaseDiff[D], logger *logrus.Entry) (reconcile.Result, error) {
	return h.in.OnDiff(ctx, o, data, NewObjectMultiphaseDiff[D, S](diff), logger)
}

func (h *objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]) Diff(ctx context.Context, o K, read MultiPhaseRead[D], data map[string]any, logger *logrus.Entry) (MultiPhaseDiff[D], reconcile.Result, error) {
	diffTmp, res, err := h.in.Diff(ctx, o, NewObjectMultiphaseRead[D, S](read), data, logger)
	return NewObjectMultiphaseDiff[S, D](diffTmp), res, err
}
