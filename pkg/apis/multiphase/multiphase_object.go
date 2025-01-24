package multiphase

import (
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
)

// DefaultMultiPhaseObjectStatus is the default status for CRD used to create multiple K8s resources
type DefaultMultiPhaseObjectStatus struct {
	apis.DefaultObjectStatus `json:",inline"`

	// Phase is the current phase
	// +operator-sdk:csv:customresourcedefinitions:type=status
	PhaseName shared.PhaseName `json:"phase,omitempty"`
}

func (h *DefaultMultiPhaseObjectStatus) GetPhaseName() shared.PhaseName {
	return h.PhaseName
}

func (h *DefaultMultiPhaseObjectStatus) SetPhaseName(name shared.PhaseName) {
	h.PhaseName = name
}
