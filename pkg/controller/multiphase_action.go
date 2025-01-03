package controller

import (
	"context"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	"github.com/sirupsen/logrus"
	condition "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/strings"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseReconcilerAction is the methode needed by step reconciler to reconcile your custom resource
type MultiPhaseReconcilerAction interface {
	BaseReconciler

	// Configure permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error)

	// Read permit to read kubernetes resources
	Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, o object.MultiPhaseObject, data map[string]any, logger *logrus.Entry) (err error)

	// OnError is call when error is throwing on current phase
	// It the right way to set status condition when error
	OnError(ctx context.Context, o object.MultiPhaseObject, data map[string]any, currentErr error, logger *logrus.Entry) (res ctrl.Result, err error)

	// OnSuccess is call at the end of current phase, if not error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, o object.MultiPhaseObject, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error)
}

// BasicMultiPhaseReconcilerAction is the basic implementation of MultiPhaseReconcilerAction interface
type BasicMultiPhaseReconcilerAction struct {
	BasicReconcilerAction
}

// NewBasicMultiPhaseReconcilerAction is the basic contructor of MultiPhaseReconcilerAction interface
func NewBasicMultiPhaseReconcilerAction(client client.Client, conditionName shared.ConditionName, recorder record.EventRecorder) (multiPhaseReconciler MultiPhaseReconcilerAction) {
	return &BasicMultiPhaseReconcilerAction{
		BasicReconcilerAction: BasicReconcilerAction{
			BaseReconciler: NewBaseReconciler(client, recorder),
			conditionName:  conditionName,
		},
	}
}

func (h *BasicMultiPhaseReconcilerAction) Configure(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error) {

	conditions := o.GetStatus().GetConditions()
	if condition.FindStatusCondition(conditions, h.conditionName.String()) == nil {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:   h.conditionName.String(),
			Status: metav1.ConditionFalse,
			Reason: "Initialize",
		})
	}
	o.GetStatus().SetConditions(conditions)

	return res, nil
}

func (h *BasicMultiPhaseReconcilerAction) Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error) {
	return
}

func (h *BasicMultiPhaseReconcilerAction) Delete(ctx context.Context, o object.MultiPhaseObject, data map[string]any, logger *logrus.Entry) (err error) {
	return
}

func (h *BasicMultiPhaseReconcilerAction) OnError(ctx context.Context, o object.MultiPhaseObject, data map[string]any, currentErr error, logger *logrus.Entry) (res ctrl.Result, err error) {

	o.GetStatus().SetIsOnError(true)
	o.GetStatus().SetLastErrorMessage(strings.ShortenString(currentErr.Error(), ShortenError))

	conditions := o.GetStatus().GetConditions()
	condition.SetStatusCondition(&conditions, metav1.Condition{
		Type:    h.conditionName.String(),
		Status:  metav1.ConditionFalse,
		Reason:  "Failed",
		Message: strings.ShortenString(currentErr.Error(), ShortenError),
	})
	o.GetStatus().SetConditions(conditions)

	return res, errors.Wrap(currentErr, "Error on reconciler")
}

func (h *BasicMultiPhaseReconcilerAction) OnSuccess(ctx context.Context, o object.MultiPhaseObject, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error) {

	conditions := o.GetStatus().GetConditions()
	if !condition.IsStatusConditionPresentAndEqual(conditions, h.conditionName.String(), metav1.ConditionTrue) {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:   h.conditionName.String(),
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		})
	}
	o.GetStatus().SetConditions(conditions)

	o.GetStatus().SetPhaseName(RunningPhase)
	o.GetStatus().SetIsOnError(false)
	o.GetStatus().SetObservedGeneration(o.GetGeneration())

	return res, nil
}
