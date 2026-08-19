package workflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestWaitForOwnedObjectsEmptyList(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "default"}}

	pods := &corev1.PodList{}
	done, res, err := workflow.WaitForOwnedObjects[*corev1.Pod](
		context.Background(),
		c,
		owner,
		pods,
		func(pod *corev1.Pod) bool { return true },
	)

	require.NoError(t, err)
	assert.False(t, done, "expected not done when no pods exist")
	assert.Greater(t, res.RequeueAfter, time.Duration(0), "expected RequeueAfter for empty list")
}

func TestWaitForOwnedObjectsAllConverged(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "default"}}

	pods := &corev1.PodList{}
	done, res, err := workflow.WaitForOwnedObjects[*corev1.Pod](
		context.Background(),
		c,
		owner,
		pods,
		func(p *corev1.Pod) bool { return p.Status.Phase == corev1.PodRunning },
	)

	require.NoError(t, err)
	assert.True(t, done, "expected done when all pods are running")
	assert.Zero(t, res.RequeueAfter)
}

func TestWaitForOwnedObjectsNotConverged(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "default"}}

	pods := &corev1.PodList{}
	done, res, err := workflow.WaitForOwnedObjects[*corev1.Pod](
		context.Background(),
		c,
		owner,
		pods,
		func(p *corev1.Pod) bool { return p.Status.Phase == corev1.PodRunning },
	)

	require.NoError(t, err)
	assert.False(t, done, "expected not done when pods are not running")
	assert.Greater(t, res.RequeueAfter, time.Duration(0), "expected RequeueAfter for unconverged")
}

func TestDefaultRequeueAfter(t *testing.T) {
	assert.Equal(t, 10*time.Second, workflow.DefaultRequeueAfter)
}
