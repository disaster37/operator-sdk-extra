package controller

import (
	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
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

	// SetupWithManager permit to setup controller with manager
	SetupWithManager(mgr ctrl.Manager) error

	// SetupIndexerWithManager permit to setup indexers with manager
	SetupIndexerWithManager(mgr ctrl.Manager, indexers ...object.Indexer)
}

// BasicReconciler is the basic implementation of BaseReconciler
// It also provide attributes needed by all reconciler
type BasicReconciler struct {
	client.Client
	finalizer shared.FinalizerName
	log       *logrus.Entry
	recorder  record.EventRecorder
	scheme    *runtime.Scheme
}

// BasicReconcilerAction provide attribute needed by all reconciler action
type BasicReconcilerAction struct {
	client.Client
	conditionName shared.ConditionName
	log           *logrus.Entry
	recorder      record.EventRecorder
}

func (h *BasicReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return errors.New("You need implement 'SetupWithManager'")
}

func (h *BasicReconciler) SetupIndexerWithManager(mgr ctrl.Manager, indexers ...object.Indexer) {
	for _, indexer := range indexers {
		indexer.SetupIndexer(mgr)
	}
}
