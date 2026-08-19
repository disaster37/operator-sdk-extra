package workflow

import (
	"context"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/workflow"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/multiphase"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/object"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// WorkflowStatusGetter is the interface that objects must implement to expose
// a WorkflowStatus sub-struct for saga phase tracking.
type WorkflowStatusGetter interface {
	GetWorkflowStatus() *workflow.WorkflowStatus
}

// WorkflowStepReconcilerAction extends MultiPhaseStepReconcilerAction with
// multi-cycle saga support. It adds a typed phase cursor persisted in a
// WorkflowStatus sub-struct (not conditions) and exposes phase-management
// helpers for convergence-gated sagas.
//
// The phase cursor follows this pattern:
//   - On each reconcile cycle, the step checks its current phase.
//   - It performs phase-appropriate work (computes desired objects in Read).
//   - When the phase requires convergence gating, it calls WaitForOwnedObjects
//     and short-circuits.
//   - When the phase is complete, it advances to the next phase.
type WorkflowStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] interface {
	multiphase.MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]

	// CurrentPhase returns the current saga phase.
	CurrentPhase(o k8sObject) workflow.WorkflowPhase

	// AdvancePhase transitions the saga to the next phase.
	AdvancePhase(ctx context.Context, o k8sObject, to workflow.WorkflowPhase, logger *logrus.Entry) (res reconcile.Result, err error)

	// IsPhase returns true when the saga is at the given phase.
	IsPhase(o k8sObject, phase workflow.WorkflowPhase) bool

	// IsPhaseEmpty returns true when no phase has been set yet.
	IsPhaseEmpty(o k8sObject) bool
}

// DefaultWorkflowStepReconcilerAction is the default implementation of
// WorkflowStepReconcilerAction. It embeds a standard multiphase step action
// and adds workflow phase management on top.
type DefaultWorkflowStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] struct {
	*multiphase.DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]
}

// NewWorkflowStepReconcilerAction creates a new workflow step reconciler action.
func NewWorkflowStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object](
	c client.Client,
	phaseName shared.PhaseName,
	conditionName shared.ConditionName,
	recorder record.EventRecorder,
	fieldManager string,
	dryRun bool,
) WorkflowStepReconcilerAction[k8sObject, k8sStepObject] {
	return &DefaultWorkflowStepReconcilerAction[k8sObject, k8sStepObject]{
		DefaultMultiPhaseStepReconcilerAction: multiphase.NewMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject](
			c,
			phaseName,
			conditionName,
			recorder,
			fieldManager,
			dryRun,
		).(*multiphase.DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]),
	}
}

func (h *DefaultWorkflowStepReconcilerAction[k8sObject, k8sStepObject]) getWorkflowStatus(o k8sObject) *workflow.WorkflowStatus {
	if ws, ok := any(o.GetStatus()).(WorkflowStatusGetter); ok {
		return ws.GetWorkflowStatus()
	}
	return &workflow.WorkflowStatus{}
}

func (h *DefaultWorkflowStepReconcilerAction[k8sObject, k8sStepObject]) CurrentPhase(o k8sObject) workflow.WorkflowPhase {
	return h.getWorkflowStatus(o).Current()
}

func (h *DefaultWorkflowStepReconcilerAction[k8sObject, k8sStepObject]) AdvancePhase(ctx context.Context, o k8sObject, to workflow.WorkflowPhase, logger *logrus.Entry) (res reconcile.Result, err error) {
	ws := h.getWorkflowStatus(o)
	ws.Advance(to)
	logger.Debugf("Workflow advanced to phase: %s", to)
	return reconcile.Result{}, nil
}

func (h *DefaultWorkflowStepReconcilerAction[k8sObject, k8sStepObject]) IsPhase(o k8sObject, phase workflow.WorkflowPhase) bool {
	return h.getWorkflowStatus(o).IsPhase(phase)
}

func (h *DefaultWorkflowStepReconcilerAction[k8sObject, k8sStepObject]) IsPhaseEmpty(o k8sObject) bool {
	return h.getWorkflowStatus(o).IsPhaseEmpty()
}
