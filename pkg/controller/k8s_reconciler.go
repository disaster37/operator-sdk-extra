package controller

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/mitchellh/copystructure"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type K8sReconciler interface {
	// Configure permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, resource client.Object) (res ctrl.Result, err error)

	// Read permit to read kubernetes resources
	Read(ctx context.Context, r client.Object, data map[string]any) (res ctrl.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, r client.Object, data map[string]any) (err error)

	// OnError is call when error is throwing on current phase
	// It the right way to set status condition when error
	OnError(ctx context.Context, r client.Object, data map[string]any, currentErr error) (res ctrl.Result, err error)

	// OnSuccess is call at the end of current phase, if not error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, r client.Object, data map[string]any) (res ctrl.Result, err error)
}

type K8sPhaseReconciler interface {
	// Configure permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, resource client.Object) (res ctrl.Result, err error)

	// Read permit to read kubernetes resources
	Read(ctx context.Context, r client.Object, data map[string]any) (res ctrl.Result, err error)

	// Create permit to create resources on kubernetes
	Create(ctx context.Context, r client.Object, data map[string]any) (res ctrl.Result, err error)

	// Update permit to update resources on kubernetes
	Update(ctx context.Context, r client.Object, data map[string]any) (res ctrl.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, r client.Object, data map[string]any) (res ctrl.Result, err error)

	// OnError is call when error is throwing on current phase
	// It the right way to set status condition when error
	OnError(ctx context.Context, r client.Object, data map[string]any, currentErr error) (res ctrl.Result, err error)

	// OnSuccess is call at the end of current phase, if not error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, r client.Object, data map[string]any, diff K8sDiff) (res ctrl.Result, err error)

	// Diff permit to compare the actual state and the expected state
	Diff(ctx context.Context, r client.Object, data map[string]any) (diff K8sDiff, res ctrl.Result, err error)

	// GetName return the reconciler name
	GetName() string
}

type K8sDiff struct {
	NeedCreate bool
	NeedUpdate bool
	NeedDelete bool
	Diff       strings.Builder
}

type StdK8sReconciler struct {
	client.Client
	finalizer  string
	reconciler K8sReconciler
	log        *logrus.Entry
	recorder   record.EventRecorder
}

func NewStdK8sReconciler(client client.Client, finalizer string, reconciler K8sReconciler, logger *logrus.Entry, recorder record.EventRecorder) (stdK8sReconciler *StdK8sReconciler, err error) {

	if recorder == nil {
		return nil, errors.New("recorder can't be nil")
	}

	stdK8sReconciler = &StdK8sReconciler{
		Client:     client,
		finalizer:  finalizer,
		reconciler: reconciler,
		recorder:   recorder,
		log:        logger,
	}

	if stdK8sReconciler.log == nil {
		stdK8sReconciler.log = logrus.NewEntry(logrus.New())
	}

	return stdK8sReconciler, nil
}

// ReconcileK8sResources permit to reconcile kubernetes resources, so the step is not the same on Reconcile.
// When handle kubernetes resources, you should to chain the reconcile on multiple resources
// It will run on following steps
// 1. Read the main object
// 2. Configure finalizer on the main object
// 3. Execute each phase that concist of:
// 3.1 Read kubernetes objects
// 3.2 Diff kubernetes resources with expected resources
// 3.3 Update / create resources if needed
// 3.4 Delete resources if needed
// 4. Delete finalizer if on delete action
func (h *StdK8sReconciler) Reconcile(ctx context.Context, req ctrl.Request, r client.Object, data map[string]interface{}, reconcilers ...K8sPhaseReconciler) (res ctrl.Result, err error) {

	// Init logger
	h.log = h.log.WithFields(logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
	})
	h.log.Infof("---> Starting reconcile loop")
	defer h.log.Info("---> Finish reconcile loop for")

	// Wait few second to be sure status is propaged througout ETCD
	time.Sleep(time.Second * 1)

	// Get current resource
	if err = h.Get(ctx, req.NamespacedName, r); err != nil {
		if k8serrors.IsNotFound(err) {
			return res, nil
		}
		return res, err
	}

	// Add finalizer
	if h.finalizer != "" {
		if !controllerutil.ContainsFinalizer(r, h.finalizer) {
			controllerutil.AddFinalizer(r, h.finalizer)
			if err = h.Update(ctx, r); err != nil {
				h.log.Errorf("Error when add finalizer: %s", err.Error())
				h.recorder.Eventf(r, core.EventTypeWarning, "Adding finalizer", "Failed to add finalizer: %s", err)
				return h.reconciler.OnError(ctx, r, data, err)
			}
			h.recorder.Event(r, core.EventTypeNormal, "Added", "Object finalizer is added")
			h.log.Debug("Add finalizer successfully")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Handle status update if exist
	if getObjectStatus(r) != nil {
		currentStatus, err := copystructure.Copy(getObjectStatus(r))
		if err != nil {
			return res, err
		}
		defer func() {
			if !reflect.DeepEqual(currentStatus, getObjectStatus(r)) {
				h.log.Debug("Detect that it need to update status")
				if err = h.Client.Status().Update(ctx, r); err != nil {
					h.log.Errorf("Error when update resource status: %s", err.Error())
				}
				h.log.Debug("Update status successfully")
			}
		}()
	}

	// Configure to optional get driver client (call meta)
	res, err = h.reconciler.Configure(ctx, req, r)
	if err != nil {
		h.log.Errorf("Error configure reconciler: %s", err.Error())
		return h.reconciler.OnError(ctx, r, data, err)
	}
	if res != (ctrl.Result{}) {
		return res, nil
	}
	h.log.Debug("Configure parent reconciler successfully")

	// Read resources
	res, err = h.reconciler.Read(ctx, r, data)
	if err != nil {
		h.log.Errorf("Error read reconciler: %s", err.Error())
		return h.reconciler.OnError(ctx, r, data, err)
	}
	if res != (ctrl.Result{}) {
		return res, nil
	}
	h.log.Debug("Read parent reconciler successfully")

	// Handle delete finalizer
	if !getObjectMeta(r).DeletionTimestamp.IsZero() {
		if h.finalizer != "" && controllerutil.ContainsFinalizer(r, h.finalizer) {
			if err = h.reconciler.Delete(ctx, r, data); err != nil {
				h.log.Errorf("Error when delete resource: %s", err.Error())
				h.recorder.Eventf(r, core.EventTypeWarning, "Failed", "Error when delete resource: %s", err.Error())
				return h.reconciler.OnError(ctx, r, data, err)
			}
			h.log.Debug("Delete successfully")

			controllerutil.RemoveFinalizer(r, h.finalizer)
			if err = h.Update(ctx, r); err != nil {
				h.log.Errorf("Failed to remove finalizer: %s", err.Error())
				h.recorder.Eventf(r, core.EventTypeWarning, "Failed", "Error when remove finalizer: %s", err.Error())
				return h.reconciler.OnError(ctx, r, data, err)
			}
			h.log.Debug("Remove finalizer successfully")
		}
		return ctrl.Result{}, nil
	}

	// Ignore if needed by annotation
	if r.GetAnnotations()[fmt.Sprintf("%s/ignoreReconcile", Base_annotation)] == "true" {
		h.log.Info("Found annotation on ressource to ignore reconcile")
		return res, nil
	}

	// Call resonsilers
	for _, reconciler := range reconcilers {
		h.log.Infof("Run phase %s", reconciler.GetName())

		data := map[string]any{}

		res, err = h.reconcilePhase(ctx, req, r, data, reconciler)
		if err != nil {
			return h.reconciler.OnError(ctx, r, data, errors.Wrapf(err, "Error when run phase %s", reconciler.GetName()))
		}

		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	return h.reconciler.OnSuccess(ctx, r, data)
}

// reconcilePhase permit to reconcile phase
// 1 Read kubernetes objects
// 2 Diff kubernetes resources with expected resources
// 3 Update / create resources if needed
// 4 Delete resources if needed
func (h *StdK8sReconciler) reconcilePhase(ctx context.Context, req ctrl.Request, r client.Object, data map[string]interface{}, reconciler K8sPhaseReconciler) (res ctrl.Result, err error) {

	var (
		diff K8sDiff
	)

	// Add setp name on logger
	log := h.log.WithFields(logrus.Fields{
		"setp": reconciler.GetName(),
	})

	// Configure
	res, err = reconciler.Configure(ctx, req, r)
	if err != nil {
		log.Errorf("Error configure reconciler: %s", err.Error())
		return reconciler.OnError(ctx, r, data, err)
	}
	if res != (ctrl.Result{}) {
		return res, nil
	}
	log.Debug("Configure reconciler successfully")

	// Read resources
	res, err = reconciler.Read(ctx, r, data)
	if err != nil {
		log.Errorf("Error when get resource: %s", err.Error())
		return reconciler.OnError(ctx, r, data, err)
	}
	if res != (ctrl.Result{}) {
		return res, nil
	}
	log.Debug("Get resource successfully")

	//Check if diff exist
	diff, res, err = reconciler.Diff(ctx, r, data)
	if err != nil {
		return reconciler.OnError(ctx, r, data, err)
	}
	if res != (ctrl.Result{}) {
		return res, nil
	}
	log.Debugf("Diff: %s", diff.Diff.String())

	// Need create resources
	if diff.NeedCreate {
		log.Debug("Start create step")
		res, err = reconciler.Create(ctx, r, data)
		if err != nil {
			return reconciler.OnError(ctx, r, data, err)
		}
	}

	// Need update resources
	if diff.NeedUpdate {
		log.Debug("Start update step")
		res, err = reconciler.Update(ctx, r, data)
		if err != nil {
			return reconciler.OnError(ctx, r, data, err)
		}
	}

	// Need Delete
	if diff.NeedDelete {
		log.Debug("Start delete step")
		res, err = reconciler.Delete(ctx, r, data)
		if err != nil {
			return reconciler.OnError(ctx, r, data, err)
		}
	}

	// Nothink to do
	if !diff.NeedCreate && !diff.NeedUpdate && !diff.NeedDelete {
		log.Debug("Nothink to do")
	}

	if res != (ctrl.Result{}) {
		return res, nil
	}

	return reconciler.OnSuccess(ctx, r, data, diff)
}
