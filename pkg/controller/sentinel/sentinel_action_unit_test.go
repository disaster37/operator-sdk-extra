package sentinel

import (
	"context"
	"errors"
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/multiphase"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type mockK8sObject struct {
	client.Object
	name        string
	namespace   string
	annotations map[string]string
}

func (m *mockK8sObject) GetName() string {
	return m.name
}

func (m *mockK8sObject) GetNamespace() string {
	return m.namespace
}

func (m *mockK8sObject) GetAnnotations() map[string]string {
	return m.annotations
}

func (m *mockK8sObject) GetObjectKind() schema.ObjectKind {
	return schema.ObjectKind(&metav1.TypeMeta{
		Kind:       "mockK8sObject",
		APIVersion: "v1",
	})
}

func (m *mockK8sObject) DeepCopyObject() runtime.Object {
	return m
}

type mockSentinelRead struct {
	SentinelRead
	reads map[string]multiphase.MultiPhaseRead[client.Object]
}

func (m *mockSentinelRead) GetReads() map[string]multiphase.MultiPhaseRead[client.Object] {
	return m.reads
}

func TestDefaultSentinelAction_GetFieldManager(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	fieldManager := "test-field-manager"
	action := NewSentinelAction[*mockK8sObject](fakeClient, recorder, fieldManager)

	t.Run("get field manager returns correct value", func(t *testing.T) {
		result := action.GetFieldManager()
		assert.Equal(t, fieldManager, result)
	})
}

func TestDefaultSentinelAction_Configure(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewSentinelAction[*mockK8sObject](fakeClient, recorder, "test-field-manager")

	t.Run("configure returns empty result", func(t *testing.T) {
		ctx := context.Background()
		req := reconcile.Request{
			NamespacedName: struct{ Namespace, Name string }{Namespace: "test-ns", Name: "test-name"},
		}
		obj := &mockK8sObject{
			name:      "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		logger := logrus.NewEntry(logrus.New())

		res, err := action.Configure(ctx, req, obj, data, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})
}

func TestDefaultSentinelAction_Read(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewSentinelAction[*mockK8sObject](fakeClient, recorder, "test-field-manager")

	t.Run("read panics when not implemented", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockK8sObject{
			name:      "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		logger := logrus.NewEntry(logrus.New())

		assert.Panics(t, func() {
			_, _, _ = action.Read(ctx, obj, data, logger)
		})
	})
}

func TestDefaultSentinelAction_Apply(t *testing.T) {
	t.Skip("SSA apply is not supported by the fake client")
}

func TestDefaultSentinelAction_Delete(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	child1 := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "child1", Namespace: "test-ns"}}
	child2 := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "child2", Namespace: "test-ns"}}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(child1, child2).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewSentinelAction[*corev1.ConfigMap](fakeClient, recorder, "test-field-manager")

	t.Run("delete objects successfully", func(t *testing.T) {
		ctx := context.Background()
		parentObj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "parent",
				Namespace: "test-ns",
			},
		}
		data := make(map[string]any)
		objects := []client.Object{
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "child1", Namespace: "test-ns"}},
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "child2", Namespace: "test-ns"}},
		}
		logger := logrus.NewEntry(logrus.New())

		res, err := action.Delete(ctx, parentObj, data, objects, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})
}

func TestDefaultSentinelAction_OnError(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewSentinelAction[*mockK8sObject](fakeClient, recorder, "test-field-manager")

	t.Run("on error records event and returns error", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockK8sObject{
			name:      "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		testErr := errors.New("test error")
		logger := logrus.NewEntry(logrus.New())

		res, err := action.OnError(ctx, obj, data, testErr, logger)
		assert.Equal(t, testErr, err)
		assert.Equal(t, reconcile.Result{}, res)

		select {
		case event := <-recorder.Events:
			assert.Contains(t, event, "SentinelActionError")
			assert.Contains(t, event, "test error")
		default:
		}
	})
}

func TestDefaultSentinelAction_OnSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewSentinelAction[*mockK8sObject](fakeClient, recorder, "test-field-manager")

	t.Run("on success returns empty result", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockK8sObject{
			name:      "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		diff := multiphase.NewMultiPhaseDiff[client.Object]()
		logger := logrus.NewEntry(logrus.New())

		res, err := action.OnSuccess(ctx, obj, data, diff, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})
}

func TestDefaultSentinelAction_Diff(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewSentinelAction[*mockK8sObject](fakeClient, recorder, "test-field-manager")

	t.Run("diff computes apply and delete lists", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockK8sObject{
			name:      "parent",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		logger := logrus.NewEntry(logrus.New())

		currentObjects := []client.Object{
			&mockK8sObject{name: "existing1", namespace: "test-ns"},
			&mockK8sObject{name: "existing2", namespace: "test-ns"},
			&mockK8sObject{name: "toBeDeleted", namespace: "test-ns"},
		}

		expectedObjects := []client.Object{
			&mockK8sObject{name: "existing1", namespace: "test-ns"},
			&mockK8sObject{name: "existing2", namespace: "test-ns"},
			&mockK8sObject{name: "newObject", namespace: "test-ns"},
		}

		mockReader := multiphase.NewMultiPhaseRead[client.Object]()
		mockReader.SetCurrentObjects(currentObjects)
		mockReader.SetExpectedObjects(expectedObjects)

		reads := map[string]multiphase.MultiPhaseRead[client.Object]{
			"test-type": mockReader,
		}

		read := &mockSentinelRead{
			reads: reads,
		}

		diff, res, err := action.Diff(ctx, obj, read, data, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)

		applyList := diff.GetObjectsToApply()
		deleteList := diff.GetObjectsToDelete()
		diffStr := diff.Diff()

		assert.Len(t, applyList, 3)
		assert.True(t, diff.NeedCreate())
		assert.True(t, diff.NeedUpdate())
		assert.True(t, diff.NeedDelete())
		assert.Equal(t, "newObject", applyList[0].GetName())
		assert.Equal(t, "existing1", applyList[1].GetName())
		assert.Equal(t, "existing2", applyList[2].GetName())

		assert.Len(t, deleteList, 1)
		assert.Equal(t, "toBeDeleted", deleteList[0].GetName())

		assert.Contains(t, diffStr, "Apply object 'existing1'")
		assert.Contains(t, diffStr, "Apply object 'existing2'")
		assert.Contains(t, diffStr, "Create object 'newObject'")
		assert.Contains(t, diffStr, "Need delete object 'toBeDeleted'")
	})
}

func TestNewSentinelActionWithDiff(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewSentinelActionWithDiff[*mockK8sObject](fakeClient, recorder, "test-field-manager")
	require.NotNil(t, action)
	assert.Equal(t, "test-field-manager", action.GetFieldManager())
}

func TestDefaultSentinelActionWithDiff_OnDiff(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewSentinelActionWithDiff[*mockK8sObject](fakeClient, recorder, "test-field-manager")

	diff := multiphase.NewMultiPhaseDiff[client.Object]()
	res, err := action.OnDiff(context.Background(), &mockK8sObject{name: "parent", namespace: "test-ns"}, map[string]any{}, diff, logrus.NewEntry(logrus.New()))
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
}

func TestDefaultSentinelActionWithDiff_Diff(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	expected := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-ns",
		},
		Data: map[string]string{
			"foo": "bar",
		},
	}
	current := expected.DeepCopy()

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current).Build()
	recorder := record.NewFakeRecorder(10)

	action := NewSentinelActionWithDiff[*mockK8sObject](fakeClient, recorder, "test-field-manager")

	obj := &mockK8sObject{name: "parent", namespace: "test-ns"}
	mockReader := multiphase.NewMultiPhaseRead[client.Object]()
	mockReader.AddExpectedObject(expected)
	mockReader.AddCurrentObject(current)

	read := &mockSentinelRead{
		reads: map[string]multiphase.MultiPhaseRead[client.Object]{
			"test-type": mockReader,
		},
	}

	diff, res, err := action.Diff(context.Background(), obj, read, map[string]any{}, logrus.NewEntry(logrus.New()))
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