package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestNewCR(t *testing.T) {
	t.Run("create builder with name and namespace", func(t *testing.T) {
		b := NewCR[*corev1.ConfigMap]("test", "default")
		assert.NotNil(t, b)
		obj := b.Build()
		assert.Equal(t, "test", obj.GetName())
		assert.Equal(t, "default", obj.GetNamespace())
	})

	t.Run("with labels", func(t *testing.T) {
		b := NewCR[*corev1.ConfigMap]("test", "default")
		obj := b.WithLabels(map[string]string{"app": "test"}).Build()
		assert.Equal(t, "test", obj.GetLabels()["app"])
	})

	t.Run("with annotations", func(t *testing.T) {
		b := NewCR[*corev1.ConfigMap]("test", "default")
		obj := b.WithAnnotations(map[string]string{"key": "value"}).Build()
		assert.Equal(t, "value", obj.GetAnnotations()["key"])
	})

	t.Run("with type meta", func(t *testing.T) {
		b := NewCR[*corev1.ConfigMap]("test", "default")
		gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
		obj := b.WithTypeMeta(gvk).Build()
		assert.Equal(t, gvk, obj.GetObjectKind().GroupVersionKind())
	})

	t.Run("builder is chainable", func(t *testing.T) {
		b := NewCR[*corev1.ConfigMap]("test", "default")
		obj := b.
			WithLabels(map[string]string{"app": "test"}).
			WithAnnotations(map[string]string{"key": "value"}).
			Build()
		assert.Equal(t, "test", obj.GetName())
		assert.Equal(t, "default", obj.GetNamespace())
		assert.Equal(t, "test", obj.GetLabels()["app"])
		assert.Equal(t, "value", obj.GetAnnotations()["key"])
	})
}

func TestReflectNewCR(t *testing.T) {
	obj := reflectNewObject[*corev1.ConfigMap]()
	assert.NotNil(t, obj)
	assert.IsType(t, &corev1.ConfigMap{}, obj)

	// Verify zero values
	var zero *corev1.ConfigMap
	assert.NotEqual(t, zero, obj)
}

func TestNewCRWithType(t *testing.T) {
	t.Run("works with ConfigMap", func(t *testing.T) {
		cm := NewCR[*corev1.ConfigMap]("test-cm", "default").Build()
		assert.Equal(t, "test-cm", cm.Name)
		assert.Equal(t, "default", cm.Namespace)
	})

	t.Run("works with different namespace", func(t *testing.T) {
		cm := NewCR[*corev1.ConfigMap]("test", "kube-system").Build()
		assert.Equal(t, "kube-system", cm.Namespace)
	})
}