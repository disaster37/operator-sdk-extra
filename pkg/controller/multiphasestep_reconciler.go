package controller

import (
	"context"
	"reflect"

	"emperror.dev/errors"
	"github.com/disaster37/k8s-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseStepReconciler is the reconciler to implement to create one step for MultiPhaseReconciler
type MultiPhaseStepReconciler interface {
	BaseReconciler

	// Reconcile permit to reconcile the step (one K8s resource)
	Reconcile(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject, data map[string]interface{}, reconciler MultiPhaseStepReconcilerAction, logger *logrus.Entry, ignoresDiff ...patch.CalculateOption) (res ctrl.Result, err error)
}

// BasicMultiPhaseStepReconciler is the basic implementation of MultiPhaseStepReconciler interface
type BasicMultiPhaseStepReconciler struct {
	BaseReconciler
}

// NewBasicMultiPhaseStepReconciler is the basic constructor of MultiPhaseStepReconciler interface
func NewBasicMultiPhaseStepReconciler(client client.Client, logger *logrus.Entry, recorder record.EventRecorder) (multiPhaseStepReconciler MultiPhaseStepReconciler) {
	return &BasicMultiPhaseStepReconciler{
		BaseReconciler: NewBaseReconciler(client, recorder),
	}
}

// Reconcile permit to reconcile the step (one K8s resource)
func (h *BasicMultiPhaseStepReconciler) Reconcile(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject, data map[string]interface{}, reconcilerAction MultiPhaseStepReconcilerAction, logger *logrus.Entry, ignoresDiff ...patch.CalculateOption) (res ctrl.Result, err error) {

	var (
		diff MultiPhaseDiff
		read MultiPhaseRead
	)

	// Init logger
	logger = logger.WithFields(logrus.Fields{
		"step": reconcilerAction.GetPhaseName().String(),
	})

	// Configure
	res, err = reconcilerAction.Configure(ctx, req, o, logger)
	if err != nil {
		logger.Errorf("Error when call 'configure' from step reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallConfigureFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'configure' from step reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Read resources
	read, res, err = reconcilerAction.Read(ctx, o, data, logger)
	if err != nil {
		logger.Errorf("Error when call 'read' from step reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallReadFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'read' from step reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	//Check if diff exist
	diff, res, err = reconcilerAction.Diff(ctx, o, read, data, logger, ignoresDiff...)
	if err != nil {
		logger.Errorf("Error when call 'diff' from step reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallDiffFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'diff' from step reconciler successfully")
	if diff.IsDiff() {
		logger.Debugf("Found diff: %s", diff.Diff())
	}
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Need create resources
	if diff.NeedCreate() {
		logger.Debug("Call 'create' from step reconciler")
		res, err = reconcilerAction.Create(ctx, o, data, diff.GetObjectsToCreate(), logger)
		if err != nil {
			logger.Errorf("Error when call 'create' from step reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallCreateFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'create' from step reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	// Need update resources
	if diff.NeedUpdate() {
		logger.Debug("Call 'update' from step reconciler")
		res, err = reconcilerAction.Update(ctx, o, data, diff.GetObjectsToUpdate(), logger)
		if err != nil {
			logger.Errorf("Error when call 'update' from step reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallUpdateFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'update' from step reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	// Need Delete
	if diff.NeedDelete() {
		logger.Debug("Call 'delete' from step reconciler")
		res, err = reconcilerAction.Delete(ctx, o, data, diff.GetObjectsToDelete(), logger)
		if err != nil {
			logger.Errorf("Error when call 'delete' from step reconciler: %s", err.Error())
			return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallDeleteFromReconciler.Error()), logger)
		}
		logger.Debug("Call 'delete' from step reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	res, err = reconcilerAction.OnSuccess(ctx, o, data, diff, logger)
	if err != nil {
		logger.Errorf("Error when call 'onSuccess' from step reconciler: %s", err.Error())
		return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallOnSuccessFromReconciler.Error()), logger)
	}
	logger.Debug("Call 'onSuccess' from step reconciler successfully")

	return res, nil
}

func MustInjectTypeMeta(src, dst client.Object) {
	var (
		rt reflect.Type
	)

	rt = reflect.TypeOf(src)
	if rt.Kind() != reflect.Ptr {
		panic("Resource must be pointer")
	}
	rt = reflect.TypeOf(dst)
	if rt.Kind() != reflect.Ptr {
		panic("Resource must be pointer")
	}

	rvSrc := reflect.ValueOf(src).Elem()
	omSrc := rvSrc.FieldByName("TypeMeta")
	if !omSrc.IsValid() {
		panic("src must have field TypeMeta")
	}
	rvDst := reflect.ValueOf(dst).Elem()
	omDst := rvDst.FieldByName("TypeMeta")
	if !omDst.IsValid() {
		panic("dst must have field TypeMeta")
	}

	omDst.Set(omSrc)
}
