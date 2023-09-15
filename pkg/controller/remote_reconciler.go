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
type RemoteReconciler[T comparable, M any] interface {

	// Reconcile permit to reconcile the step (one K8s resource)
	Reconcile(ctx context.Context, req ctrl.Request, o object.RemoteObject, data map[string]interface{}, reconciler RemoteReconcilerAction[T, M]) (res ctrl.Result, err error)
}

// BasicRemoteReconciler is the basic implementation of RemoteReconciler interface
type BasicRemoteReconciler[T comparable, M any] struct {
	BasicReconciler
}

// NewBasicMultiPhaseReconciler permit to instanciate new basic multiphase resonciler
func NewBasicRemoteReconciler[T comparable, M any](client client.Client, name string, finalizer shared.FinalizerName, logger *logrus.Entry, recorder record.EventRecorder) (remoteReconciler RemoteReconciler[T, M]) {

	if recorder == nil {
		panic("recorder can't be nil")
	}

	basicRemoteReconciler := &BasicRemoteReconciler[T, M]{
		BasicReconciler: BasicReconciler{
			BaseReconciler: BaseReconciler{
				Client: client,
				Log: logger.WithFields(logrus.Fields{
					"reconciler": name,
				}),
				Recorder: recorder,
			},
			finalizer: finalizer,
		},
	}

	if basicRemoteReconciler.Log == nil {
		basicRemoteReconciler.Log = logrus.NewEntry(logrus.New())
	}

	return basicRemoteReconciler
}

func (h *BasicRemoteReconciler[T, M]) Reconcile(ctx context.Context, req ctrl.Request, o object.RemoteObject, data map[string]interface{}, reconciler RemoteReconcilerAction[T, M]) (res ctrl.Result, err error) {

	var (
		meta M
		read RemoteRead[T]
		diff RemoteDiff[T]
	)

	// Init logger
	h.Log = h.Log.WithFields(logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
	})

	h.Log.Infof("---> Starting reconcile loop")
	defer h.Log.Info("---> Finish reconcile loop for")

	// Wait few second to be sure status is propaged througout ETCD
	time.Sleep(time.Second * 1)

	// Get current resource
	if err = h.Get(ctx, req.NamespacedName, o); err != nil {
		if k8serrors.IsNotFound(err) {
			return res, nil
		}
		h.Log.Errorf("Error when get object: %s", err.Error())
		return res, errors.Wrap(err, ErrWhenGetObjectFromReconciler.Error())
	}
	h.Log.Debug("Get object successfully")

	// Add finalizer
	if h.finalizer != "" {
		if !controllerutil.ContainsFinalizer(o, h.finalizer.String()) {
			controllerutil.AddFinalizer(o, h.finalizer.String())
			if err = h.Update(ctx, o); err != nil {
				h.Log.Errorf("Error when add finalizer: %s", err.Error())
				return reconciler.OnError(ctx, o, data, meta, errors.Wrap(err, ErrWhenAddFinalizer.Error()))
			}
			h.Log.Debug("Add finalizer successfully, force requeue object")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Handle status update if exist
	if getObjectStatus(o) != nil {
		currentStatus, err := copystructure.Copy(getObjectStatus(o))
		if err != nil {
			h.Log.Errorf("Error when get object status: %s", err.Error())
			return res, errors.Wrap(err, ErrWhenGetObjectStatus.Error())
		}
		defer func() {
			if !reflect.DeepEqual(currentStatus, getObjectStatus(o)) {
				h.Log.Debugf("Detect that it need to update status with diff:\n%s", cmp.Diff(currentStatus, getObjectStatus(o)))
				if err = h.Client.Status().Update(ctx, o); err != nil {
					h.Log.Errorf("Error when update resource status: %s", err.Error())
				}
				h.Log.Debug("Update status successfully")
			}
		}()
	}

	// Configure to optional get driver client (call meta)
	meta, res, err = reconciler.Configure(ctx, req, o)
	if err != nil {
		h.Log.Errorf("Error when call 'configure' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, meta, errors.Wrap(err, ErrWhenCallConfigureFromReconciler.Error()))
	}
	h.Log.Debug("Call 'configure' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Read resources
	read, res, err = reconciler.Read(ctx, o, data, meta)
	if err != nil {
		h.Log.Errorf("Error when call 'read' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, meta, errors.Wrap(err, ErrWhenCallReadFromReconciler.Error()))
	}
	h.Log.Debug("Call 'read' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Handle delete finalizer
	if !getObjectMeta(o).DeletionTimestamp.IsZero() {
		if h.finalizer.String() != "" && controllerutil.ContainsFinalizer(o, h.finalizer.String()) {
			if err = reconciler.Delete(ctx, o, data, meta); err != nil {
				h.Log.Errorf("Error when call 'delete' from reconciler: %s", err.Error())
				return reconciler.OnError(ctx, o, data, meta, errors.Wrap(err, ErrWhenCallDeleteFromReconciler.Error()))
			}
			h.Log.Debug("Delete successfully")

			controllerutil.RemoveFinalizer(o, h.finalizer.String())
			if err = h.Update(ctx, o); err != nil {
				h.Log.Errorf("Failed to remove finalizer: %s", err.Error())
				return reconciler.OnError(ctx, o, data, meta, errors.Wrap(err, ErrWhenDeleteFinalizer.Error()))
			}
			h.Log.Debug("Remove finalizer successfully")
		}
		return ctrl.Result{}, nil
	}

	// Ignore if needed by annotation
	if o.GetAnnotations()[fmt.Sprintf("%s/ignoreReconcile", BaseAnnotation)] == "true" {
		h.Log.Info("Found annotation on ressource to ignore reconcile")
		return res, nil
	}

	// Check if diff exist
	diff, res, err = reconciler.Diff(ctx, o, read, data, meta, reconciler.GetIgnoresDiff()...)
	if err != nil {
		h.Log.Errorf("Failed to call 'diff' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, meta, errors.Wrap(err, ErrWhenCallDiffFromReconciler.Error()))
	}
	if res != (ctrl.Result{}) {
		return res, nil
	}

	if diff.NeedCreate() {
		res, err = reconciler.Create(ctx, o, data, meta, diff.GetObjectToCreate())
		if err != nil {
			h.Log.Errorf("Failed to call 'create' from reconciler: %s", err.Error())
			return reconciler.OnError(ctx, o, data, meta, errors.Wrap(err, ErrWhenCallCreateFromReconciler.Error()))
		}
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	if diff.NeedUpdate() {
		res, err = reconciler.Update(ctx, o, data, meta, diff.GetObjectToUpdate())
		if err != nil {
			h.Log.Errorf("Failed to call 'update' from reconciler: %s", err.Error())
			return reconciler.OnError(ctx, o, data, meta, errors.Wrap(err, ErrWhenCallUpdateFromReconciler.Error()))
		}
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	res, err = reconciler.OnSuccess(ctx, o, data, meta, diff)
	if err != nil {
		h.Log.Errorf("Error when call 'onSuccess' from reconciler: %s", err.Error())
		return reconciler.OnError(ctx, o, data, meta, errors.Wrap(err, ErrWhenCallOnSuccessFromReconciler.Error()))
	}

	return res, nil
}
