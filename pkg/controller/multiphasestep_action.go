package controller

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/k8s-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/pkg/helper"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
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

type MultiPhaseStepReconcilerAction interface {
	// Configure permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject) (res ctrl.Result, err error)

	// Read permit to read kubernetes resources
	Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (read MultiPhaseRead, res ctrl.Result, err error)

	// Create permit to create resources on kubernetes
	Create(ctx context.Context, o object.MultiPhaseObject, data map[string]any, objects []client.Object) (res ctrl.Result, err error)

	// Update permit to update resources on kubernetes
	Update(ctx context.Context, o object.MultiPhaseObject, data map[string]any, objects []client.Object) (res ctrl.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, o object.MultiPhaseObject, data map[string]any, objects []client.Object) (res ctrl.Result, err error)

	// OnError is call when error is throwing on current phase
	// It the right way to set status condition when error
	OnError(ctx context.Context, o object.MultiPhaseObject, data map[string]any, currentErr error) (res ctrl.Result, err error)

	// OnSuccess is call at the end of current phase, if not error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, o object.MultiPhaseObject, data map[string]any, diff MultiPhaseDiff) (res ctrl.Result, err error)

	// Diff permit to compare the actual state and the expected state
	Diff(ctx context.Context, o object.MultiPhaseObject, read MultiPhaseRead, data map[string]any, ignoreDiff ...patch.CalculateOption) (diff MultiPhaseDiff, res ctrl.Result, err error)

	// GetPhaseName permit to get the phase name
	GetPhaseName() shared.PhaseName

	GetIgnoresDiff() []patch.CalculateOption

	GetClient() client.Client
}

type BasicMultiPhaseStepReconcilerAction struct {
	BasicReconcilerAction
	scheme    *runtime.Scheme
	phaseName shared.PhaseName
}

func NewBasicMultiPhaseStepReconcilerAction(client client.Client, phaseName shared.PhaseName, conditionName shared.ConditionName, logger *logrus.Entry, recorder record.EventRecorder, scheme *runtime.Scheme) (multiPhaseStepReconciler MultiPhaseStepReconcilerAction) {
	if recorder == nil {
		panic("recorder can't be nil")
	}

	return &BasicMultiPhaseStepReconcilerAction{
		BasicReconcilerAction: BasicReconcilerAction{
			recorder: recorder,
			log: logger.WithFields(logrus.Fields{
				"phase": phaseName.String(),
			}),
			Client:        client,
			conditionName: conditionName,
		},
		scheme:    scheme,
		phaseName: phaseName,
	}
}

func (h *BasicMultiPhaseStepReconcilerAction) GetIgnoresDiff() []patch.CalculateOption {
	return make([]patch.CalculateOption, 0)
}

func (h *BasicMultiPhaseStepReconcilerAction) GetClient() client.Client {
	return h.Client
}

func (h *BasicMultiPhaseStepReconcilerAction) Configure(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject) (res ctrl.Result, err error) {
	conditions := o.GetStatus().GetConditions()

	// Init condition
	if condition.FindStatusCondition(conditions, h.conditionName.String()) == nil {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:   h.conditionName.String(),
			Status: metav1.ConditionFalse,
			Reason: "Initialize",
		})
	}

	// Init phase
	o.GetStatus().SetPhaseName(h.GetPhaseName())

	return res, nil
}
func (h *BasicMultiPhaseStepReconcilerAction) Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (read MultiPhaseRead, res ctrl.Result, err error) {
	panic("You need implement it")
}

func (h *BasicMultiPhaseStepReconcilerAction) Create(ctx context.Context, o object.MultiPhaseObject, data map[string]any, objects []client.Object) (res ctrl.Result, err error) {

	for _, oChild := range objects {
		if err = h.Client.Create(ctx, oChild); err != nil {
			return res, errors.Wrapf(err, "Error when create object of type '%s' with name '%s'", oChild.GetObjectKind().GroupVersionKind().Kind, oChild.GetName())
		}
		h.log.Debugf("Create object '%s' of type '%s' successfully", oChild.GetName(), oChild.GetObjectKind().GroupVersionKind().Kind)
		h.recorder.Eventf(o, corev1.EventTypeNormal, "CreateCompleted", "Object '%s' of type '%s' successfully created", oChild.GetName(), oChild.GetObjectKind().GroupVersionKind().Kind)
	}

	return res, nil
}

func (h *BasicMultiPhaseStepReconcilerAction) Update(ctx context.Context, o object.MultiPhaseObject, data map[string]any, objects []client.Object) (res ctrl.Result, err error) {

	for _, oChild := range objects {
		if err = h.Client.Update(ctx, oChild); err != nil {
			return res, errors.Wrapf(err, "Error when update object of type '%s' with name '%s'", oChild.GetObjectKind().GroupVersionKind().Kind, oChild.GetName())
		}
		h.log.Debugf("Update object '%s' of type '%s' successfully", oChild.GetName(), oChild.GetObjectKind().GroupVersionKind().Kind)
		h.recorder.Eventf(o, corev1.EventTypeNormal, "UpdateCompleted", "Object '%s' of type '%s' successfully created", oChild.GetName(), oChild.GetObjectKind().GroupVersionKind().Kind)
	}

	return res, nil
}

func (h *BasicMultiPhaseStepReconcilerAction) Delete(ctx context.Context, o object.MultiPhaseObject, data map[string]any, objects []client.Object) (res ctrl.Result, err error) {

	for _, oChild := range objects {
		if err = h.Client.Delete(ctx, oChild); err != nil {
			return res, errors.Wrapf(err, "Error when delete object of type '%s' with name '%s'", oChild.GetObjectKind().GroupVersionKind().Kind, oChild.GetName())
		}
		h.log.Debugf("Delete object '%s' of type '%s' successfully", oChild.GetName(), oChild.GetObjectKind().GroupVersionKind().Kind)
		h.recorder.Eventf(o, corev1.EventTypeNormal, "DeleteCompleted", "Object '%s' of type '%s' successfully created", oChild.GetName(), oChild.GetObjectKind().GroupVersionKind().Kind)
	}

	return res, nil
}

func (h *BasicMultiPhaseStepReconcilerAction) OnError(ctx context.Context, o object.MultiPhaseObject, data map[string]any, currentErr error) (res ctrl.Result, err error) {
	conditions := o.GetStatus().GetConditions()

	condition.SetStatusCondition(&conditions, metav1.Condition{
		Type:    h.conditionName.String(),
		Status:  metav1.ConditionFalse,
		Reason:  "Failed",
		Message: k8sstrings.ShortenString(currentErr.Error(), shared.ShortenError),
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

func (h *BasicMultiPhaseStepReconcilerAction) OnSuccess(ctx context.Context, o object.MultiPhaseObject, data map[string]any, diff MultiPhaseDiff) (res ctrl.Result, err error) {
	conditions := o.GetStatus().GetConditions()

	// Update condition status if needed
	if !condition.IsStatusConditionPresentAndEqual(conditions, h.conditionName.String(), metav1.ConditionTrue) {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:    h.conditionName.String(),
			Reason:  "Success",
			Status:  metav1.ConditionTrue,
			Message: "Ready",
		})
	}

	return res, nil
}

func (h *BasicMultiPhaseStepReconcilerAction) Diff(ctx context.Context, o object.MultiPhaseObject, read MultiPhaseRead, data map[string]any, ignoreDiff ...patch.CalculateOption) (diff MultiPhaseDiff, res ctrl.Result, err error) {

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

func (h *BasicMultiPhaseStepReconcilerAction) GetPhaseName() shared.PhaseName {
	return h.phaseName
}
