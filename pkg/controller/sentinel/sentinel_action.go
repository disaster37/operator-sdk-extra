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
// This is the simple variant: it has no OnDiff hook and Diff always uses
// always-apply classification (all expected objects with current counterparts are applied).
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

	// Diff permit to compare the actual state and the expected state.
	// The simple variant always uses always-apply classification: all expected objects
	// with current counterparts go to the update list (legacy always-apply).
	Diff(ctx context.Context, o k8sObject, read SentinelRead, data map[string]any, logger *logrus.Entry) (diff multiphase.MultiPhaseDiff[client.Object], res reconcile.Result, err error)

	// GetFieldManager returns the field manager name for SSA
	GetFieldManager() string
}

// SentinelReconcilerActionWithDiff is the diff variant of SentinelReconcilerAction.
// It embeds the base interface and adds OnDiff. Diff always uses SSA dry-run classification,
// so diff.NeedUpdate() / diff.GetObjectsToUpdate() are reliable in OnDiff.
type SentinelReconcilerActionWithDiff[k8sObject client.Object] interface {
	SentinelReconcilerAction[k8sObject]

	// OnDiff is called between Diff and Apply, after the diff has been computed.
	// The diff variant always uses SSA dry-run classification. The default implementation
	// returns nil (safe no-op); override it to run pre-apply tasks.
	OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (res reconcile.Result, err error)
}

// DefaultSentinelAction is the default implementation of SentinelAction
// (simple variant, always-apply classification).
type DefaultSentinelAction[k8sObject client.Object] struct {
	controller.ReconcilerAction
	fieldManager string
}

// NewSentinelAction creates a new sentinel action using Server-Side Apply.
// The fieldManager parameter identifies this controller as the owner of the fields it manages.
// This is the simple variant: Diff always uses always-apply classification and no OnDiff hook is available.
func NewSentinelAction[k8sObject client.Object](client client.Client, recorder record.EventRecorder, fieldManager string) (sentinelReconciler SentinelReconcilerAction[k8sObject]) {
	return &DefaultSentinelAction[k8sObject]{
		ReconcilerAction: controller.NewReconcilerAction(client, recorder, controller.ReadyCondition),
		fieldManager:     fieldManager,
	}
}

// DefaultSentinelActionWithDiff is the default implementation of SentinelReconcilerActionWithDiff.
// It embeds the simple default action, overrides Diff to use SSA dry-run classification,
// and adds an OnDiff hook that returns nil by default.
type DefaultSentinelActionWithDiff[k8sObject client.Object] struct {
	*DefaultSentinelAction[k8sObject]
}

// NewSentinelActionWithDiff creates a new sentinel action using Server-Side Apply.
// The fieldManager parameter identifies this controller as the owner of the fields it manages.
// This is the diff variant: Diff always uses SSA dry-run classification and OnDiff defaults to a safe no-op.
func NewSentinelActionWithDiff[k8sObject client.Object](client client.Client, recorder record.EventRecorder, fieldManager string) (sentinelReconciler SentinelReconcilerActionWithDiff[k8sObject]) {
	return &DefaultSentinelActionWithDiff[k8sObject]{
		DefaultSentinelAction: NewSentinelAction[k8sObject](
			client,
			recorder,
			fieldManager,
		).(*DefaultSentinelAction[k8sObject]),
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

		//nolint:staticcheck
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

func (h *DefaultSentinelAction[k8sObject]) Diff(ctx context.Context, o k8sObject, read SentinelRead, data map[string]any, logger *logrus.Entry) (diff multiphase.MultiPhaseDiff[client.Object], res reconcile.Result, err error) {
	return h.classifyDiff(ctx, read, logger, false)
}

// classifyDiff classifies the read objects into create/update/delete lists using SSA.
// When dryRun is true the classification uses an SSA dry-run apply; otherwise it uses
// legacy always-apply classification.
func (h *DefaultSentinelAction[k8sObject]) classifyDiff(ctx context.Context, read SentinelRead, logger *logrus.Entry, dryRun bool) (diff multiphase.MultiPhaseDiff[client.Object], res reconcile.Result, err error) {
	diff = multiphase.NewMultiPhaseDiff[client.Object]()

	for objectType, reader := range read.GetReads() {
		logger.Debugf("Start process object type '%s'", objectType)

		creates, updates, deletes, diffStrs, err := multiphase.ClassifyObjects(ctx, h.Client(), reader.GetExpectedObjects(), reader.GetCurrentObjects(), h.fieldManager, dryRun)
		if err != nil {
			return diff, res, err
		}
		multiphase.PopulateDiff(diff, creates, updates, deletes, diffStrs)
	}

	return diff, res, nil
}

func (h *DefaultSentinelActionWithDiff[k8sObject]) OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (res reconcile.Result, err error) {
	return res, nil
}

func (h *DefaultSentinelActionWithDiff[k8sObject]) Diff(ctx context.Context, o k8sObject, read SentinelRead, data map[string]any, logger *logrus.Entry) (diff multiphase.MultiPhaseDiff[client.Object], res reconcile.Result, err error) {
	return h.classifyDiff(ctx, read, logger, true)
}
