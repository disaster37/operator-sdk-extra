package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"emperror.dev/errors"
	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/copystructure"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SentinelReconciler must be used when you look resource that your operator is not the owner like ingress, secret, configMap, etc.
// Some time you should to generate somme resource from labels or annotations ...
// It the use case of this controller
type SentinelReconciler interface {

	// Reconcile permit to orchestrate all phase needed to successfully reconcile the object
	Reconcile(ctx context.Context, req ctrl.Request, o client.Object, data map[string]interface{}, reconciler SentinelReconcilerAction) (res ctrl.Result, err error)
}

// BasicSentinelReconciler is the basic sentinel reconsiler
type BasicSentinelReconciler struct {
	BasicReconciler
}

// NewBasicSentinelReconciler permit to instanciate new basic sentinel resonciler
func NewBasicSentinelReconciler(client client.Client, name string, logger *logrus.Entry, recorder record.EventRecorder) (sentinelReconciler SentinelReconciler) {

	return &BasicSentinelReconciler{
		BasicReconciler: NewBasicReconciler(
			client,
			recorder,
			"",
			logger.WithFields(logrus.Fields{
				"reconciler": name,
			}),
		),
	}
}

// No need to add finalizer and manage delete
// All sub resources must be children of main parent. So the clean is handled by kubelet in lazy effort
func (h *BasicSentinelReconciler) Reconcile(ctx context.Context, req ctrl.Request, o client.Object, data map[string]interface{}, reconcilerAction SentinelReconcilerAction) (res ctrl.Result, err error) {

	var (
		read SentinelRead
		diff SentinelDiff
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
	res, err = reconcilerAction.Configure(ctx, req, o, data, logger)
	if err != nil {
		logger.Errorf("Error when call 'configure' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallConfigureFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'configure' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Read resources
	read, res, err = reconcilerAction.Read(ctx, o, data, logger)
	if err != nil {
		logger.Errorf("Error when call 'read' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallReadFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'read' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Check if diff exist
	diff, res, err = reconcilerAction.Diff(ctx, o, read, data, logger, reconcilerAction.GetIgnoresDiff()...)
	if err != nil {
		logger.Errorf("Failed to call 'diff' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallDiffFromReconciler.Error()), logger)
	}
	logger.Debugf("Call 'diff' from reconciler successfully with diff:\n%s", diff.Diff())
	if res != (ctrl.Result{}) {
		return res, nil
	}

	if diff.NeedCreate() {
		res, err = reconcilerAction.Create(ctx, o, data, diff.GetObjectsToCreate(), logger)
		if err != nil {
			logger.Errorf("Failed to call 'create' from reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallCreateFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'create' from reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	if diff.NeedUpdate() {
		res, err = reconcilerAction.Update(ctx, o, data, diff.GetObjectsToUpdate(), logger)
		if err != nil {
			logger.Errorf("Failed to call 'update' from reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallUpdateFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'update' from reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	if diff.NeedDelete() {
		err = reconcilerAction.Delete(ctx, o, data, diff.GetObjectsToDelete(), logger)
		if err != nil {
			logger.Errorf("Failed to call 'delete' from reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallUpdateFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'delete' from reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	res, err = reconcilerAction.OnSuccess(ctx, o, data, diff, logger)
	if err != nil {
		logger.Errorf("Error when call 'onSuccess' from reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallOnSuccessFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'onSuccess' from reconciler")

	return res, nil
}
