package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestController(t *testing.T) {
	c := NewController()

	assert.Panics(t, func() {
		c.SetupWithManager(nil)
	})

	assert.Panics(t, func() {
		c.Reconcile(context.Background(), reconcile.Request{})
	})
}

func (t *ControllerTestSuite) TestSetupIndexerWithManager() {

	indexer := func(mgr ctrl.Manager) error {
		if err := t.k8sManager.GetFieldIndexer().IndexField(context.Background(), &corev1.ConfigMap{}, "test.indexer", func(o client.Object) []string {
			return []string{"test"}
		}); err != nil {
			return err
		}
		return nil
	}

	err := SetupIndexerWithManager(t.k8sManager, indexer)
	assert.NoError(t.T(), err)
}

func (t *ControllerTestSuite) TestSetupWebhookWithManager() {
	webhook := func(mgr ctrl.Manager, client client.Client) error {
		return ctrl.NewWebhookManagedBy(mgr).
			For(&corev1.ConfigMap{}).
			Complete()
	}

	err := SetupWebhookWithManager(t.k8sManager, t.k8sClient, webhook)
	assert.NoError(t.T(), err)
}
