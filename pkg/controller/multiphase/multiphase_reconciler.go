package multiphase

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/object"
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
type MultiPhaseReconciler[k8sObject object.MultiPhaseObject] interface {
	controller.Reconciler

	// Reconcile permit to orchestrate all phase needed to successfully reconcile the object
	Reconcile(ctx context.Context, req ctrl.Request, o k8sObject, data map[string]interface{}, reconcilerAction MultiPhaseReconcilerAction[k8sObject], reconcilersStepAction ...MultiPhaseStepReconcilerAction[k8sObject, client.Object]) (res ctrl.Result, err error)
}

// DefaultMultiPhaseReconciler is the default multi phase reconsiler you can used when  you should to create multiple k8s resources
type DefaultMultiPhaseReconciler[k8sObject object.MultiPhaseObject] struct {
	controller.Reconciler
	reconcilerStep MultiPhaseStepReconciler[k8sObject, client.Object]
}

// NewMultiPhaseReconciler is the default implementation of MultiPhaseReconciler
func NewMultiPhaseReconciler[k8sObject object.MultiPhaseObject](c client.Client, name string, finalizer shared.FinalizerName, logger *logrus.Entry, recorder record.EventRecorder) (multiPhaseReconciler MultiPhaseReconciler[k8sObject]) {

	return &DefaultMultiPhaseReconciler[k8sObject]{
		Reconciler: controller.NewReconciler(
			c,
			recorder,
			finalizer,
			logger.WithFields(logrus.Fields{
				"reconciler": name,
			}),
		),
		reconcilerStep: NewMultiPhaseStepReconciler[k8sObject, client.Object](c, logger, recorder),
	}
}

func (h *DefaultMultiPhaseReconciler[k8sObject]) Reconcile(ctx context.Context, req ctrl.Request, o k8sObject, data map[string]interface{}, reconcilerAction MultiPhaseReconcilerAction[k8sObject], reconcilersStepAction ...MultiPhaseStepReconcilerAction[k8sObject, client.Object]) (res ctrl.Result, err error) {

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
	if err = h.Client().Get(ctx, req.NamespacedName, o); err != nil {
		if k8serrors.IsNotFound(err) {
			return res, nil
		}
		logger.Errorf("Error when get object: %s", err.Error())
		return res, errors.Wrap(err, controller.ErrWhenGetObjectFromReconciler.Error())
	}
	logger.Debug("Get object successfully")

	// Add finalizer
	if h.Finalizer() != "" {
		if !controllerutil.ContainsFinalizer(o, h.Finalizer().String()) {
			controllerutil.AddFinalizer(o, h.Finalizer().String())
			if err = h.Client().Update(ctx, o); err != nil {
				logger.Errorf("Error when add finalizer: %s", err.Error())
				return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenAddFinalizer.Error()), logger)
			}
			logger.Debug("Add finalizer successfully, force requeue object")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Handle status update if exist
	if controller.GetObjectStatus(o) != nil {
		currentStatus, err := copystructure.Copy(controller.GetObjectStatus(o))
		if err != nil {
			logger.Errorf("Error when get object status: %s", err.Error())
			return res, errors.Wrap(err, controller.ErrWhenGetObjectStatus.Error())
		}
		defer func() {
			if !reflect.DeepEqual(currentStatus, controller.GetObjectStatus(o)) {
				logger.Debugf("Detect that it need to update status with diff:\n%s", cmp.Diff(currentStatus, controller.GetObjectStatus(o)))
				if err = h.Client().Status().Update(ctx, o); err != nil {
					logger.Errorf("Error when update resource status: %s", err.Error())
				}
				logger.Debug("Update status successfully")
			}
		}()
	}

	// Ignore if needed by annotation
	if o.GetAnnotations()[fmt.Sprintf("%s/ignoreReconcile", controller.BaseAnnotation)] == "true" {
		logger.Info("Found annotation on ressource to ignore reconcile")
		return res, nil
	}

	// Configure to optional get driver client (call meta)
	res, err = reconcilerAction.Configure(ctx, req, o, data, logger)
	if err != nil {
		logger.Errorf("Error when call 'configure' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallConfigureFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'configure' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Read resources
	res, err = reconcilerAction.Read(ctx, o, data, logger)
	if err != nil {
		logger.Errorf("Error when call 'read' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallReadFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'read' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Handle delete finalizer
	if !controller.GetObjectMeta(o).DeletionTimestamp.IsZero() {
		if h.Finalizer().String() != "" && controllerutil.ContainsFinalizer(o, h.Finalizer().String()) {
			if err = reconcilerAction.Delete(ctx, o, data, logger); err != nil {
				logger.Errorf("Error when call 'delete' from reconciler: %s", err.Error())
				return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallDeleteFromReconciler.Error()), logger)
			}
			logger.Debug("Delete successfully")

			controllerutil.RemoveFinalizer(o, h.Finalizer().String())
			if err = h.Client().Update(ctx, o); err != nil {
				logger.Errorf("Failed to remove finalizer: %s", err.Error())
				return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenDeleteFinalizer.Error()), logger)
			}
			logger.Debug("Remove finalizer successfully")
		}
		return ctrl.Result{}, nil
	}

	// Call step resonsilers
	for _, reconciler := range reconcilersStepAction {
		logger.Infof("Run phase %s", reconciler.GetPhaseName().String())

		res, err = h.reconcilerStep.Reconcile(ctx, req, o, data, reconciler, logger, reconciler.GetIgnoresDiff()...)
		if err != nil {
			logger.Errorf("Error when call 'reconcile' from step reconciler %s", reconciler.GetPhaseName().String())
			return reconciler.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallStepReconcilerFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'reconcile' from step reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}

		time.Sleep(time.Millisecond * 1)
	}

	res, err = reconcilerAction.OnSuccess(ctx, o, data, logger)
	if err != nil {
		logger.Errorf("Error when call 'onSuccess' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallOnSuccessFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'onSuccess' from reconciler")

	return res, nil
}
