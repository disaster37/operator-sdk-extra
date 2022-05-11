package test

import (
	"errors"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TestCase struct {
	// Permit to lauch some preaction, like init mock
	PreTest func(data map[string]any) error
	Steps   []TestStep

	data   map[string]any
	t      *testing.T
	client client.Client
}

type TestStep struct {
	Do    func(c client.Client, data map[string]any) error
	Check func(t *testing.T, c client.Client, data map[string]any) error
}

func NewTestCase(t *testing.T, c client.Client) *TestCase {
	return &TestCase{
		t:      t,
		client: c,
	}
}

func (h *TestCase) Run() {

	var err error

	// Run pre test
	if h.PreTest != nil {
		if err = h.PreTest(); err != nil {
			panic(err)
		}
	}

	for _, step := range h.Steps {
		if err = step.Do(h.client); err != nil {
			h.t.Fatal(err)
		}
		if err = step.Check(h.t, h.client); err != nil {
			h.t.Fatal(err)
		}
	}
}
