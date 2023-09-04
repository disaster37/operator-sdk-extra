package controller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Object interface is the extension of client object
type Object interface {
	client.Object

	// GetStatus permit to get the status interface
	GetStatus() ObjectStatus
}

// BasicObject implement the Object interface
type BasicObject struct {
	client.Object
	Status BasicObjectStatus `json:"Status,omitempty"`
}

func (h *BasicObject) GetStatus() ObjectStatus {
	return &h.Status
}

// ObjectStatus is the interface for object status
type ObjectStatus interface {

	// GetConditions permit to get conditions from Status
	GetConditions() []metav1.Condition

	// SetConditions permit to set conditions from status
	SetConditions(conditions []metav1.Condition)

	// IsOnError permit to get if the current object is on error from status
	GetIsOnError() bool

	// SetIsOnError permit to set if the current object is on error from status
	SetIsOnError(isError bool)

	// LastErrorMessage display the current error
	GetLastErrorMessage() string

	// SetLastErrorMessage permit to set the current error
	SetLastErrorMessage(message string)
}

// BasicObjectStatus is the default status for basic Object
type BasicObjectStatus struct {

	// IsOnError is true if controller is stuck on Error
	// +operator-sdk:csv:customresourcedefinitions:type=status
	IsOnError *bool `json:"isOnError,omitempty"`

	// List of conditions
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// LastErrorMessage is the current error message
	// +operator-sdk:csv:customresourcedefinitions:type=status
	LastErrorMessage string `json:"lastErrorMessage,omitempty"`
}

func (h *BasicObjectStatus) GetConditions() []metav1.Condition {
	return h.Conditions
}

func (h *BasicObjectStatus) SetConditions(conditions []metav1.Condition) {
	h.Conditions = conditions
}

func (h *BasicObjectStatus) GetIsOnError() bool {
	if h.IsOnError == nil || !*h.IsOnError {
		return false
	}

	return true
}

func (h *BasicObjectStatus) SetIsOnError(isError bool) {
	h.IsOnError = ptr.To[bool](isError)
}

func (h *BasicObjectStatus) GetLastErrorMessage() string {
	return h.LastErrorMessage
}

func (h *BasicObjectStatus) SetLastErrorMessage(message string) {
	h.LastErrorMessage = message
}
