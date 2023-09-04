package controller

import (
	"fmt"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseObject is used when your CRD is used to create multiple K8s resources
type RemoteObject interface {
	client.Object

	GetStatus() RemoteObjectStatus

	// GetOriginalObject permit to get the original object from annotations (like kubectl do)
	// The goal is to apply 3 way patch merge
	GetOriginalObject() string

	// SetOriginalOject permit to set the original object from annotations (like kubectl do)
	// The goal is to apply 3 way patch merge
	SetOriginalOject(object string)
}

// BasicMultiPhaseObject is the implementation of MultiPhaseObject interface
type BasicRemoteObject struct {
	client.Object
	Status BasicRemoteObjectStatus `json:"Status,omitempty"`
}

func (h *BasicRemoteObject) GetStatus() RemoteObjectStatus {
	return &h.Status
}

func (h *BasicRemoteObject) GetOriginalObject() string {
	return h.GetAnnotations()[fmt.Sprintf("%s/last-applied-configuration", BaseAnnotation)]
}

func (h *BasicRemoteObject) SetGetOriginalOject(object string) {
	annotations := h.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[fmt.Sprintf("%s/last-applied-configuration", BaseAnnotation)] = object
	h.SetAnnotations(annotations)
}

// RemoteObject is use when your CRD is used to call remote API (not create K8s resources)
type RemoteObjectStatus interface {
	ObjectStatus

	// GetIsSync permit to get if object is sync from status
	GetIsSync() bool

	// SetIsSync permit to set if object is sync from status
	SetIsSync(isSync bool)
}

// RemoteObjectStatus is the default status for CRD used to call remote API (not create K8s resources)
type BasicRemoteObjectStatus struct {
	BasicObjectStatus `json:",inline"`

	// IsSync is true if controller successfully apply on remote API
	// +operator-sdk:csv:customresourcedefinitions:type=status
	IsSync *bool `json:"isSync,omitempty"`
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
