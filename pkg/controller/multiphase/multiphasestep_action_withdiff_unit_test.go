package multiphase

import (
	"context"
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// mockWithDiffInnerAction is a MultiPhaseStepReconcilerActionWithDiff that
// records OnDiff calls for forwarding assertions.
type mockWithDiffInnerAction struct {
	MultiPhaseStepReconcilerActionWithDiff[*mockMultiPhaseObject, *mockStepObject]
	onDiffCalled bool
}

func (m *mockWithDiffInnerAction) OnDiff(ctx context.Context, o *mockMultiPhaseObject, data map[string]any, diff MultiPhaseDiff[*mockStepObject], logger *logrus.Entry) (reconcile.Result, error) {
	m.onDiffCalled = true
	return reconcile.Result{}, nil
}

func TestNewMultiPhaseStepReconcilerActionWithDiff(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewMultiPhaseStepReconcilerActionWithDiff[*mockMultiPhaseObject, *mockStepObject](
		fakeClient,
		"test-phase",
		"Ready",
		recorder,
		"test-controller",
	)

	require.NotNil(t, action)
	assert.Equal(t, shared.PhaseName("test-phase"), action.GetPhaseName())
}

func TestDefaultMultiPhaseStepReconcilerActionWithDiff_OnDiff(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewMultiPhaseStepReconcilerActionWithDiff[*mockMultiPhaseObject, *mockStepObject](
		fakeClient,
		"test-phase",
		"Ready",
		recorder,
		"test-controller",
	)

	diff := NewMultiPhaseDiff[*mockStepObject]()
	res, err := action.OnDiff(context.Background(), &mockMultiPhaseObject{name: "parent", namespace: "test-ns"}, map[string]any{}, diff, logrus.NewEntry(logrus.New()))
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
}

func TestDefaultMultiPhaseStepReconcilerActionWithDiff_Diff(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	expected := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string]string{
			"foo": "bar",
		},
	}
	current := expected.DeepCopy()

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewMultiPhaseStepReconcilerActionWithDiff[*mockMultiPhaseObject, *corev1.ConfigMap](
		fakeClient,
		"test-phase",
		"Ready",
		recorder,
		"test-controller",
	)

	read := NewMultiPhaseRead[*corev1.ConfigMap]()
	read.AddExpectedObject(expected)
	read.AddCurrentObject(current)

	diff, res, err := action.Diff(context.Background(), &mockMultiPhaseObject{name: "parent", namespace: "test-ns"}, read, map[string]any{}, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Skipf("fake client does not support SSA dry-run apply: %s", err)
	}
	assert.Equal(t, reconcile.Result{}, res)
	// The diff variant uses SSA dry-run classification: an unchanged object must not
	// be classified as update.
	assert.False(t, diff.NeedUpdate())
	assert.False(t, diff.NeedCreate())
	assert.False(t, diff.NeedDelete())
}

func TestNewObjectMultiPhaseStepReconcilerActionWithDiff(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	t.Run("AsWithDiff[D]() method on default implementation", func(t *testing.T) {
		inner := NewMultiPhaseStepReconcilerActionWithDiff[*mockMultiPhaseObject, *mockStepObject](
			fakeClient,
			"test-phase",
			"Ready",
			recorder,
			"test-controller",
		).(*DefaultMultiPhaseStepReconcilerActionWithDiff[*mockMultiPhaseObject, *mockStepObject])

		wrapped := inner.AsWithDiff[client.Object]()
		require.NotNil(t, wrapped)

		// GetPhaseName should forward
		assert.Equal(t, shared.PhaseName("test-phase"), wrapped.GetPhaseName())
	})

	t.Run("Deprecated NewObjectMultiPhaseStepReconcilerActionWithDiff with custom implementation", func(t *testing.T) {
		inner := &mockWithDiffInnerAction{
			MultiPhaseStepReconcilerActionWithDiff: NewMultiPhaseStepReconcilerActionWithDiff[*mockMultiPhaseObject, *mockStepObject](
				fakeClient,
				"test-phase",
				"Ready",
				recorder,
				"test-controller",
			),
		}

		wrapped := NewObjectMultiPhaseStepReconcilerActionWithDiff[*mockMultiPhaseObject, *mockStepObject, client.Object](inner)
		require.NotNil(t, wrapped)

		// OnDiff forwards to the inner action.
		diff := NewMultiPhaseDiff[client.Object]()
		res, err := wrapped.OnDiff(context.Background(), &mockMultiPhaseObject{name: "parent", namespace: "test-ns"}, map[string]any{}, diff, logrus.NewEntry(logrus.New()))
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
		assert.True(t, inner.onDiffCalled)
	})
}

func TestObjectMultiPhaseStepReconcilerAction_SimpleDoesNotImplementOnDiff(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	t.Run("As[D]() from simple action", func(t *testing.T) {
		inner := NewMultiPhaseStepReconcilerAction[*mockMultiPhaseObject, *mockStepObject](
			fakeClient,
			"test-phase",
			"Ready",
			recorder,
			"test-controller",
		).(*DefaultMultiPhaseStepReconcilerAction[*mockMultiPhaseObject, *mockStepObject])

		wrapped := inner.As[client.Object]()

		_, ok := any(wrapped).(MultiPhaseStepReconcilerActionWithDiff[*mockMultiPhaseObject, client.Object])
		assert.False(t, ok)
	})

	t.Run("Deprecated constructor from simple action", func(t *testing.T) {
		inner := NewMultiPhaseStepReconcilerAction[*mockMultiPhaseObject, *mockStepObject](
			fakeClient,
			"test-phase",
			"Ready",
			recorder,
			"test-controller",
		)

		wrapped := NewObjectMultiPhaseStepReconcilerAction[*mockMultiPhaseObject, *mockStepObject, client.Object](inner)

		_, ok := any(wrapped).(MultiPhaseStepReconcilerActionWithDiff[*mockMultiPhaseObject, client.Object])
		assert.False(t, ok)
	})
}
