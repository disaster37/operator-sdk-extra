package sentinel

import (
	"context"
	"time"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/multiphase"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SentinelReconciler must be used when you look resource that your operator is not the owner like ingress, secret, configMap, etc.
// Some time you should to generate some resource from labels or annotations ...
// It the use case of this controller
type SentinelReconciler[k8sObject client.Object] interface {
	controller.Reconciler

	// Reconcile permit to orchestrate all phase needed to successfully reconcile the object
	Reconcile(ctx context.Context, req reconcile.Request, o k8sObject, data map[string]interface{}, reconciler SentinelReconcilerAction[k8sObject]) (res reconcile.Result, err error)
}

// DefaultSentinelReconciler is the default implementation of SentinelReconciler interface
type DefaultSentinelReconciler[k8sObject client.Object] struct {
	controller.Reconciler
}

// NewSentinelReconciler is the default implementation of SentinelReconciler interface
func NewSentinelReconciler[k8sObject client.Object](client client.Client, name string, logger *logrus.Entry, recorder record.EventRecorder) (sentinelReconciler SentinelReconciler[k8sObject]) {
	return &DefaultSentinelReconciler[k8sObject]{
		Reconciler: controller.NewReconciler(
			client,
			recorder,
			"",
			logger.WithFields(logrus.Fields{
				"reconciler": name,
			}),
		),
	}
}

// No need to add finalizer and manage delete
// All sub resources must be children of main parent. So the clean is handled by kubelet in lazy effort
func (h *DefaultSentinelReconciler[k8sObject]) Reconcile(ctx context.Context, req reconcile.Request, o k8sObject, data map[string]interface{}, reconcilerAction SentinelReconcilerAction[k8sObject]) (res reconcile.Result, err error) {
	var (
		read SentinelRead
		diff multiphase.MultiPhaseDiff[client.Object]
	)

	// Init logger
	logger := h.Logger().WithFields(logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
	})
	logger.Infof("Starting reconcile loop")
	defer logger.Info("Finish reconcile loop")

	// Wait few second to be sure status is propaged througout ETCD
	time.Sleep(time.Second * 1)

	// Get current resource
	found, err := controller.GetObjectFromReconciler(ctx, h.Client(), req, o, logger)
	if err != nil {
		return res, err
	}
	if !found {
		return res, nil
	}

	// Handle status update if exist
	deferStatusUpdate, err := controller.DeferStatusUpdate(ctx, h.Client(), o, logger)
	if err != nil {
		return res, err
	}
	defer func() {
		if statusErr := deferStatusUpdate(); statusErr != nil {
			err = statusErr
		}
	}()

	// Ignore if needed by annotation
	if controller.IsReconcileIgnored(o) {
		logger.Info("Found annotation on ressource to ignore reconcile")
		return res, nil
	}

	// Configure to optional get driver client (call meta)
	res, err = reconcilerAction.Configure(ctx, req, o, data, logger)
	if err != nil {
		logger.Errorf("Error when call 'configure' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallConfigureFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'configure' from reconciler successfully")
	if res != (reconcile.Result{}) {
		return res, nil
	}

	// Read resources
	read, res, err = reconcilerAction.Read(ctx, o, data, logger)
	if err != nil {
		logger.Errorf("Error when call 'read' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallReadFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'read' from reconciler successfully")
	if res != (reconcile.Result{}) {
		return res, nil
	}

	// Compute diff (orphan detection + classified create/update/delete)
	diff, res, err = reconcilerAction.Diff(ctx, o, read, data, logger)
	if err != nil {
		logger.Errorf("Failed to call 'diff' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallDiffFromReconciler.Error()), logger)
	}
	logger.Debugf("Call 'diff' from reconciler successfully with diff:\n%s", diff.Diff())
	if res != (reconcile.Result{}) {
		return res, nil
	}

	// OnDiff: pre-apply hook
	// It is optional: only called when the action implements the WithDiff variant.
	if diffAction, ok := reconcilerAction.(SentinelReconcilerActionWithDiff[k8sObject]); ok {
		res, err = diffAction.OnDiff(ctx, o, data, diff, logger)
		if err != nil {
			logger.Errorf("Failed to call 'onDiff' from reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallOnDiffFromReconciler.Error()), logger)
		}
		if res != (reconcile.Result{}) {
			return res, nil
		}
	}

	// Apply resources via SSA
	if diff.NeedCreate() || diff.NeedUpdate() {
		res, err = reconcilerAction.Apply(ctx, o, data, diff.GetObjectsToApply(), logger)
		if err != nil {
			logger.Errorf("Failed to call 'apply' from reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallApplyFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'apply' from reconciler successfully")
		if res != (reconcile.Result{}) {
			return res, nil
		}
	}

	// Delete orphans
	if diff.NeedDelete() {
		res, err = reconcilerAction.Delete(ctx, o, data, diff.GetObjectsToDelete(), logger)
		if err != nil {
			logger.Errorf("Failed to call 'delete' from reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallDeleteFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'delete' from reconciler successfully")
		if res != (reconcile.Result{}) {
			return res, nil
		}
	}

	res, err = reconcilerAction.OnSuccess(ctx, o, data, diff, logger)
	if err != nil {
		logger.Errorf("Error when call 'onSuccess' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallOnSuccessFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'onSuccess' from reconciler")

	return res, nil
}
