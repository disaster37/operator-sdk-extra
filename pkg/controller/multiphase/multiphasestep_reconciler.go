package multiphase

import (
	"context"

	"emperror.dev/errors"
	"github.com/disaster37/k8s-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/object"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseStepReconciler is the reconciler to implement to create one step for MultiPhaseReconciler
type MultiPhaseStepReconciler[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] interface {
	controller.BaseReconciler

	// Reconcile permit to reconcile the step (one K8s resource)
	Reconcile(ctx context.Context, req ctrl.Request, o k8sObject, data map[string]interface{}, reconciler MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject], logger *logrus.Entry, ignoresDiff ...patch.CalculateOption) (res ctrl.Result, err error)
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

// Reconcile permit to reconcile the step (one K8s resource)
func (h *DefaultMultiPhaseStepReconciler[k8sObject, k8sStepObject]) Reconcile(ctx context.Context, req ctrl.Request, o k8sObject, data map[string]interface{}, reconcilerAction MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject], logger *logrus.Entry, ignoresDiff ...patch.CalculateOption) (res ctrl.Result, err error) {

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
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Read resources
	read, res, err = reconcilerAction.Read(ctx, o, data, logger)
	if err != nil {
		logger.Errorf("Error when call 'read' from step reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallReadFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'read' from step reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	//Check if diff exist
	diff, res, err = reconcilerAction.Diff(ctx, o, read, data, logger, ignoresDiff...)
	if err != nil {
		logger.Errorf("Error when call 'diff' from step reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallDiffFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'diff' from step reconciler successfully")
	if diff.IsDiff() {
		logger.Debugf("Found diff: %s", diff.Diff())
	}
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Need create resources
	if diff.NeedCreate() {
		logger.Debug("Call 'create' from step reconciler")
		res, err = reconcilerAction.Create(ctx, o, data, diff.GetObjectsToCreate(), logger)
		if err != nil {
			logger.Errorf("Error when call 'create' from step reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallCreateFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'create' from step reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	// Need update resources
	if diff.NeedUpdate() {
		logger.Debug("Call 'update' from step reconciler")
		res, err = reconcilerAction.Update(ctx, o, data, diff.GetObjectsToUpdate(), logger)
		if err != nil {
			logger.Errorf("Error when call 'update' from step reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallUpdateFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'update' from step reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	// Need Delete
	if diff.NeedDelete() {
		logger.Debug("Call 'delete' from step reconciler")
		res, err = reconcilerAction.Delete(ctx, o, data, diff.GetObjectsToDelete(), logger)
		if err != nil {
			logger.Errorf("Error when call 'delete' from step reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallDeleteFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'delete' from step reconciler successfully")
		if res != (ctrl.Result{}) {
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
