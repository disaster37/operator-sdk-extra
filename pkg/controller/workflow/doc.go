// Package workflow provides a multi-cycle saga abstraction on top of the
// multiphase reconciler pattern. A WorkflowStepReconcilerAction extends the
// standard step reconciler with a typed phase cursor persisted in a dedicated
// status sub-struct (not conditions) and a WaitForOwnedObjects primitive for
// convergence gating.
package workflow
