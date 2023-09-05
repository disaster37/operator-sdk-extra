package object

import (
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	"k8s.io/utils/ptr"
)

// BasicMultiPhaseObject is the implementation of MultiPhaseObject interface
type BasicRemoteObject struct {
	Status BasicRemoteObjectStatus `json:"Status,omitempty"`
}

func (h *BasicRemoteObject) GetStatus() object.RemoteObjectStatus {
	return &h.Status
}

// RemoteObjectStatus is the default status for CRD used to call remote API (not create K8s resources)
type BasicRemoteObjectStatus struct {
	BasicObjectStatus `json:",inline"`

	// IsSync is true if controller successfully apply on remote API
	// +operator-sdk:csv:customresourcedefinitions:type=status
	IsSync *bool `json:"isSync,omitempty"`

	// LastAppliedConfiguration is the last applied configuration to use 3-way diff
	// +operator-sdk:csv:customresourcedefinitions:type=status
	LastAppliedConfiguration string `json:"lastAppliedConfiguration,omitempty"`
}

func (h *BasicRemoteObjectStatus) GetIsSync() bool {
	if h.IsSync == nil || !*h.IsSync {
		return false
	}

	return true
}

func (h *BasicRemoteObjectStatus) SetIsSync(isSync bool) {
	h.IsSync = ptr.To[bool](isSync)
}

func (h *BasicRemoteObjectStatus) GetLastAppliedConfiguration() string {
	return h.LastAppliedConfiguration
}

func (h *BasicRemoteObjectStatus) SetLastAppliedConfiguration(object string) {
	h.LastAppliedConfiguration = object
}
