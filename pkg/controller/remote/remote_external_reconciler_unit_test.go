package remote

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Mock for RemoteObject
type mockRemoteObject struct {
	object.RemoteObject
	name string
	namespace string
	generation int64
	annotations map[string]string
	deletionTimestamp *metav1.Time
	status mockRemoteStatus
	Status interface{}
}

func (m *mockRemoteObject) GetName() string {
	return m.name
}

func (m *mockRemoteObject) GetNamespace() string {
	return m.namespace
}

func (m *mockRemoteObject) GetObjectMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: m.name,
		Namespace: m.namespace,
		Generation: m.generation,
		Annotations: m.annotations,
		DeletionTimestamp: m.deletionTimestamp,
	}
}

func (m *mockRemoteObject) GetGeneration() int64 {
	return m.generation
}

func (m *mockRemoteObject) GetAnnotations() map[string]string {
	return m.annotations
}

func (m *mockRemoteObject) GetStatus() object.RemoteObjectStatus {
	return &m.status
}

// Mock for RemoteObjectStatus
type mockRemoteStatus struct {
	object.RemoteObjectStatus
	isOnError bool
	isSync bool
	lastErrorMessage string
	lastAppliedConfiguration string
	observedGeneration int64
	conditions []metav1.Condition
}

func (m *mockRemoteStatus) GetIsOnError() bool {
	return m.isOnError
}

func (m *mockRemoteStatus) SetIsOnError(isOnError bool) {
	m.isOnError = isOnError
}

func (m *mockRemoteStatus) GetIsSync() bool {
	return m.isSync
}

func (m *mockRemoteStatus) SetIsSync(isSync bool) {
	m.isSync = isSync
}

func (m *mockRemoteStatus) GetLastErrorMessage() string {
	return m.lastErrorMessage
}

func (m *mockRemoteStatus) SetLastErrorMessage(message string) {
	m.lastErrorMessage = message
}

func (m *mockRemoteStatus) GetLastAppliedConfiguration() string {
	return m.lastAppliedConfiguration
}

func (m *mockRemoteStatus) SetLastAppliedConfiguration(config string) {
	m.lastAppliedConfiguration = config
}

func (m *mockRemoteStatus) GetObservedGeneration() int64 {
	return m.observedGeneration
}

func (m *mockRemoteStatus) SetObservedGeneration(gen int64) {
	m.observedGeneration = gen
}

func (m *mockRemoteStatus) GetConditions() []metav1.Condition {
	return m.conditions
}

func (m *mockRemoteStatus) SetConditions(conditions []metav1.Condition) {
	m.conditions = conditions
}

// Mock API object
type mockAPIObject struct {
	ID   string
	Name string
	Value int
}

// Mock API client
type mockAPIClient struct {
	name string
}

func TestDefaultRemoteExternalReconciler_Diff(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)

	_ = fake.NewClientBuilder().WithScheme(scheme).Build()
	
	handler := mockAPIClient{name: "test-client"}
	remoteRec := NewRemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient](handler)

	t.Run("diff with current object is nil - returns patch result for creation", func(t *testing.T) {
		obj := mockAPIObject{
			ID:    "test-id",
			Name:  "test-name",
			Value: 42,
		}
		k8sObj := &mockRemoteObject{name: "test-k8s", namespace: "test-ns"}

		patchResult, err := remoteRec.Diff(mockAPIObject{}, obj, mockAPIObject{}, k8sObj)
		assert.NoError(t, err)
		assert.NotNil(t, patchResult)
		
		// Verify the patch result has the expected content
		assert.Equal(t, obj, patchResult.Patched)
		assert.NotNil(t, patchResult.Modified)
		assert.NotNil(t, patchResult.Current)
		assert.Nil(t, patchResult.Original)
	})

	t.Run("diff panics for unimplemented methods", func(t *testing.T) {
		obj := &mockRemoteObject{name: "test", namespace: "test-ns"}
		
		assert.Panics(t, func() {
			_, _ = remoteRec.Build(obj)
		})
		
		assert.Panics(t, func() {
			_, _ = remoteRec.Get(obj)
		})
		
		assert.Panics(t, func() {
			_ = remoteRec.Create(mockAPIObject{}, obj)
		})
		
		assert.Panics(t, func() {
			_ = remoteRec.Update(mockAPIObject{}, obj)
		})
		
		assert.Panics(t, func() {
			_ = remoteRec.Delete(obj)
		})
	})

	t.Run("client returns correct client", func(t *testing.T) {
		c := remoteRec.Client()
		assert.Equal(t, handler, c)
	})
}

func TestDefaultRemoteExternalReconciler_EdgeCases(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)

	handler := mockAPIClient{name: "test-client"}
	remoteRec := NewRemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient](handler)

t.Run("diff with nil pointer typed value", func(t *testing.T) {
		k8sObj := &mockRemoteObject{name: "test-k8s", namespace: "test-ns"}

		val := mockAPIObject{}
		zeroVal := mockAPIObject{}

		patchResult, err := remoteRec.Diff(zeroVal, val, zeroVal, k8sObj)
		assert.NoError(t, err)
		assert.NotNil(t, patchResult)
	})
}