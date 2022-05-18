package controller

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/disaster37/operator-sdk-extra/pkg/resource"
	"github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Reconciler interface {
	// Confirgure permit to init external provider driver (API client REST)
	// It can also permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, resource resource.Resource) (meta any, err error)

	// Read permit to read the actual resource state from provider and set it on data map
	Read(ctx context.Context, r resource.Resource, data map[string]any, meta any) (res ctrl.Result, err error)

	// Create permit to create resource on provider
	// It only call if diff.NeeCreated is true
	Create(ctx context.Context, r resource.Resource, data map[string]any, meta any) (res ctrl.Result, err error)

	// Update permit to update resource on provider
	// It only call if diff.NeedUpdated is true
	Update(ctx context.Context, r resource.Resource, data map[string]any, meta any) (res ctrl.Result, err error)

	// Delete permit to delete resource on provider
	// It only call if you have specified finalizer name when you create reconciler and if resource as marked to be deleted
	Delete(ctx context.Context, r resource.Resource, data map[string]any, meta any) (err error)

	// OnError is call when error is throwing
	// It the right way to set status condition when error
	OnError(ctx context.Context, r resource.Resource, data map[string]any, meta any, err error)

	// OnSuccess is call at the end if no error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, r resource.Resource, data map[string]any, meta any, diff Diff) (err error)

	// Diff permit to compare the actual state and the expected state
	Diff(r resource.Resource, data map[string]any, meta any) (diff Diff, err error)
}

type Diff struct {
	NeedCreate bool
	NeedUpdate bool
	Diff       string
}

type StdReconciler struct {
	client.Client
	finalizer           string
	reconciler          Reconciler
	log                 *logrus.Entry
	recorder            record.EventRecorder
	waitDurationOnError time.Duration
}

func NewStdReconciler(client client.Client, finalizer string, reconciler Reconciler, logger *logrus.Entry, recorder record.EventRecorder, waitDurationOnError time.Duration) (stdReconciler *StdReconciler, err error) {

	if recorder == nil {
		return nil, errors.New("recorder can't be nil")
	}

	stdReconciler = &StdReconciler{
		Client:              client,
		finalizer:           finalizer,
		reconciler:          reconciler,
		recorder:            recorder,
		log:                 logger,
		waitDurationOnError: waitDurationOnError,
	}

	if stdReconciler.log == nil {
		stdReconciler.log = logrus.NewEntry(logrus.New())
	}

	return stdReconciler, nil
}

func (h *StdReconciler) Reconcile(ctx context.Context, req ctrl.Request, r resource.Resource, data map[string]interface{}) (res ctrl.Result, err error) {
	var (
		meta any
		diff Diff
		o    client.Object
	)

	h.log = h.log.WithFields(logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
	})
	h.log.Infof("---> Starting reconcile loop")
	defer h.log.Info("---> Finish reconcile loop for")

	// Resource can be composition of real resource that we need to extract in order to use with client
	o = r
	rv := reflect.ValueOf(r).Elem()
	h.log.Debugf("Num field: %d", rv.NumField())
	if rv.NumField() == 1 {
		v := rv.Field(0)
		h.log.Debugf("Kind: %s, %s", v.Kind(), v.Type())
		if v.Elem().Kind() == reflect.Struct {
			h.log.Debugf("Detect composition of type %s", v.Kind())
			o = v.Interface().(client.Object)
		}
	}

	// Get current resource
	if err = h.Get(ctx, req.NamespacedName, o); err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle status update
	currentStatus := r.DeepCopyObject().(resource.Resource).GetStatus()
	defer func() {
		if err != nil {
			h.reconciler.OnError(ctx, r, data, meta, err)
		}
		if !reflect.DeepEqual(currentStatus, r.GetStatus()) {
			h.log.Debug("Detect that it need to update status")
			if err = h.Client.Status().Update(ctx, o); err != nil {
				h.log.Errorf("Error when update resource status: %s", err.Error())
			}
			h.log.Debug("Update status successfully")
		}
	}()

	// Add finalizer
	if h.finalizer != "" {
		if !controllerutil.ContainsFinalizer(o, h.finalizer) {
			controllerutil.AddFinalizer(o, h.finalizer)
			if err = h.Update(ctx, o); err != nil {
				h.log.Errorf("Error when add finalizer: %s", err.Error())
				h.recorder.Eventf(o, core.EventTypeWarning, "Adding finalizer", "Failed to add finalizer: %s", err)
				return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
			}
			h.recorder.Event(o, core.EventTypeNormal, "Added", "Object finalizer is added")
			h.log.Debug("Add finalizer successfully")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Configure to optional get driver client (call meta)
	meta, err = h.reconciler.Configure(ctx, req, r)
	if err != nil {
		h.log.Errorf("Error configure reconciler: %s", err.Error())
		return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
	}
	h.log.Debug("Configure reconciler successfully")

	// Read external resources
	res, err = h.reconciler.Read(ctx, r, data, meta)
	if err != nil {
		h.log.Errorf("Error when get resource: %s", err.Error())
		return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
	}
	if res != (ctrl.Result{}) {
		return res, nil
	}
	h.log.Debug("Get resource successfully")

	// Check if resource need to be deleted
	if !r.GetObjectMeta().DeletionTimestamp.IsZero() {
		if h.finalizer != "" && controllerutil.ContainsFinalizer(o, h.finalizer) {
			h.log.Info("Start delete step")
			if err = h.reconciler.Delete(ctx, r, data, meta); err != nil {
				h.log.Errorf("Error when delete resource: %s", err.Error())
				h.recorder.Eventf(o, core.EventTypeWarning, "Failed", "Error when delete resource: %s", err.Error())
				return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
			}
			h.log.Debug("Delete successfully")

			controllerutil.RemoveFinalizer(o, h.finalizer)
			if err = h.Update(ctx, o); err != nil {
				h.log.Errorf("Failed to remove finalizer: %s", err.Error())
				h.recorder.Eventf(o, core.EventTypeWarning, "Failed", "Error when remove finalizer: %s", err.Error())
				return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
			}
			h.log.Debug("Remove finalizer successfully")
		}
		return ctrl.Result{}, nil
	}

	//Check if diff exist
	diff, err = h.reconciler.Diff(r, data, meta)
	if err != nil {
		return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
	}

	// Need create
	if diff.NeedCreate {
		h.log.Info("Start create step")
		res, err = h.reconciler.Create(ctx, r, data, meta)
		if err != nil {
			return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
		}
	}

	// Need update
	if diff.NeedUpdate {
		h.log.Infof("Start update step with diff:\n%s", diff.Diff)
		res, err = h.reconciler.Update(ctx, r, data, meta)
		if err != nil {
			return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
		}
	}

	// Nothink to do
	if !diff.NeedCreate && !diff.NeedUpdate {
		h.log.Debug("Nothink to do")
	}

	if res != (ctrl.Result{}) {
		return res, nil
	}

	if err = h.reconciler.OnSuccess(ctx, r, data, meta, diff); err != nil {
		return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
	}

	return ctrl.Result{}, nil
}
