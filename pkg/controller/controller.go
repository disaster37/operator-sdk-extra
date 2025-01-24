package controller

import (
	"context"

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

// DefaultController is the default controller implementation
type DefaultController struct{}

// NewController is the default implementation of Controller
// index can be nil
func NewController() Controller {
	return &DefaultController{}
}

func (h *DefaultController) SetupWithManager(mgr ctrl.Manager) error {
	panic("You need implement it")
}

func (h *DefaultController) Reconcile(context.Context, reconcile.Request) (res reconcile.Result, err error) {
	panic("You need implement it")
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
