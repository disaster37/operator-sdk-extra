package controller

import (
	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/pkg/apis/shared"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
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

// BaseReconciler is the base interface for all reconciler
type BaseReconciler interface {

	// GetLogger permit to get logger
	GetLogger() *logrus.Entry

	// GetRecorder permit to get recorder
	GetRecorder() record.EventRecorder

	// SetupWithManager permit to setup controller with manager
	SetupWithManager(mgr ctrl.Manager) error
}

type BasicReconciler struct {
	client.Client
	finalizer     shared.FinalizerName
	conditionName shared.ConditionName
	log           *logrus.Entry
	recorder      record.EventRecorder
	name          string
}

func (h *BasicReconciler) GetLogger() *logrus.Entry {
	return h.log
}

func (h *BasicReconciler) GetRecorder() record.EventRecorder {
	return h.recorder
}

func (h *BasicReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return errors.New("You need implement 'SetupWithManager'")
}
