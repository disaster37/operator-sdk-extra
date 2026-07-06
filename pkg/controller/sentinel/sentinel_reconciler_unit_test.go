package sentinel

import (
	"context"
	"errors"
	"testing"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/multiphase"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type mockSentinelReconcilerAction struct {
	SentinelReconcilerAction[*corev1.Pod]
	configureRes reconcile.Result
	configureErr error
	readRes      reconcile.Result
	readErr      error
	readObj      SentinelRead
	applyRes     reconcile.Result
	applyErr     error
	deleteRes    reconcile.Result
	deleteErr    error
	onSuccessRes reconcile.Result
	onSuccessErr error
	onErrorRes   reconcile.Result
	diffRes      reconcile.Result
	diffErr      error
	diffObj      multiphase.MultiPhaseDiff[client.Object]
}

func (m *mockSentinelReconcilerAction) Configure(ctx context.Context, req reconcile.Request, o *corev1.Pod, data map[string]any, logger *logrus.Entry) (reconcile.Result, error) {
	return m.configureRes, m.configureErr
}

func (m *mockSentinelReconcilerAction) Read(ctx context.Context, o *corev1.Pod, data map[string]any, logger *logrus.Entry) (SentinelRead, reconcile.Result, error) {
	return m.readObj, m.readRes, m.readErr
}

func (m *mockSentinelReconcilerAction) Apply(ctx context.Context, o *corev1.Pod, data map[string]any, objects []client.Object, logger *logrus.Entry) (reconcile.Result, error) {
	return m.applyRes, m.applyErr
}

func (m *mockSentinelReconcilerAction) Delete(ctx context.Context, o *corev1.Pod, data map[string]any, objects []client.Object, logger *logrus.Entry) (reconcile.Result, error) {
	return m.deleteRes, m.deleteErr
}

func (m *mockSentinelReconcilerAction) OnError(ctx context.Context, o *corev1.Pod, data map[string]any, currentErr error, logger *logrus.Entry) (reconcile.Result, error) {
	return m.onErrorRes, currentErr
}

func (m *mockSentinelReconcilerAction) OnSuccess(ctx context.Context, o *corev1.Pod, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (reconcile.Result, error) {
	return m.onSuccessRes, m.onSuccessErr
}

func (m *mockSentinelReconcilerAction) OnDiff(ctx context.Context, o *corev1.Pod, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (m *mockSentinelReconcilerAction) Diff(ctx context.Context, o *corev1.Pod, read SentinelRead, data map[string]any, logger *logrus.Entry) (multiphase.MultiPhaseDiff[client.Object], reconcile.Result, error) {
	return m.diffObj, m.diffRes, m.diffErr
}

func (m *mockSentinelReconcilerAction) GetFieldManager() string {
	return "test-manager"
}

func TestNewSentinelReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := logrus.NewEntry(logrus.StandardLogger())
	recorder := record.NewFakeRecorder(10)

	reconciler := NewSentinelReconciler[*corev1.Pod](c, "test-reconciler", logger, recorder)
	assert.NotNil(t, reconciler)
}

func TestDefaultSentinelReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	mockObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
	}

	t.Run("get object returns not found - returns empty result", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewSentinelReconciler[*corev1.Pod](c, "test", logger, recorder)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"}}
		mockAction := &mockSentinelReconcilerAction{}

		res, err := reconciler.Reconcile(context.Background(), req, mockObj, map[string]any{}, mockAction)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})

	t.Run("ignore reconcile with annotation", func(t *testing.T) {
		obj := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-anno",
				Namespace: "default",
				UID:       types.UID("test-uid"),
				Annotations: map[string]string{
					"operator-sdk-extra.webcenter.fr/ignoreReconcile": "true",
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewSentinelReconciler[*corev1.Pod](c, "test", logger, recorder)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-anno", Namespace: "default"}}
		mockAction := &mockSentinelReconcilerAction{}

		res, err := reconciler.Reconcile(context.Background(), req, obj, map[string]any{}, mockAction)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})

	t.Run("configure error calls onError", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockObj).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewSentinelReconciler[*corev1.Pod](c, "test", logger, recorder)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
		mockAction := &mockSentinelReconcilerAction{
			configureErr: errors.New("configure failed"),
			onErrorRes:   reconcile.Result{Requeue: true},
		}

		res, err := reconciler.Reconcile(context.Background(), req, mockObj, map[string]any{}, mockAction)
		assert.Error(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("configure returns requeue - short circuits", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockObj).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewSentinelReconciler[*corev1.Pod](c, "test", logger, recorder)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
		mockAction := &mockSentinelReconcilerAction{
			configureRes: reconcile.Result{Requeue: true},
		}

		res, err := reconciler.Reconcile(context.Background(), req, mockObj, map[string]any{}, mockAction)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("read error calls onError", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockObj).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewSentinelReconciler[*corev1.Pod](c, "test", logger, recorder)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
		mockAction := &mockSentinelReconcilerAction{
			readErr:    errors.New("read failed"),
			onErrorRes: reconcile.Result{Requeue: true},
		}

		res, err := reconciler.Reconcile(context.Background(), req, mockObj, map[string]any{}, mockAction)
		assert.Error(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("diff error calls onError", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockObj).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewSentinelReconciler[*corev1.Pod](c, "test", logger, recorder)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
		mockAction := &mockSentinelReconcilerAction{
			readObj:    NewSentinelRead(scheme),
			diffErr:    errors.New("diff failed"),
			onErrorRes: reconcile.Result{Requeue: true},
		}

		res, err := reconciler.Reconcile(context.Background(), req, mockObj, map[string]any{}, mockAction)
		assert.Error(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("successful reconcile with apply and delete", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockObj).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewSentinelReconciler[*corev1.Pod](c, "test", logger, recorder)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}

		read := NewSentinelRead(scheme)
		diff := multiphase.NewMultiPhaseDiff[client.Object]()
		diff.AddObjectToCreate(mockObj)
		diff.AddObjectToDelete(mockObj)

		mockAction := &mockSentinelReconcilerAction{
			readObj: read,
			diffObj: diff,
		}

		res, err := reconciler.Reconcile(context.Background(), req, mockObj, map[string]any{}, mockAction)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})

	t.Run("apply error calls onError", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockObj).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewSentinelReconciler[*corev1.Pod](c, "test", logger, recorder)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}

		read := NewSentinelRead(scheme)
		diff := multiphase.NewMultiPhaseDiff[client.Object]()
		diff.AddObjectToCreate(mockObj)

		mockAction := &mockSentinelReconcilerAction{
			readObj:    read,
			diffObj:    diff,
			applyErr:   errors.New("apply failed"),
			onErrorRes: reconcile.Result{Requeue: true},
		}

		res, err := reconciler.Reconcile(context.Background(), req, mockObj, map[string]any{}, mockAction)
		assert.Error(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("delete error calls onError", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockObj).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())
		recorder := record.NewFakeRecorder(10)

		reconciler := NewSentinelReconciler[*corev1.Pod](c, "test", logger, recorder)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}

		read := NewSentinelRead(scheme)
		diff := multiphase.NewMultiPhaseDiff[client.Object]()
		diff.AddObjectToDelete(mockObj)

		mockAction := &mockSentinelReconcilerAction{
			readObj:    read,
			diffObj:    diff,
			deleteErr:  errors.New("delete failed"),
			onErrorRes: reconcile.Result{Requeue: true},
		}

		res, err := reconciler.Reconcile(context.Background(), req, mockObj, map[string]any{}, mockAction)
		assert.Error(t, err)
		assert.True(t, res.Requeue)
	})
}