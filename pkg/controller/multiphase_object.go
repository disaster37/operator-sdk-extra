package controller

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// MultiPhaseObject is used when your CRD is used to create multiple K8s resources
type MultiPhaseObject interface {
	Object

	// GetPhaseName permit to get the current phase name
	GetPhaseName() PhaseName

	// SetPhaseName permit to set the current phase
	SetPhaseName(name PhaseName)
}

// MultiPhaseObjectStatus is the default status for CRD used to create multiple K8s resources
type MultiPhaseObjectStatus struct {
	BaseObjectStatus `json:",inline"`

	// Phase is the current phase
	// +operator-sdk:csv:customresourcedefinitions:type=status
	PhaseName PhaseName `json:"phase,omitempty"`
}

// BasicMultiPhaseObject is the implementation of MultiPhaseObject interface
type BasicMultiPhaseObject struct {
	Status MultiPhaseObjectStatus `json:"status,omitempty"`
}

func (h *BasicMultiPhaseObject) GetPhaseName() PhaseName {
	return h.Status.PhaseName
}

func (h *BasicMultiPhaseObject) SetPhaseName(name PhaseName) {
	h.Status.PhaseName = name
}

func (h *BasicMultiPhaseObject) GetConditions() []metav1.Condition {
	return h.Status.Conditions
}

func (h *BasicMultiPhaseObject) SetConditions(conditions []metav1.Condition) {
	h.Status.Conditions = conditions
}

func (h *BasicMultiPhaseObject) IsOnError() bool {
	if h.Status.IsOnError == nil || !*h.Status.IsOnError {
		return false
	}

	return true
}

func (h *BasicMultiPhaseObject) SetIsOnError(isError bool) {
	*h.Status.IsOnError = isError
}

func (h *BasicMultiPhaseObject) LastErrorMessage() string {
	return h.Status.LastErrorMessage
}

func (h *BasicMultiPhaseObject) SetLastErrorMessage(message string) {
	h.Status.LastErrorMessage = message
}
