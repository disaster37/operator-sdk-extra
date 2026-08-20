package multiphase

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

const lastAppliedConfigAnnotationKey = "kubectl.kubernetes.io/last-applied-configuration"

func newCleanupConfigMap(name, namespace string, annotations map[string]string) *corev1.ConfigMap {
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

func TestCleanupReadLastAppliedAnnotations_SkipsOrphans(t *testing.T) {
	a := newCleanupConfigMap("a", "default", map[string]string{lastAppliedConfigAnnotationKey: `{"kind":"ConfigMap"}`})
	b := newCleanupConfigMap("b", "default", map[string]string{lastAppliedConfigAnnotationKey: `{"kind":"ConfigMap"}`})
	orphan := newCleanupConfigMap("orphan", "default", map[string]string{lastAppliedConfigAnnotationKey: `{"kind":"ConfigMap"}`})

	c := newCleanupTestClient(t, a, b, orphan)

	read := NewMultiPhaseRead[*corev1.ConfigMap]()
	read.AddCurrentObject(a)
	read.AddCurrentObject(b)
	read.AddCurrentObject(orphan)
	read.AddExpectedObject(a)
	read.AddExpectedObject(b)

	logger := logrus.NewEntry(logrus.New())
	cleaned := CleanupReadLastAppliedAnnotations(context.Background(), c, read, logger)
	assert.Equal(t, 2, cleaned)

	gotA := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "a", Namespace: "default"}, gotA))
	assert.NotContains(t, gotA.Annotations, lastAppliedConfigAnnotationKey)

	gotB := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "b", Namespace: "default"}, gotB))
	assert.NotContains(t, gotB.Annotations, lastAppliedConfigAnnotationKey)

	gotOrphan := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "orphan", Namespace: "default"}, gotOrphan))
	assert.Contains(t, gotOrphan.Annotations, lastAppliedConfigAnnotationKey)
}

func TestCleanupReadLastAppliedAnnotations_ContinuesOnError(t *testing.T) {
	good := newCleanupConfigMap("good", "default", map[string]string{lastAppliedConfigAnnotationKey: `{"kind":"ConfigMap"}`})
	// "bad" is not present in the fake client, so patching it fails.
	bad := newCleanupConfigMap("bad", "default", map[string]string{lastAppliedConfigAnnotationKey: `{"kind":"ConfigMap"}`})

	c := newCleanupTestClient(t, good)

	read := NewMultiPhaseRead[*corev1.ConfigMap]()
	read.AddCurrentObject(bad)
	read.AddCurrentObject(good)
	read.AddExpectedObject(bad)
	read.AddExpectedObject(good)

	logger := logrus.NewEntry(logrus.New())
	cleaned := CleanupReadLastAppliedAnnotations(context.Background(), c, read, logger)
	assert.Equal(t, 1, cleaned)

	got := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "good", Namespace: "default"}, got))
	assert.NotContains(t, got.Annotations, lastAppliedConfigAnnotationKey)
}

func TestCleanupReadLastAppliedAnnotations_NoopWhenNoAnnotation(t *testing.T) {
	a := newCleanupConfigMap("a", "default", map[string]string{"keep-me": "value"})
	b := newCleanupConfigMap("b", "default", nil)

	c := newCleanupTestClient(t, a, b)

	read := NewMultiPhaseRead[*corev1.ConfigMap]()
	read.AddCurrentObject(a)
	read.AddCurrentObject(b)
	read.AddExpectedObject(a)
	read.AddExpectedObject(b)

	logger := logrus.NewEntry(logrus.New())
	cleaned := CleanupReadLastAppliedAnnotations(context.Background(), c, read, logger)
	assert.Equal(t, 0, cleaned)

	gotA := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "a", Namespace: "default"}, gotA))
	assert.Equal(t, "value", gotA.Annotations["keep-me"])
	assert.NotContains(t, gotA.Annotations, lastAppliedConfigAnnotationKey)
}
