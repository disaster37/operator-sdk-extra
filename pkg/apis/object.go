package apis

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

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

	// CurrentVersion is the current version applied
	// +operator-sdk:csv:customresourcedefinitions:type=status
	CurrentVersion string `json:"currentVersion,omitempty"`
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

func (h *BasicObjectStatus) GetCurrentVersion() string {
	return h.CurrentVersion
}

func (h *BasicObjectStatus) SetCurrentVersion(version string) {
	h.CurrentVersion = version
}
