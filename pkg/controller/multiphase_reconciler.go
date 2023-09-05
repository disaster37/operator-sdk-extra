package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	"github.com/mitchellh/copystructure"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	condition "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/strings"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// MultiPhaseReconciler is the reconciler to implement whe you need to create multiple resources on k8s
type MultiPhaseReconciler interface {
	BaseReconciler

	// Configure permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject) (res ctrl.Result, err error)

	// Read permit to read kubernetes resources
	Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (res ctrl.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (err error)

	// OnError is call when error is throwing on current phase
	// It the right way to set status condition when error
	OnError(ctx context.Context, o object.MultiPhaseObject, data map[string]any, currentErr error) (res ctrl.Result, err error)

	// OnSuccess is call at the end of current phase, if not error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (res ctrl.Result, err error)

	// Reconcile permit to orchestrate all phase needed to successfully reconcile the object
	Reconcile(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject, data map[string]interface{}, reconcilers ...MultiPhaseStepReconciler) (res ctrl.Result, err error)

	// GetConditionName permit to get the main condition name
	GetConditionName() shared.ConditionName
}

// BasicMultiPhaseReconciler is the basic multi phase reconsiler you can used whe  you should to create multiple k8s resources
type BasicMultiPhaseReconciler struct {
	BasicReconciler
}

// NewBasicMultiPhaseReconciler permit to instanciate new basic multiphase resonciler
func NewBasicMultiPhaseReconciler(client client.Client, name string, finalizer shared.FinalizerName, conditionName shared.ConditionName, logger *logrus.Entry, recorder record.EventRecorder) (multiPhaseReconciler MultiPhaseReconciler, err error) {

	if recorder == nil {
		return nil, errors.New("recorder can't be nil")
	}

	basicMultiPhaseReconciler := &BasicMultiPhaseReconciler{
		BasicReconciler: BasicReconciler{
			finalizer: finalizer,
			log: logger.WithFields(logrus.Fields{
				"reconciler": name,
			}),
			recorder:      recorder,
			Client:        client,
			conditionName: conditionName,
			name:          name,
		},
	}

	if basicMultiPhaseReconciler.log == nil {
		basicMultiPhaseReconciler.log = logrus.NewEntry(logrus.New())
	}

	return basicMultiPhaseReconciler, nil
}

func (h *BasicMultiPhaseReconciler) GetConditionName() shared.ConditionName {
	return h.conditionName
}

func (h *BasicMultiPhaseReconciler) Configure(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject) (res ctrl.Result, err error) {
	o.GetStatus().SetIsOnError(false)
	o.GetStatus().SetLastErrorMessage("")

	conditions := o.GetStatus().GetConditions()

	// Init condition status if not exist
	if condition.FindStatusCondition(conditions, h.GetConditionName().String()) == nil {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:   h.GetConditionName().String(),
			Status: metav1.ConditionFalse,
			Reason: "Initialize",
		})
	}

	return res, nil
}

func (h *BasicMultiPhaseReconciler) Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (res ctrl.Result, err error) {
	return
}

func (h *BasicMultiPhaseReconciler) Delete(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (err error) {
	return
}

func (h *BasicMultiPhaseReconciler) OnError(ctx context.Context, o object.MultiPhaseObject, data map[string]any, currentErr error) (res ctrl.Result, err error) {

	o.GetStatus().SetIsOnError(true)
	o.GetStatus().SetLastErrorMessage(strings.ShortenString(err.Error(), shared.ShortenError))

	conditions := o.GetStatus().GetConditions()

	condition.SetStatusCondition(&conditions, metav1.Condition{
		Type:    h.GetConditionName().String(),
		Status:  metav1.ConditionFalse,
		Reason:  "Failed",
		Message: strings.ShortenString(err.Error(), shared.ShortenError),
	})

	return res, errors.Wrapf(err, "Error on %s controller", h.name)
}
func (h *BasicMultiPhaseReconciler) OnSuccess(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (res ctrl.Result, err error) {
	conditions := o.GetStatus().GetConditions()

	if !condition.IsStatusConditionPresentAndEqual(conditions, h.GetConditionName().String(), metav1.ConditionTrue) {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:   h.GetConditionName().String(),
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		})
	}

	o.GetStatus().SetPhaseName(shared.RunningPhase)

	return res, nil
}
func (h *BasicMultiPhaseReconciler) Reconcile(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject, data map[string]interface{}, reconcilers ...MultiPhaseStepReconciler) (res ctrl.Result, err error) {

	// Init logger
	h.log = h.log.WithFields(logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
	})

	log := h.log.WithFields(logrus.Fields{
		"module": "multiPhaseReconciler",
	})

	log.Infof("---> Starting reconcile loop")
	defer log.Info("---> Finish reconcile loop for")

	// Wait few second to be sure status is propaged througout ETCD
	time.Sleep(time.Second * 1)

	// Get current resource
	if err = h.Get(ctx, req.NamespacedName, o); err != nil {
		if k8serrors.IsNotFound(err) {
			return res, nil
		}
		log.Errorf("Error when get object: %s", err.Error())
		return res, errors.Wrap(err, ErrWhenGetObjectFromReconciler.Error())
	}
	log.Debug("Get object successfully")

	// Add finalizer
	if h.finalizer != "" {
		if !controllerutil.ContainsFinalizer(o, h.finalizer.String()) {
			controllerutil.AddFinalizer(o, h.finalizer.String())
			if err = h.Update(ctx, o); err != nil {
				log.Errorf("Error when add finalizer: %s", err.Error())
				return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenAddFinalizer.Error()))
			}
			log.Debug("Add finalizer successfully, force requeue object")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Handle status update if exist
	if getObjectStatus(o) != nil {
		currentStatus, err := copystructure.Copy(getObjectStatus(o))
		if err != nil {
			log.Errorf("Error when get object status: %s", err.Error())
			return res, errors.Wrap(err, ErrWhenGetObjectStatus.Error())
		}
		defer func() {
			if !reflect.DeepEqual(currentStatus, getObjectStatus(o)) {
				log.Debug("Detect that it need to update status")
				if err = h.Client.Status().Update(ctx, o); err != nil {
					log.Errorf("Error when update resource status: %s", err.Error())
				}
				log.Debug("Update status successfully")
			}
		}()
	}

	// Configure to optional get driver client (call meta)
	res, err = h.Configure(ctx, req, o)
	if err != nil {
		log.Errorf("Error when call 'configure' from reconciler: %s", err.Error())
		return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallConfigureFromReconciler.Error()))
	}
	log.Debug("Call 'configure' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Read resources
	res, err = h.Read(ctx, o, data)
	if err != nil {
		log.Errorf("Error when call 'read' from reconciler: %s", err.Error())
		return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallReadFromReconciler.Error()))
	}
	log.Debug("Call 'read' from reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Handle delete finalizer
	if !getObjectMeta(o).DeletionTimestamp.IsZero() {
		if h.finalizer.String() != "" && controllerutil.ContainsFinalizer(o, h.finalizer.String()) {
			if err = h.Delete(ctx, o, data); err != nil {
				log.Errorf("Error when call 'delete' from reconciler: %s", err.Error())
				return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallDeleteFromReconciler.Error()))
			}
			log.Debug("Delete successfully")

			controllerutil.RemoveFinalizer(o, h.finalizer.String())
			if err = h.Update(ctx, o); err != nil {
				log.Errorf("Failed to remove finalizer: %s", err.Error())
				return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenDeleteFinalizer.Error()))
			}
			log.Debug("Remove finalizer successfully")
		}
		return ctrl.Result{}, nil
	}

	// Ignore if needed by annotation
	if o.GetAnnotations()[fmt.Sprintf("%s/ignoreReconcile", shared.BaseAnnotation)] == "true" {
		log.Info("Found annotation on ressource to ignore reconcile")
		return res, nil
	}

	// Call step resonsilers
	for _, reconciler := range reconcilers {
		log.Infof("Run phase %s", reconciler.GetPhaseName().String())

		data := map[string]any{}

		res, err = reconciler.Reconcile(ctx, req, o, data)
		if err != nil {
			log.Errorf("Error when call 'reconcile' from step reconciler %s", reconciler.GetPhaseName().String())
			return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallStepReconcilerFromReconciler.Error()))
		}
		log.Debug("Call 'reconcile' from step reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	res, err = h.OnSuccess(ctx, o, data)
	if err != nil {
		log.Errorf("Error when call 'onSuccess' from reconciler: %s", err.Error())
		return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallOnSuccessFromReconciler.Error()))
	}

	return res, nil
}
