package controller

import (
	"context"
	"reflect"
	"strings"

	"github.com/mitchellh/copystructure"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Reconciler interface {
	// Confirgure permit to init external provider driver (API client REST)
	// It can also permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, resource client.Object) (meta any, err error)

	// Read permit to read the actual resource state from provider and set it on data map
	Read(ctx context.Context, r client.Object, data map[string]any, meta any) (res ctrl.Result, err error)

	// Create permit to create resource on provider
	// It only call if diff.NeeCreated is true
	Create(ctx context.Context, r client.Object, data map[string]any, meta any) (res ctrl.Result, err error)

	// Update permit to update resource on provider
	// It only call if diff.NeedUpdated is true
	Update(ctx context.Context, r client.Object, data map[string]any, meta any) (res ctrl.Result, err error)

	// Delete permit to delete resource on provider
	// It only call if you have specified finalizer name when you create reconciler and if resource as marked to be deleted
	Delete(ctx context.Context, r client.Object, data map[string]any, meta any) (err error)

	// OnError is call when error is throwing
	// It the right way to set status condition when error
	OnError(ctx context.Context, r client.Object, data map[string]any, meta any, err error)

	// OnSuccess is call at the end if no error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, r client.Object, data map[string]any, meta any, diff Diff) (err error)

	// Diff permit to compare the actual state and the expected state
	Diff(r client.Object, data map[string]any, meta any) (diff Diff, err error)

}

type K8sReconciler interface {
	// Configure permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, resource client.Object) (meta any, err error)

	// Read permit to read kubernetes resources
	Read(ctx context.Context, r client.Object, data map[string]any, meta any) (res ctrl.Result, err error)

	// CreateOrUpdate permit to create or update resources on kubernetes
	CreateOrUpdate(ctx context.Context, r client.Object, data map[string]any, meta any) (res ctrl.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, r client.Object, data map[string]any, meta any) (res ctrl.Result, err error)

	// OnError is call when error is throwing on current phase
	// It the right way to set status condition when error
	OnError(ctx context.Context, r client.Object, data map[string]any, meta any, currentErr error) (res ctrl.Result, err error)

	// OnSuccess is call at the end of current phase, if not error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, r client.Object, data map[string]any, meta any, diff K8sDiff) (res ctrl.Result, err error)

	// Diff permit to compare the actual state and the expected state
	Diff(r client.Object, data map[string]any, meta any) (diff K8sDiff, err error)

	// Name return the reconciler name
	Name() string
}

type Diff struct {
	NeedCreate bool
	NeedUpdate bool
	Diff       string
}

type K8sDiff struct {
	NeedCreateOrUpdate bool
	NeedDelete bool
	Diff       strings.Builder
}

type StdReconciler struct {
	client.Client
	finalizer           string
	reconciler          Reconciler
	log                 *logrus.Entry
	recorder            record.EventRecorder
}


func NewStdReconciler(client client.Client, finalizer string, reconciler Reconciler, logger *logrus.Entry, recorder record.EventRecorder) (stdReconciler *StdReconciler, err error) {

	if recorder == nil {
		return nil, errors.New("recorder can't be nil")
	}

	stdReconciler = &StdReconciler{
		Client:              client,
		finalizer:           finalizer,
		reconciler:          reconciler,
		recorder:            recorder,
		log:                 logger,
	}

	if stdReconciler.log == nil {
		stdReconciler.log = logrus.NewEntry(logrus.New())
	}

	return stdReconciler, nil
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
func (h *StdReconciler) ReconcileK8sResources(ctx context.Context, req ctrl.Request, r client.Object, data map[string]interface{}, reconcilers ...K8sReconciler) (res ctrl.Result, err error) {

	var (
		meta any
	)

	// Init logger
	h.log = h.log.WithFields(logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
	})
	h.log.Infof("---> Starting reconcile loop")
	defer h.log.Info("---> Finish reconcile loop for")

	// Get current resource
	if err = h.Get(ctx, req.NamespacedName, r); err != nil {
		if k8serrors.IsNotFound(err) {
			return res, nil
		}
		return res, err
	}

	// Handle status update if exist
	if getObjectStatus(r) != nil {
		currentStatus, err := copystructure.Copy(getObjectStatus(r))
		if err != nil {
			return res, err
		}
		defer func() {
			if err != nil {
				h.reconciler.OnError(ctx, r, data, meta, err)
			}
			if !reflect.DeepEqual(currentStatus, getObjectStatus(r)) {
				h.log.Debug("Detect that it need to update status")
				if err = h.Client.Status().Update(ctx, r); err != nil {
					h.log.Errorf("Error when update resource status: %s", err.Error())
				}
				h.log.Debug("Update status successfully")
			}
		}()
	}
	

	// Add finalizer
	if h.finalizer != "" {
		if !controllerutil.ContainsFinalizer(r, h.finalizer) {
			controllerutil.AddFinalizer(r, h.finalizer)
			if err = h.Update(ctx, r); err != nil {
				h.log.Errorf("Error when add finalizer: %s", err.Error())
				h.recorder.Eventf(r, core.EventTypeWarning, "Adding finalizer", "Failed to add finalizer: %s", err)
				return res, err
			}
			h.recorder.Event(r, core.EventTypeNormal, "Added", "Object finalizer is added")
			h.log.Debug("Add finalizer successfully")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Call resonsilers
	for _, reconciler := range reconcilers {
		h.log.Infof("Run phase %s", reconciler.Name())

		res, err = h.reconcilePhase(ctx, req, r, data, reconciler)
		if err != nil {
			return res, errors.Wrapf(err, "Error when run phase %s", reconciler.Name())
		}

		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	// Handle delete finalizer
	if !getObjectMeta(r).DeletionTimestamp.IsZero() {
		if h.finalizer != "" && controllerutil.ContainsFinalizer(r, h.finalizer) {
			controllerutil.RemoveFinalizer(r, h.finalizer)
			if err = h.Update(ctx, r); err != nil {
				h.log.Errorf("Failed to remove finalizer: %s", err.Error())
				h.recorder.Eventf(r, core.EventTypeWarning, "Failed", "Error when remove finalizer: %s", err.Error())
				return res, err
			}
			h.log.Debug("Remove finalizer successfully")
		}
		return ctrl.Result{}, nil
	}

	return res, nil
}

// Reconcile permit to reconcile resource one external service, throughtout API
// It will handle all aspect of that
// 1. Read resource on kubernetes
// 2. Configure finalizer
// 3. Read external resources
// 4. Check if on delete phase, delete external resources if on it
// 5. Diff external resources with the expected resources
// 6. Create or update external resources if needed
func (h *StdReconciler) Reconcile(ctx context.Context, req ctrl.Request, r client.Object, data map[string]interface{}) (res ctrl.Result, err error) {
	var (
		meta any
		diff Diff
	)

	h.log = h.log.WithFields(logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
	})
	h.log.Infof("---> Starting reconcile loop")
	defer h.log.Info("---> Finish reconcile loop for")

	// Get current resource
	if err = h.Get(ctx, req.NamespacedName, r); err != nil {
		if k8serrors.IsNotFound(err) {
			return res, nil
		}
		return res, err
	}

	// Handle status update if exist
	if getObjectStatus(r) != nil {
		currentStatus, err := copystructure.Copy(getObjectStatus(r))
		if err != nil {
			return res, err
		}
		defer func() {
			if err != nil {
				h.reconciler.OnError(ctx, r, data, meta, err)
			}
			if !reflect.DeepEqual(currentStatus, getObjectStatus(r)) {
				h.log.Debug("Detect that it need to update status")
				if err = h.Client.Status().Update(ctx, r); err != nil {
					h.log.Errorf("Error when update resource status: %s", err.Error())
				}
				h.log.Debug("Update status successfully")
			}
		}()
	}
	

	// Add finalizer
	if h.finalizer != "" {
		if !controllerutil.ContainsFinalizer(r, h.finalizer) {
			controllerutil.AddFinalizer(r, h.finalizer)
			if err = h.Update(ctx, r); err != nil {
				h.log.Errorf("Error when add finalizer: %s", err.Error())
				h.recorder.Eventf(r, core.EventTypeWarning, "Adding finalizer", "Failed to add finalizer: %s", err)
				return res, err
			}
			h.recorder.Event(r, core.EventTypeNormal, "Added", "Object finalizer is added")
			h.log.Debug("Add finalizer successfully")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Configure to optional get driver client (call meta)
	meta, err = h.reconciler.Configure(ctx, req, r)
	if err != nil {
		h.log.Errorf("Error configure reconciler: %s", err.Error())
		return res, err
	}
	h.log.Debug("Configure reconciler successfully")

	// Read resources
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
	if !getObjectMeta(r).DeletionTimestamp.IsZero() {
		if h.finalizer != "" && controllerutil.ContainsFinalizer(r, h.finalizer) {
			h.log.Info("Start delete step")
			if err = h.reconciler.Delete(ctx, r, data, meta); err != nil {
				h.log.Errorf("Error when delete resource: %s", err.Error())
				h.recorder.Eventf(r, core.EventTypeWarning, "Failed", "Error when delete resource: %s", err.Error())
				return res, err
			}
			h.log.Debug("Delete successfully")

			controllerutil.RemoveFinalizer(r, h.finalizer)
			if err = h.Update(ctx, r); err != nil {
				h.log.Errorf("Failed to remove finalizer: %s", err.Error())
				h.recorder.Eventf(r, core.EventTypeWarning, "Failed", "Error when remove finalizer: %s", err.Error())
				return res, err
			}
			h.log.Debug("Remove finalizer successfully")
		}
		return ctrl.Result{}, nil
	}

	//Check if diff exist
	diff, err = h.reconciler.Diff(r, data, meta)
	if err != nil {
		return res, err
	}

	// Need create
	if diff.NeedCreate {
		h.log.Info("Start create step")
		res, err = h.reconciler.Create(ctx, r, data, meta)
		if err != nil {
			return res, err
		}
	}

	// Need update
	if diff.NeedUpdate {
		h.log.Infof("Start update step with diff:\n%s", diff.Diff)
		res, err = h.reconciler.Update(ctx, r, data, meta)
		if err != nil {
			return res, err
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
		return res, err
	}

	return res, nil
}

func getObjectMeta(r client.Object) metav1.ObjectMeta {
	rt := reflect.TypeOf(r)
	if rt.Kind() != reflect.Ptr {
		panic("Resource must be pointer")
	}
	rv := reflect.ValueOf(r).Elem()
	om := rv.FieldByName("ObjectMeta")
	if !om.IsValid() {
		panic("Resouce must have field ObjectMeta")
	}
	return om.Interface().(metav1.ObjectMeta)
}

func getObjectStatus(r client.Object) any {
	rt := reflect.TypeOf(r)
	if rt.Kind() != reflect.Ptr {
		panic("Resource must be pointer")
	}
	rv := reflect.ValueOf(r).Elem()
	om := rv.FieldByName("Status")
	if !om.IsValid() {
		return nil
	}
	return om.Interface()
}

// reconcilePhase permit to reconcile phase
// 1 Read kubernetes objects
// 2 Diff kubernetes resources with expected resources
// 3 Update / create resources if needed
// 4 Delete resources if needed
func (h *StdReconciler) reconcilePhase(ctx context.Context, req ctrl.Request, r client.Object, data map[string]interface{}, reconciler K8sReconciler) (res ctrl.Result, err error) {
	
	var (
		meta any
		diff K8sDiff
	)

	// Configure 
	meta, err = reconciler.Configure(ctx, req, r)
	if err != nil {
		h.log.Errorf("Error configure reconciler: %s", err.Error())
		return reconciler.OnError(ctx, r, data, meta, err)
	}
	h.log.Debug("Configure reconciler successfully")

	// Read resources
	res, err = reconciler.Read(ctx, r, data, meta)
	if err != nil {
		h.log.Errorf("Error when get resource: %s", err.Error())
		return reconciler.OnError(ctx, r, data, meta, err)
	}
	if res != (ctrl.Result{}) {
		return res, nil
	}
	h.log.Debug("Get resource successfully")

	//Check if diff exist
	diff, err = reconciler.Diff(r, data, meta)
	if err != nil {
		return reconciler.OnError(ctx, r, data, meta, err)
	}
	h.log.Debugf("Diff: %s", diff.Diff.String())

	// Need create or update resources
	if diff.NeedCreateOrUpdate {
		h.log.Debug("Start create / update step")
		res, err = reconciler.CreateOrUpdate(ctx, r, data, meta)
		if err != nil {
			return reconciler.OnError(ctx, r, data, meta, err)
		}
	}

	// Need Delete
	if diff.NeedDelete {
		h.log.Debug("Start delete step")
		res, err = reconciler.Delete(ctx, r, data, meta)
		if err != nil {
			return reconciler.OnError(ctx, r, data, meta, err)
		}
	}

	// Nothink to do
	if !diff.NeedCreateOrUpdate && !diff.NeedDelete {
		h.log.Debug("Nothink to do")
	}

	if res != (ctrl.Result{}) {
		return res, nil
	}

	return reconciler.OnSuccess(ctx, r, data, meta, diff)
}