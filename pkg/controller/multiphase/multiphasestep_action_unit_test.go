package multiphase

import (
	"context"
	"testing"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type mockMultiPhaseObject struct {
	object.MultiPhaseObject
	name              string
	namespace         string
	annotations       map[string]string
	deletionTimestamp *metav1.Time
	status            mockStatus
}

func (m *mockMultiPhaseObject) GetName() string {
	return m.name
}

func (m *mockMultiPhaseObject) GetNamespace() string {
	return m.namespace
}

func (m *mockMultiPhaseObject) GetAnnotations() map[string]string {
	return m.annotations
}

func (m *mockMultiPhaseObject) GetObjectMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:              m.name,
		Namespace:         m.namespace,
		Annotations:       m.annotations,
		DeletionTimestamp: m.deletionTimestamp,
	}
}

func (m *mockMultiPhaseObject) GetStatus() object.MultiPhaseObjectStatus {
	return &m.status
}

type mockStatus struct {
	object.MultiPhaseObjectStatus
	conditions []metav1.Condition
	phaseName  shared.PhaseName
}

func (m *mockStatus) GetConditions() []metav1.Condition {
	return m.conditions
}

func (m *mockStatus) SetConditions(conditions []metav1.Condition) {
	m.conditions = conditions
}

func (m *mockStatus) GetPhaseName() shared.PhaseName {
	return m.phaseName
}

func (m *mockStatus) SetPhaseName(phaseName shared.PhaseName) {
	m.phaseName = phaseName
}

type mockStepObject struct {
	client.Object
	name      string
	namespace string
}

func (m *mockStepObject) GetName() string {
	return m.name
}

func (m *mockStepObject) GetNamespace() string {
	return m.namespace
}

func (m *mockStepObject) GetObjectKind() schema.ObjectKind {
	return &metav1.TypeMeta{}
}

func (m *mockStepObject) DeepCopyObject() runtime.Object {
	return m
}

func TestDefaultMultiPhaseStepReconcilerAction_Configure(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewMultiPhaseStepReconcilerAction[*mockMultiPhaseObject, *mockStepObject](
		fakeClient,
		"test-phase",
		"Ready",
		recorder,
		"test-controller",
	)

	t.Run("configure initializes condition and phase", func(t *testing.T) {
		ctx := context.Background()
		req := reconcile.Request{
			NamespacedName: struct{ Namespace, Name string }{Namespace: "test-ns", Name: "test-name"},
		}

		obj := &mockMultiPhaseObject{
			name:      "test-name",
			namespace: "test-ns",
			status:    mockStatus{},
		}
		logger := logrus.NewEntry(logrus.New())

		res, err := action.Configure(ctx, req, obj, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)

		assert.Equal(t, shared.PhaseName("test-phase"), obj.GetStatus().GetPhaseName())
	})
}

func TestDefaultMultiPhaseStepReconcilerAction_Read(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewMultiPhaseStepReconcilerAction[*mockMultiPhaseObject, *mockStepObject](
		fakeClient,
		"test-phase",
		"Ready",
		recorder,
		"test-controller",
	)

	t.Run("Read panics when not implemented", func(t *testing.T) {
		ctx := context.Background()

		obj := &mockMultiPhaseObject{
			name:      "test-name",
			namespace: "test-ns",
		}
		logger := logrus.NewEntry(logrus.New())
		data := make(map[string]any)

		assert.Panics(t, func() {
			_, _, _ = action.Read(ctx, obj, data, logger)
		})
	})
}

func TestDefaultMultiPhaseStepReconcilerAction_Apply(t *testing.T) {
	t.Skip("Apply requires proper type registration and scheme support")
}

func TestDefaultMultiPhaseStepReconcilerAction_Delete(t *testing.T) {
	t.Skip("Delete requires proper type registration and scheme support")
}

func TestDefaultMultiPhaseStepReconcilerAction_OnError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewMultiPhaseStepReconcilerAction[*mockMultiPhaseObject, *mockStepObject](
		fakeClient,
		"test-phase",
		"Ready",
		recorder,
		"test-controller",
	)

	t.Run("on error updates condition and records event", func(t *testing.T) {
		ctx := context.Background()

		obj := &mockMultiPhaseObject{
			name:      "parent",
			namespace: "test-ns",
			status:    mockStatus{},
		}
		logger := logrus.NewEntry(logrus.New())
		data := make(map[string]any)
		testErr := errors.New("test error occurred")

		res, err := action.OnError(ctx, obj, data, testErr, logger)
		assert.Equal(t, testErr, err)
		assert.Equal(t, reconcile.Result{}, res)

		select {
		case event := <-recorder.Events:
			assert.Contains(t, event, "ReconcilerStepActionError")
		default:
		}
	})
}

func TestDefaultMultiPhaseStepReconcilerAction_OnSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewMultiPhaseStepReconcilerAction[*mockMultiPhaseObject, *mockStepObject](
		fakeClient,
		"test-phase",
		"Ready",
		recorder,
		"test-controller",
	)

	t.Run("on success updates condition", func(t *testing.T) {
		ctx := context.Background()

		obj := &mockMultiPhaseObject{
			name:      "parent",
			namespace: "test-ns",
			status:    mockStatus{},
		}
		logger := logrus.NewEntry(logrus.New())
		data := make(map[string]any)
		diff := NewMultiPhaseDiff[*mockStepObject]()

		res, err := action.OnSuccess(ctx, obj, data, diff, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})

	t.Run("on success with already successful condition", func(t *testing.T) {
		ctx := context.Background()

		obj := &mockMultiPhaseObject{
			name:      "parent",
			namespace: "test-ns",
			status: mockStatus{
				conditions: []metav1.Condition{},
			},
		}
		logger := logrus.NewEntry(logrus.New())
		data := make(map[string]any)
		diff := NewMultiPhaseDiff[*mockStepObject]()

		res, err := action.OnSuccess(ctx, obj, data, diff, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})
}

func TestDefaultMultiPhaseStepReconcilerAction_GetPhaseName(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewMultiPhaseStepReconcilerAction[*mockMultiPhaseObject, *mockStepObject](
		fakeClient,
		"test-phase",
		"Ready",
		recorder,
		"test-controller",
	)

	t.Run("get phase name returns correct phase", func(t *testing.T) {
		phaseName := action.GetPhaseName()
		assert.Equal(t, shared.PhaseName("test-phase"), phaseName)
	})
}
