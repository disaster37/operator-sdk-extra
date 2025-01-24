package controller

import (
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BaseReconciler is the interface for all reconciler
type BaseReconciler interface {
	// Client get the client
	Client() client.Client

	// Recorder get the recorder
	Recorder() record.EventRecorder
}

// DefaultBaseReconciler is the default implementation of BaseReconciler interface
type DefaultBaseReconciler struct {
	client   client.Client
	recorder record.EventRecorder
}

func (h *DefaultBaseReconciler) Client() client.Client {
	return h.client
}

func (h *DefaultBaseReconciler) Recorder() record.EventRecorder {
	return h.recorder
}

// NewBaseReconciler return the default implementation of BaseReconciler interface
func NewBaseReconciler(client client.Client, recorder record.EventRecorder) BaseReconciler {
	if recorder == nil {
		panic("recorder can't be nil")
	}

	if client == nil {
		panic("client can't be nil")
	}

	return &DefaultBaseReconciler{
		client:   client,
		recorder: recorder,
	}
}
