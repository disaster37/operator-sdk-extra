package controller

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/davecgh/go-spew/spew"
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
	Configure(ctx context.Context, req ctrl.Request, resource resource.Resource) (meta any, err error)
	Read(ctx context.Context, r resource.Resource, data map[string]any, meta any) (res ctrl.Result, err error)
	Create(ctx context.Context, r resource.Resource, data map[string]any, meta any) (res ctrl.Result, err error)
	Update(ctx context.Context, r resource.Resource, data map[string]any, meta any) (res ctrl.Result, err error)
	Delete(ctx context.Context, r resource.Resource, data map[string]any, meta any) (err error)
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
	h.log = h.log.WithFields(logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
	})
	h.log.Infof("---> Starting reconcile loop")
	defer h.log.Info("---> Finish reconcile loop for")

	// Get current resource
	if err = h.Get(ctx, req.NamespacedName, r); err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
	}

	// Add finalizer
	if h.finalizer != "" {
		if !controllerutil.ContainsFinalizer(r, h.finalizer) {
			controllerutil.AddFinalizer(r, h.finalizer)
			if err = h.Update(ctx, r); err != nil {
				h.log.Errorf("Error when add finalizer: %s", err.Error())
				h.recorder.Eventf(r, core.EventTypeWarning, "Adding finalizer", "Failed to add finalizer: %s", err)
				return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
			}
			h.recorder.Event(r, core.EventTypeNormal, "Added", "Object finalizer is added")
			h.log.Debug("Add finalizer successfully")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Configure to optional get driver client (call meta)
	meta, err := h.reconciler.Configure(ctx, req, r)
	if err != nil {
		h.log.Errorf("Error configure reconciler: %s", err.Error())
		return res, err
	}
	h.log.Debug("Configure reconciler successfully")

	// Read external resources
	res, err = h.reconciler.Read(ctx, r, data, meta)
	if err != nil {
		h.log.Errorf("Error when get resource: %s", err.Error())
		return res, err
	}
	if res != (ctrl.Result{}) {
		return res, nil
	}
	h.log.Debug("Get resource successfully")

	// Check if resource need to be deleted
	if !r.GetObjectMeta().DeletionTimestamp.IsZero() {
		if h.finalizer != "" && controllerutil.ContainsFinalizer(r, h.finalizer) {
			if err = h.reconciler.Delete(ctx, r, data, meta); err != nil {
				h.log.Errorf("Error when delete resource: %s", err.Error())
				h.recorder.Eventf(r, core.EventTypeWarning, "Failed", "Error when delete resource: %s", err.Error())
				return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
			}
			h.log.Debug("Delete successfully")

			controllerutil.RemoveFinalizer(r, h.finalizer)
			if err := h.Update(ctx, r); err != nil {
				h.log.Errorf("Failed to remove finalizer: %s", err.Error())
				h.recorder.Eventf(r, core.EventTypeWarning, "Failed", "Error when remove finalizer: %s", err.Error())
				return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
			}
			h.log.Debug("Remove finalizer successfully")
		}
		return ctrl.Result{}, nil
	}

	// Handle status
	currentStatus := r.DeepCopyObject().(resource.Resource).GetStatus()
	defer func() {
		h.log.Debugf("Current status: %s", spew.Sdump(currentStatus))
		h.log.Debugf("Expected status: %s", spew.Sdump(r.GetStatus()))
		if !reflect.DeepEqual(currentStatus, r.GetStatus()) {
			h.log.Debug("Detect that it need to update status")
			if err = h.Client.Status().Update(ctx, r); err != nil {
				h.log.Errorf("Error when update resource status: %s", err.Error())
			}
			h.log.Debug("Update status successfully")
		}
	}()

	//Check if diff exist
	diff, err := h.reconciler.Diff(r, data, meta)
	if err != nil {
		return res, err
	}

	// Need create
	if diff.NeedCreate {
		return h.reconciler.Create(ctx, r, data, meta)
	}

	// Need update
	if diff.NeedUpdate {
		h.log.Infof("Diff found:\n", diff.Diff)
		return h.reconciler.Update(ctx, r, data, meta)
	}

	h.log.Info("Nothink to do")

	return ctrl.Result{}, nil
}
