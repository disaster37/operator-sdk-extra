package workflow

import (
	"context"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/workflow"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/multiphase"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
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

// WorkflowStepReconcilerActionWithDiff is the diff variant of WorkflowStepReconcilerAction.
// It embeds the multiphase diff variant (SSA dry-run classification + optional OnDiff)
// and adds the same saga phase-management helpers as the simple variant.
type WorkflowStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] interface {
	multiphase.MultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]

	// CurrentPhase returns the current saga phase.
	CurrentPhase(o k8sObject) workflow.WorkflowPhase

	// AdvancePhase transitions the saga to the next phase.
	AdvancePhase(ctx context.Context, o k8sObject, to workflow.WorkflowPhase, logger *logrus.Entry) (res reconcile.Result, err error)

	// IsPhase returns true when the saga is at the given phase.
	IsPhase(o k8sObject, phase workflow.WorkflowPhase) bool

	// IsPhaseEmpty returns true when no phase has been set yet.
	IsPhaseEmpty(o k8sObject) bool
}

// workflowPhaseManager provides the saga phase-management helpers shared by
// both the simple and diff variants of DefaultWorkflowStepReconcilerAction.
type workflowPhaseManager[k8sObject object.MultiPhaseObject] struct{}

func (m *workflowPhaseManager[k8sObject]) getWorkflowStatus(o k8sObject) *workflow.WorkflowStatus {
	if ws, ok := any(o.GetStatus()).(WorkflowStatusGetter); ok {
		return ws.GetWorkflowStatus()
	}
	return &workflow.WorkflowStatus{}
}

func (m *workflowPhaseManager[k8sObject]) CurrentPhase(o k8sObject) workflow.WorkflowPhase {
	return m.getWorkflowStatus(o).Current()
}

func (m *workflowPhaseManager[k8sObject]) AdvancePhase(ctx context.Context, o k8sObject, to workflow.WorkflowPhase, logger *logrus.Entry) (res reconcile.Result, err error) {
	ws := m.getWorkflowStatus(o)
	ws.Advance(to)
	logger.Debugf("Workflow advanced to phase: %s", to)
	return reconcile.Result{}, nil
}

func (m *workflowPhaseManager[k8sObject]) IsPhase(o k8sObject, phase workflow.WorkflowPhase) bool {
	return m.getWorkflowStatus(o).IsPhase(phase)
}

func (m *workflowPhaseManager[k8sObject]) IsPhaseEmpty(o k8sObject) bool {
	return m.getWorkflowStatus(o).IsPhaseEmpty()
}

// DefaultWorkflowStepReconcilerAction is the default implementation of
// WorkflowStepReconcilerAction. It embeds a standard multiphase step action
// and adds workflow phase management on top.
type DefaultWorkflowStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] struct {
	*multiphase.DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]
	workflowPhaseManager[k8sObject]
}

// NewWorkflowStepReconcilerAction creates a new workflow step reconciler action.
func NewWorkflowStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object](
	c client.Client,
	phaseName shared.PhaseName,
	conditionName shared.ConditionName,
	recorder record.EventRecorder,
	fieldManager string,
) WorkflowStepReconcilerAction[k8sObject, k8sStepObject] {
	return &DefaultWorkflowStepReconcilerAction[k8sObject, k8sStepObject]{
		DefaultMultiPhaseStepReconcilerAction: multiphase.NewMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject](
			c,
			phaseName,
			conditionName,
			recorder,
			fieldManager,
		).(*multiphase.DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]),
	}
}

// DefaultWorkflowStepReconcilerActionWithDiff is the default implementation of
// WorkflowStepReconcilerActionWithDiff. It embeds a multiphase diff step action
// and adds the same workflow phase management as the simple variant.
type DefaultWorkflowStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] struct {
	*multiphase.DefaultMultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]
	workflowPhaseManager[k8sObject]
}

// NewWorkflowStepReconcilerActionWithDiff creates a new workflow step reconciler action
// using SSA dry-run diff classification.
func NewWorkflowStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObject client.Object](
	c client.Client,
	phaseName shared.PhaseName,
	conditionName shared.ConditionName,
	recorder record.EventRecorder,
	fieldManager string,
) WorkflowStepReconcilerActionWithDiff[k8sObject, k8sStepObject] {
	return &DefaultWorkflowStepReconcilerActionWithDiff[k8sObject, k8sStepObject]{
		DefaultMultiPhaseStepReconcilerActionWithDiff: multiphase.NewMultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject](
			c,
			phaseName,
			conditionName,
			recorder,
			fieldManager,
		).(*multiphase.DefaultMultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]),
	}
}
