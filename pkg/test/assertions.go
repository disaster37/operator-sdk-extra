package test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/object"
	"github.com/stretchr/testify/assert"
	condition "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AssertReadyCondition[T any](t *testing.T, o client.Object, getConditions func(T) []metav1.Condition) {
	var zero T
	conditions := getConditions(zero)
	if !condition.IsStatusConditionPresentAndEqual(conditions, "Ready", metav1.ConditionTrue) {
		assert.Fail(t, fmt.Sprintf("Ready condition should be true for %s/%s", o.GetNamespace(), o.GetName()))
	}
}

func AssertErrorCondition[T any](t *testing.T, o client.Object, getConditions func(T) []metav1.Condition, expectedReason string) {
	var zero T
	conditions := getConditions(zero)
	c := condition.FindStatusCondition(conditions, "Ready")
	if c == nil {
		assert.Fail(t, fmt.Sprintf("Ready condition not found for %s/%s", o.GetNamespace(), o.GetName()))
		return
	}
	if c.Status != metav1.ConditionFalse {
		assert.Fail(t, fmt.Sprintf("Ready condition should be false for %s/%s", o.GetNamespace(), o.GetName()))
	}
	if expectedReason != "" && c.Reason != expectedReason {
		assert.Fail(t, fmt.Sprintf("Expected reason %s but got %s for %s/%s", expectedReason, c.Reason, o.GetNamespace(), o.GetName()))
	}
}

func AssertIsSync(t *testing.T, o object.RemoteObject) {
	if !o.GetStatus().GetIsSync() {
		assert.Fail(t, fmt.Sprintf("Expected IsSync to be true for %s/%s", o.GetNamespace(), o.GetName()))
	}
}

func AssertIsOnError(t *testing.T, o object.RemoteObject, expected bool) {
	if o.GetStatus().GetIsOnError() != expected {
		assert.Fail(t, fmt.Sprintf("Expected IsOnError to be %v for %s/%s", expected, o.GetNamespace(), o.GetName()))
	}
}

func AssertHasPhase(t *testing.T, o object.MultiPhaseObject, phase string) {
	if string(o.GetStatus().GetPhaseName()) != phase {
		assert.Fail(t, fmt.Sprintf("Expected phase %s but got %s for %s/%s", phase, o.GetStatus().GetPhaseName(), o.GetNamespace(), o.GetName()))
	}
}

func WaitForReadyCondition[T client.Object](t *testing.T, c client.Client, key types.NamespacedName, timeout time.Duration) {
	isTimeout, err := RunWithTimeout(func() error {
		var obj T
		obj = reflectNewObject[T]()
		if err := c.Get(context.Background(), key, obj); err != nil {
			return err
		}
		conditions := reflectGetConditions(obj)
		if condition.IsStatusConditionPresentAndEqual(conditions, "Ready", metav1.ConditionTrue) {
			return nil
		}
		return fmt.Errorf("object %s/%s not ready yet", key.Namespace, key.Name)
	}, timeout, 1*time.Second)
	if err != nil || isTimeout {
		if err == nil {
			t.Fatalf("Timed out waiting for Ready condition on %s/%s", key.Namespace, key.Name)
		}
		t.Fatalf("Error waiting for Ready condition on %s/%s: %s", key.Namespace, key.Name, err.Error())
	}
}

func WaitForGenerationIncrement[T client.Object](t *testing.T, c client.Client, key types.NamespacedName, minGeneration int64, timeout time.Duration) {
	isTimeout, err := RunWithTimeout(func() error {
		var obj T
		obj = reflectNewObject[T]()
		if err := c.Get(context.Background(), key, obj); err != nil {
			return err
		}
		observedGen := reflectGetObservedGeneration(obj)
		if observedGen >= minGeneration {
			return nil
		}
		return fmt.Errorf("observed generation %d < %d", observedGen, minGeneration)
	}, timeout, 1*time.Second)
	if err != nil || isTimeout {
		if err == nil {
			t.Fatalf("Timed out waiting for generation >= %d on %s/%s", minGeneration, key.Namespace, key.Name)
		}
		t.Fatalf("Error waiting for generation on %s/%s: %s", key.Namespace, key.Name, err.Error())
	}
}

func WaitForResourceVersionChange[T client.Object](t *testing.T, c client.Client, key types.NamespacedName, oldVersion string, timeout time.Duration) {
	isTimeout, err := RunWithTimeout(func() error {
		var obj T
		obj = reflectNewObject[T]()
		if err := c.Get(context.Background(), key, obj); err != nil {
			return err
		}
		if obj.GetResourceVersion() != oldVersion {
			return nil
		}
		return fmt.Errorf("resource version not changed yet")
	}, timeout, 1*time.Second)
	if err != nil || isTimeout {
		if err == nil {
			t.Fatalf("Timed out waiting for resource version change on %s/%s", key.Namespace, key.Name)
		}
		t.Fatalf("Error waiting for resource version change on %s/%s: %s", key.Namespace, key.Name, err.Error())
	}
}

func AssertOwnerReference(t *testing.T, obj client.Object, owner metav1.Object) {
	refs := obj.GetOwnerReferences()
	found := false
	for _, ref := range refs {
		if ref.Name == owner.GetName() && ref.UID == owner.GetUID() {
			found = true
			break
		}
	}
	if !found {
		assert.Fail(t, fmt.Sprintf("Expected owner reference to %s/%s not found on %s/%s", owner.GetNamespace(), owner.GetName(), obj.GetNamespace(), obj.GetName()))
	}
}

func AssertManagedByOperator(t *testing.T, obj client.Object) {
	if len(obj.GetOwnerReferences()) == 0 {
		assert.Fail(t, fmt.Sprintf("Expected %s/%s to have owner references (managed by operator)", obj.GetNamespace(), obj.GetName()))
	}
}

func reflectNewObject[T any]() T {
	return reflect.New(reflect.TypeOf((*T)(nil)).Elem().Elem()).Interface().(T)
}

func reflectGetConditions(obj client.Object) []metav1.Condition {
	rv := reflect.ValueOf(obj).Elem()
	statusField := rv.FieldByName("Status")
	if !statusField.IsValid() {
		return nil
	}
	conditionsField := statusField.FieldByName("Conditions")
	if !conditionsField.IsValid() {
		return nil
	}
	if !conditionsField.CanInterface() {
		return nil
	}
	conditions, ok := conditionsField.Interface().([]metav1.Condition)
	if !ok {
		return nil
	}
	return conditions
}

func reflectGetObservedGeneration(obj client.Object) int64 {
	rv := reflect.ValueOf(obj).Elem()
	statusField := rv.FieldByName("Status")
	if !statusField.IsValid() {
		return 0
	}
	ogField := statusField.FieldByName("ObservedGeneration")
	if !ogField.IsValid() {
		return 0
	}
	if !ogField.CanInterface() {
		return 0
	}
	return ogField.Int()
}