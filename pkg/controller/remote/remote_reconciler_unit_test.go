package remote

import (
	"context"
	"errors"
	"testing"

	"github.com/disaster37/generic-objectmatcher/patch"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	testpkg "github.com/disaster37/operator-sdk-extra/v2/pkg/test"
)

type mockRemoteReconcilerAction2 struct {
	RemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient]
	getRemoteHandlerRes     reconcile.Result
	getRemoteHandlerErr     error
	getRemoteHandlerHandler RemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient]
	configureRes            reconcile.Result
	configureErr            error
	readRes                 reconcile.Result
	readErr                 error
	readObj                 RemoteRead[mockAPIObject]
	deleteErr               error
	onErrorRes              reconcile.Result
	onSuccessRes            reconcile.Result
	onSuccessErr            error
	diffRes                 reconcile.Result
	diffErr                 error
	diffObj                 RemoteDiff[mockAPIObject]
	createRes               reconcile.Result
	createErr               error
	updateRes               reconcile.Result
	updateErr               error
}

func (m *mockRemoteReconcilerAction2) GetRemoteHandler(ctx context.Context, req reconcile.Request, o *mockRemoteObject, logger *logrus.Entry) (RemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient], reconcile.Result, error) {
	return m.getRemoteHandlerHandler, m.getRemoteHandlerRes, m.getRemoteHandlerErr
}

func (m *mockRemoteReconcilerAction2) Configure(ctx context.Context, o *mockRemoteObject, data map[string]any, handler RemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient], logger *logrus.Entry) (reconcile.Result, error) {
	return m.configureRes, m.configureErr
}

func (m *mockRemoteReconcilerAction2) Read(ctx context.Context, o *mockRemoteObject, data map[string]any, handler RemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient], logger *logrus.Entry) (RemoteRead[mockAPIObject], reconcile.Result, error) {
	return m.readObj, m.readRes, m.readErr
}

func (m *mockRemoteReconcilerAction2) Delete(ctx context.Context, o *mockRemoteObject, data map[string]any, handler RemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient], logger *logrus.Entry) error {
	return m.deleteErr
}

func (m *mockRemoteReconcilerAction2) OnError(ctx context.Context, o *mockRemoteObject, data map[string]any, handler RemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient], currentErr error, logger *logrus.Entry) (reconcile.Result, error) {
	return m.onErrorRes, currentErr
}

func (m *mockRemoteReconcilerAction2) OnSuccess(ctx context.Context, o *mockRemoteObject, data map[string]any, handler RemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient], diff RemoteDiff[mockAPIObject], logger *logrus.Entry) (reconcile.Result, error) {
	return m.onSuccessRes, m.onSuccessErr
}

func (m *mockRemoteReconcilerAction2) Diff(ctx context.Context, o *mockRemoteObject, read RemoteRead[mockAPIObject], data map[string]any, handler RemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient], logger *logrus.Entry, ignoreDiff ...patch.CalculateOption) (RemoteDiff[mockAPIObject], reconcile.Result, error) {
	return m.diffObj, m.diffRes, m.diffErr
}

func (m *mockRemoteReconcilerAction2) GetIgnoresDiff() []patch.CalculateOption {
	return nil
}

func TestNewRemoteReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := logrus.NewEntry(logrus.StandardLogger())
	recorder := record.NewFakeRecorder(10)

	reconciler := NewRemoteReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient](c, "test-reconciler", "finalizer", logger, recorder)
	assert.NotNil(t, reconciler)
}

func TestDefaultRemoteReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	obj := &mockRemoteObject{
		name:      "test",
		namespace: "default",
		generation: 1,
	}

	t.Run("get object not found - returns empty result", func(t *testing.T) {
		baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		c := testpkg.NewInterceptorClient(baseClient)
		c.GetInterceptor = func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			return k8serrors.NewNotFound(schema.GroupResource{Resource: "mock"}, key.Name)
		}
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewRemoteReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient](c, "test", "", logger, recorder)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"}}

		mockAction := &mockRemoteReconcilerAction2{}
		res, err := reconciler.Reconcile(context.Background(), req, obj, map[string]any{}, mockAction)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})

	t.Run("get remote handler error calls onError", func(t *testing.T) {
		baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		c := testpkg.NewInterceptorClient(baseClient)
		c.GetInterceptor = func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			return nil
		}
		c.StatusUpdateInterceptor = func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			return nil
		}
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewRemoteReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient](c, "test", "", logger, recorder)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}

		onErrorRes := reconcile.Result{Requeue: true}
		mockAction := &mockRemoteReconcilerAction2{
			getRemoteHandlerErr: errors.New("handler error"),
			onErrorRes:          onErrorRes,
		}

		res, err := reconciler.Reconcile(context.Background(), req, obj, map[string]any{}, mockAction)
		assert.Error(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("configure error calls onError", func(t *testing.T) {
		baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		c := testpkg.NewInterceptorClient(baseClient)
		c.GetInterceptor = func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			return nil
		}
		c.StatusUpdateInterceptor = func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			return nil
		}
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewRemoteReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient](c, "test", "", logger, recorder)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}

		mockAction := &mockRemoteReconcilerAction2{
			getRemoteHandlerHandler: &DefaultRemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient]{},
			configureErr:            errors.New("configure failed"),
			onErrorRes:              reconcile.Result{Requeue: true},
		}

		res, err := reconciler.Reconcile(context.Background(), req, obj, map[string]any{}, mockAction)
		assert.Error(t, err)
		assert.True(t, res.Requeue)
	})
}