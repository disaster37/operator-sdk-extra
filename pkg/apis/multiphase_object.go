package object

import (
	"github.com/disaster37/operator-sdk-extra/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
)

// BasicMultiPhaseObject is the implementation of MultiPhaseObject interface
type BasicMultiPhaseObject struct {
	Status BasicMultiPhaseObjectStatus `json:"Status,omitempty"`
}

func (h *BasicMultiPhaseObject) GetStatus() object.MultiPhaseObjectStatus {
	return &h.Status
}

// MultiPhaseObjectStatus is the default status for CRD used to create multiple K8s resources
type BasicMultiPhaseObjectStatus struct {
	BasicObjectStatus `json:",inline"`

	// Phase is the current phase
	// +operator-sdk:csv:customresourcedefinitions:type=status
	PhaseName shared.PhaseName `json:"phase,omitempty"`
}

func (h *BasicMultiPhaseObjectStatus) GetPhaseName() shared.PhaseName {
	return h.PhaseName
}

func (h *BasicMultiPhaseObjectStatus) SetPhaseName(name shared.PhaseName) {
	h.PhaseName = name
}
