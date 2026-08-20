package test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Mock object for testing
type mockObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (m *mockObject) GetName() string {
	return m.Name
}

func (m *mockObject) GetNamespace() string {
	return m.Namespace
}

func (m *mockObject) GetObjectKind() schema.ObjectKind {
	return &m.TypeMeta
}

func (m *mockObject) DeepCopyObject() runtime.Object {
	return &mockObject{
		TypeMeta:   m.TypeMeta,
		ObjectMeta: m.ObjectMeta,
	}
}

var mockObjectGVK = schema.GroupVersionKind{
	Group:   "",
	Version: "v1",
	Kind:    "MockObject",
}

var mockObjectListGVK = schema.GroupVersionKind{
	Group:   "",
	Version: "v1",
	Kind:    "MockObjectList",
}

// Mock object list for testing
type mockObjectList struct {
	client.ObjectList
	metav1.TypeMeta
	Items []mockObject
}

func (m *mockObjectList) GetObjectKind() schema.ObjectKind {
	return &m.TypeMeta
}

func (m *mockObjectList) DeepCopyObject() runtime.Object {
	return m
}

func TestNewInterceptorClient(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("new interceptor client wraps the underlying client", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		assert.NotNil(t, interceptor)
		assert.Equal(t, fakeClient, interceptor.Client)
	})
}

func TestInterceptorClient_Get(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	scheme.AddKnownTypeWithName(mockObjectGVK, &mockObject{})

	t.Run("get delegates to underlying client when no interceptor", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		ctx := context.Background()
		key := types.NamespacedName{Namespace: "test-ns", Name: "test-name"}
		obj := &mockObject{}

		err := interceptor.Get(ctx, key, obj)
		// Should return not found error from the fake client
		assert.Error(t, err)
	})

	t.Run("get calls interceptor when provided", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		// Set up interceptor
		intercepted := false
		testErr := errors.New("intercepted error")
		interceptor.GetInterceptor = func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			intercepted = true
			return testErr
		}

		ctx := context.Background()
		key := types.NamespacedName{Namespace: "test-ns", Name: "test-name"}
		obj := &mockObject{}

		err := interceptor.Get(ctx, key, obj)
		assert.True(t, intercepted)
		assert.Equal(t, testErr, err)
	})
}

func TestInterceptorClient_List(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	scheme.AddKnownTypeWithName(mockObjectListGVK, &mockObjectList{})

	t.Run("list delegates to underlying client when no interceptor", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		ctx := context.Background()
		list := &mockObjectList{}

		err := interceptor.List(ctx, list)
		// Should succeed with empty list
		assert.NoError(t, err)
	})

	t.Run("list calls interceptor when provided", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		// Set up interceptor
		intercepted := false
		testErr := errors.New("intercepted error")
		interceptor.ListInterceptor = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			intercepted = true
			return testErr
		}

		ctx := context.Background()
		list := &mockObjectList{}

		err := interceptor.List(ctx, list)
		assert.True(t, intercepted)
		assert.Equal(t, testErr, err)
	})
}

func TestInterceptorClient_Create(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	scheme.AddKnownTypeWithName(mockObjectGVK, &mockObject{})

	t.Run("create delegates to underlying client when no interceptor", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		ctx := context.Background()
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}

		err := interceptor.Create(ctx, obj)
		// Should succeed
		assert.NoError(t, err)
	})

	t.Run("create calls interceptor when provided", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		// Set up interceptor
		intercepted := false
		testErr := errors.New("intercepted error")
		interceptor.CreateInterceptor = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			intercepted = true
			return testErr
		}

		ctx := context.Background()
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}

		err := interceptor.Create(ctx, obj)
		assert.True(t, intercepted)
		assert.Equal(t, testErr, err)
	})
}

func TestInterceptorClient_Delete(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	scheme.AddKnownTypeWithName(mockObjectGVK, &mockObject{})

	t.Run("delete delegates to underlying client when no interceptor", func(t *testing.T) {
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
		interceptor := NewInterceptorClient(fakeClient)

		ctx := context.Background()

		err := interceptor.Delete(ctx, obj)
		// Should succeed since object exists
		assert.NoError(t, err)
	})

	t.Run("delete calls interceptor when provided", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		// Set up interceptor
		intercepted := false
		testErr := errors.New("intercepted error")
		interceptor.DeleteInterceptor = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			intercepted = true
			return testErr
		}

		ctx := context.Background()
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}

		err := interceptor.Delete(ctx, obj)
		assert.True(t, intercepted)
		assert.Equal(t, testErr, err)
	})
}

func TestInterceptorClient_Update(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	scheme.AddKnownTypeWithName(mockObjectGVK, &mockObject{})

	t.Run("update delegates to underlying client when no interceptor", func(t *testing.T) {
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
		interceptor := NewInterceptorClient(fakeClient)

		ctx := context.Background()

		err := interceptor.Update(ctx, obj)
		// Should succeed since object exists
		assert.NoError(t, err)
	})

	t.Run("update calls interceptor when provided", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		// Set up interceptor
		intercepted := false
		testErr := errors.New("intercepted error")
		interceptor.UpdateInterceptor = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			intercepted = true
			return testErr
		}

		ctx := context.Background()
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}

		err := interceptor.Update(ctx, obj)
		assert.True(t, intercepted)
		assert.Equal(t, testErr, err)
	})
}

func TestInterceptorClient_Patch(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	scheme.AddKnownTypeWithName(mockObjectGVK, &mockObject{})

	t.Run("patch delegates to underlying client when no interceptor", func(t *testing.T) {
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
		interceptor := NewInterceptorClient(fakeClient)

		ctx := context.Background()
		patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))

		err := interceptor.Patch(ctx, obj, patch)
		// Should succeed since object exists
		assert.NoError(t, err)
	})

	t.Run("patch calls interceptor when provided", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		// Set up interceptor
		intercepted := false
		testErr := errors.New("intercepted error")
		interceptor.PatchInterceptor = func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			intercepted = true
			return testErr
		}

		ctx := context.Background()
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}
		patch := client.MergeFrom(obj)

		err := interceptor.Patch(ctx, obj, patch)
		assert.True(t, intercepted)
		assert.Equal(t, testErr, err)
	})
}

func TestInterceptorClient_DeleteAllOf(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	scheme.AddKnownTypeWithName(mockObjectGVK, &mockObject{})
	scheme.AddKnownTypeWithName(mockObjectListGVK, &mockObjectList{})

	t.Run("deleteallof delegates to underlying client when no interceptor", func(t *testing.T) {
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
		interceptor := NewInterceptorClient(fakeClient)

		ctx := context.Background()

		err := interceptor.DeleteAllOf(ctx, &mockObject{}, client.InNamespace("test-ns"))
		// Should succeed
		assert.NoError(t, err)
	})

	t.Run("deleteallof calls interceptor when provided", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		// Set up interceptor
		intercepted := false
		testErr := errors.New("intercepted error")
		interceptor.DeleteAllOfInterceptor = func(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
			intercepted = true
			return testErr
		}

		ctx := context.Background()
		obj := &mockObject{}

		err := interceptor.DeleteAllOf(ctx, obj)
		assert.True(t, intercepted)
		assert.Equal(t, testErr, err)
	})
}

func TestInterceptorClient_Status(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("status returns interceptor status client", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		statusClient := interceptor.Status()
		assert.NotNil(t, statusClient)
		// Should be our interceptor status client
		_, ok := statusClient.(*InterceptorStatusClient)
		assert.True(t, ok)
	})
}

func TestInterceptorClient_Scheme(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("scheme returns underlying client scheme", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		result := interceptor.Scheme()
		assert.Equal(t, scheme, result)
	})
}

func TestInterceptorClient_RESTMapper(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("restmapper returns underlying client restmapper", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		result := interceptor.RESTMapper()
		assert.NotNil(t, result)
	})
}

func TestInterceptorClient_GroupVersionKindFor(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("groupversionkindfor returns underlying client result", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		obj := &mockObject{}
		gvk, err := interceptor.GroupVersionKindFor(obj)
		// Should return error since our mock isn't registered
		assert.Error(t, err)
		assert.Equal(t, schema.GroupVersionKind{}, gvk)
	})
}

func TestInterceptorClient_IsObjectNamespaced(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("isobjectnamespaced returns underlying client result", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		obj := &mockObject{}
		namespaced, err := interceptor.IsObjectNamespaced(obj)
		// Should return error since our mock isn't registered
		assert.Error(t, err)
		assert.False(t, namespaced)
	})
}

func TestInterceptorClient_SubResource(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("subresource returns underlying client result", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		interceptor := NewInterceptorClient(fakeClient)

		subResourceClient := interceptor.SubResource("status")
		assert.NotNil(t, subResourceClient)
	})
}
