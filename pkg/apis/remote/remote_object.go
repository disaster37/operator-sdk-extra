package remote

import (
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis"
	"k8s.io/utils/ptr"
)

// DefaultRemoteObjectStatus is the default status for CRD used to call remote API (not create K8s resources)
type DefaultRemoteObjectStatus struct {
	apis.DefaultObjectStatus `json:",inline"`

	// IsSync is true if controller successfully apply on remote API
	// +operator-sdk:csv:customresourcedefinitions:type=status
	IsSync *bool `json:"isSync,omitempty"`

	// LastAppliedConfiguration is the last applied configuration to use 3-way diff
	// +operator-sdk:csv:customresourcedefinitions:type=status
	LastAppliedConfiguration string `json:"lastAppliedConfiguration,omitempty"`
}

func (h *DefaultRemoteObjectStatus) GetIsSync() bool {
	if h.IsSync == nil || !*h.IsSync {
		return false
	}

	return true
}

func (h *DefaultRemoteObjectStatus) SetIsSync(isSync bool) {
	h.IsSync = ptr.To[bool](isSync)
}

func (h *DefaultRemoteObjectStatus) GetLastAppliedConfiguration() string {
	return h.LastAppliedConfiguration
}

func (h *DefaultRemoteObjectStatus) SetLastAppliedConfiguration(object string) {
	h.LastAppliedConfiguration = object
}
