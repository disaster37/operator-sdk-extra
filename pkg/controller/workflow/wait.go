package workflow

import (
	"context"
	"reflect"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// DefaultRequeueAfter is the default pause duration when owned objects have
// not yet converged.
const DefaultRequeueAfter = 10 * time.Second

// OwnedObjectPredicate is a function that checks whether a given owned object
// has reached the desired state. Return true when the object is converged.
type OwnedObjectPredicate[T client.Object] func(obj T) (converged bool)

// WaitForOwnedObjects lists owned objects matching the selector and checks
// whether all of them satisfy the predicate. If not all are converged, it
// returns a short-circuit reconcile.Result with RequeueAfter.
//
// This generalises the hand-coded StatefulSet CurrentReplicas / sequence-number
// polling found in the elasticsearch-operator reference.
func WaitForOwnedObjects[T client.Object](
	ctx context.Context,
	c client.Client,
	owner client.Object,
	list client.ObjectList,
	predicate OwnedObjectPredicate[T],
	listOpts ...client.ListOption,
) (done bool, res reconcile.Result, err error) {
	if err := c.List(ctx, list, listOpts...); err != nil {
		return false, reconcile.Result{}, err
	}

	items := getItems[T](list)

	if len(items) == 0 {
		return false, reconcile.Result{RequeueAfter: DefaultRequeueAfter}, nil
	}

	for _, item := range items {
		if !predicate(item) {
			return false, reconcile.Result{RequeueAfter: DefaultRequeueAfter}, nil
		}
	}

	return true, reconcile.Result{}, nil
}

// getItems extracts typed items from a client.ObjectList using reflection.
func getItems[T client.Object](list client.ObjectList) []T {
	val := reflect.ValueOf(list).Elem()
	itemsField := val.FieldByName("Items")
	if !itemsField.IsValid() {
		return nil
	}

	items := make([]T, itemsField.Len())
	for i := range items {
		if item, ok := itemsField.Index(i).Addr().Interface().(T); ok {
			items[i] = item
		}
	}

	return items
}
