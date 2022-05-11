package test

import (
	"context"
	"testing"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TestCase struct {
	// Permit to lauch some preaction, like init mock
	PreTest func(data map[string]any) error
	Steps   []TestStep

	key    types.NamespacedName
	wait   time.Duration
	object client.Object
	data   map[string]any
	t      *testing.T
	client client.Client
}

type TestStep struct {
	Mock  any
	Pre   func(c client.Client, mock any, isSubmitted *bool, data map[string]any) error
	Do    func(c client.Client, key types.NamespacedName, o client.Object, data map[string]any) error
	Check func(t *testing.T, c client.Client, mock any, key types.NamespacedName, o client.Object, data map[string]any) error
}

func NewTestCase(t *testing.T, c client.Client, key types.NamespacedName, o client.Object, wait time.Duration, data map[string]any) *TestCase {
	return &TestCase{
		t:      t,
		client: c,
		object: o,
		wait:   wait,
		key:    key,
		data:   data,
		Steps:  make([]TestStep, 0),
	}
}

func (h *TestCase) Run() {

	var err error
	var isSubmitted *bool

	bTrue := true
	bFalse := false

	// Run pre test
	if h.PreTest != nil {
		if err = h.PreTest(h.data); err != nil {
			h.t.Fatal(err)
		}
	}

	for _, step := range h.Steps {
		isSubmitted = &bFalse
		if step.Pre != nil {
			if err = step.Pre(h.client, step.Mock, isSubmitted, h.data); err != nil {
				h.t.Fatal(err)
			}
		}

		if err = h.client.Get(context.Background(), h.key, h.object); err != nil {
			if k8serrors.IsNotFound(err) {
				h.object = nil
			} else {
				h.t.Fatal(err)
			}
		}
		if err = step.Do(h.client, h.key, h.object, h.data); err != nil {
			h.t.Fatal(err)
		}
		isSubmitted = &bTrue

		if err = h.client.Get(context.Background(), h.key, h.object); err != nil {
			if k8serrors.IsNotFound(err) {
				h.object = nil
			} else {
				h.t.Fatal(err)
			}
		}
		if err = step.Check(h.t, h.client, step.Mock, h.key, h.object, h.data); err != nil {
			h.t.Fatal(err)
		}
		time.Sleep(h.wait)
	}
}
