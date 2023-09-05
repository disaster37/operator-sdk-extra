package object

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Object interface is the extension of client object
type Object interface {
	client.Object

	// GetStatus permit to get the status interface
	GetStatus() ObjectStatus
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