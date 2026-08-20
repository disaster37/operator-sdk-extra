package test

import (
	"context"
	"errors"
	"reflect"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewDeleteStep[T client.Object]() TestStep[T] {
	return TestStep[T]{
		Name: "delete",
		Do: func(c client.Client, key types.NamespacedName, o T, data map[string]any) error {
			v := reflect.ValueOf(o)
			if any(o) == nil || (v.Kind() == reflect.Pointer && v.IsNil()) {
				return errors.New("object is null")
			}
			wait := int64(0)
			return c.Delete(context.Background(), o, &client.DeleteOptions{GracePeriodSeconds: &wait})
		},
	}
}

func NewCreateStep[T client.Object](builder func(c client.Client, key types.NamespacedName) (T, error)) TestStep[T] {
	return TestStep[T]{
		Name: "create",
		Do: func(c client.Client, key types.NamespacedName, o T, data map[string]any) error {
			obj, err := builder(c, key)
			if err != nil {
				return err
			}
			return c.Create(context.Background(), obj)
		},
	}
}
