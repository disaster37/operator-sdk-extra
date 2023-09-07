package controller

import (
	"context"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Controller is the controller interface
type Controller interface {
	Reconcile(context.Context, reconcile.Request) (reconcile.Result, error)

	// SetupWithManager permit to setup controller with manager
	SetupWithManager(mgr ctrl.Manager) error

	// SetupIndexerWithManager permit to setup indexer with manager
	SetupIndexerWithManager(mgr ctrl.Manager, indexer object.Indexer)
}

// BasicController is the default controller implementation
type BasicController struct{}

// NewBasicController is the default constructor for Controller
func NewBasicController() Controller {
	return &BasicController{}
}

func (h *BasicController) SetupWithManager(mgr ctrl.Manager) error {
	return errors.New("You need implement 'SetupWithManager'")
}

func (h *BasicController) SetupIndexerWithManager(mgr ctrl.Manager, indexer object.Indexer) {
	indexer.SetupIndexer(mgr)
}

func (h *BasicController) Reconcile(context.Context, reconcile.Request) (res reconcile.Result, err error) {
	return res, errors.New("You need implement 'Reconcil'")
}
