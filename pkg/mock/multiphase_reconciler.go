package mock

import (
	"context"

	"github.com/disaster37/operator-sdk-extra/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	ctrl "sigs.k8s.io/controller-runtime"
)

type MockMultiPhaseReconcilerAction struct {
	reconciler controller.MultiPhaseReconcilerAction
	MockBase
}

func NewMockMultiPhaseReconcilerAction(reconciler controller.MultiPhaseReconcilerAction) controller.MultiPhaseReconcilerAction {
	return &MockMultiPhaseReconcilerAction{
		reconciler: reconciler,
		MockBase:   NewMockDefault(),
	}
}

// Configure permit to init condition on status
func (h *MockMultiPhaseReconcilerAction) Configure(ctx context.Context, req ctrl.Request, o object.MultiPhaseObject) (res ctrl.Result, err error) {
	h.MockBase.StartReconcile()
	return h.reconciler.Configure(ctx, req, o)
}

// Read permit to read kubernetes resources
func (h *MockMultiPhaseReconcilerAction) Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (res ctrl.Result, err error) {
	return h.reconciler.Read(ctx, o, data)
}

// Delete permit to delete resources on kubernetes
func (h *MockMultiPhaseReconcilerAction) Delete(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (err error) {
	return h.reconciler.Delete(ctx, o, data)
}

// OnError is call when error is throwing on current phase
// It the right way to set status condition when error
func (h *MockMultiPhaseReconcilerAction) OnError(ctx context.Context, o object.MultiPhaseObject, data map[string]any, currentErr error) (res ctrl.Result, err error) {
	res, err = h.reconciler.OnError(ctx, o, data, currentErr)
	h.MockBase.FinishReconcile()
	return res, err
}

// OnSuccess is call at the end of current phase, if not error
// It's the right way to set status condition when everithink is good
func (h *MockMultiPhaseReconcilerAction) OnSuccess(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (res ctrl.Result, err error) {
	res, err = h.reconciler.OnSuccess(ctx, o, data)
	h.MockBase.FinishReconcile()
	return res, err
}
