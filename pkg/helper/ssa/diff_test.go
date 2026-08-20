package ssa

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNormalize(t *testing.T) {
	u := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":              "test",
				"namespace":         "default",
				"resourceVersion":   "12345",
				"generation":        int64(1),
				"creationTimestamp": "2023-01-01T00:00:00Z",
				"uid":               "abc-def",
				"managedFields": []interface{}{
					map[string]interface{}{"manager": "test"},
				},
				"annotations": map[string]interface{}{
					lastAppliedConfigAnnotation: "old-config",
					"keep-me":                   "value",
				},
			},
			"data": map[string]interface{}{
				"key": "value",
			},
			"status": map[string]interface{}{
				"phase": "Active",
			},
		},
	}

	Normalize(u)

	assert.NotContains(t, u.Object["metadata"], "managedFields")
	assert.NotContains(t, u.Object["metadata"], "resourceVersion")
	assert.NotContains(t, u.Object["metadata"], "generation")
	assert.NotContains(t, u.Object["metadata"], "creationTimestamp")
	assert.NotContains(t, u.Object["metadata"], "uid")
	assert.NotContains(t, u.Object, "status")

	annotations := u.GetAnnotations()
	assert.NotContains(t, annotations, lastAppliedConfigAnnotation)
	assert.Contains(t, annotations, "keep-me")
}

func TestNormalize_OnlyLastAppliedAnnotation(t *testing.T) {
	u := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test",
				"annotations": map[string]interface{}{
					lastAppliedConfigAnnotation: "old-config",
				},
			},
		},
	}

	Normalize(u)

	_, hasAnnotations, _ := unstructured.NestedFieldNoCopy(u.Object, "metadata", "annotations")
	assert.False(t, hasAnnotations, "annotations should be removed when only last-applied remains")
}

func TestNormalize_NoAnnotations(t *testing.T) {
	u := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test",
			},
		},
	}

	Normalize(u)
	assert.Equal(t, "test", u.GetName())
}

func testUnstructured(cm *corev1.ConfigMap) *unstructured.Unstructured {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cm)
	require.NoError(&testing.T{}, err, "this is a test helper")
	return &unstructured.Unstructured{Object: u}
}

func TestIsObjectDiff_Unchanged(t *testing.T) {
	cm1 := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	changed, human, err := IsObjectDiff(cm1, testUnstructured(cm1))
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Empty(t, human)
}

func TestIsObjectDiff_Changed(t *testing.T) {
	cm1 := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "value",
		},
	}
	cm2 := cm1.DeepCopy()
	cm2.Data["key"] = "different"

	changed, human, err := IsObjectDiff(cm1, testUnstructured(cm2))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.NotEmpty(t, human)
}

func TestIsObjectDiff_NoiseFieldsIgnored(t *testing.T) {
	cm1 := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "value",
		},
	}
	cm2 := cm1.DeepCopy()
	cm2.ResourceVersion = "99999"
	cm2.Generation = 5
	cm2.UID = "some-uid"
	cm2.CreationTimestamp = metav1.Now()
	cm2.ManagedFields = []metav1.ManagedFieldsEntry{
		{Manager: "other-operator", Operation: metav1.ManagedFieldsOperationApply},
	}

	changed, _, err := IsObjectDiff(cm1, testUnstructured(cm2))
	require.NoError(t, err)
	assert.False(t, changed, "noise fields like resourceVersion, generation, uid, creationTimestamp, managedFields should be ignored")
}

func TestIsObjectDiff_AnnotationsNormalized(t *testing.T) {
	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Annotations: map[string]string{
				"keep":                      "me",
				lastAppliedConfigAnnotation: "config1",
			},
		},
	}
	cm2 := cm1.DeepCopy()
	cm2.Annotations[lastAppliedConfigAnnotation] = "config2"

	changed, _, err := IsObjectDiff(cm1, testUnstructured(cm2))
	require.NoError(t, err)
	assert.False(t, changed, "last-applied-configuration annotation should be ignored in diff")
}

func TestIsObjectDiff_PredictedHasNoiseFields(t *testing.T) {
	// predicted from DryRunApply may have resourceVersion/generation/uid filled in by the API server.
	// These should all be stripped during normalization.
	cm1 := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string]string{"key": "val"},
	}
	cm2 := cm1.DeepCopy()
	cm2.ResourceVersion = "42"
	cm2.Generation = 3
	cm2.UID = "abc-123-def"
	cm2.CreationTimestamp = metav1.Now()
	cm2.ManagedFields = []metav1.ManagedFieldsEntry{
		{Manager: "some-operator", Operation: metav1.ManagedFieldsOperationApply},
	}

	changed, _, err := IsObjectDiff(cm1, testUnstructured(cm2))
	require.NoError(t, err)
	assert.False(t, changed, "predicted with server-populated noise fields should still compare equal")
}

func TestDryRunApply(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "val",
		},
	}

	predicted, err := DryRunApply(context.Background(), c, cm, "test-controller")
	require.NoError(t, err)
	assert.NotNil(t, predicted)
	assert.Equal(t, "test-cm", predicted.GetName())
}

func TestDryRunApply_WithExistingObject(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))

	existing := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "old-val",
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "new-val",
		},
	}

	predicted, err := DryRunApply(context.Background(), c, cm, "test-controller")
	require.NoError(t, err)
	assert.NotNil(t, predicted)
}

func TestDryRunApply_DoesNotMutateOriginal(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-cm",
			Namespace:         "default",
			ResourceVersion:   "original-rv",
			Generation:        5,
			CreationTimestamp: metav1.Now(),
			UID:               "original-uid",
		},
		Data: map[string]string{"key": "val"},
	}

	_, err := DryRunApply(context.Background(), c, cm, "test-controller")
	require.NoError(t, err)

	// Original must not be mutated
	assert.Equal(t, "original-rv", cm.ResourceVersion)
	assert.Equal(t, int64(5), cm.Generation)
	assert.Equal(t, "original-uid", string(cm.UID))
	assert.Equal(t, "val", cm.Data["key"])
}
