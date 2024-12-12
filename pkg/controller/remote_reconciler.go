package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/copystructure"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// RemoteReconciler is the reconciler to reconcile the remote resource
type RemoteReconciler[k8sObject comparable, apiObject comparable, apiClient any] interface {

	// Reconcile permit to reconcile the step (one K8s resource)
	Reconcile(ctx context.Context, req ctrl.Request, o object.RemoteObject, data map[string]interface{}, reconciler RemoteReconcilerAction[k8sObject, apiObject, apiClient]) (res ctrl.Result, err error)
}

// BasicRemoteReconciler is the basic implementation of RemoteReconciler interface
type BasicRemoteReconciler[k8sObject comparable, apiObject comparable, apiClient any] struct {
	BasicReconciler
}

// NewBasicMultiPhaseReconciler permit to instanciate new basic multiphase resonciler
func NewBasicRemoteReconciler[k8sObject comparable, apiObject comparable, apiClient any](client client.Client, name string, finalizer shared.FinalizerName, logger *logrus.Entry, recorder record.EventRecorder) (remoteReconciler RemoteReconciler[k8sObject, apiObject, apiClient]) {

	return &BasicRemoteReconciler[k8sObject, apiObject, apiClient]{
		BasicReconciler: NewBasicReconciler(
			client,
			recorder,
			finalizer,
			logger.WithFields(logrus.Fields{
				"reconciler": name,
			}),
		),
	}
}

func (h *BasicRemoteReconciler[k8sObject, apiObject, apiClient]) Reconcile(ctx context.Context, req ctrl.Request, o object.RemoteObject, data map[string]interface{}, reconciler RemoteReconcilerAction[k8sObject, apiObject, apiClient]) (res ctrl.Result, err error) {

	var (
		handler RemoteExternalReconciler[k8sObject, apiObject, apiClient]
		read    RemoteRead[apiObject]
		diff    RemoteDiff[apiObject]
	)

	// Init logger
	logger := h.BasicReconciler.logger.WithFields(logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
	})

	logger.Infof("Starting reconcile loop")
	defer logger.Info("Finish reconcile loop")

	// Wait few second to be sure status is propaged througout ETCD
	time.Sleep(time.Second * 1)

	// Get current resource
	if err = h.Client().Get(ctx, req.NamespacedName, o); err != nil {
		if k8serrors.IsNotFound(err) {
			return res, nil
		}
		logger.Errorf("Error when get object: %s", err.Error())
		return res, errors.Wrap(err, ErrWhenGetObjectFromReconciler.Error())
	}
	logger.Debug("Get object successfully")

	// Add finalizer
	if h.finalizer != "" {
		if !controllerutil.ContainsFinalizer(o, h.finalizer.String()) {
			controllerutil.AddFinalizer(o, h.finalizer.String())
			if err = h.Client().Update(ctx, o); err != nil {
				logger.Errorf("Error when add finalizer: %s", err.Error())
				return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, ErrWhenAddFinalizer.Error()), logger)
			}
			logger.Debug("Add finalizer successfully, force requeue object")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Handle status update if exist
	if getObjectStatus(o) != nil {
		currentStatus, err := copystructure.Copy(getObjectStatus(o))
		if err != nil {
			logger.Errorf("Error when get object status: %s", err.Error())
			return res, errors.Wrap(err, ErrWhenGetObjectStatus.Error())
		}
		defer func() {
			if !reflect.DeepEqual(currentStatus, getObjectStatus(o)) {
				logger.Debugf("Detect that it need to update status with diff:\n%s", cmp.Diff(currentStatus, getObjectStatus(o)))
				if err = h.Client().Status().Update(ctx, o); err != nil {
					logger.Errorf("Error when update resource status: %s", err.Error())
				}
				logger.Debug("Update status successfully")
			}
		}()
	}

	// Ignore if needed by annotation
	if o.GetAnnotations()[fmt.Sprintf("%s/ignoreReconcile", BaseAnnotation)] == "true" {
		logger.Info("Found annotation on ressource to ignore reconcile")
		return res, nil
	}

	// Get the remote handler
	handler, res, err = reconciler.GetRemoteHandler(ctx, req, o, logger)
	if err != nil {
		logger.Errorf("Error when call 'getRemoteHandler' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, ErrWhenCallConfigureFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'getRemoteHandler' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}
	if handler == nil && !o.GetDeletionTimestamp().IsZero() {
		// Delete finalizer to finish to destroy current resource
		controllerutil.RemoveFinalizer(o, h.finalizer.String())
		if err = h.Client().Update(ctx, o); err != nil {
			logger.Errorf("Failed to remove finalizer: %s", err.Error())
			return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, ErrWhenDeleteFinalizer.Error()), logger)
		}
		logger.Debug("Remove finalizer successfully")

		return res, nil
	}

	// Configure resource
	res, err = reconciler.Configure(ctx, o, data, handler, logger)
	if err != nil {
		logger.Errorf("Error when call 'configure' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, ErrWhenCallReadFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'configure' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Read resources
	read, res, err = reconciler.Read(ctx, o, data, handler, logger)
	if err != nil {
		logger.Errorf("Error when call 'read' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, ErrWhenCallReadFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'read' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Handle delete finalizer
	if !getObjectMeta(o).DeletionTimestamp.IsZero() {
		if h.finalizer.String() != "" && controllerutil.ContainsFinalizer(o, h.finalizer.String()) {
			if err = reconciler.Delete(ctx, o, data, handler, logger); err != nil {
				logger.Errorf("Error when call 'delete' from reconciler: %s", err.Error())
				return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, ErrWhenCallDeleteFromReconciler.Error()), logger)
			}
			logger.Debug("Call 'delete' from reconciler successfully")

			controllerutil.RemoveFinalizer(o, h.finalizer.String())
			if err = h.Client().Update(ctx, o); err != nil {
				logger.Errorf("Failed to remove finalizer: %s", err.Error())
				return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, ErrWhenDeleteFinalizer.Error()), logger)
			}
			logger.Debug("Remove finalizer successfully")
		}
		return ctrl.Result{}, nil
	}

	// Check if diff exist
	diff, res, err = reconciler.Diff(ctx, o, read, data, handler, logger, reconciler.GetIgnoresDiff()...)
	if err != nil {
		logger.Errorf("Failed to call 'diff' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, ErrWhenCallDiffFromReconciler.Error()), logger)
	}
	logger.Debugf("Call 'diff' from reconciler successfully with diff:\n%s", diff.Diff())
	if res != (ctrl.Result{}) {
		return res, nil
	}

	if diff.NeedCreate() {
		res, err = reconciler.Create(ctx, o, data, handler, diff.GetObjectToCreate(), logger)
		if err != nil {
			logger.Errorf("Failed to call 'create' from reconciler: %s", err.Error())
			return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, ErrWhenCallCreateFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'create' from reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	if diff.NeedUpdate() {
		res, err = reconciler.Update(ctx, o, data, handler, diff.GetObjectToUpdate(), logger)
		if err != nil {
			logger.Errorf("Failed to call 'update' from reconciler: %s", err.Error())
			return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, ErrWhenCallUpdateFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'update' from reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	res, err = reconciler.OnSuccess(ctx, o, data, handler, diff, logger)
	if err != nil {
		logger.Errorf("Error when call 'onSuccess' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, handler, errors.Wrap(err, ErrWhenCallOnSuccessFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'onSuccess' from reconciler successfully")

	return res, nil
}
