package test

import (
	"context"
	"reflect"
	"testing"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TestCase[k8sObject client.Object] struct {
	// Permit to lauch some preaction, like init mock
	PreTest func(stepName *string, data map[string]any) error
	Steps   []TestStep[k8sObject]

	key    types.NamespacedName
	wait   time.Duration
	data   map[string]any
	t      *testing.T
	client client.Client
}

type TestStep[k8sObject client.Object] struct {
	Name  string
	Pre   func(c client.Client, data map[string]any) error
	Do    func(c client.Client, key types.NamespacedName, o k8sObject, data map[string]any) error
	Check func(t *testing.T, c client.Client, key types.NamespacedName, o k8sObject, data map[string]any) error
}

func NewTestCase[k8sObject client.Object](t *testing.T, c client.Client, key types.NamespacedName, wait time.Duration, data map[string]any) *TestCase[k8sObject] {
	return &TestCase[k8sObject]{
		t:       t,
		client:  c,
		wait:    wait,
		key:     key,
		data:    data,
		PreTest: nil,
		Steps:   make([]TestStep[k8sObject], 0),
	}
}

func (h *TestCase[k8sObject]) Run() {
	var (
		err error
		o   k8sObject
	)

	stepName := new(string)
	// Run pre test
	if h.PreTest != nil {
		if err = h.PreTest(stepName, h.data); err != nil {
			h.t.Fatal(err)
		}
	}

	for _, step := range h.Steps {
		*stepName = step.Name

		if step.Pre != nil {
			if err = step.Pre(h.client, h.data); err != nil {
				h.t.Fatal(err)
			}
		}

		o = h.getObjectOrNil(o)
		if err = step.Do(h.client, h.key, o, h.data); err != nil {
			h.t.Fatal(err)
		}

		o = h.getObjectOrNil(o)
		if step.Check != nil {
			if err = step.Check(h.t, h.client, h.key, o, h.data); err != nil {
				h.t.Fatal(err)
			}
		}
		time.Sleep(h.wait)
	}
}

// getObjectOrNil fetches the object at h.key into a fresh instance, returning the
// zero value when the object does not exist.
func (h *TestCase[k8sObject]) getObjectOrNil(o k8sObject) k8sObject {
	var nilObject k8sObject
	o = getNewObject(o)
	if err := h.client.Get(context.Background(), h.key, o); err != nil {
		if !k8serrors.IsNotFound(err) {
			h.t.Fatal(err)
		}
		return nilObject
	}
	return o
}

func (h *TestCase[k8sObject]) Eventually(fn func(c client.Client) error, timeout, interval time.Duration) error {
	isTimeout, err := RunWithTimeout(func() error {
		return fn(h.client)
	}, timeout, interval)
	if err != nil || isTimeout {
		if err == nil {
			return ErrEventuallyTimeout
		}
		return err
	}
	return nil
}

func getNewObject[k8sObject client.Object](o k8sObject) k8sObject {
	return reflect.New(reflect.TypeOf(o).Elem()).Interface().(k8sObject)
}
