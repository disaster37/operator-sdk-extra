package mock

import (
	"context"
	"errors"
	"testing"
	"time"

	remotepkg "github.com/disaster37/operator-sdk-extra/v3/pkg/controller/remote"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type mockRemoteObject struct {
	object.RemoteObject
	name      string
	namespace string
	status    mockRemoteObjectStatus
}

func (m *mockRemoteObject) GetName() string {
	return m.name
}

func (m *mockRemoteObject) GetNamespace() string {
	return m.namespace
}

func (m *mockRemoteObject) GetObjectMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      m.name,
		Namespace: m.namespace,
	}
}

func (m *mockRemoteObject) GetStatus() object.RemoteObjectStatus {
	return &m.status
}

type mockRemoteObjectStatus struct {
	object.RemoteObjectStatus
	lastAppliedConfiguration string
	isOnError                bool
	isSync                   bool
	lastErrorMessage         string
	observedGeneration       int64
	conditions               []metav1.Condition
}

func (m *mockRemoteObjectStatus) GetIsOnError() bool {
	return m.isOnError
}

func (m *mockRemoteObjectStatus) SetIsOnError(isOnError bool) {
	m.isOnError = isOnError
}

func (m *mockRemoteObjectStatus) GetIsSync() bool {
	return m.isSync
}

func (m *mockRemoteObjectStatus) SetIsSync(isSync bool) {
	m.isSync = isSync
}

func (m *mockRemoteObjectStatus) GetLastErrorMessage() string {
	return m.lastErrorMessage
}

func (m *mockRemoteObjectStatus) SetLastErrorMessage(message string) {
	m.lastErrorMessage = message
}

func (m *mockRemoteObjectStatus) GetLastAppliedConfiguration() string {
	return m.lastAppliedConfiguration
}

func (m *mockRemoteObjectStatus) SetLastAppliedConfiguration(config string) {
	m.lastAppliedConfiguration = config
}

func (m *mockRemoteObjectStatus) GetObservedGeneration() int64 {
	return m.observedGeneration
}

func (m *mockRemoteObjectStatus) SetObservedGeneration(gen int64) {
	m.observedGeneration = gen
}

func (m *mockRemoteObjectStatus) GetConditions() []metav1.Condition {
	return m.conditions
}

func (m *mockRemoteObjectStatus) SetConditions(conditions []metav1.Condition) {
	m.conditions = conditions
}

type mockAPIObject struct {
	ID   string
	Name string
}

type mockAPIClient struct{}

func TestNewMockRemoteReconcilerAction(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	baseAction := remotepkg.NewRemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient](c, recorder)

	t.Run("creates a new mock action", func(t *testing.T) {
		mockHandler := func(ctx context.Context, req reconcile.Request, o *mockRemoteObject, logger *logrus.Entry) (remotepkg.RemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient], reconcile.Result, error) {
			return nil, reconcile.Result{}, nil
		}

		action := NewMockRemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient](baseAction, mockHandler)
		assert.NotNil(t, action)
	})

	t.Run("GetRemoteHandler calls the provided mock function", func(t *testing.T) {
		called := false
		mockHandler := func(ctx context.Context, req reconcile.Request, o *mockRemoteObject, logger *logrus.Entry) (remotepkg.RemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient], reconcile.Result, error) {
			called = true
			return nil, reconcile.Result{RequeueAfter: time.Millisecond}, errors.New("mock error")
		}

		action := NewMockRemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient](baseAction, mockHandler)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
		obj := &mockRemoteObject{name: "test", namespace: "default"}

		handler, res, err := action.GetRemoteHandler(context.Background(), req, obj, &logrus.Entry{})
		assert.True(t, called)
		assert.Nil(t, handler)
		assert.Greater(t, res.RequeueAfter, time.Duration(0))
		assert.EqualError(t, err, "mock error")
	})
}
