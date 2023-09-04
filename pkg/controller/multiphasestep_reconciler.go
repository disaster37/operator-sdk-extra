package controller

import (
	"context"
	"fmt"
	"reflect"

	"emperror.dev/errors"
	"github.com/disaster37/k8s-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/pkg/helper"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	condition "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	k8sstrings "k8s.io/utils/strings"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseStepReconciler is the reconciler to implement to create one step for MultiPhaseReconciler
type MultiPhaseStepReconciler interface {
	// Configure permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, o MultiPhaseObject) (res ctrl.Result, err error)

	// Read permit to read kubernetes resources
	Read(ctx context.Context, o MultiPhaseObject, data map[string]any) (read MultiPhaseRead, res ctrl.Result, err error)

	// Create permit to create resources on kubernetes
	Create(ctx context.Context, o MultiPhaseObject, data map[string]any, objects []client.Object) (res ctrl.Result, err error)

	// Update permit to update resources on kubernetes
	Update(ctx context.Context, o MultiPhaseObject, data map[string]any, objects []client.Object) (res ctrl.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, o MultiPhaseObject, data map[string]any, objects []client.Object) (res ctrl.Result, err error)

	// OnError is call when error is throwing on current phase
	// It the right way to set status condition when error
	OnError(ctx context.Context, o MultiPhaseObject, data map[string]any, currentErr error) (res ctrl.Result, err error)

	// OnSuccess is call at the end of current phase, if not error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, o MultiPhaseObject, data map[string]any, diff MultiPhaseDiff) (res ctrl.Result, err error)

	// Diff permit to compare the actual state and the expected state
	Diff(ctx context.Context, o MultiPhaseObject, read MultiPhaseRead, data map[string]any, ignoreDiff ...patch.CalculateOption) (diff MultiPhaseDiff, res ctrl.Result, err error)

	// GetPhaseName permit to get the phase name
	GetPhaseName() PhaseName

	// GetConditionName permit to get the main condition name
	GetConditionName() ConditionName

	// Reconcile permit to reconcile the step (one K8s resource)
	Reconcile(ctx context.Context, req ctrl.Request, o MultiPhaseObject, data map[string]interface{}) (res ctrl.Result, err error)
}

type BasicMultiPhaseStepReconciler struct {
	recorder record.EventRecorder
	client.Client
	log           *logrus.Entry
	scheme        *runtime.Scheme
	phaseName     PhaseName
	conditionName ConditionName
	ignoresDiff   []patch.CalculateOption
}

func NewBasicMultiPhaseStepReconciler(client client.Client, phaseName PhaseName, conditionName ConditionName, logger *logrus.Entry, recorder record.EventRecorder, scheme *runtime.Scheme, ignoresDiff ...patch.CalculateOption) (multiPhaseStepReconciler MultiPhaseStepReconciler, err error) {
	if recorder == nil {
		return nil, errors.New("recorder can't be nil")
	}

	return &BasicMultiPhaseStepReconciler{
		recorder: recorder,
		log: logger.WithFields(logrus.Fields{
			"phase": phaseName.String(),
		}),
		scheme:        scheme,
		phaseName:     phaseName,
		conditionName: conditionName,
		Client:        client,
		ignoresDiff:   ignoresDiff,
	}, nil
}
func (h *BasicMultiPhaseStepReconciler) Configure(ctx context.Context, req ctrl.Request, o MultiPhaseObject) (res ctrl.Result, err error) {
	conditions := o.GetConditions()

	// Init condition
	if condition.FindStatusCondition(conditions, h.GetConditionName().String()) == nil {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:   h.GetConditionName().String(),
			Status: metav1.ConditionFalse,
			Reason: "Initialize",
		})
	}

	// Init phase
	o.SetPhaseName(h.GetPhaseName())

	return res, nil
}
func (h *BasicMultiPhaseStepReconciler) Read(ctx context.Context, o MultiPhaseObject, data map[string]any) (read MultiPhaseRead, res ctrl.Result, err error) {
	panic("You need implement it")
}

func (h *BasicMultiPhaseStepReconciler) Create(ctx context.Context, o MultiPhaseObject, data map[string]any, objects []client.Object) (res ctrl.Result, err error) {

	for _, oChild := range objects {
		if err = h.Client.Create(ctx, oChild); err != nil {
			return res, errors.Wrapf(err, "Error when create object of type '%s' with name '%s'", oChild.GetObjectKind().GroupVersionKind().Kind, oChild.GetName())
		}
		h.log.Debugf("Create object '%s' of type '%s' successfully", oChild.GetName(), oChild.GetObjectKind().GroupVersionKind().Kind)
		h.recorder.Eventf(o, corev1.EventTypeNormal, "CreateCompleted", "Object '%s' of type '%s' successfully created", oChild.GetName(), oChild.GetObjectKind().GroupVersionKind().Kind)
	}

	return res, nil
}

func (h *BasicMultiPhaseStepReconciler) Update(ctx context.Context, o MultiPhaseObject, data map[string]any, objects []client.Object) (res ctrl.Result, err error) {

	for _, oChild := range objects {
		if err = h.Client.Update(ctx, oChild); err != nil {
			return res, errors.Wrapf(err, "Error when update object of type '%s' with name '%s'", oChild.GetObjectKind().GroupVersionKind().Kind, oChild.GetName())
		}
		h.log.Debugf("Update object '%s' of type '%s' successfully", oChild.GetName(), oChild.GetObjectKind().GroupVersionKind().Kind)
		h.recorder.Eventf(o, corev1.EventTypeNormal, "UpdateCompleted", "Object '%s' of type '%s' successfully created", oChild.GetName(), oChild.GetObjectKind().GroupVersionKind().Kind)
	}

	return res, nil
}

func (h *BasicMultiPhaseStepReconciler) Delete(ctx context.Context, o MultiPhaseObject, data map[string]any, objects []client.Object) (res ctrl.Result, err error) {

	for _, oChild := range objects {
		if err = h.Client.Delete(ctx, oChild); err != nil {
			return res, errors.Wrapf(err, "Error when delete object of type '%s' with name '%s'", oChild.GetObjectKind().GroupVersionKind().Kind, oChild.GetName())
		}
		h.log.Debugf("Delete object '%s' of type '%s' successfully", oChild.GetName(), oChild.GetObjectKind().GroupVersionKind().Kind)
		h.recorder.Eventf(o, corev1.EventTypeNormal, "DeleteCompleted", "Object '%s' of type '%s' successfully created", oChild.GetName(), oChild.GetObjectKind().GroupVersionKind().Kind)
	}

	return res, nil
}

func (h *BasicMultiPhaseStepReconciler) OnError(ctx context.Context, o MultiPhaseObject, data map[string]any, currentErr error) (res ctrl.Result, err error) {
	conditions := o.GetConditions()

	condition.SetStatusCondition(&conditions, metav1.Condition{
		Type:    h.GetConditionName().String(),
		Status:  metav1.ConditionFalse,
		Reason:  "Failed",
		Message: k8sstrings.ShortenString(currentErr.Error(), ShortenError),
	})

	var (
		errorMessage string
		reason       string
	)
	switch errors.Cause(currentErr) {
	case ErrWhenCallConfigureFromReconciler:
		errorMessage = fmt.Sprintf("Error when call 'configure' on step %s", h.GetPhaseName().String())
		reason = "ConfigureFailed"
	case ErrWhenCallReadFromReconciler:
		errorMessage = fmt.Sprintf("Error when call 'read' on step %s", h.GetPhaseName().String())
		reason = "ReadFailed"
	case ErrWhenCallDiffFromReconciler:
		errorMessage = fmt.Sprintf("Error when call 'diff' on step %s", h.GetPhaseName().String())
		reason = "DiffFailed"
	case ErrWhenCallCreateFromReconciler:
		errorMessage = fmt.Sprintf("Error when call 'create' on step %s", h.GetPhaseName().String())
		reason = "CreateFailed"
	case ErrWhenCallUpdateFromReconciler:
		errorMessage = fmt.Sprintf("Error when call 'update' on step %s", h.GetPhaseName().String())
		reason = "UpdateFailed"
	case ErrWhenCallDeleteFromReconciler:
		errorMessage = fmt.Sprintf("Error when call 'delete' on step %s", h.GetPhaseName().String())
		reason = "DeleteFailed"
	case ErrWhenCallOnSuccessFromReconciler:
		errorMessage = fmt.Sprintf("Error when call 'onSuccess' on step %s", h.GetPhaseName().String())
		reason = "OnSuccessFailed"
	default:
		errorMessage = fmt.Sprintf("Framework error on step %s", h.GetPhaseName().String())
		reason = "FrameworkFailed"
	}
	h.recorder.Event(o, corev1.EventTypeWarning, reason, errorMessage)
	return res, errors.New(errorMessage)

}

func (h *BasicMultiPhaseStepReconciler) OnSuccess(ctx context.Context, o MultiPhaseObject, data map[string]any, diff MultiPhaseDiff) (res ctrl.Result, err error) {
	conditions := o.GetConditions()

	// Update condition status if needed
	if !condition.IsStatusConditionPresentAndEqual(conditions, h.GetConditionName().String(), metav1.ConditionTrue) {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:    h.GetConditionName().String(),
			Reason:  "Success",
			Status:  metav1.ConditionTrue,
			Message: "Ready",
		})
	}

	return res, nil
}

func (h *BasicMultiPhaseStepReconciler) Diff(ctx context.Context, o MultiPhaseObject, read MultiPhaseRead, data map[string]any, ignoreDiff ...patch.CalculateOption) (diff MultiPhaseDiff, res ctrl.Result, err error) {

	tmpCurrentObjects := make([]client.Object, len(read.GetCurrentObjects()))
	copy(tmpCurrentObjects, read.GetCurrentObjects())

	diff = NewBasicMultiPhaseDiff()

	patchOptions := []patch.CalculateOption{
		patch.CleanMetadata(),
		patch.IgnoreStatusFields(),
	}
	patchOptions = append(patchOptions, ignoreDiff...)

	toUpdate := make([]client.Object, 0)
	toCreate := make([]client.Object, 0)

	for _, expectedObject := range read.GetExpectedObjects() {
		isFound := false
		for i, currentObject := range tmpCurrentObjects {
			// Need compare same object
			if currentObject.GetName() == expectedObject.GetName() {
				isFound = true

				// Copy TypeMeta to work with some ignore rules like IgnorePDBSelector()
				mustInjectTypeMeta(currentObject, expectedObject)
				patchResult, err := patch.DefaultPatchMaker.Calculate(currentObject, expectedObject, patchOptions...)
				if err != nil {
					return diff, res, errors.Wrapf(err, "Error when diffing object '%s' of type '%s'", currentObject.GetName(), currentObject.GetObjectKind().GroupVersionKind().Kind)
				}
				if !patchResult.IsEmpty() {
					updatedObject := patchResult.Patched.(client.Object)
					diff.AddDiff(fmt.Sprintf("diff %s: %s", updatedObject.GetName(), string(patchResult.Patch)))
					toUpdate = append(toUpdate, updatedObject)
					h.log.Debugf("Need update object '%s' of type '%s'", updatedObject.GetName(), updatedObject.GetObjectKind().GroupVersionKind().Kind)
				}

				// Remove items found
				tmpCurrentObjects = helper.DeleteItemFromSlice(tmpCurrentObjects, i).([]client.Object)

				break
			}
		}

		if !isFound {
			// Need create object
			diff.AddDiff(fmt.Sprintf("Need Create object '%s' of type '%s'", expectedObject.GetName(), expectedObject.GetObjectKind().GroupVersionKind().Kind))

			// Set owner
			err = ctrl.SetControllerReference(o, expectedObject, h.scheme)
			if err != nil {
				return diff, res, errors.Wrapf(err, "Error when set owner reference on object '%s' of type '%s'", expectedObject.GetName(), expectedObject.GetObjectKind().GroupVersionKind().Kind)
			}

			if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(expectedObject); err != nil {
				return diff, res, errors.Wrapf(err, "Error when set annotation for 3-way diff on  object '%s' of type '%s'", expectedObject.GetName(), expectedObject.GetObjectKind().GroupVersionKind().Kind)
			}

			toCreate = append(toCreate, expectedObject)

			h.log.Debugf("Need create object '%s' of type '%s'", expectedObject.GetName(), expectedObject.GetObjectKind().GroupVersionKind().Kind)
		}
	}

	if len(tmpCurrentObjects) > 0 {
		for _, object := range tmpCurrentObjects {
			diff.AddDiff(fmt.Sprintf("Need delete object '%s' of type '%s'", object.GetName(), object.GetObjectKind().GroupVersionKind().Kind))
		}
	}

	diff.SetObjectsToCreate(toCreate)
	diff.SetObjectsToUpdate(toUpdate)
	diff.SetObjectsToDelete(tmpCurrentObjects)

	return diff, res, nil
}

func (h *BasicMultiPhaseStepReconciler) GetPhaseName() PhaseName {
	return h.phaseName
}

func (h *BasicMultiPhaseStepReconciler) GetConditionName() ConditionName {
	return h.conditionName
}

func (h *BasicMultiPhaseStepReconciler) Reconcile(ctx context.Context, req ctrl.Request, o MultiPhaseObject, data map[string]interface{}) (res ctrl.Result, err error) {

	var (
		diff MultiPhaseDiff
		read MultiPhaseRead
	)

	// Init logger
	h.log = h.log.WithFields(logrus.Fields{
		"name":      req.Name,
		"namespace": req.Namespace,
		"step":      h.phaseName.String(),
	})

	// Add setp name to logger
	log := h.log.WithFields(logrus.Fields{
		"module": "multiPhaseStepReconciler",
	})

	// Configure
	res, err = h.Configure(ctx, req, o)
	if err != nil {
		log.Errorf("Error when call 'configure' from step reconciler: %s", err.Error())
		return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallConfigureFromReconciler.Error()))
	}
	log.Debug("Call 'configure' from step reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Read resources
	read, res, err = h.Read(ctx, o, data)
	if err != nil {
		log.Errorf("Error when call 'read' from step reconciler: %s", err.Error())
		return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallReadFromReconciler.Error()))
	}
	log.Debug("Call 'read' from step reconciler successfully")
	if res != (ctrl.Result{}) {
		return res, nil
	}

	//Check if diff exist
	diff, res, err = h.Diff(ctx, o, read, data, h.ignoresDiff...)
	if err != nil {
		log.Errorf("Error when call 'diff' from step reconciler: %s", err.Error())
		return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallDiffFromReconciler.Error()))
	}
	log.Debug("Call 'diff' from step reconciler successfully")
	if diff.IsDiff() {
		log.Debugf("Found diff: %s", diff.Diff())
	}
	if res != (ctrl.Result{}) {
		return res, nil
	}

	// Need create resources
	if diff.NeedCreate() {
		log.Debug("Call 'create' from step reconciler")
		res, err = h.Create(ctx, o, data, diff.GetObjectsToCreate())
		if err != nil {
			log.Errorf("Error when call 'create' from step reconciler: %s", err.Error())
			return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallCreateFromReconciler.Error()))
		}
		log.Debug("Call 'create' from step reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	// Need update resources
	if diff.NeedUpdate() {
		log.Debug("Call 'update' from step reconciler")
		res, err = h.Update(ctx, o, data, diff.GetObjectsToUpdate())
		if err != nil {
			log.Errorf("Error when call 'update' from step reconciler: %s", err.Error())
			return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallUpdateFromReconciler.Error()))
		}
		log.Debug("Call 'update' from step reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	// Need Delete
	if diff.NeedDelete() {
		log.Debug("Call 'delete' from step reconciler")
		res, err = h.Delete(ctx, o, data, diff.GetObjectsToDelete())
		if err != nil {
			log.Errorf("Error when call 'delete' from step reconciler: %s", err.Error())
			return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallDeleteFromReconciler.Error()))
		}
		log.Debug("Call 'delete' from step reconciler successfully")
		if res != (ctrl.Result{}) {
			return res, nil
		}
	}

	res, err = h.OnSuccess(ctx, o, data, diff)
	if err != nil {
		log.Errorf("Error when call 'onSuccess' from step reconciler: %s", err.Error())
		return h.OnError(ctx, o, data, errors.Wrap(err, ErrWhenCallOnSuccessFromReconciler.Error()))
	}
	log.Debug("Call 'onSuccess' from step reconciler successfully")

	return res, nil
}

func mustInjectTypeMeta(src, dst client.Object) {
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
