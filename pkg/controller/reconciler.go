package controller

import (
	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrWhenCallConfigureFromReconciler      = errors.Sentinel("Error when call 'configure' from reconciler")
	ErrWhenCallReadFromReconciler           = errors.Sentinel("Error when call 'read' from reconciler")
	ErrWhenCallDeleteFromReconciler         = errors.Sentinel("Error when call 'delete' from reconciler")
	ErrWhenCallDiffFromReconciler           = errors.Sentinel("Error when call 'diff' from reconciler")
	ErrWhenCallCreateFromReconciler         = errors.Sentinel("Error when call 'create' from reconciler")
	ErrWhenCallUpdateFromReconciler         = errors.Sentinel("Error when call 'update' from reconciler")
	ErrWhenCallOnSuccessFromReconciler      = errors.Sentinel("Error when call 'onSuccess' from reconciler")
	ErrWhenCallStepReconcilerFromReconciler = errors.Sentinel("Error when call 'reconcile' from step reconciler")
	ErrWhenGetObjectFromReconciler          = errors.Sentinel("Error when get object from reconciler")
	ErrWhenAddFinalizer                     = errors.Sentinel("Error when add finalizer")
	ErrWhenDeleteFinalizer                  = errors.Sentinel("Error when delete finalizer")
	ErrWhenGetObjectStatus                  = errors.Sentinel("Error when get object status")
)

type Reconciler interface {
	BaseReconciler

	// Finalizer get the finalier name
	Finalizer() shared.FinalizerName

	// Logger get the logger
	Logger() *logrus.Entry
}

// DefaultReconciler is the default implementation of Reconciler interface
type DefaultReconciler struct {
	BaseReconciler
	finalizer shared.FinalizerName
	logger    *logrus.Entry
}

func (h *DefaultReconciler) Finalizer() shared.FinalizerName {
	return h.finalizer
}

func (h *DefaultReconciler) Logger() *logrus.Entry {
	return h.logger
}

// NewReconciler is the default implementation of the Reconciler interface
func NewReconciler(client client.Client, recorder record.EventRecorder, finalizer shared.FinalizerName, logger *logrus.Entry) Reconciler {
	if logger == nil {
		panic("logger can't be nil")
	}

	return &DefaultReconciler{
		BaseReconciler: NewBaseReconciler(client, recorder),
		finalizer:      finalizer,
		logger:         logger,
	}
}
