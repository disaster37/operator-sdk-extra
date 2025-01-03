package controller

import (
	"context"

	"emperror.dev/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Indexer is a function to add indexer on manager
type Indexer func(mgr ctrl.Manager) error

// WebhookRegister is a function to add Webhook on manager
type WebhookRegister func(mgr ctrl.Manager, client client.Client) error

// Controller is the controller interface
type Controller interface {
	Reconcile(context.Context, reconcile.Request) (reconcile.Result, error)

	// SetupWithManager permit to setup controller with manager
	SetupWithManager(mgr ctrl.Manager) error
}

// BasicController is the default controller implementation
type BasicController struct{}

// NewBasicController is the default constructor for Controller
// index can be nil
func NewBasicController() Controller {
	return &BasicController{}
}

func (h *BasicController) SetupWithManager(mgr ctrl.Manager) error {
	return errors.New("You need implement 'SetupWithManager'")
}

func (h *BasicController) Reconcile(context.Context, reconcile.Request) (res reconcile.Result, err error) {
	return res, errors.New("You need implement 'Reconcil'")
}

// SetupIndexerWithManager permit to registers indexers on manager
func SetupIndexerWithManager(mgr ctrl.Manager, indexers ...Indexer) (err error) {
	for _, indexer := range indexers {
		if err = indexer(mgr); err != nil {
			return err
		}
	}

	return nil
}

// SetupWebhookWithManager permit to registers webhooks on manager
func SetupWebhookWithManager(mgr ctrl.Manager, client client.Client, webhookRegisters ...WebhookRegister) (err error) {
	for _, webhookRegister := range webhookRegisters {
		if err = webhookRegister(mgr, client); err != nil {
			return err
		}
	}

	return nil
}
