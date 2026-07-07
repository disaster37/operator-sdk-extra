package test

import (
	"context"
	"fmt"
	"reflect"
	"slices"
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

// failOnWaitError fails the test with an appropriate message depending on whether
// the wait timed out or errored.
func failOnWaitError(t *testing.T, isTimeout bool, err error, timeoutMsg, errPrefix string) {
	if err == nil && !isTimeout {
		return
	}
	if err == nil {
		t.Fatalf("%s", timeoutMsg)
	}
	t.Fatalf("%s: %s", errPrefix, err.Error())
}

func WaitForReadyCondition[T client.Object](t *testing.T, c client.Client, key types.NamespacedName, timeout time.Duration) {
	isTimeout, err := RunWithTimeout(func() error {
		obj := reflectNewObject[T]()
		if err := c.Get(context.Background(), key, obj); err != nil {
			return err
		}
		conditions := reflectGetConditions(obj)
		if condition.IsStatusConditionPresentAndEqual(conditions, "Ready", metav1.ConditionTrue) {
			return nil
		}
		return fmt.Errorf("object %s/%s not ready yet", key.Namespace, key.Name)
	}, timeout, 1*time.Second)
	failOnWaitError(t, isTimeout, err,
		fmt.Sprintf("Timed out waiting for Ready condition on %s/%s", key.Namespace, key.Name),
		fmt.Sprintf("Error waiting for Ready condition on %s/%s", key.Namespace, key.Name))
}

func WaitForGenerationIncrement[T client.Object](t *testing.T, c client.Client, key types.NamespacedName, minGeneration int64, timeout time.Duration) {
	isTimeout, err := RunWithTimeout(func() error {
		obj := reflectNewObject[T]()
		if err := c.Get(context.Background(), key, obj); err != nil {
			return err
		}
		observedGen := reflectGetObservedGeneration(obj)
		if observedGen >= minGeneration {
			return nil
		}
		return fmt.Errorf("observed generation %d < %d", observedGen, minGeneration)
	}, timeout, 1*time.Second)
	failOnWaitError(t, isTimeout, err,
		fmt.Sprintf("Timed out waiting for generation >= %d on %s/%s", minGeneration, key.Namespace, key.Name),
		fmt.Sprintf("Error waiting for generation on %s/%s", key.Namespace, key.Name))
}

func WaitForResourceVersionChange[T client.Object](t *testing.T, c client.Client, key types.NamespacedName, oldVersion string, timeout time.Duration) {
	isTimeout, err := RunWithTimeout(func() error {
		obj := reflectNewObject[T]()
		if err := c.Get(context.Background(), key, obj); err != nil {
			return err
		}
		if obj.GetResourceVersion() != oldVersion {
			return nil
		}
		return fmt.Errorf("resource version not changed yet")
	}, timeout, 1*time.Second)
	failOnWaitError(t, isTimeout, err,
		fmt.Sprintf("Timed out waiting for resource version change on %s/%s", key.Namespace, key.Name),
		fmt.Sprintf("Error waiting for resource version change on %s/%s", key.Namespace, key.Name))
}

func AssertOwnerReference(t *testing.T, obj client.Object, owner metav1.Object) {
	found := slices.ContainsFunc(obj.GetOwnerReferences(), func(ref metav1.OwnerReference) bool {
		return ref.Name == owner.GetName() && ref.UID == owner.GetUID()
	})
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

// reflectStatusField returns the named field of the object's Status struct.
// The returned value is invalid when Status or the field is absent or unexported.
func reflectStatusField(obj client.Object, name string) reflect.Value {
	statusField := reflect.ValueOf(obj).Elem().FieldByName("Status")
	if !statusField.IsValid() {
		return reflect.Value{}
	}
	f := statusField.FieldByName(name)
	if !f.IsValid() || !f.CanInterface() {
		return reflect.Value{}
	}
	return f
}

func reflectGetConditions(obj client.Object) []metav1.Condition {
	f := reflectStatusField(obj, "Conditions")
	if !f.IsValid() {
		return nil
	}
	conditions, ok := f.Interface().([]metav1.Condition)
	if !ok {
		return nil
	}
	return conditions
}

func reflectGetObservedGeneration(obj client.Object) int64 {
	f := reflectStatusField(obj, "ObservedGeneration")
	if !f.IsValid() {
		return 0
	}
	return f.Int()
}
