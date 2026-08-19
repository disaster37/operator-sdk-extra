package workflow

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkflowPhase is a typed string enum for saga phase cursors.
type WorkflowPhase string

func (p WorkflowPhase) String() string { return string(p) }

// WorkflowStatus is a standalone embeddable status sub-struct for multi-cycle
// workflow sagas. It stores the saga cursor (CurrentPhase) separately from
// conditions, so conditions remain user-facing readiness indicators.
type WorkflowStatus struct {
	// CurrentPhase is the active saga phase. Operators use this to determine
	// what work to perform on the current reconcile cycle.
	// +optional
	CurrentPhase WorkflowPhase `json:"currentPhase,omitempty"`

	// PhaseConditions holds per-phase one-shot conditions that track
	// completion status of individual saga phases.
	// +optional
	PhaseConditions []metav1.Condition `json:"phaseConditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=phaseConditions"`
}

// Current returns the current saga phase.
func (s *WorkflowStatus) Current() WorkflowPhase {
	return s.CurrentPhase
}

// Advance transitions the saga to the next phase.
func (s *WorkflowStatus) Advance(to WorkflowPhase) {
	s.CurrentPhase = to
}

// IsPhase returns true when the saga is at the given phase.
func (s *WorkflowStatus) IsPhase(phase WorkflowPhase) bool {
	return s.CurrentPhase == phase
}

// IsPhaseEmpty returns true when no phase has been set yet (initial state).
func (s *WorkflowStatus) IsPhaseEmpty() bool {
	return s.CurrentPhase == ""
}
