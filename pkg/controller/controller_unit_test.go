package controller

import (
	"context"
	"errors"
	"testing"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/stretchr/testify/assert"
)

// Mock implementations for testing
type mockManager struct {
	ctrl.Manager
	fieldIndexer  client.FieldIndexer
	webhookServer webhook.Server
}

func (m *mockManager) GetFieldIndexer() client.FieldIndexer {
	return m.fieldIndexer
}

func (m *mockManager) GetWebhookServer() webhook.Server {
	return m.webhookServer
}

type mockFieldIndexer struct {
	client.FieldIndexer
	indexErr error
}

func (mfi *mockFieldIndexer) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	return mfi.indexErr
}

func TestSetupIndexerWithManager(t *testing.T) {
	t.Run("nominal case - no indexers", func(t *testing.T) {
		manager := &mockManager{}

		err := SetupIndexerWithManager(manager)
		assert.NoError(t, err)
	})

	t.Run("nominal case - successful indexer", func(t *testing.T) {
		manager := &mockManager{
			fieldIndexer: &mockFieldIndexer{},
		}

		called := false
		indexer := func(mgr ctrl.Manager) error {
			called = true
			return nil
		}

		err := SetupIndexerWithManager(manager, indexer)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("multiple successful indexers", func(t *testing.T) {
		manager := &mockManager{
			fieldIndexer: &mockFieldIndexer{},
		}

		calls := 0
		indexer1 := func(mgr ctrl.Manager) error {
			calls++
			return nil
		}
		indexer2 := func(mgr ctrl.Manager) error {
			calls++
			return nil
		}

		err := SetupIndexerWithManager(manager, indexer1, indexer2)
		assert.NoError(t, err)
		assert.Equal(t, 2, calls)
	})

	t.Run("error in indexer - returns error", func(t *testing.T) {
		manager := &mockManager{
			fieldIndexer: &mockFieldIndexer{},
		}

		expectedErr := errors.New("indexer error")
		failingIndexer := func(mgr ctrl.Manager) error {
			return expectedErr
		}

		workingIndexer := func(mgr ctrl.Manager) error {
			return nil
		}

		err := SetupIndexerWithManager(manager, workingIndexer, failingIndexer)
		assert.Equal(t, expectedErr, err)
	})
}

func TestSetupWebhookWithManager(t *testing.T) {
	t.Run("nominal case - no webhooks", func(t *testing.T) {
		manager := &mockManager{}
		mockClient := fake.NewClientBuilder().Build()

		err := SetupWebhookWithManager(manager, mockClient)
		assert.NoError(t, err)
	})

	t.Run("nominal case - successful webhook register", func(t *testing.T) {
		manager := &mockManager{}
		mockClient := fake.NewClientBuilder().Build()

		called := false
		webhookRegister := func(mgr ctrl.Manager, c client.Client) error {
			called = true
			return nil
		}

		err := SetupWebhookWithManager(manager, mockClient, webhookRegister)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("multiple successful webhook registers", func(t *testing.T) {
		manager := &mockManager{}
		mockClient := fake.NewClientBuilder().Build()

		calls := 0
		webhookRegister1 := func(mgr ctrl.Manager, c client.Client) error {
			calls++
			return nil
		}
		webhookRegister2 := func(mgr ctrl.Manager, c client.Client) error {
			calls++
			return nil
		}

		err := SetupWebhookWithManager(manager, mockClient, webhookRegister1, webhookRegister2)
		assert.NoError(t, err)
		assert.Equal(t, 2, calls)
	})

	t.Run("error in webhook register - returns error", func(t *testing.T) {
		manager := &mockManager{}
		mockClient := fake.NewClientBuilder().Build()

		expectedErr := errors.New("webhook error")
		failingWebhookRegister := func(mgr ctrl.Manager, c client.Client) error {
			return expectedErr
		}

		workingWebhookRegister := func(mgr ctrl.Manager, c client.Client) error {
			return nil
		}

		err := SetupWebhookWithManager(manager, mockClient, workingWebhookRegister, failingWebhookRegister)
		assert.Equal(t, expectedErr, err)
	})
}

func TestDefaultController(t *testing.T) {
	controller := NewController()

	t.Run("NewController returns non-nil controller", func(t *testing.T) {
		assert.NotNil(t, controller)
	})

	t.Run("SetupWithManager panics when called", func(t *testing.T) {
		manager := &mockManager{}
		assert.Panics(t, func() {
			_ = controller.SetupWithManager(manager)
		})
	})

	t.Run("Reconcile panics when called", func(t *testing.T) {
		req := reconcile.Request{}
		assert.Panics(t, func() {
			_, _ = controller.Reconcile(context.Background(), req)
		})
	})
}
