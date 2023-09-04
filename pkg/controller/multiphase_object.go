package controller

import "sigs.k8s.io/controller-runtime/pkg/client"

// MultiPhaseObject is used when your CRD is used to create multiple K8s resources
type MultiPhaseObject interface {
	client.Object

	// GetStatus permit to get the Status interface
	GetStatus() MultiPhaseObjectStatus
}

// BasicMultiPhaseObject is the implementation of MultiPhaseObject interface
type BasicMultiPhaseObject struct {
	client.Object `json:"-"`
	Status        BasicMultiPhaseObjectStatus `json:"Status,omitempty"`
}

func (h *BasicMultiPhaseObject) GetStatus() MultiPhaseObjectStatus {
	return &h.Status
}

// MultiPhaseObjectStatus is the interface to control status of multi phase object
type MultiPhaseObjectStatus interface {
	ObjectStatus

	// GetPhaseName permit to get the current phase name
	GetPhaseName() PhaseName

	// SetPhaseName permit to set the current phase
	SetPhaseName(name PhaseName)
}

// MultiPhaseObjectStatus is the default status for CRD used to create multiple K8s resources
type BasicMultiPhaseObjectStatus struct {
	BasicObjectStatus `json:",inline"`

	// Phase is the current phase
	// +operator-sdk:csv:customresourcedefinitions:type=status
	PhaseName PhaseName `json:"phase,omitempty"`
}

func (h *BasicMultiPhaseObjectStatus) GetPhaseName() PhaseName {
	return h.PhaseName
}

func (h *BasicMultiPhaseObjectStatus) SetPhaseName(name PhaseName) {
	h.PhaseName = name
}
