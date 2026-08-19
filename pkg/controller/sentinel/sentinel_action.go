package sentinel

import (
	"context"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/multiphase"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SentinelReconcilerAction is the interface that use by sentinel reconciler
// It uses Server-Side Apply to manage child resources.
type SentinelReconcilerAction[k8sObject client.Object] interface {
	controller.ReconcilerAction

	// Configure permit to init external provider driver (API client REST)
	// It can also permit to init condition on status
	Configure(ctx context.Context, req reconcile.Request, o k8sObject, data map[string]any, logger *logrus.Entry) (res reconcile.Result, err error)

	// Read permit to read the actual resource state from provider and set it on data map
	Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (read SentinelRead, res reconcile.Result, err error)

	// Apply permit to apply resources on kubernetes using Server-Side Apply
	Apply(ctx context.Context, o k8sObject, data map[string]any, objects []client.Object, logger *logrus.Entry) (res reconcile.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, o k8sObject, data map[string]any, objects []client.Object, logger *logrus.Entry) (res reconcile.Result, err error)

	// OnError is call when error is throwing
	// It the right way to set status condition when error
	OnError(ctx context.Context, o k8sObject, data map[string]any, currentErr error, logger *logrus.Entry) (res reconcile.Result, err error)

	// OnSuccess is call at the end if no error
	// It's the right way to set status condition when everything is good
	OnSuccess(ctx context.Context, o k8sObject, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (res reconcile.Result, err error)

	// OnDiff is called between Diff and Apply, after the diff has been computed.
	// When dry-run diff detection is disabled, the default implementation returns ErrDiffDisabled.
	// Controllers that need pre-apply tasks can check diff.NeedUpdate() / diff.GetObjectsToUpdate() here.
	OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (res reconcile.Result, err error)

	// Diff permit to compare the actual state and the expected state
	// When dryRun is enabled, uses SSA dry-run to classify objects into create/update/delete.
	// When dryRun is disabled, all expected objects go to the update list (legacy always-apply).
	Diff(ctx context.Context, o k8sObject, read SentinelRead, data map[string]any, logger *logrus.Entry) (diff multiphase.MultiPhaseDiff[client.Object], res reconcile.Result, err error)

	// GetFieldManager returns the field manager name for SSA
	GetFieldManager() string
}

// DefaultSentinelAction is the default implementation of SentinelAction
type DefaultSentinelAction[k8sObject client.Object] struct {
	controller.ReconcilerAction
	fieldManager string
	dryRun       bool
}

// NewSentinelAction creates a new sentinel action using Server-Side Apply.
// The fieldManager parameter identifies this controller as the owner of the fields it manages.
// The dryRun parameter enables SSA dry-run diff detection (one extra API call per existing object).
func NewSentinelAction[k8sObject client.Object](client client.Client, recorder record.EventRecorder, fieldManager string, dryRun bool) (sentinelReconciler SentinelReconcilerAction[k8sObject]) {
	return &DefaultSentinelAction[k8sObject]{
		ReconcilerAction: controller.NewReconcilerAction(client, recorder, controller.ReadyCondition),
		fieldManager:     fieldManager,
		dryRun:           dryRun,
	}
}

func (h *DefaultSentinelAction[k8sObject]) GetFieldManager() string {
	return h.fieldManager
}

func (h *DefaultSentinelAction[k8sObject]) Configure(ctx context.Context, req reconcile.Request, o k8sObject, data map[string]any, logger *logrus.Entry) (res reconcile.Result, err error) {
	return res, nil
}

func (h *DefaultSentinelAction[k8sObject]) Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (read SentinelRead, res reconcile.Result, err error) {
	panic("You need implement it")
}

func (h *DefaultSentinelAction[k8sObject]) Apply(ctx context.Context, o k8sObject, data map[string]any, objects []client.Object, logger *logrus.Entry) (res reconcile.Result, err error) {
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

// Delete deletes objects
func (h *DefaultSentinelAction[k8sObject]) Delete(ctx context.Context, o k8sObject, data map[string]any, objects []client.Object, logger *logrus.Entry) (res reconcile.Result, err error) {
	for _, oChild := range objects {
		if err = h.Client().Delete(ctx, oChild); err != nil {
			return res, errors.Wrapf(err, "Error when delete object '%s'", oChild.GetName())
		}
		logger.Debugf("Delete object '%s' successfully", oChild.GetName())
		h.Recorder().Eventf(o, corev1.EventTypeNormal, "DeleteCompleted", "Object '%s' successfully deleted", oChild.GetName())
	}

	return res, nil
}

func (h *DefaultSentinelAction[k8sObject]) OnError(ctx context.Context, o k8sObject, data map[string]any, currentErr error, logger *logrus.Entry) (res reconcile.Result, err error) {
	h.Recorder().Event(o, corev1.EventTypeWarning, "SentinelActionError", controller.UserFacingError(currentErr, controller.MaxEventMessage))
	return res, currentErr
}

func (h *DefaultSentinelAction[k8sObject]) OnSuccess(ctx context.Context, o k8sObject, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (res reconcile.Result, err error) {
	return res, nil
}

func (h *DefaultSentinelAction[k8sObject]) OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (res reconcile.Result, err error) {
	if !h.dryRun {
		return res, controller.ErrDiffDisabled
	}
	return res, nil
}

func (h *DefaultSentinelAction[k8sObject]) Diff(ctx context.Context, o k8sObject, read SentinelRead, data map[string]any, logger *logrus.Entry) (diff multiphase.MultiPhaseDiff[client.Object], res reconcile.Result, err error) {
	diff = multiphase.NewMultiPhaseDiff[client.Object]()

	for objectType, reader := range read.GetReads() {
		logger.Debugf("Start process object type '%s'", objectType)

		creates, updates, deletes, diffStrs, err := multiphase.ClassifyObjects(ctx, h.Client(), reader.GetExpectedObjects(), reader.GetCurrentObjects(), h.fieldManager, h.dryRun)
		if err != nil {
			return diff, res, err
		}
		multiphase.PopulateDiff(diff, creates, updates, deletes, diffStrs)
	}

	return diff, res, nil
}
