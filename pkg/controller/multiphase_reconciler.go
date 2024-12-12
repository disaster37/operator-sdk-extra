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

// MultiPhaseReconciler the reconciler to implement whe you need to create multiple resources on k8s
type MultiPhaseReconciler interface {

	// Reconcile permit to orchestrate all phase needed to successfully reconcile the object
	Reconcile(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject, data map[string]interface{}, reconciler MultiPhaseReconcilerAction, reconcilersStep ...MultiPhaseStepReconcilerAction) (res ctrl.Result, err error)
}

// BasicMultiPhaseReconciler is the basic multi phase reconsiler you can used whe  you should to create multiple k8s resources
type BasicMultiPhaseReconciler struct {
	BasicReconciler
}

// NewBasicMultiPhaseReconciler permit to instanciate new basic multiphase resonciler
func NewBasicMultiPhaseReconciler(client client.Client, name string, finalizer shared.FinalizerName, logger *logrus.Entry, recorder record.EventRecorder) (multiPhaseReconciler MultiPhaseReconciler) {

	return &BasicMultiPhaseReconciler{
		BasicReconciler: BasicReconciler{
			BaseReconciler: NewDefaultBaseReconciler(
				client,
				recorder,
				logger.WithFields(logrus.Fields{
					"reconciler": name,
				}),
			),
			finalizer: finalizer,
		},
	}
}

func (h *BasicMultiPhaseReconciler) Reconcile(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject, data map[string]interface{}, reconcilerAction MultiPhaseReconcilerAction, reconcilersStepAction ...MultiPhaseStepReconcilerAction) (res ctrl.Result, err error) {

	// Init logger
	loggerFields := logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
	}
	logger := h.Logger().WithFields(loggerFields)
	logger.Infof("Starting reconcile loop")
	defer logger.Info("Finish reconcile loop")
	reconcilerAction.WithLoggerFields(loggerFields)
	stepReconciler := NewBasicMultiPhaseStepReconciler(h.Client(), logger, h.Recorder())

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
				return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenAddFinalizer.Error()))
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

	// Configure to optional get driver client (call meta)
	res, err = reconcilerAction.Configure(ctx, req, o)
	if err != nil {
		logger.Errorf("Error when call 'configure' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallConfigureFromReconciler.Error()))
	}
	logger.Debug("Call 'configure' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Read resources
	res, err = reconcilerAction.Read(ctx, o, data)
	if err != nil {
		logger.Errorf("Error when call 'read' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallReadFromReconciler.Error()))
	}
	logger.Debug("Call 'read' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Handle delete finalizer
	if !getObjectMeta(o).DeletionTimestamp.IsZero() {
		if h.finalizer.String() != "" && controllerutil.ContainsFinalizer(o, h.finalizer.String()) {
			if err = reconcilerAction.Delete(ctx, o, data); err != nil {
				logger.Errorf("Error when call 'delete' from reconciler: %s", err.Error())
				return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallDeleteFromReconciler.Error()))
			}
			logger.Debug("Delete successfully")

			controllerutil.RemoveFinalizer(o, h.finalizer.String())
			if err = h.Client().Update(ctx, o); err != nil {
				logger.Errorf("Failed to remove finalizer: %s", err.Error())
				return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenDeleteFinalizer.Error()))
			}
			logger.Debug("Remove finalizer successfully")
		}
		return ctrl.Result{}, nil
	}

	// Call step resonsilers
	for _, reconciler := range reconcilersStepAction {
		logger.Infof("Run phase %s", reconciler.GetPhaseName().String())
		reconciler.WithLoggerFields(loggerFields)

		data := map[string]any{}

		res, err = stepReconciler.Reconcile(ctx, req, o, data, reconciler, reconciler.GetIgnoresDiff()...)
		if err != nil {
			logger.Errorf("Error when call 'reconcile' from step reconciler %s", reconciler.GetPhaseName().String())
			return reconciler.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallStepReconcilerFromReconciler.Error()))
		}
		logger.Debug("Call 'reconcile' from step reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}

		time.Sleep(time.Millisecond * 1)
	}

	res, err = reconcilerAction.OnSuccess(ctx, o, data)
	if err != nil {
		logger.Errorf("Error when call 'onSuccess' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallOnSuccessFromReconciler.Error()))
	}
	logger.Debug("Call 'onSuccess' from reconciler")

	return res, nil
}
