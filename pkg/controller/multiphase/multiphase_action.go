package multiphase

import (
	"context"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/object"
	"github.com/sirupsen/logrus"
	condition "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/strings"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseReconcilerAction is the methode needed by step reconciler to reconcile your custom resource
type MultiPhaseReconcilerAction[k8sObject object.MultiPhaseObject] interface {
	controller.ReconcilerAction

	// Configure permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, o k8sObject, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error)

	// Read permit to read kubernetes resources
	Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (err error)

	// OnError is call when error is throwing on current phase
	// It the right way to set status condition when error
	OnError(ctx context.Context, o k8sObject, data map[string]any, currentErr error, logger *logrus.Entry) (res ctrl.Result, err error)

	// OnSuccess is call at the end of current phase, if not error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error)
}

// BasicMultiPhaseReconcilerAction is the default implementation of MultiPhaseReconcilerAction interface
type BasicMultiPhaseReconcilerAction[k8sObject object.MultiPhaseObject] struct {
	controller.ReconcilerAction
}

// NewMultiPhaseReconcilerAction is the default implementation of MultiPhaseReconcilerAction interface
func NewMultiPhaseReconcilerAction[k8sObject object.MultiPhaseObject](client client.Client, conditionName shared.ConditionName, recorder record.EventRecorder) (multiPhaseReconciler MultiPhaseReconcilerAction[k8sObject]) {
	return &BasicMultiPhaseReconcilerAction[k8sObject]{
		ReconcilerAction: controller.NewReconcilerAction(client, recorder, conditionName),
	}
}

func (h *BasicMultiPhaseReconcilerAction[k8sObject]) Configure(ctx context.Context, req ctrl.Request, o k8sObject, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error) {

	conditions := o.GetStatus().GetConditions()
	if condition.FindStatusCondition(conditions, h.Condition().String()) == nil {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:   h.Condition().String(),
			Status: metav1.ConditionFalse,
			Reason: "Initialize",
		})
	}
	o.GetStatus().SetConditions(conditions)

	return res, nil
}

func (h *BasicMultiPhaseReconcilerAction[k8sObject]) Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error) {
	return
}

func (h *BasicMultiPhaseReconcilerAction[k8sObject]) Delete(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (err error) {
	return
}

func (h *BasicMultiPhaseReconcilerAction[k8sObject]) OnError(ctx context.Context, o k8sObject, data map[string]any, currentErr error, logger *logrus.Entry) (res ctrl.Result, err error) {

	o.GetStatus().SetIsOnError(true)
	o.GetStatus().SetLastErrorMessage(strings.ShortenString(currentErr.Error(), controller.ShortenError))

	conditions := o.GetStatus().GetConditions()
	condition.SetStatusCondition(&conditions, metav1.Condition{
		Type:    h.Condition().String(),
		Status:  metav1.ConditionFalse,
		Reason:  "Failed",
		Message: strings.ShortenString(currentErr.Error(), controller.ShortenError),
	})
	o.GetStatus().SetConditions(conditions)

	return res, errors.Wrap(currentErr, "Error on reconciler")
}

func (h *BasicMultiPhaseReconcilerAction[k8sObject]) OnSuccess(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (res ctrl.Result, err error) {

	conditions := o.GetStatus().GetConditions()
	if !condition.IsStatusConditionPresentAndEqual(conditions, h.Condition().String(), metav1.ConditionTrue) {
		condition.SetStatusCondition(&conditions, metav1.Condition{
			Type:   h.Condition().String(),
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		})
	}
	o.GetStatus().SetConditions(conditions)

	o.GetStatus().SetPhaseName(controller.RunningPhase)
	o.GetStatus().SetIsOnError(false)
	o.GetStatus().SetObservedGeneration(o.GetGeneration())

	return res, nil
}
