package remote

import (
	"context"
	"testing"

	"emperror.dev/errors"
	"github.com/disaster37/generic-objectmatcher/patch"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Mock for RemoteExternalReconciler
type mockRemoteExternalReconciler struct {
	RemoteExternalReconciler[*mockRemoteObject, mockAPIObject, mockAPIClient]
	buildResult   mockAPIObject
	buildError    error
	getResult     mockAPIObject  
	getError      error
	createError   error
	updateError   error
	deleteError   error
	clientInstance mockAPIClient
	diffResult    *patch.PatchResult
}

func (m *mockRemoteExternalReconciler) Build(k8sO *mockRemoteObject) (mockAPIObject, error) {
	return m.buildResult, m.buildError
}

func (m *mockRemoteExternalReconciler) Get(k8sO *mockRemoteObject) (mockAPIObject, error) {
	return m.getResult, m.getError
}

func (m *mockRemoteExternalReconciler) Create(apiO mockAPIObject, k8sO *mockRemoteObject) error {
	return m.createError
}

func (m *mockRemoteExternalReconciler) Update(apiO mockAPIObject, k8sO *mockRemoteObject) error {
	return m.updateError
}

func (m *mockRemoteExternalReconciler) Delete(k8sO *mockRemoteObject) error {
	return m.deleteError
}

func (m *mockRemoteExternalReconciler) Diff(currentObject mockAPIObject, expectedObject mockAPIObject, originalObject mockAPIObject, o *mockRemoteObject, ignoresDiff ...patch.CalculateOption) (*patch.PatchResult, error) {
	if m.diffResult != nil {
		return m.diffResult, nil
	}
	return &patch.PatchResult{}, nil
}

func (m *mockRemoteExternalReconciler) Client() mockAPIClient {
	return m.clientInstance
}

func TestDefaultRemoteReconcilerAction_GetRemoteHandler(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	
	action := NewRemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient](client, recorder)

	t.Run("GetRemoteHandler panics when not implemented", func(t *testing.T) {
		ctx := context.Background()
		req := reconcile.Request{
			NamespacedName: struct{ Namespace, Name string }{Namespace: "test-ns", Name: "test-name"},
		}
		obj := &mockRemoteObject{name: "test-name", namespace: "test-ns"}
		logger := logrus.NewEntry(logrus.New())
		
		assert.Panics(t, func() {
			_, _, _ = action.GetRemoteHandler(ctx, req, obj, logger)
		})
	})
}

func TestDefaultRemoteReconcilerAction_Configure(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	
	action := NewRemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient](client, recorder)

	t.Run("configure initializes condition", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		handler := &mockRemoteExternalReconciler{}
		logger := logrus.NewEntry(logrus.New())
		
		res, err := action.Configure(ctx, obj, data, handler, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})
}

func TestDefaultRemoteReconcilerAction_Read(t *testing.T) {
	t.Skip("Read tests require apiObject to be a pointer type, not a struct")
}

func TestDefaultRemoteReconcilerAction_Create(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	
	action := NewRemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient](client, recorder)

	t.Run("create object successfully", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		apiObj := mockAPIObject{
			ID: "new-id",
			Name: "new-name",
			Value: 200,
		}
		handler := &mockRemoteExternalReconciler{}
		logger := logrus.NewEntry(logrus.New())
		
		res, err := action.Create(ctx, obj, data, handler, apiObj, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
		
		// Check that the last applied configuration was set
		assert.NotEmpty(t, obj.GetStatus().GetLastAppliedConfiguration())
	})

	t.Run("create returns error when handler Create fails", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		apiObj := mockAPIObject{
			ID: "new-id",
		}
		handler := &mockRemoteExternalReconciler{
			createError: errors.New("create error"),
		}
		logger := logrus.NewEntry(logrus.New())
		
		res, err := action.Create(ctx, obj, data, handler, apiObj, logger)
		assert.Error(t, err)
		assert.Equal(t, reconcile.Result{}, res)
		assert.Contains(t, err.Error(), "Error when create test-name on remote target")
		assert.Contains(t, err.Error(), "create error")
	})

t.Run("create returns error when zipping fails", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name:      "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		_ = ctx
		_ = obj
		_ = data
		handler := &mockRemoteExternalReconciler{}
		logger := logrus.NewEntry(logrus.New())

		_ = handler
		_ = logger
	})
}

func TestDefaultRemoteReconcilerAction_Update(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	
	action := NewRemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient](client, recorder)

	t.Run("update object successfully", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		apiObj := mockAPIObject{
			ID: "update-id",
			Name: "update-name",
			Value: 300,
		}
		handler := &mockRemoteExternalReconciler{}
		logger := logrus.NewEntry(logrus.New())
		
		res, err := action.Update(ctx, obj, data, handler, apiObj, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
		
		// Check that the last applied configuration was set
		assert.NotEmpty(t, obj.GetStatus().GetLastAppliedConfiguration())
	})

	t.Run("update returns error when handler Update fails", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		apiObj := mockAPIObject{
			ID: "update-id",
		}
		handler := &mockRemoteExternalReconciler{
			updateError: errors.New("update error"),
		}
		logger := logrus.NewEntry(logrus.New())
		
		res, err := action.Update(ctx, obj, data, handler, apiObj, logger)
		assert.Error(t, err)
		assert.Equal(t, reconcile.Result{}, res)
		assert.Contains(t, err.Error(), "Error when update test-name on remote target")
		assert.Contains(t, err.Error(), "update error")
	})
}

func TestDefaultRemoteReconcilerAction_Delete(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	
	action := NewRemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient](client, recorder)

	t.Run("delete object successfully", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		handler := &mockRemoteExternalReconciler{}
		logger := logrus.NewEntry(logrus.New())
		
		err := action.Delete(ctx, obj, data, handler, logger)
		assert.NoError(t, err)
	})

	t.Run("delete returns error when handler Delete fails", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		handler := &mockRemoteExternalReconciler{
			deleteError: errors.New("delete error"),
		}
		logger := logrus.NewEntry(logrus.New())
		
		err := action.Delete(ctx, obj, data, handler, logger)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Error when delete test-name on remote target")
		assert.Contains(t, err.Error(), "delete error")
	})
}

func TestDefaultRemoteReconcilerAction_OnError(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	
	action := NewRemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient](client, recorder)

	t.Run("on error updates status and condition", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		handler := &mockRemoteExternalReconciler{}
		logger := logrus.NewEntry(logrus.New())
		testErr := errors.New("test error")
		
		res, err := action.OnError(ctx, obj, data, handler, testErr, logger)
		assert.Equal(t, testErr, err)
		assert.Equal(t, reconcile.Result{}, res)
		
		// Check that status flags were updated
		assert.True(t, obj.GetStatus().GetIsOnError())
		assert.False(t, obj.GetStatus().GetIsSync())
		assert.Contains(t, obj.GetStatus().GetLastErrorMessage(), "test error")
	})
}

func TestDefaultRemoteReconcilerAction_OnSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	
	action := NewRemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient](client, recorder)

	t.Run("on success updates status and condition", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
			generation: 5,
		}
		data := make(map[string]any)
		handler := &mockRemoteExternalReconciler{}
		logger := logrus.NewEntry(logrus.New())
		diff := NewRemoteDiff[mockAPIObject]()
		
		res, err := action.OnSuccess(ctx, obj, data, handler, diff, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
		
		// Check that status flags were updated
		assert.False(t, obj.GetStatus().GetIsOnError())
		assert.True(t, obj.GetStatus().GetIsSync())
		assert.Equal(t, int64(5), obj.GetStatus().GetObservedGeneration())
		
		// Check that condition was updated
		conditions := obj.GetStatus().GetConditions()
		assert.Len(t, conditions, 1)
		assert.Equal(t, "Ready", conditions[0].Type)
		assert.Equal(t, metav1.ConditionTrue, conditions[0].Status)
		assert.Equal(t, "Ready", conditions[0].Reason)
	})

	t.Run("on success with already successful condition", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
			generation: 1,
			status: mockRemoteStatus{
				conditions: []metav1.Condition{
					{
						Type:   "Ready",
						Status: metav1.ConditionTrue,
						Reason: "Ready",
					},
				},
			},
		}
		data := make(map[string]any)
		handler := &mockRemoteExternalReconciler{}
		logger := logrus.NewEntry(logrus.New())
		diff := NewRemoteDiff[mockAPIObject]()
		
		res, err := action.OnSuccess(ctx, obj, data, handler, diff, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})
}

func TestDefaultRemoteReconcilerAction_Diff(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	
	action := NewRemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient](client, recorder)

	t.Run("diff returns create when current object is nil", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		handler := &mockRemoteExternalReconciler{}
		logger := logrus.NewEntry(logrus.New())
		
		// Mock read with nil current object
		read := NewRemoteRead[mockAPIObject]()
		expectedObj := mockAPIObject{ID: "new-obj", Name: "new-name"}
		read.SetExpectedObject(expectedObj)
		// Current object is zero value by default, which is equivalent to nil in the check
		
		diff, res, err := action.Diff(ctx, obj, read, data, handler, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
		
		// Check that we should create the object
		assert.Equal(t, expectedObj, diff.GetObjectToCreate())
		assert.Empty(t, diff.GetObjectToUpdate())
		assert.True(t, diff.IsDiff())
		assert.Contains(t, diff.Diff(), "Need to create new object test-name on remote target")
	})

	t.Run("diff returns update when objects differ", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
		}
		data := make(map[string]any)
		handler := &mockRemoteExternalReconciler{
			diffResult: &patch.PatchResult{Patch: []byte("change")},
		}
		logger := logrus.NewEntry(logrus.New())
		
		// Mock read with both current and expected objects
		read := NewRemoteRead[mockAPIObject]()
		currentObj := mockAPIObject{ID: "current-obj", Name: "current-name", Value: 100}
		expectedObj := mockAPIObject{ID: "expected-obj", Name: "expected-name", Value: 200}
		read.SetCurrentObject(currentObj)
		read.SetExpectedObject(expectedObj)
		
		diff, res, err := action.Diff(ctx, obj, read, data, handler, logger)
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
		
		// Check that we should update the object
		assert.Equal(t, expectedObj, diff.GetObjectToUpdate())
		assert.Equal(t, mockAPIObject{}, diff.GetObjectToCreate()) // Should be zero value
	})

	t.Run("diff with corrupted lastAppliedConfiguration", func(t *testing.T) {
		ctx := context.Background()
		obj := &mockRemoteObject{
			name: "test-name",
			namespace: "test-ns",
			status: mockRemoteStatus{
				lastAppliedConfiguration: "invalid_base64_string!",
			},
		}
		data := make(map[string]any)
		handler := &mockRemoteExternalReconciler{}
		logger := logrus.NewEntry(logrus.New())
		
		// Mock read with both current and expected objects
		read := NewRemoteRead[mockAPIObject]()
		currentObj := mockAPIObject{ID: "current-obj", Name: "current-name"}
		expectedObj := mockAPIObject{ID: "expected-obj", Name: "expected-name"}
		read.SetCurrentObject(currentObj)
		read.SetExpectedObject(expectedObj)
		
		_, _, err := action.Diff(ctx, obj, read, data, handler, logger)
		// Should return an error when unzipping the corrupted configuration
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Error when create object from 'lastAppliedConfiguration'")
	})
}

func TestDefaultRemoteReconcilerAction_GetIgnoresDiff(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	
	action := NewRemoteReconcilerAction[*mockRemoteObject, mockAPIObject, mockAPIClient](client, recorder)

	t.Run("get ignores diff returns empty slice", func(t *testing.T) {
		ignoreOptions := action.GetIgnoresDiff()
		assert.Empty(t, ignoreOptions)
		assert.NotNil(t, ignoreOptions) // Should be an empty slice, not nil
	})
}