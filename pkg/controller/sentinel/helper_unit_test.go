package sentinel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type mockObject struct {
	client.Object
	name      string
	namespace string
}

func (m *mockObject) GetName() string {
	return m.name
}

func (m *mockObject) GetNamespace() string {
	return m.namespace
}

func (m *mockObject) GetObjectKind() schema.ObjectKind {
	return &metav1.TypeMeta{
		Kind:       "MockObject",
		APIVersion: "v1",
	}
}

func (m *mockObject) DeepCopyObject() runtime.Object {
	return m
}

func TestGetObjectWithMeta(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("panics when object is nil", func(t *testing.T) {
		var obj *mockObject = nil

		assert.Panics(t, func() {
			GetObjectWithMeta[*mockObject](obj, scheme)
		})
	})

	t.Run("panics when object is nil typed pointer", func(t *testing.T) {
		obj := (*mockObject)(nil)

		assert.Panics(t, func() {
			GetObjectWithMeta[*mockObject](obj, scheme)
		})
	})
}

func TestGetObjectType(t *testing.T) {
	t.Run("panics when object is nil", func(t *testing.T) {
		var obj schema.ObjectKind = nil

		assert.Panics(t, func() {
			GetObjectType(obj)
		})
	})

	t.Run("returns correct object type string", func(t *testing.T) {
		obj := &mockObject{
			name:      "test",
			namespace: "test-ns",
		}

		result := GetObjectType(obj.GetObjectKind())
		assert.Equal(t, "/v1/MockObject", result)
	})
}

func TestCloneObject(t *testing.T) {
	t.Run("panics when object is not a pointer", func(t *testing.T) {
		var obj mockObject

		assert.Panics(t, func() {
			CloneObject[mockObject](obj)
		})
	})

	t.Run("panics when object is nil", func(t *testing.T) {
		var obj *mockObject = nil

		assert.Panics(t, func() {
			CloneObject[*mockObject](obj)
		})
	})

	t.Run("panics when object is nil typed pointer", func(t *testing.T) {
		obj := (*mockObject)(nil)

		assert.Panics(t, func() {
			CloneObject[*mockObject](obj)
		})
	})

	t.Run("clones object successfully", func(t *testing.T) {
		obj := &mockObject{
			name:      "test",
			namespace: "test-ns",
		}

		clone := CloneObject[*mockObject](obj)
		assert.NotNil(t, clone)
		assert.NotSame(t, obj, clone)
		assert.IsType(t, obj, clone)
	})
}
