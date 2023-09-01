package controller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Object is the extension of client.Object
type Object interface {
	client.Object

	// GetConditions permit to get conditions from Status
	GetConditions() []metav1.Condition

	// SetConditions permit to set conditions from status
	SetConditions(conditions []metav1.Condition)

	// IsOnError permit to get if the current object is on error from status
	IsOnError() bool

	// SetIsOnError permit to set if the current object is on error from status
	SetIsOnError(isError bool)

	// LastErrorMessage display the current error
	LastErrorMessage() string

	// SetLastErrorMessage permit to set the current error
	SetLastErrorMessage(message string)
}

// RemoteObject is use when your CRD is used to call remote API (not create K8s resources)
type RemoteObject interface {
	Object

	// IsSync permit to get if object is sync from status
	IsSync() bool

	// SetIsSync permit to set if object is sync from status
	SetIsSync(isSync bool)

	// GetOriginalObject permit to get the original object from annotations (like kubectl do)
	// The goal is to apply 3 way patch merge
	GetOriginalObject() string

	// SetGetOriginalOject permit to set the original object from annotations (like kubectl do)
	// The goal is to apply 3 way patch merge
	SetGetOriginalOject(object string)
}

// MultiPhaseObject is used when your CRD is used to create multiple K8s resources
type MultiPhaseObject interface {
	Object

	// GetPhaseName permit to get the current phase name
	GetPhaseName() PhaseName

	// SetPhaseName permit to set the current phase
	SetPhaseName(name PhaseName)
}

// BaseObjectStatus is the default status for basic Object
type BaseObjectStatus struct {

	// IsOnError is true if controller is stuck on Error
	// +operator-sdk:csv:customresourcedefinitions:type=status
	IsOnError *bool `json:"isOnError,omitempty"`

	// List of conditions
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastErrorMessage is the current error message
	// +operator-sdk:csv:customresourcedefinitions:type=status
	LastErrorMessage string `json:"lastErrorMessage,omitempty"`
}

// RemoteObjectStatus is the default status for CRD used to call remote API (not create K8s resources)
type RemoteObjectStatus struct {
	BaseObjectStatus `json:",inline"`

	// IsSync is true if controller successfully apply on remote API
	// +operator-sdk:csv:customresourcedefinitions:type=status
	IsSync *bool `json:"isSync,omitempty"`
}

// MultiPhaseObjectStatus is the default status for CRD used to create multiple K8s resources
type MultiPhaseObjectStatus struct {
	BaseObjectStatus `json:",inline"`

	// Phase is the current phase
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Phase string `json:"phase,omitempty"`
}
