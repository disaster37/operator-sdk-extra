package multiphase

import (
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
)

// MultiPhaseObjectStatus is the default status for CRD used to create multiple K8s resources
type BasicMultiPhaseObjectStatus struct {
	apis.BasicObjectStatus `json:",inline"`

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
