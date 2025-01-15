package controller

import (
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ReconcilerAction interface {
	BaseReconciler

	// Condition get the condition name
	Condition() shared.ConditionName
}

// DefaultReconcilerAction is the default implementation of ReconcilerAction interface
type DefaultReconcilerAction struct {
	BaseReconciler
	conditionName shared.ConditionName
}

func(h *DefaultReconcilerAction) Condition() shared.ConditionName {
	return h.conditionName
}

// NewReconcilerAction return the default implementation of ReconcilerAction
func NewReconcilerAction(client client.Client, recorder record.EventRecorder, conditionName shared.ConditionName) ReconcilerAction {

	if conditionName == "" {
		panic("Condition name must be provided")
	}

	return &DefaultReconcilerAction{
		BaseReconciler: NewBaseReconciler(client, recorder),
		conditionName:  conditionName,
	}
}
