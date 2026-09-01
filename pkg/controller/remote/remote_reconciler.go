package remote

import (
	"context"
	"time"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// RemoteReconciler is the reconciler to reconcile the remote resource
type RemoteReconciler[k8sObject object.RemoteObject, apiObject comparable, apiClient any] interface {
	controller.Reconciler

	// Reconcile permit to reconcile the step (one K8s resource)
	Reconcile(ctx context.Context, req reconcile.Request, o k8sObject, data map[string]interface{}, reconciler RemoteReconcilerAction[k8sObject, apiObject, apiClient]) (res reconcile.Result, err error)
}

// DefaultRemoteReconciler is the default implementation of RemoteReconciler interface
type DefaultRemoteReconciler[k8sObject object.RemoteObject, apiObject comparable, apiClient any] struct {
	controller.Reconciler
}

// NewRemoteReconciler permit to instanciate new basic multiphase resonciler
func NewRemoteReconciler[k8sObject object.RemoteObject, apiObject comparable, apiClient any](client client.Client, name string, finalizer shared.FinalizerName, logger *logrus.Entry, recorder record.EventRecorder) (remoteReconciler RemoteReconciler[k8sObject, apiObject, apiClient]) {
	return &DefaultRemoteReconciler[k8sObject, apiObject, apiClient]{
		Reconciler: controller.NewReconciler(
			client,
			recorder,
			finalizer,
			logger.WithFields(logrus.Fields{
				"reconciler": name,
			}),
		),
	}
}

func (h *DefaultRemoteReconciler[k8sObject, apiObject, apiClient]) Reconcile(ctx context.Context, req reconcile.Request, o k8sObject, data map[string]interface{}, reconciler RemoteReconcilerAction[k8sObject, apiObject, apiClient]) (res reconcile.Result, err error) {
	var (
		handler RemoteExternalReconciler[k8sObject, apiObject, apiClient]
		read    RemoteRead[apiObject]
		diff    RemoteDiff[apiObject]
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

	// Add finalizer
	if h.Finalizer().String() != "" {
		if !controllerutil.ContainsFinalizer(o, h.Finalizer().String()) {
			controllerutil.AddFinalizer(o, h.Finalizer().String())
			if err = h.Client().Update(ctx, o); err != nil {
				logger.Errorf("Error when add finalizer: %s", err.Error())
				return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, controller.ErrWhenAddFinalizer.Error()), logger)
			}
			logger.Debug("Add finalizer successfully, force requeue object")
			return reconcile.Result{RequeueAfter: time.Millisecond}, nil
		}
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

	// Get the remote handler
	handler, res, err = reconciler.GetRemoteHandler(ctx, req, o, logger)
	if err != nil {
		logger.Errorf("Error when call 'getRemoteHandler' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, controller.ErrWhenCallConfigureFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'getRemoteHandler' from reconciler successfully")
	if res != (reconcile.Result{}) {
		return res, nil
	}
	if handler == nil && !o.GetDeletionTimestamp().IsZero() {
		// Delete finalizer to finish to destroy current resource
		controllerutil.RemoveFinalizer(o, h.Finalizer().String())
		if err = h.Client().Update(ctx, o); err != nil {
			logger.Errorf("Failed to remove finalizer: %s", err.Error())
			return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, controller.ErrWhenDeleteFinalizer.Error()), logger)
		}
		logger.Debug("Remove finalizer successfully")

		return res, nil
	}

	// Configure resource
	res, err = reconciler.Configure(ctx, o, data, handler, logger)
	if err != nil {
		logger.Errorf("Error when call 'configure' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, controller.ErrWhenCallReadFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'configure' from reconciler successfully")
	if res != (reconcile.Result{}) {
		return res, nil
	}

	// Read resources
	read, res, err = reconciler.Read(ctx, o, data, handler, logger)
	if err != nil {
		logger.Errorf("Error when call 'read' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, controller.ErrWhenCallReadFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'read' from reconciler successfully")
	if res != (reconcile.Result{}) {
		return res, nil
	}

	// Handle delete finalizer
	if !controller.GetObjectMeta(o).DeletionTimestamp.IsZero() {
		if h.Finalizer().String() != "" && controllerutil.ContainsFinalizer(o, h.Finalizer().String()) {
			if err = reconciler.Delete(ctx, o, data, handler, logger); err != nil {
				logger.Errorf("Error when call 'delete' from reconciler: %s", err.Error())
				return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, controller.ErrWhenCallDeleteFromReconciler.Error()), logger)
			}
			logger.Debug("Call 'delete' from reconciler successfully")

			controllerutil.RemoveFinalizer(o, h.Finalizer().String())
			if err = h.Client().Update(ctx, o); err != nil {
				logger.Errorf("Failed to remove finalizer: %s", err.Error())
				return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, controller.ErrWhenDeleteFinalizer.Error()), logger)
			}
			logger.Debug("Remove finalizer successfully")
		}
		return reconcile.Result{}, nil
	}

	// Check if diff exist
	diff, res, err = reconciler.Diff(ctx, o, read, data, handler, logger, reconciler.GetIgnoresDiff()...)
	if err != nil {
		logger.Errorf("Failed to call 'diff' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, controller.ErrWhenCallDiffFromReconciler.Error()), logger)
	}
	logger.Debugf("Call 'diff' from reconciler successfully with diff:\n%s", diff.Diff())
	if res != (reconcile.Result{}) {
		return res, nil
	}

	if diff.NeedCreate() {
		res, err = reconciler.Create(ctx, o, data, handler, diff.GetObjectToCreate(), logger)
		if err != nil {
			logger.Errorf("Failed to call 'create' from reconciler: %s", err.Error())
			return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, controller.ErrWhenCallCreateFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'create' from reconciler successfully")
		if res != (reconcile.Result{}) {
			return res, nil
		}
	}

	if diff.NeedUpdate() {
		res, err = reconciler.Update(ctx, o, data, handler, diff.GetObjectToUpdate(), logger)
		if err != nil {
			logger.Errorf("Failed to call 'update' from reconciler: %s", err.Error())
			return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, controller.ErrWhenCallUpdateFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'update' from reconciler successfully")
		if res != (reconcile.Result{}) {
			return res, nil
		}
	}

	res, err = reconciler.OnSuccess(ctx, o, data, handler, diff, logger)
	if err != nil {
		logger.Errorf("Error when call 'onSuccess' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, controller.ErrWhenCallOnSuccessFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'onSuccess' from reconciler successfully")

	return res, nil
}
