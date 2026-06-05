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

// Mock object for testing
type mockClientObject struct {
	client.Object
	name string
	namespace string
	group string
	version string
	kind string
}

func (m *mockClientObject) GetName() string {
	return m.name
}

func (m *mockClientObject) GetNamespace() string {
	return m.namespace
}

func (m *mockClientObject) GetObjectKind() schema.ObjectKind {
	return schema.ObjectKind(&metav1.TypeMeta{
		Kind:       m.kind,
		APIVersion: m.group + "/" + m.version,
	})
}

func (m *mockClientObject) DeepCopyObject() runtime.Object {
	return m
}

func TestNewSentinelRead(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("new sentinel read creates empty reads map", func(t *testing.T) {
		read := NewSentinelRead(scheme)
		assert.NotNil(t, read)
		
		reads := read.GetReads()
		assert.NotNil(t, reads)
		assert.Empty(t, reads)
	})
}

func TestDefaultSentinelRead_GetReads(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("get reads returns the internal map", func(t *testing.T) {
		read := NewSentinelRead(scheme)
		reads := read.GetReads()
		assert.NotNil(t, reads)
		assert.Empty(t, reads)
	})
}

func TestDefaultSentinelRead_SetCurrentObjects(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("set current objects adds objects to the right list", func(t *testing.T) {
		read := NewSentinelRead(scheme)
		
		objects := []client.Object{
			&mockClientObject{
				name: "obj1",
				namespace: "test-ns",
				group: "",
				version: "v1",
				kind: "Pod",
			},
			&mockClientObject{
				name: "obj2",
				namespace: "test-ns",
				group: "",
				version: "v1",
				kind: "Service",
			},
		}
		
		read.SetCurrentObjects(objects)
		
		reads := read.GetReads()
		// Should have 2 different object types
		assert.Len(t, reads, 2)
		
		// Check that each type has its own list
		podType := "/v1/Pod"
		serviceType := "/v1/Service"
		
		assert.Contains(t, reads, podType)
		assert.Contains(t, reads, serviceType)
		
		// Check that each list has the correct object
		podRead := reads[podType]
		serviceRead := reads[serviceType]
		
		assert.Len(t, podRead.GetCurrentObjects(), 1)
		assert.Equal(t, "obj1", podRead.GetCurrentObjects()[0].GetName())
		
		assert.Len(t, serviceRead.GetCurrentObjects(), 1)
		assert.Equal(t, "obj2", serviceRead.GetCurrentObjects()[0].GetName())
	})
	
t.Run("set current objects with nil objects skips typed nil", func(t *testing.T) {
		read := NewSentinelRead(scheme)

		var obj *mockClientObject = (*mockClientObject)(nil)
		objects := []client.Object{
			obj,
			&mockClientObject{
				name:      "obj1",
				namespace: "test-ns",
				kind:      "Pod",
			},
			obj,
		}

		read.SetCurrentObjects(objects)

		reads := read.GetReads()
		assert.Len(t, reads, 1)
	})
}

func TestDefaultSentinelRead_AddCurrentObject(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("add current object adds object to the right list", func(t *testing.T) {
		read := NewSentinelRead(scheme)
		
		obj := &mockClientObject{
			name: "test-obj",
			namespace: "test-ns",
			group: "",
			version: "v1",
			kind: "Pod",
		}
		
		read.AddCurrentObject(obj)
		
		reads := read.GetReads()
		// Should have 1 object type
		assert.Len(t, reads, 1)
		
		// Check that the object was added
		podType := "/v1/Pod"
		assert.Contains(t, reads, podType)
		assert.Len(t, reads[podType].GetCurrentObjects(), 1)
		assert.Equal(t, "test-obj", reads[podType].GetCurrentObjects()[0].GetName())
	})
	
t.Run("add current object with nil object skips it", func(t *testing.T) {
		read := NewSentinelRead(scheme)

		var obj *mockClientObject = (*mockClientObject)(nil)

		read.AddCurrentObject(obj)

		reads := read.GetReads()
		assert.Empty(t, reads)
	})

	t.Run("add current object with nil typed pointer skips it", func(t *testing.T) {
		read := NewSentinelRead(scheme)

		var obj *mockClientObject = (*mockClientObject)(nil)

		read.AddCurrentObject(obj)

		reads := read.GetReads()
		assert.Empty(t, reads)
	})
}

func TestDefaultSentinelRead_SetExpectedObjects(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("set expected objects adds objects to the right list", func(t *testing.T) {
		read := NewSentinelRead(scheme)
		
		objects := []client.Object{
			&mockClientObject{
				name: "exp1",
				namespace: "test-ns",
				group: "",
				version: "v1",
				kind: "Pod",
			},
			&mockClientObject{
				name: "exp2",
				namespace: "test-ns",
				group: "",
				version: "v1",
				kind: "Service",
			},
		}
		
		read.SetExpectedObjects(objects)
		
		reads := read.GetReads()
		// Should have 2 different object types
		assert.Len(t, reads, 2)
		
		// Check that each type has its own list
		podType := "/v1/Pod"
		serviceType := "/v1/Service"
		
		assert.Contains(t, reads, podType)
		assert.Contains(t, reads, serviceType)
		
		// Check that each list has the correct object
		podRead := reads[podType]
		serviceRead := reads[serviceType]
		
		assert.Len(t, podRead.GetExpectedObjects(), 1)
		assert.Equal(t, "exp1", podRead.GetExpectedObjects()[0].GetName())
		
		assert.Len(t, serviceRead.GetExpectedObjects(), 1)
		assert.Equal(t, "exp2", serviceRead.GetExpectedObjects()[0].GetName())
	})
	
t.Run("set expected objects with nil objects skips them", func(t *testing.T) {
		read := NewSentinelRead(scheme)

		var nilObj *mockClientObject = (*mockClientObject)(nil)
		objects := []client.Object{
			nilObj,
			&mockClientObject{
				name: "exp1",
				namespace: "test-ns",
				group: "",
				version: "v1",
				kind: "Pod",
			},
			nilObj,
		}

		read.SetExpectedObjects(objects)

		reads := read.GetReads()
		assert.Len(t, reads, 1)

		podType := "/v1/Pod"
		assert.Contains(t, reads, podType)
		assert.Len(t, reads[podType].GetExpectedObjects(), 1)
	})
}

func TestDefaultSentinelRead_AddExpectedObject(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	t.Run("add expected object adds object to the right list", func(t *testing.T) {
		read := NewSentinelRead(scheme)
		
		obj := &mockClientObject{
			name: "test-exp",
			namespace: "test-ns",
			group: "",
			version: "v1",
			kind: "Pod",
		}
		
		read.AddExpectedObject(obj)
		
		reads := read.GetReads()
		// Should have 1 object type
		assert.Len(t, reads, 1)
		
		// Check that the object was added
		podType := "/v1/Pod"
		assert.Contains(t, reads, podType)
		assert.Len(t, reads[podType].GetExpectedObjects(), 1)
		assert.Equal(t, "test-exp", reads[podType].GetExpectedObjects()[0].GetName())
	})
	
t.Run("add expected object with nil object skips it", func(t *testing.T) {
		read := NewSentinelRead(scheme)

		var obj *mockClientObject = (*mockClientObject)(nil)

		read.AddExpectedObject(obj)

		reads := read.GetReads()
		assert.Empty(t, reads)
	})

	t.Run("add expected object with nil typed pointer skips it", func(t *testing.T) {
		read := NewSentinelRead(scheme)

		var obj *mockClientObject = (*mockClientObject)(nil)

		read.AddExpectedObject(obj)

		reads := read.GetReads()
		assert.Empty(t, reads)
	})
}