package apis

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// DefaultObjectStatus is the default status for default Object
type DefaultObjectStatus struct {

	// IsOnError is true if controller is stuck on Error
	// +operator-sdk:csv:customresourcedefinitions:type=status
	IsOnError *bool `json:"isOnError,omitempty"`

	// List of conditions
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// LastErrorMessage is the current error message
	// +operator-sdk:csv:customresourcedefinitions:type=status
	LastErrorMessage string `json:"lastErrorMessage,omitempty"`

	// observedGeneration is the current generation applied
	// +operator-sdk:csv:customresourcedefinitions:type=status
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

func (h *DefaultObjectStatus) GetConditions() []metav1.Condition {
	return h.Conditions
}

func (h *DefaultObjectStatus) SetConditions(conditions []metav1.Condition) {
	h.Conditions = conditions
}

func (h *DefaultObjectStatus) GetIsOnError() bool {
	if h.IsOnError == nil || !*h.IsOnError {
		return false
	}

	return true
}

func (h *DefaultObjectStatus) SetIsOnError(isError bool) {
	h.IsOnError = ptr.To[bool](isError)
}

func (h *DefaultObjectStatus) GetLastErrorMessage() string {
	return h.LastErrorMessage
}

func (h *DefaultObjectStatus) SetLastErrorMessage(message string) {
	h.LastErrorMessage = message
}

func (h *DefaultObjectStatus) GetObservedGeneration() int64 {
	return h.ObservedGeneration
}

func (h *DefaultObjectStatus) SetObservedGeneration(version int64) {
	h.ObservedGeneration = version
}
