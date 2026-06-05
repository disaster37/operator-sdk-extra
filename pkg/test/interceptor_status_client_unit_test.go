package test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestInterceptorStatusClient_Create(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	scheme.AddKnownTypeWithName(mockObjectGVK, &mockObject{})

	t.Run("create delegates to underlying client when no interceptor", func(t *testing.T) {
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
		statusClient := &InterceptorStatusClient{client: fakeClient}
		
		ctx := context.Background()
		subResource := &mockObject{}
		
		err := statusClient.Create(ctx, obj, subResource)
		// Status subresource create is not supported by fake client, expect error
		assert.Error(t, err)
	})
	
	t.Run("create calls interceptor when provided", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		statusClient := &InterceptorStatusClient{client: fakeClient}
		
		// Set up interceptor
		intercepted := false
		testErr := errors.New("intercepted error")
		statusClient.createInterceptor = func(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
			intercepted = true
			return testErr
		}
		
		ctx := context.Background()
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}
		subResource := &mockObject{}
		
		err := statusClient.Create(ctx, obj, subResource)
		assert.True(t, intercepted)
		assert.Equal(t, testErr, err)
	})
}

func TestInterceptorStatusClient_Update(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	scheme.AddKnownTypeWithName(mockObjectGVK, &mockObject{})

	t.Run("update delegates to underlying client when no interceptor", func(t *testing.T) {
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).WithStatusSubresource(obj).Build()
		statusClient := &InterceptorStatusClient{client: fakeClient}
		
		ctx := context.Background()
		
		err := statusClient.Update(ctx, obj)
		// Should succeed since object exists
		assert.NoError(t, err)
	})
	
	t.Run("update calls interceptor when provided", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		statusClient := &InterceptorStatusClient{client: fakeClient}
		
		// Set up interceptor
		intercepted := false
		testErr := errors.New("intercepted error")
		statusClient.updateInterceptor = func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			intercepted = true
			return testErr
		}
		
		ctx := context.Background()
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}
		
		err := statusClient.Update(ctx, obj)
		assert.True(t, intercepted)
		assert.Equal(t, testErr, err)
	})
}

func TestInterceptorStatusClient_Patch(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	scheme.AddKnownTypeWithName(mockObjectGVK, &mockObject{})

	t.Run("patch delegates to underlying client when no interceptor", func(t *testing.T) {
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).WithStatusSubresource(obj).Build()
		statusClient := &InterceptorStatusClient{client: fakeClient}
		
		ctx := context.Background()
		patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
		
		err := statusClient.Patch(ctx, obj, patch)
		// Should succeed since object exists
		assert.NoError(t, err)
	})
	
	t.Run("patch calls interceptor when provided", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		statusClient := &InterceptorStatusClient{client: fakeClient}
		
		// Set up interceptor
		intercepted := false
		testErr := errors.New("intercepted error")
		statusClient.patchInterceptor = func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			intercepted = true
			return testErr
		}
		
		ctx := context.Background()
		obj := &mockObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"}}
		patch := client.MergeFrom(obj)
		
		err := statusClient.Patch(ctx, obj, patch)
		assert.True(t, intercepted)
		assert.Equal(t, testErr, err)
	})
}