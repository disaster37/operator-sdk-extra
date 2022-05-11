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
	mock   any
}

type TestStep struct {
	Pre   func(c client.Client, mock any, data map[string]any) error
	Do    func(c client.Client, o client.Object, data map[string]any) error
	Check func(t *testing.T, c client.Client, o client.Object, data map[string]any) error
}

func NewTestCase(t *testing.T, c client.Client, mock any, key types.NamespacedName, o client.Object, wait time.Duration) *TestCase {
	return &TestCase{
		t:      t,
		client: c,
		object: o,
		wait:   wait,
		key:    key,
		mock:   mock,
		data:   map[string]any{},
		Steps:  make([]TestStep, 0),
	}
}

func (h *TestCase) Run() {

	var err error

	// Run pre test
	if h.PreTest != nil {
		if err = h.PreTest(h.data); err != nil {
			h.t.Fatal(err)
		}
	}

	for _, step := range h.Steps {
		if step.Pre != nil {
			if err = step.Pre(h.client, h.mock, h.data); err != nil {
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
		if err = step.Do(h.client, h.object, h.data); err != nil {
			h.t.Fatal(err)
		}
		if err = h.client.Get(context.Background(), h.key, h.object); err != nil {
			if k8serrors.IsNotFound(err) {
				h.object = nil
			} else {
				h.t.Fatal(err)
			}
		}
		if err = step.Check(h.t, h.client, h.object, h.data); err != nil {
			h.t.Fatal(err)
		}
		time.Sleep(h.wait)
	}
}
