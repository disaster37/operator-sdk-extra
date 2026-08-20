package ssa

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
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

func newCleanupTestConfigMap(name, namespace string, annotations map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Data: map[string]string{
			"key": "value",
		},
	}
}

func newCleanupTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestCleanupLastAppliedAnnotation_RemovesWhenPresent(t *testing.T) {
	cm := newCleanupTestConfigMap("test-cm", "default", map[string]string{
		lastAppliedConfigAnnotation: `{"kind":"ConfigMap"}`,
		"keep-me":                   "value",
	})
	c := newCleanupTestClient(t, cm)

	logger := logrus.NewEntry(logrus.New())
	removed, err := CleanupLastAppliedAnnotation(context.Background(), c, cm, logger)
	require.NoError(t, err)
	assert.True(t, removed)

	got := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, got))
	assert.NotContains(t, got.Annotations, lastAppliedConfigAnnotation)
	assert.Equal(t, "value", got.Annotations["keep-me"])
	assert.Equal(t, "value", got.Data["key"])
}

func TestCleanupLastAppliedAnnotation_NoopWhenAbsent(t *testing.T) {
	cm := newCleanupTestConfigMap("test-cm", "default", map[string]string{
		"keep-me": "value",
	})
	c := newCleanupTestClient(t, cm)

	logger := logrus.NewEntry(logrus.New())
	removed, err := CleanupLastAppliedAnnotation(context.Background(), c, cm, logger)
	require.NoError(t, err)
	assert.False(t, removed)

	got := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, got))
	assert.Equal(t, "value", got.Annotations["keep-me"])
	assert.Equal(t, "value", got.Data["key"])
}

func TestCleanupLastAppliedAnnotation_NoopWhenDeletionTimestamp(t *testing.T) {
	now := metav1.Now()
	cm := newCleanupTestConfigMap("test-cm", "default", map[string]string{
		lastAppliedConfigAnnotation: `{"kind":"ConfigMap"}`,
	})
	cm.DeletionTimestamp = &now
	cm.Finalizers = []string{"test-finalizer"}
	c := newCleanupTestClient(t, cm)

	logger := logrus.NewEntry(logrus.New())
	removed, err := CleanupLastAppliedAnnotation(context.Background(), c, cm, logger)
	require.NoError(t, err)
	assert.False(t, removed)

	got := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, got))
	assert.Contains(t, got.Annotations, lastAppliedConfigAnnotation)
}

func TestCleanupLastAppliedAnnotation_OnlyAnnotationKey(t *testing.T) {
	cm := newCleanupTestConfigMap("test-cm", "default", map[string]string{
		lastAppliedConfigAnnotation: `{"kind":"ConfigMap"}`,
	})
	c := newCleanupTestClient(t, cm)

	logger := logrus.NewEntry(logrus.New())
	removed, err := CleanupLastAppliedAnnotation(context.Background(), c, cm, logger)
	require.NoError(t, err)
	assert.True(t, removed)

	got := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, got))
	assert.NotContains(t, got.Annotations, lastAppliedConfigAnnotation)
}

func TestCleanupLastAppliedAnnotation_PreservesThirdPartyAnnotations(t *testing.T) {
	cm := newCleanupTestConfigMap("test-cm", "default", map[string]string{
		lastAppliedConfigAnnotation:    `{"kind":"ConfigMap"}`,
		"foo":                          "bar",
		"app.kubernetes.io/managed-by": "other",
	})
	c := newCleanupTestClient(t, cm)

	logger := logrus.NewEntry(logrus.New())
	removed, err := CleanupLastAppliedAnnotation(context.Background(), c, cm, logger)
	require.NoError(t, err)
	assert.True(t, removed)

	got := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, got))
	assert.NotContains(t, got.Annotations, lastAppliedConfigAnnotation)
	assert.Equal(t, "bar", got.Annotations["foo"])
	assert.Equal(t, "other", got.Annotations["app.kubernetes.io/managed-by"])
}
