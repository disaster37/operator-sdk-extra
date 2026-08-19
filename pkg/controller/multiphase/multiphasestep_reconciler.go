package multiphase

import (
	"context"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MultiPhaseStepReconciler is the reconciler to implement to create one step for MultiPhaseReconciler
type MultiPhaseStepReconciler[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] interface {
	controller.BaseReconciler

	// Reconcile permit to reconcile the step (one K8s resource)
	Reconcile(ctx context.Context, req reconcile.Request, o k8sObject, data map[string]interface{}, reconciler MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject], logger *logrus.Entry) (res reconcile.Result, err error)
}

// DefaultMultiPhaseStepReconciler is the default implementation of MultiPhaseStepReconciler interface
type DefaultMultiPhaseStepReconciler[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] struct {
	controller.BaseReconciler
}

// NewMultiPhaseStepReconciler is the default implementation of MultiPhaseStepReconciler interface
func NewMultiPhaseStepReconciler[k8sObject object.MultiPhaseObject, k8sStepObject client.Object](client client.Client, logger *logrus.Entry, recorder record.EventRecorder) (multiPhaseStepReconciler MultiPhaseStepReconciler[k8sObject, k8sStepObject]) {
	return &DefaultMultiPhaseStepReconciler[k8sObject, k8sStepObject]{
		BaseReconciler: controller.NewBaseReconciler(client, recorder),
	}
}

// Reconcile permit to reconcile the step (one K8s resource) using Server-Side Apply
func (h *DefaultMultiPhaseStepReconciler[k8sObject, k8sStepObject]) Reconcile(ctx context.Context, req reconcile.Request, o k8sObject, data map[string]interface{}, reconcilerAction MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject], logger *logrus.Entry) (res reconcile.Result, err error) {
	var (
		diff MultiPhaseDiff[k8sStepObject]
		read MultiPhaseRead[k8sStepObject]
	)

	// Init logger
	logger = logger.WithFields(logrus.Fields{
		"step": reconcilerAction.GetPhaseName().String(),
	})

	// Configure
	res, err = reconcilerAction.Configure(ctx, req, o, logger)
	if err != nil {
		logger.Errorf("Error when call 'configure' from step reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallConfigureFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'configure' from step reconciler successfully")
	if res != (reconcile.Result{}) {
		return res, nil
	}

	// Read resources
	read, res, err = reconcilerAction.Read(ctx, o, data, logger)
	if err != nil {
		logger.Errorf("Error when call 'read' from step reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallReadFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'read' from step reconciler successfully")
	if res != (reconcile.Result{}) {
		return res, nil
	}

	// Compute diff (orphan detection + classified create/update/delete)
	diff, res, err = reconcilerAction.Diff(ctx, o, read, data, logger)
	if err != nil {
		logger.Errorf("Error when call 'diff' from step reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallDiffFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'diff' from step reconciler successfully")
	if diff.IsDiff() {
		logger.Debugf("Found diff: %s", diff.Diff())
	}
	if res != (reconcile.Result{}) {
		return res, nil
	}

	// OnDiff: pre-apply hook for pre-tasks (drain, etc.)
	res, err = reconcilerAction.OnDiff(ctx, o, data, diff, logger)
	if err != nil {
		logger.Errorf("Error when call 'onDiff' from step reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallOnDiffFromReconciler.Error()), logger)
	}
	if res != (reconcile.Result{}) {
		return res, nil
	}

	// Apply resources via SSA
	if diff.NeedCreate() || diff.NeedUpdate() {
		logger.Debug("Call 'apply' from step reconciler")
		res, err = reconcilerAction.Apply(ctx, o, data, diff.GetObjectsToApply(), logger)
		if err != nil {
			logger.Errorf("Error when call 'apply' from step reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallApplyFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'apply' from step reconciler successfully")
		if res != (reconcile.Result{}) {
			return res, nil
		}
	}

	// Delete orphans
	if diff.NeedDelete() {
		logger.Debug("Call 'delete' from step reconciler")
		res, err = reconcilerAction.Delete(ctx, o, data, diff.GetObjectsToDelete(), logger)
		if err != nil {
			logger.Errorf("Error when call 'delete' from step reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallDeleteFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'delete' from step reconciler successfully")
		if res != (reconcile.Result{}) {
			return res, nil
		}
	}

	res, err = reconcilerAction.OnSuccess(ctx, o, data, diff, logger)
	if err != nil {
		logger.Errorf("Error when call 'onSuccess' from step reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallOnSuccessFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'onSuccess' from step reconciler successfully")

	return res, nil
}
