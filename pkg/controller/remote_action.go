package controller

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/generic-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/pkg/helper"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	condition "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	k8sstrings "k8s.io/utils/strings"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RemoteReconcilerAction is the interface that use by reconciler remote to reconcile your remote resource
type RemoteReconcilerAction[k8sObject comparable, apiObject comparable, apiClient any] interface {

	// GetRemoteHandler permit to get the handler to manage the remote resources
	GetRemoteHandler(ctx context.Context, req ctrl.Request, o object.RemoteObject) (handler RemoteExternalReconciler[k8sObject, apiObject, apiClient], res ctrl.Result, err error)

	// Confirgure permit to init external provider driver (API client REST)
	// It can also permit to init condition on status
	Configure(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient]) (res ctrl.Result, err error)

	// Read permit to read the actual resource state from provider and set it on data map
	Read(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient]) (read RemoteRead[apiObject], res ctrl.Result, err error)

	// Create permit to create resource on provider
	// It only call if diff.NeeCreated is true
	Create(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient], object apiObject) (res ctrl.Result, err error)

	// Update permit to update resource on provider
	// It only call if diff.NeedUpdated is true
	Update(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient], object apiObject) (res ctrl.Result, err error)

	// Delete permit to delete resource on provider
	// It only call if you have specified finalizer name when you create reconciler and if resource as marked to be deleted
	Delete(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient]) (err error)

	// OnError is call when error is throwing
	// It the right way to set status condition when error
	OnError(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient], currentErr error) (res ctrl.Result, err error)

	// OnSuccess is call at the end if no error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient], diff RemoteDiff[apiObject]) (res ctrl.Result, err error)

	// Diff permit to compare the actual state and the expected state
	Diff(ctx context.Context, o object.RemoteObject, read RemoteRead[apiObject], data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient], ignoreDiff ...patch.CalculateOption) (diff RemoteDiff[apiObject], res ctrl.Result, err error)

	GetIgnoresDiff() []patch.CalculateOption
}

// BasicRemoteReconcilerAction is the basic implementation of RemoteReconcilerAction
type BasicRemoteReconcilerAction[k8sObject comparable, apiObject comparable, apiClient any] struct {
	BasicReconcilerAction
}

// NewRemoteReconcilerAction is the basic constructor of RemoteReconcilerAction interface
func NewRemoteReconcilerAction[k8sObject comparable, apiObject comparable, apiClient any](client client.Client, logger *logrus.Entry, recorder record.EventRecorder) (remoteReconciler RemoteReconcilerAction[k8sObject, apiObject, apiClient]) {
	if recorder == nil {
		panic("recorder can't be nil")
	}

	return &BasicRemoteReconcilerAction[k8sObject, apiObject, apiClient]{
		BasicReconcilerAction: BasicReconcilerAction{
			BaseReconciler: BaseReconciler{
				Client:   client,
				Log:      logger,
				Recorder: recorder,
			},
			conditionName: ReadyCondition,
		},
	}
}

func (h *BasicRemoteReconcilerAction[k8sObject, apiObject, apiClient]) GetRemoteHandler(ctx context.Context, req ctrl.Request, o object.RemoteObject) (handler RemoteExternalReconciler[k8sObject, apiObject, apiClient], res ctrl.Result, err error) {
	panic("You need to implement GetRemoteHandler")
}

func (h *BasicRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Configure(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient]) (res ctrl.Result, err error) {
	conditions := o.GetStatus().GetConditions()

	// Init condition
	if condition.FindStatusCondition(conditions, h.conditionName.String()) == nil {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:   h.conditionName.String(),
			Status: metav1.ConditionFalse,
			Reason: "Initialize",
		})
	}

	return res, nil
}

func (h *BasicRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Read(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient]) (read RemoteRead[apiObject], res ctrl.Result, err error) {
	read = NewBasicRemoteRead[apiObject]()

	// Read current object
	currentObject, err := handler.Get(o.(k8sObject))
	if err != nil {
		return read, res, errors.Wrapf(err, "Error when read object %s on remote target", o.GetName())
	}
	read.SetCurrentObject(currentObject)

	// Build expected object
	expectedObject, err := handler.Build(o.(k8sObject))
	if err != nil {
		return read, res, errors.Wrapf(err, "Error when build object %s for remote target", o.GetName())
	}
	read.SetExpectedObject(expectedObject)

	return read, res, nil
}

func (h *BasicRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Create(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient], object apiObject) (res ctrl.Result, err error) {

	if err = handler.Create(object, o.(k8sObject)); err != nil {
		return res, errors.Wrapf(err, "Error when create %s on remote target", o.GetName())
	}

	zip, err := helper.ZipAndBase64Encode(object)
	if err != nil {
		return res, errors.Wrapf(err, "Error when generate 'lastAppliedConfiguration' from %s", o.GetName())
	}
	o.GetStatus().SetLastAppliedConfiguration(zip)

	h.Log.Debugf("Create object '%s' successfully on remote target", o.GetName())
	h.Recorder.Eventf(o, corev1.EventTypeNormal, "CreateCompleted", "Object '%s' successfully created on remote target", o.GetName())

	return res, nil
}

// Update can be call on your own version
// It only add some log / events
func (h *BasicRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Update(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient], object apiObject) (res ctrl.Result, err error) {

	if err = handler.Update(object, o.(k8sObject)); err != nil {
		return res, errors.Wrapf(err, "Error when update %s on remote target", o.GetName())
	}

	zip, err := helper.ZipAndBase64Encode(object)
	if err != nil {
		return res, errors.Wrapf(err, "Error when generate 'lastAppliedConfiguration' from %s", o.GetName())
	}
	o.GetStatus().SetLastAppliedConfiguration(zip)

	h.Log.Debugf("Update object '%s' successfully on remote target", o.GetName())
	h.Recorder.Eventf(o, corev1.EventTypeNormal, "UpdateCompleted", "Object '%s' successfully updated on remote target", o.GetName())

	return res, nil
}

// Delete can be call on your own version
// It only add some log / events
func (h *BasicRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Delete(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient]) (err error) {

	if err = handler.Delete(o.(k8sObject)); err != nil {
		return errors.Wrapf(err, "Error when delete %s on remote target", o.GetName())
	}

	h.Log.Debugf("Delete object '%s' successfully on remote target", o.GetName())
	h.Recorder.Eventf(o, corev1.EventTypeNormal, "DeleteCompleted", "Object '%s' successfully deleted on remote target", o.GetName())

	return nil
}

func (h *BasicRemoteReconcilerAction[k8sObject, apiObject, apiClient]) OnError(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient], currentErr error) (res ctrl.Result, err error) {

	o.GetStatus().SetIsOnError(true)
	o.GetStatus().SetLastErrorMessage(k8sstrings.ShortenString(currentErr.Error(), ShortenError))
	o.GetStatus().SetIsSync(false)

	conditions := o.GetStatus().GetConditions()

	condition.SetStatusCondition(&conditions, metav1.Condition{
		Type:    h.conditionName.String(),
		Status:  metav1.ConditionFalse,
		Reason:  "Failed",
		Message: k8sstrings.ShortenString(currentErr.Error(), ShortenError),
	})

	h.Recorder.Event(o, corev1.EventTypeWarning, "ReconcilerActionError", k8sstrings.ShortenString(currentErr.Error(), ShortenError))

	return res, currentErr
}

func (h *BasicRemoteReconcilerAction[k8sObject, apiObject, apiClient]) OnSuccess(ctx context.Context, o object.RemoteObject, data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient], diff RemoteDiff[apiObject]) (res ctrl.Result, err error) {

	conditions := o.GetStatus().GetConditions()
	if !condition.IsStatusConditionPresentAndEqual(conditions, h.conditionName.String(), metav1.ConditionTrue) {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:   h.conditionName.String(),
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		})
	}
	o.GetStatus().SetConditions(conditions)

	o.GetStatus().SetIsOnError(false)
	o.GetStatus().SetIsSync(true)

	return res, nil
}

func (h *BasicRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Diff(ctx context.Context, o object.RemoteObject, read RemoteRead[apiObject], data map[string]any, handler RemoteExternalReconciler[k8sObject, apiObject, apiClient], ignoreDiff ...patch.CalculateOption) (diff RemoteDiff[apiObject], res ctrl.Result, err error) {

	// Get the original object from status to use 3-way diff
	var (
		originalObject *apiObject
		nilObject      apiObject
	)

	originalObject = new(apiObject)
	if o.GetStatus().GetLastAppliedConfiguration() != "" {
		if err = helper.UnZipBase64Decode(o.GetStatus().GetLastAppliedConfiguration(), originalObject); err != nil {
			return diff, res, errors.Wrap(err, "Error when create object from 'lastAppliedConfiguration'")
		}
	}

	diff = NewBasicRemoteDiff[apiObject]()

	// Check if need to create object on remote
	if read.GetCurrentObject() == nilObject {
		diff.SetObjectToCreate(read.GetExpectedObject())
		diff.AddDiff(fmt.Sprintf("Need to create new object %s on remote target", o.GetName()))

		return diff, res, nil
	}

	differ, err := handler.Diff(read.GetCurrentObject(), read.GetExpectedObject(), *originalObject, ignoreDiff...)
	if err != nil {
		return diff, res, errors.Wrapf(err, "Error when diffing %s for remote target", o.GetName())
	}

	if !differ.IsEmpty() {
		diff.AddDiff(string(differ.Patch))
		diff.SetObjectToUpdate(read.GetExpectedObject())
	}

	return diff, res, nil
}

func (h *BasicRemoteReconcilerAction[k8sObject, apiObject, apiClient]) GetIgnoresDiff() []patch.CalculateOption {
	return make([]patch.CalculateOption, 0)
}
