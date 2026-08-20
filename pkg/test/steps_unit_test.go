package test

import (
	"context"
	"errors"
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

func TestNewDeleteStep(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("delete existing object", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
		key := types.NamespacedName{Namespace: "default", Name: "test"}

		step := NewDeleteStep[*corev1.ConfigMap]()
		assert.Equal(t, "delete", step.Name)

		err := step.Do(c, key, cm, nil)
		assert.NoError(t, err)
	})

	t.Run("returns error when object is nil", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		key := types.NamespacedName{Namespace: "default", Name: "test"}

		step := NewDeleteStep[*corev1.ConfigMap]()
		err := step.Do(c, key, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "object is null")
	})
}

func TestNewCreateStep(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("create object from builder", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		key := types.NamespacedName{Namespace: "default", Name: "test"}

		step := NewCreateStep[*corev1.ConfigMap](func(c client.Client, key types.NamespacedName) (*corev1.ConfigMap, error) {
			return &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
			}, nil
		})
		assert.Equal(t, "create", step.Name)

		err := step.Do(c, key, nil, nil)
		assert.NoError(t, err)

		// Verify the object was created in the fake client
		cm := &corev1.ConfigMap{}
		err = c.Get(context.Background(), key, cm)
		assert.NoError(t, err)
		assert.Equal(t, "test", cm.Name)
	})

	t.Run("returns error from builder", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		key := types.NamespacedName{Namespace: "default", Name: "test"}

		step := NewCreateStep[*corev1.ConfigMap](func(c client.Client, key types.NamespacedName) (*corev1.ConfigMap, error) {
			return nil, errors.New("builder error")
		})

		err := step.Do(c, key, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "builder error")
	})
}

func TestEventuallyTimeoutError(t *testing.T) {
	isTimeout, err := RunWithTimeout(func() error {
		return assert.AnError
	}, 50*time.Millisecond, 10*time.Millisecond)
	assert.True(t, isTimeout)
	assert.Error(t, err)
}

func TestEventuallySuccess(t *testing.T) {
	counter := 0
	isTimeout, err := RunWithTimeout(func() error {
		counter++
		if counter >= 3 {
			return nil
		}
		return assert.AnError
	}, 1*time.Second, 10*time.Millisecond)
	assert.False(t, isTimeout)
	assert.NoError(t, err)
}
