package apis

import (
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// BasicObject implement the Object interface
type BasicObject struct {
	Status BasicObjectStatus `json:"Status,omitempty"`
}

func (h *BasicObject) GetStatus() object.ObjectStatus {
	return &h.Status
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
