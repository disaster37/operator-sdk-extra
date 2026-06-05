package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewTestCase(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	key := types.NamespacedName{Namespace: "default", Name: "test"}

	tc := NewTestCase[*corev1.ConfigMap](t, c, key, 10*time.Millisecond, map[string]any{})
	assert.NotNil(t, tc)
	assert.Equal(t, key, tc.key)
	assert.Equal(t, c, tc.client)
	assert.Empty(t, tc.Steps)
	assert.Nil(t, tc.PreTest)
}

func TestTestCaseRun(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string]string{"key": "value"},
	}

	t.Run("successful run with steps", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
		key := types.NamespacedName{Namespace: "default", Name: "test"}
		tc := NewTestCase[*corev1.ConfigMap](t, c, key, 10*time.Millisecond, map[string]any{})

		stepExecuted := false
		tc.Steps = []TestStep[*corev1.ConfigMap]{
			{
				Name: "step1",
				Do: func(c client.Client, key types.NamespacedName, o *corev1.ConfigMap, data map[string]any) error {
					stepExecuted = true
					return nil
				},
				Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *corev1.ConfigMap, data map[string]any) error {
					assert.Equal(t, "value", o.Data["key"])
					return nil
				},
			},
		}

		tc.Run()
		assert.True(t, stepExecuted)
	})

	t.Run("preTest is called before steps", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
		key := types.NamespacedName{Namespace: "default", Name: "test"}
		tc := NewTestCase[*corev1.ConfigMap](t, c, key, 10*time.Millisecond, map[string]any{})

		preTestCalled := false
		tc.PreTest = func(stepName *string, data map[string]any) error {
			preTestCalled = true
			return nil
		}

		stepCalled := false
		tc.Steps = []TestStep[*corev1.ConfigMap]{
			{
				Name: "step1",
				Do: func(c client.Client, key types.NamespacedName, o *corev1.ConfigMap, data map[string]any) error {
					stepCalled = true
					return nil
				},
				Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *corev1.ConfigMap, data map[string]any) error {
					return nil
				},
			},
		}

		tc.Run()
		assert.True(t, preTestCalled)
		assert.True(t, stepCalled)
	})

	t.Run("pre is called before do", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
		key := types.NamespacedName{Namespace: "default", Name: "test"}
		tc := NewTestCase[*corev1.ConfigMap](t, c, key, 10*time.Millisecond, map[string]any{})

		preCalled := false
		tc.Steps = []TestStep[*corev1.ConfigMap]{
			{
				Name: "step1",
				Pre: func(c client.Client, data map[string]any) error {
					preCalled = true
					return nil
				},
				Do: func(c client.Client, key types.NamespacedName, o *corev1.ConfigMap, data map[string]any) error {
					assert.True(t, preCalled)
					return nil
				},
				Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *corev1.ConfigMap, data map[string]any) error {
					return nil
				},
			},
		}

		tc.Run()
	})
}

func TestGetNewObject(t *testing.T) {
	t.Run("getNewObject creates new instance of same type", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		}

		newObj := getNewObject(cm)
		assert.NotNil(t, newObj)
		assert.NotSame(t, cm, newObj)
		assert.IsType(t, cm, newObj)
		// New object should have zero values
		assert.Empty(t, newObj.Name)
	})

	t.Run("getNewObject creates new instance from typed nil pointer", func(t *testing.T) {
		var nilCm *corev1.ConfigMap
		newObj := getNewObject(nilCm)
		assert.NotNil(t, newObj)
		assert.IsType(t, &corev1.ConfigMap{}, newObj)
	})
}