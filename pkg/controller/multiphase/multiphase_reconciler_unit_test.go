package multiphase

import (
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type mockMultiPhaseReconcilerAction struct {
	MultiPhaseReconcilerAction[*MockMultiPhaseObject]
	configureRes reconcile.Result
	configureErr error
	readRes      reconcile.Result
	readErr      error
	deleteErr    error
	onSuccessRes reconcile.Result
	onSuccessErr error
	onErrorRes   reconcile.Result
	onErrorErr   error
}

func (m *mockMultiPhaseReconcilerAction) Configure(ctx context.Context, req reconcile.Request, o *MockMultiPhaseObject, data map[string]any, logger *logrus.Entry) (res reconcile.Result, err error) {
	return m.configureRes, m.configureErr
}

func (m *mockMultiPhaseReconcilerAction) Read(ctx context.Context, o *MockMultiPhaseObject, data map[string]any, logger *logrus.Entry) (res reconcile.Result, err error) {
	return m.readRes, m.readErr
}

func (m *mockMultiPhaseReconcilerAction) Delete(ctx context.Context, o *MockMultiPhaseObject, data map[string]any, logger *logrus.Entry) (err error) {
	return m.deleteErr
}

func (m *mockMultiPhaseReconcilerAction) OnSuccess(ctx context.Context, o *MockMultiPhaseObject, data map[string]any, logger *logrus.Entry) (res reconcile.Result, err error) {
	return m.onSuccessRes, m.onSuccessErr
}

func (m *mockMultiPhaseReconcilerAction) OnError(ctx context.Context, o *MockMultiPhaseObject, data map[string]any, currentErr error, logger *logrus.Entry) (res reconcile.Result, err error) {
	return m.onErrorRes, m.onErrorErr
}

func TestMultiPhaseReconcilerNew(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := logrus.NewEntry(logrus.StandardLogger())
	recorder := record.NewFakeRecorder(10)

	reconciler := NewMultiPhaseReconciler[*MockMultiPhaseObject](c, "test-reconciler", "finalizer", logger, recorder)
	assert.NotNil(t, reconciler)
}

func TestDefaultMultiPhaseReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	gvk := schema.GroupVersionKind{Group: "test", Version: "v1", Kind: "MockMultiPhaseObject"}
	scheme.AddKnownTypeWithName(gvk, &MockMultiPhaseObject{})

	obj := &MockMultiPhaseObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Status: MockMultiPhaseObjectStatus{},
	}

	t.Run("get object returns not found - returns empty result", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)
		reconciler := NewMultiPhaseReconciler[*MockMultiPhaseObject](c, "test", "", logger, recorder)

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"}}
		mockAction := &mockMultiPhaseReconcilerAction{}

		res, err := reconciler.Reconcile(context.Background(), req, obj, map[string]any{}, mockAction)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})

	t.Run("configure error calls onError", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewMultiPhaseReconciler[*MockMultiPhaseObject](c, "test", "", logger, recorder)

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
		mockAction := &mockMultiPhaseReconcilerAction{
			configureErr: errors.New("configure failed"),
			onErrorRes:   reconcile.Result{Requeue: true},
		}

		res, _ := reconciler.Reconcile(context.Background(), req, obj, map[string]any{}, mockAction)
		assert.True(t, res.Requeue)
	})

	t.Run("configure returns requeue - short circuits", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewMultiPhaseReconciler[*MockMultiPhaseObject](c, "test", "", logger, recorder)

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
		mockAction := &mockMultiPhaseReconcilerAction{
			configureRes: reconcile.Result{Requeue: true},
		}

		res, err := reconciler.Reconcile(context.Background(), req, obj, map[string]any{}, mockAction)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("read error calls onError", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewMultiPhaseReconciler[*MockMultiPhaseObject](c, "test", "", logger, recorder)

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
		mockAction := &mockMultiPhaseReconcilerAction{
			readErr:    errors.New("read failed"),
			onErrorRes: reconcile.Result{Requeue: true},
		}

		res, _ := reconciler.Reconcile(context.Background(), req, obj, map[string]any{}, mockAction)
		assert.True(t, res.Requeue)
	})

	t.Run("add finalizer on object without finalizer", func(t *testing.T) {
		objNoFinalizer := &MockMultiPhaseObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-finalizer",
				Namespace: "default",
				UID:       types.UID("test-uid"),
			},
			Status: MockMultiPhaseObjectStatus{},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objNoFinalizer).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewMultiPhaseReconciler[*MockMultiPhaseObject](c, "test", "test-finalizer", logger, recorder)

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-finalizer", Namespace: "default"}}
		mockAction := &mockMultiPhaseReconcilerAction{}

		res, err := reconciler.Reconcile(context.Background(), req, objNoFinalizer, map[string]any{}, mockAction)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("ignore reconcile with annotation", func(t *testing.T) {
		objWithAnnotation := &MockMultiPhaseObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-anno",
				Namespace: "default",
				UID:       types.UID("test-uid"),
				Annotations: map[string]string{
					"operator-sdk-extra.webcenter.fr/ignoreReconcile": "true",
				},
			},
			Status: MockMultiPhaseObjectStatus{},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objWithAnnotation).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewMultiPhaseReconciler[*MockMultiPhaseObject](c, "test", "", logger, recorder)

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-anno", Namespace: "default"}}
		mockAction := &mockMultiPhaseReconcilerAction{}

		res, err := reconciler.Reconcile(context.Background(), req, objWithAnnotation, map[string]any{}, mockAction)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})

	t.Run("deleting object with finalizer - removes finalizer", func(t *testing.T) {
		now := metav1.Now()
		obj := &MockMultiPhaseObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-delete",
				Namespace:         "default",
				UID:               types.UID("test-uid"),
				DeletionTimestamp: &now,
				Finalizers:        []string{"test-finalizer"},
			},
			Status: MockMultiPhaseObjectStatus{},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewMultiPhaseReconciler[*MockMultiPhaseObject](c, "test", "test-finalizer", logger, recorder)

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-delete", Namespace: "default"}}
		mockAction := &mockMultiPhaseReconcilerAction{}

		res, err := reconciler.Reconcile(context.Background(), req, obj, map[string]any{}, mockAction)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})
}
