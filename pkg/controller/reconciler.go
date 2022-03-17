package controller

import (
	"context"
	"errors"
	"time"

	"github.com/disaster37/operator-sdk-extra/pkg/resource"
	"github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Reconciler interface {
	Read(ctx context.Context, req ctrl.Request, resource resource.Resource, data map[string]interface{}, meta interface{}) (res *ctrl.Result, err error)
	Create(ctx context.Context, resource resource.Resource, data map[string]interface{}, meta interface{}) (res ctrl.Result, err error)
	Update(ctx context.Context, resource resource.Resource, data map[string]interface{}, meta interface{}) (res ctrl.Result, err error)
	Delete(ctx context.Context, resource resource.Resource, data map[string]interface{}, meta interface{}) (err error)
	Diff(resource resource.Resource, data map[string]interface{}, meta interface{}) (diff Diff, err error)
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

func (h *StdReconciler) Reconcile(ctx context.Context, req ctrl.Request, resource resource.Resource, data map[string]interface{}, meta interface{}) (res ctrl.Result, err error) {
	h.log = h.log.WithFields(logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
	})
	h.log.Infof("---> Starting reconcile loop")
	defer h.log.Info("---> Finish reconcile loop for")

	// Get main resource and external resources
	resTmp, err := h.reconciler.Read(ctx, req, resource, data, meta)
	if err != nil {
		h.log.Errorf("Error when get resource: %s", err.Error())
		return res, err
	}
	if resTmp != nil {
		h.log.Infof("Resource not exist, skip")
		return *resTmp, nil
	}

	// Add finalizer
	if h.finalizer != "" {
		if !controllerutil.ContainsFinalizer(resource, h.finalizer) {
			controllerutil.AddFinalizer(resource, h.finalizer)
			if err = h.Update(ctx, resource); err != nil {
				h.log.Errorf("Error when add finalizer: %s", err.Error())
				h.recorder.Eventf(resource, core.EventTypeWarning, "Adding finalizer", "Failed to add finalizer: %s", err)
				return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
			}
			h.recorder.Event(resource, core.EventTypeNormal, "Added", "Object finalizer is added")
			h.log.Debug("Add finalizer successfully")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Check if resource need to be deleted
	if !resource.GetObjectMeta().DeletionTimestamp.IsZero() {
		if h.finalizer != "" && controllerutil.ContainsFinalizer(resource, h.finalizer) {
			if err = h.reconciler.Delete(ctx, resource, data, meta); err != nil {
				h.log.Errorf("Error when delete resource: %s", err.Error())
				h.recorder.Eventf(resource, core.EventTypeWarning, "Failed", "Error when delete resource: %s", err.Error())
				return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
			}

			controllerutil.RemoveFinalizer(resource, h.finalizer)
			if err := h.Update(ctx, resource); err != nil {
				h.log.Errorf("Failed to remove finalizer: %s", err.Error())
				h.recorder.Eventf(resource, core.EventTypeWarning, "Failed", "Error when remove finalizer: %s", err.Error())
				return ctrl.Result{RequeueAfter: h.waitDurationOnError}, err
			}
			h.log.Debug("Remove finalizer successfully")
		}
		return ctrl.Result{}, nil
	}

	//Check if diff exist
	diff, err := h.reconciler.Diff(resource, data)
	if err != nil {
		return res, err
	}

	// Need create
	if diff.NeedCreate {
		return h.reconciler.Create(ctx, resource, data, meta)
	}

	// Need update
	if diff.NeedUpdate {
		h.log.Infof("Diff found:\n", diff.Diff)
		return h.reconciler.Update(ctx, resource, data, meta)
	}

	h.log.Info("Nothink to do")

	return ctrl.Result{}, nil
}
