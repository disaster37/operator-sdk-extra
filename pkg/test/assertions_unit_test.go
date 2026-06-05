package test

import (
	"context"
	"testing"
	"time"

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

func TestAssertReadyCondition(t *testing.T) {
	t.Run("ready condition is true", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		}
		AssertReadyCondition(t, cm, func(cm *corev1.ConfigMap) []metav1.Condition {
			return []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			}
		})
	})
}

func TestAssertOwnerReference(t *testing.T) {
	owner := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "owner", Namespace: "default", UID: "test-uid",
		},
	}
	child := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "child", Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "owner", UID: "test-uid"},
			},
		},
	}
	AssertOwnerReference(t, child, owner)
}

func TestAssertManagedByOperator(t *testing.T) {
	withRef := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test", Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{Name: "parent"}},
		},
	}
	AssertManagedByOperator(t, withRef)
}

func TestReflectGetConditions(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	conditions := reflectGetConditions(cm)
	assert.Nil(t, conditions)
}

func TestReflectNewObject(t *testing.T) {
	obj := reflectNewObject[*corev1.ConfigMap]()
	assert.NotNil(t, obj)
	assert.IsType(t, &corev1.ConfigMap{}, obj)
}

func TestEventually(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	key := types.NamespacedName{Namespace: "default", Name: "test"}

	tc := NewTestCase[*corev1.ConfigMap](t, c, key, 10*time.Millisecond, map[string]any{})

	t.Run("succeeds when condition met", func(t *testing.T) {
		err := tc.Eventually(func(c client.Client) error {
			obj := &corev1.ConfigMap{}
			if err := c.Get(context.Background(), key, obj); err != nil {
				return err
			}
			if obj.Data["key"] == "value" {
				return nil
			}
			return assert.AnError
		}, 1*time.Second, 10*time.Millisecond)
		assert.NoError(t, err)
	})

	t.Run("times out when condition not met", func(t *testing.T) {
		err := tc.Eventually(func(c client.Client) error {
			return assert.AnError
		}, 50*time.Millisecond, 10*time.Millisecond)
		assert.Error(t, err)
	})
}