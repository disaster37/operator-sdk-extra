package test

import (
	"context"
	"testing"
	"time"

	"github.com/disaster37/operator-sdk-extra/pkg/mock"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TestCase struct {
	// Permit to lauch some preaction, like init mock
	PreTest func(stepName *string, data map[string]any) error
	Steps   []TestStep

	key    types.NamespacedName
	wait   time.Duration
	object client.Object
	data   map[string]any
	t      *testing.T
	m      mock.MockBase
	client client.Client
}

type TestStep struct {
	Name  string
	Pre   func(c client.Client, data map[string]any) error
	Do    func(c client.Client, key types.NamespacedName, o client.Object, data map[string]any) error
	Check func(m mock.MockBase, t *testing.T, c client.Client, key types.NamespacedName, o client.Object, data map[string]any) error
}

func NewTestCase(m mock.MockBase, t *testing.T, c client.Client, key types.NamespacedName, o client.Object, wait time.Duration, data map[string]any) *TestCase {
	return &TestCase{
		t:       t,
		m:       m,
		client:  c,
		object:  o,
		wait:    wait,
		key:     key,
		data:    data,
		PreTest: nil,
		Steps:   make([]TestStep, 0),
	}
}

func (h *TestCase) Run() {

	var (
		err error
		o   client.Object
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

		o = h.object
		if err = h.client.Get(context.Background(), h.key, o); err != nil {
			o = nil
		}
		if err = step.Do(h.client, h.key, o, h.data); err != nil {
			h.t.Fatal(err)
		}

		o = h.object
		if err = h.client.Get(context.Background(), h.key, o); err != nil {
			o = nil
		}
		if err = step.Check(h.m, h.t, h.client, h.key, o, h.data); err != nil {
			h.t.Fatal(err)
		}
		time.Sleep(h.wait)
	}
}
