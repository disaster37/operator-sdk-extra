package controller

import (
	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/pkg/apis/shared"
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

// BaseReconciler is the interface for all reconciler
type BaseReconciler interface {

	// Client get the client
	Client() client.Client

	// Recorder get the recorder
	Recorder() record.EventRecorder
}

type DefaultBaseReconciler struct {
	client   client.Client
	recorder record.EventRecorder
}

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

func (h *DefaultBaseReconciler) Client() client.Client {
	return h.client
}

func (h *DefaultBaseReconciler) Recorder() record.EventRecorder {
	return h.recorder
}

// BasicReconciler is the basic implementation of BaseReconciler
// It also provide attributes needed by all reconciler
type BasicReconciler struct {
	BaseReconciler
	finalizer shared.FinalizerName
	logger    *logrus.Entry
}

func NewBasicReconciler(client client.Client, recorder record.EventRecorder, finalizer shared.FinalizerName, logger *logrus.Entry) BasicReconciler {
	if logger == nil {
		panic("logger can't be nil")
	}

	return BasicReconciler{
		BaseReconciler: NewBaseReconciler(client, recorder),
		finalizer:      finalizer,
		logger:         logger,
	}
}

// BasicReconcilerAction provide attribute needed by all reconciler action
type BasicReconcilerAction struct {
	BaseReconciler
	conditionName shared.ConditionName
}

func NewBasicReconcilerAction(client client.Client, recorder record.EventRecorder, conditionName shared.ConditionName) BasicReconcilerAction {

	if conditionName == "" {
		panic("Condition name must be provided")
	}

	return BasicReconcilerAction{
		BaseReconciler: NewBaseReconciler(client, recorder),
		conditionName:  conditionName,
	}
}
