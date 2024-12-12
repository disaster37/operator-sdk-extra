package mock

import (
	"context"

	"github.com/disaster37/generic-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockRemoteReconcilerAction[k8sObject comparable, apiObject comparable, apiClient any] struct {
	reconciler        controller.RemoteReconcilerAction[k8sObject, apiObject, apiClient]
	mockRemoteHandler func(ctx context.Context, req ctrl.Request, o object.RemoteObject) (handler controller.RemoteExternalReconciler[k8sObject, apiObject, apiClient], res ctrl.Result, err error)
}

func NewMockRemoteReconcilerAction[k8sObject comparable, apiObject comparable, apiClient any](reconciler controller.RemoteReconcilerAction[k8sObject, apiObject, apiClient], mockRemoteHandler func(ctx context.Context, req ctrl.Request, o object.RemoteObject) (handler controller.RemoteExternalReconciler[k8sObject, apiObject, apiClient], res ctrl.Result, err error)) controller.RemoteReconcilerAction[k8sObject, apiObject, apiClient] {
	return &MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]{
		reconciler:        reconciler,
		mockRemoteHandler: mockRemoteHandler,
	}
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) GetRemoteHandler(ctx context.Context, req ctrl.Request, o object.RemoteObject) (handler controller.RemoteExternalReconciler[k8sObject, apiObject, apiClient], res ctrl.Result, err error) {
	return h.mockRemoteHandler(ctx, req, o)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Configure(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject, apiClient]) (res ctrl.Result, err error) {
	return h.reconciler.Configure(ctx, o, data, handler)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Read(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject, apiClient]) (read controller.RemoteRead[apiObject], res ctrl.Result, err error) {
	return h.reconciler.Read(ctx, o, data, handler)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Create(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject, apiClient], object apiObject) (res ctrl.Result, err error) {
	return h.reconciler.Create(ctx, o, data, handler, object)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Update(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject, apiClient], object apiObject) (res ctrl.Result, err error) {
	return h.reconciler.Update(ctx, o, data, handler, object)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Delete(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject, apiClient]) (err error) {
	return h.reconciler.Delete(ctx, o, data, handler)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) OnError(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject, apiClient], currentErr error) (res ctrl.Result, err error) {
	return h.reconciler.OnError(ctx, o, data, handler, currentErr)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) OnSuccess(ctx context.Context, o object.RemoteObject, data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject, apiClient], diff controller.RemoteDiff[apiObject]) (res ctrl.Result, err error) {
	return h.reconciler.OnSuccess(ctx, o, data, handler, diff)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Diff(ctx context.Context, o object.RemoteObject, read controller.RemoteRead[apiObject], data map[string]any, handler controller.RemoteExternalReconciler[k8sObject, apiObject, apiClient], ignoreDiff ...patch.CalculateOption) (diff controller.RemoteDiff[apiObject], res ctrl.Result, err error) {
	return h.reconciler.Diff(ctx, o, read, data, handler, ignoreDiff...)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) GetIgnoresDiff() []patch.CalculateOption {
	return h.reconciler.GetIgnoresDiff()
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Logger() *logrus.Entry {
	return h.reconciler.Logger()
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) WithLoggerFields(fields logrus.Fields) {
	h.reconciler.WithLoggerFields(fields)
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Client() client.Client {
	return h.reconciler.Client()
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) Recorder() record.EventRecorder {
	return h.reconciler.Recorder()
}
