package controller

import (
	"context"
	"errors"
	"time"

	"github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Reconciler interface {
	Read(req ctrl.Request, resource client.Object, data map[string]interface{}, meta interface{}) (err error)
	Create(resource client.Object, data map[string]interface{}, meta interface{}) (res ctrl.Result, err error)
	Update(resource client.Object, data map[string]interface{}, meta interface{}) (res ctrl.Result, err error)
	Delete(resource client.Object, data map[string]interface{}, meta interface{}) (res ctrl.Result, err error)
	Diff(resource client.Object, data map[string]interface{}) (diff Diff, err error)
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

func (h *StdReconciler) Reconcile(ctx context.Context, req ctrl.Request, resource client.Object, data map[string]interface{}, meta interface{}) (res ctrl.Result, err error) {
	h.log = h.log.WithFields(logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
	})
	h.log.Infof("---> Starting reconcile loop")
	defer h.log.Info("---> Finish reconcile loop for")

	// Get main resource and external resources
	if err = h.reconciler.Read(req, resource, data, meta); err != nil {
		h.log.Errorf("Error when get resource: %s", err.Error())
		return res, err
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
	diff, err := h.reconciler.Diff(resource, data)
	if err != nil {
		return res, err
	}

	// Need create
	if diff.NeedCreate {
		return h.reconciler.Create(resource, data, meta)
	}

	// Need update
	if diff.NeedUpdate {
		return h.reconciler.Update(resource, data, meta)
	}

	h.log.Info("Nothink to do")

	return ctrl.Result{}, nil
}
