package test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewDeleteStep[T client.Object]() TestStep[T] {
	return TestStep[T]{
		Name: "delete",
		Do: func(c client.Client, key types.NamespacedName, o T, data map[string]any) error {
			if any(o) == nil || (reflect.ValueOf(o).Kind() == reflect.Ptr && reflect.ValueOf(o).IsNil()) {
				return errors.New("object is null")
			}
			wait := int64(0)
			if err := c.Delete(context.Background(), o, &client.DeleteOptions{GracePeriodSeconds: &wait}); err != nil {
				return err
			}
			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o T, data map[string]any) error {
			return nil
		},
	}
}

func NewCreateStep[T client.Object](builder func(c client.Client, key types.NamespacedName) (T, error)) TestStep[T] {
	return TestStep[T]{
		Name: "create",
		Pre: func(c client.Client, data map[string]any) error {
			return nil
		},
		Do: func(c client.Client, key types.NamespacedName, o T, data map[string]any) error {
			obj, err := builder(c, key)
			if err != nil {
				return err
			}
			return c.Create(context.Background(), obj)
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o T, data map[string]any) error {
			return nil
		},
	}
}