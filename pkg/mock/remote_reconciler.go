package mock

import (
	"context"

	"github.com/disaster37/generic-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	ctrl "sigs.k8s.io/controller-runtime"
)

type MockRemoteReconcilerAction[k8sObject comparable, apiObject comparable] struct {
	reconciler        controller.RemoteReconcilerAction[k8sObject, apiObject]
	mockRemoteHandler func(ctx context.Context, req ctrl.Request, o object.RemoteObject) (handler controller.RemoteExternalReconciler[k8sObject, apiObject], res ctrl.Result, err error)
}

func NewMockRemoteReconcilerAction[k8sObject comparable, apiObject comparable](reconciler controller.RemoteReconcilerAction[k8sObject, apiObject], mockRemoteHandler func(ctx context.Context, req ctrl.Request, o object.RemoteObject) (handler controller.RemoteExternalReconciler[k8sObject, apiObject], res ctrl.Result, err error)) controller.RemoteReconcilerAction[k8sObject, apiObject] {
	return &MockRemoteReconcilerAction[k8sObject, apiObject]{
		reconciler:        reconciler,
		mockRemoteHandler: mockRemoteHandler,
	}
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject]) GetRemoteHandler(ctx context.Context, req ctrl.Request, o object.RemoteObject) (handler controller.RemoteExternalReconciler[k8sObject, apiObject], res ctrl.Result, err error) {
	return h.mockRemoteHandler(ctx, req, o)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject]) Configure(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject]) (res ctrl.Result, err error) {
	return h.reconciler.Configure(ctx, o, data, handler)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject]) Read(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject]) (read controller.RemoteRead[apiObject], res ctrl.Result, err error) {
	return h.reconciler.Read(ctx, o, data, handler)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject]) Create(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject], object apiObject) (res ctrl.Result, err error) {
	return h.reconciler.Create(ctx, o, data, handler, object)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject]) Update(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject], object apiObject) (res ctrl.Result, err error) {
	return h.reconciler.Update(ctx, o, data, handler, object)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject]) Delete(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject]) (err error) {
	return h.reconciler.Delete(ctx, o, data, handler)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject]) OnError(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject], currentErr error) (res ctrl.Result, err error) {
	return h.reconciler.OnError(ctx, o, data, handler, currentErr)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject]) OnSuccess(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject], diff controller.RemoteDiff[apiObject]) (res ctrl.Result, err error) {
	return h.reconciler.OnSuccess(ctx, o, data, handler, diff)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject]) Diff(ctx context.Context, o object.RemoteObject, read controller.RemoteRead[apiObject], data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject], ignoreDiff ...patch.CalculateOption) (diff controller.RemoteDiff[apiObject], res ctrl.Result, err error) {
	return h.reconciler.Diff(ctx, o, read, data, handler, ignoreDiff...)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject]) GetIgnoresDiff() []patch.CalculateOption {
	return h.reconciler.GetIgnoresDiff()
}
