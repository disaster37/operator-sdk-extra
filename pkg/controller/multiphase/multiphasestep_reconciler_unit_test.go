package multiphase

import (
	"context"
	"testing"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// mockStepReconcilerAction is a simple (base) MultiPhaseStepReconcilerAction mock.
// It does NOT implement OnDiff.
type mockStepReconcilerAction struct {
	MultiPhaseStepReconcilerAction[*mockMultiPhaseObject, *mockStepObject]
	phaseName       shared.PhaseName
	configureRes    reconcile.Result
	configureErr    error
	read            MultiPhaseRead[*mockStepObject]
	readRes         reconcile.Result
	readErr         error
	diff            MultiPhaseDiff[*mockStepObject]
	diffRes         reconcile.Result
	diffErr         error
	applyRes        reconcile.Result
	applyErr        error
	deleteRes       reconcile.Result
	deleteErr       error
	onSuccessRes    reconcile.Result
	onSuccessErr    error
	onErrorRes      reconcile.Result
	onSuccessCalled bool
	onSuccessDiff   MultiPhaseDiff[*mockStepObject]
}

func (m *mockStepReconcilerAction) GetPhaseName() shared.PhaseName {
	return m.phaseName
}

func (m *mockStepReconcilerAction) Configure(ctx context.Context, req reconcile.Request, o *mockMultiPhaseObject, logger *logrus.Entry) (reconcile.Result, error) {
	return m.configureRes, m.configureErr
}

func (m *mockStepReconcilerAction) Read(ctx context.Context, o *mockMultiPhaseObject, data map[string]any, logger *logrus.Entry) (MultiPhaseRead[*mockStepObject], reconcile.Result, error) {
	read := m.read
	if read == nil {
		read = NewMultiPhaseRead[*mockStepObject]()
	}
	return read, m.readRes, m.readErr
}

func (m *mockStepReconcilerAction) Diff(ctx context.Context, o *mockMultiPhaseObject, read MultiPhaseRead[*mockStepObject], data map[string]any, logger *logrus.Entry) (MultiPhaseDiff[*mockStepObject], reconcile.Result, error) {
	return m.diff, m.diffRes, m.diffErr
}

func (m *mockStepReconcilerAction) Apply(ctx context.Context, o *mockMultiPhaseObject, data map[string]any, objects []*mockStepObject, logger *logrus.Entry) (reconcile.Result, error) {
	return m.applyRes, m.applyErr
}

func (m *mockStepReconcilerAction) Delete(ctx context.Context, o *mockMultiPhaseObject, data map[string]any, objects []*mockStepObject, logger *logrus.Entry) (reconcile.Result, error) {
	return m.deleteRes, m.deleteErr
}

func (m *mockStepReconcilerAction) OnError(ctx context.Context, o *mockMultiPhaseObject, data map[string]any, currentErr error, logger *logrus.Entry) (reconcile.Result, error) {
	return m.onErrorRes, currentErr
}

func (m *mockStepReconcilerAction) OnSuccess(ctx context.Context, o *mockMultiPhaseObject, data map[string]any, diff MultiPhaseDiff[*mockStepObject], logger *logrus.Entry) (reconcile.Result, error) {
	m.onSuccessCalled = true
	m.onSuccessDiff = diff
	return m.onSuccessRes, m.onSuccessErr
}

// mockStepReconcilerActionWithDiff is a WithDiff variant of the step action mock.
type mockStepReconcilerActionWithDiff struct {
	mockStepReconcilerAction
	onDiffRes    reconcile.Result
	onDiffErr    error
	onDiffCalled bool
}

func (m *mockStepReconcilerActionWithDiff) OnDiff(ctx context.Context, o *mockMultiPhaseObject, data map[string]any, diff MultiPhaseDiff[*mockStepObject], logger *logrus.Entry) (reconcile.Result, error) {
	m.onDiffCalled = true
	return m.onDiffRes, m.onDiffErr
}

func newStepReconcilerTestFixture(t *testing.T) (*DefaultMultiPhaseStepReconciler[*mockMultiPhaseObject, *mockStepObject], *logrus.Entry) {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := logrus.NewEntry(logrus.New())
	recorder := record.NewFakeRecorder(10)

	reconciler := NewMultiPhaseStepReconciler[*mockMultiPhaseObject, *mockStepObject](c, logger, recorder)

	return reconciler.(*DefaultMultiPhaseStepReconciler[*mockMultiPhaseObject, *mockStepObject]), logger
}

func TestDefaultMultiPhaseStepReconciler_Reconcile_SimpleAction_OnDiffNotCalled(t *testing.T) {
	reconciler, logger := newStepReconcilerTestFixture(t)

	diff := NewMultiPhaseDiff[*mockStepObject]()
	action := &mockStepReconcilerAction{
		phaseName: "test-phase",
		diff:      diff,
	}

	o := &mockMultiPhaseObject{name: "parent", namespace: "test-ns"}
	req := reconcile.Request{NamespacedName: struct{ Namespace, Name string }{Namespace: "test-ns", Name: "parent"}}

	res, err := reconciler.Reconcile(context.Background(), req, o, map[string]any{}, action, logger)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.True(t, action.onSuccessCalled)
	assert.Equal(t, diff, action.onSuccessDiff)
}

func TestDefaultMultiPhaseStepReconciler_Reconcile_WithDiffAction_OnDiffCalled(t *testing.T) {
	reconciler, logger := newStepReconcilerTestFixture(t)

	diff := NewMultiPhaseDiff[*mockStepObject]()
	action := &mockStepReconcilerActionWithDiff{
		mockStepReconcilerAction: mockStepReconcilerAction{
			phaseName: "test-phase",
			diff:      diff,
		},
	}

	o := &mockMultiPhaseObject{name: "parent", namespace: "test-ns"}
	req := reconcile.Request{NamespacedName: struct{ Namespace, Name string }{Namespace: "test-ns", Name: "parent"}}

	res, err := reconciler.Reconcile(context.Background(), req, o, map[string]any{}, action, logger)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.True(t, action.onDiffCalled)
	assert.True(t, action.onSuccessCalled)
}

func TestDefaultMultiPhaseStepReconciler_Reconcile_WithDiffAction_OnDiffError(t *testing.T) {
	reconciler, logger := newStepReconcilerTestFixture(t)

	diff := NewMultiPhaseDiff[*mockStepObject]()
	action := &mockStepReconcilerActionWithDiff{
		mockStepReconcilerAction: mockStepReconcilerAction{
			phaseName:  "test-phase",
			diff:       diff,
			onErrorRes: reconcile.Result{Requeue: true},
		},
		onDiffErr: errors.New("onDiff failed"),
	}

	o := &mockMultiPhaseObject{name: "parent", namespace: "test-ns"}
	req := reconcile.Request{NamespacedName: struct{ Namespace, Name string }{Namespace: "test-ns", Name: "parent"}}

	res, err := reconciler.Reconcile(context.Background(), req, o, map[string]any{}, action, logger)
	assert.Error(t, err)
	assert.True(t, res.Requeue)
	assert.True(t, action.onDiffCalled)
	assert.False(t, action.onSuccessCalled)
}

// mockConfigMapStepReconcilerAction is a minimal MultiPhaseStepReconcilerAction
// mock typed over *corev1.ConfigMap so the reconciler cleanup path can be
// exercised against real objects in the fake client.
type mockConfigMapStepReconcilerAction struct {
	MultiPhaseStepReconcilerAction[*mockMultiPhaseObject, *corev1.ConfigMap]
	phaseName shared.PhaseName
	read      MultiPhaseRead[*corev1.ConfigMap]
	diff      MultiPhaseDiff[*corev1.ConfigMap]
}

func (m *mockConfigMapStepReconcilerAction) GetPhaseName() shared.PhaseName {
	return m.phaseName
}

func (m *mockConfigMapStepReconcilerAction) Configure(ctx context.Context, req reconcile.Request, o *mockMultiPhaseObject, logger *logrus.Entry) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (m *mockConfigMapStepReconcilerAction) Read(ctx context.Context, o *mockMultiPhaseObject, data map[string]any, logger *logrus.Entry) (MultiPhaseRead[*corev1.ConfigMap], reconcile.Result, error) {
	return m.read, reconcile.Result{}, nil
}

func (m *mockConfigMapStepReconcilerAction) Diff(ctx context.Context, o *mockMultiPhaseObject, read MultiPhaseRead[*corev1.ConfigMap], data map[string]any, logger *logrus.Entry) (MultiPhaseDiff[*corev1.ConfigMap], reconcile.Result, error) {
	return m.diff, reconcile.Result{}, nil
}

func (m *mockConfigMapStepReconcilerAction) Apply(ctx context.Context, o *mockMultiPhaseObject, data map[string]any, objects []*corev1.ConfigMap, logger *logrus.Entry) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (m *mockConfigMapStepReconcilerAction) Delete(ctx context.Context, o *mockMultiPhaseObject, data map[string]any, objects []*corev1.ConfigMap, logger *logrus.Entry) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (m *mockConfigMapStepReconcilerAction) OnError(ctx context.Context, o *mockMultiPhaseObject, data map[string]any, currentErr error, logger *logrus.Entry) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (m *mockConfigMapStepReconcilerAction) OnSuccess(ctx context.Context, o *mockMultiPhaseObject, data map[string]any, diff MultiPhaseDiff[*corev1.ConfigMap], logger *logrus.Entry) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func TestDefaultMultiPhaseStepReconciler_Reconcile_CleansLastAppliedAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: "test-ns",
			Annotations: map[string]string{
				lastAppliedConfigAnnotationKey: `{"kind":"ConfigMap"}`,
				"keep-me":                      "value",
			},
		},
		Data: map[string]string{"key": "value"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	logger := logrus.NewEntry(logrus.New())
	recorder := record.NewFakeRecorder(10)

	reconciler := NewMultiPhaseStepReconciler[*mockMultiPhaseObject, *corev1.ConfigMap](c, logger, recorder)

	read := NewMultiPhaseRead[*corev1.ConfigMap]()
	read.AddCurrentObject(cm)
	read.AddExpectedObject(cm.DeepCopy())

	action := &mockConfigMapStepReconcilerAction{
		phaseName: "test-phase",
		read:      read,
		diff:      NewMultiPhaseDiff[*corev1.ConfigMap](),
	}

	o := &mockMultiPhaseObject{name: "parent", namespace: "test-ns"}
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "parent"}}

	res, err := reconciler.Reconcile(context.Background(), req, o, map[string]any{}, action, logger)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	got := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "child", Namespace: "test-ns"}, got))
	assert.NotContains(t, got.Annotations, lastAppliedConfigAnnotationKey)
	assert.Equal(t, "value", got.Annotations["keep-me"])
	assert.Equal(t, "value", got.Data["key"])
}

func TestDefaultMultiPhaseStepReconciler_Reconcile_WithDiffAction_OnDiffRequeue(t *testing.T) {
	reconciler, logger := newStepReconcilerTestFixture(t)

	diff := NewMultiPhaseDiff[*mockStepObject]()
	action := &mockStepReconcilerActionWithDiff{
		mockStepReconcilerAction: mockStepReconcilerAction{
			phaseName: "test-phase",
			diff:      diff,
		},
		onDiffRes: reconcile.Result{Requeue: true},
	}

	o := &mockMultiPhaseObject{name: "parent", namespace: "test-ns"}
	req := reconcile.Request{NamespacedName: struct{ Namespace, Name string }{Namespace: "test-ns", Name: "parent"}}

	res, err := reconciler.Reconcile(context.Background(), req, o, map[string]any{}, action, logger)
	assert.NoError(t, err)
	assert.True(t, res.Requeue)
	assert.True(t, action.onDiffCalled)
	assert.False(t, action.onSuccessCalled)
}
