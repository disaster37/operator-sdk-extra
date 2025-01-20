package mock

import (
	"context"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/remote"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/object"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type MockRemoteReconcilerAction[k8sObject object.RemoteObject, apiObject comparable, apiClient any] struct {
	remote.RemoteReconcilerAction[k8sObject, apiObject, apiClient]
	mockRemoteHandler func(ctx context.Context, req reconcile.Request, o k8sObject, logger *logrus.Entry) (handler remote.RemoteExternalReconciler[k8sObject, apiObject, apiClient], res reconcile.Result, err error)
}

func NewMockRemoteReconcilerAction[k8sObject object.RemoteObject, apiObject comparable, apiClient any](reconciler remote.RemoteReconcilerAction[k8sObject, apiObject, apiClient], mockRemoteHandler func(ctx context.Context, req reconcile.Request, o k8sObject, logger *logrus.Entry) (handler remote.RemoteExternalReconciler[k8sObject, apiObject, apiClient], res reconcile.Result, err error)) remote.RemoteReconcilerAction[k8sObject, apiObject, apiClient] {
	return &MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]{
		RemoteReconcilerAction: reconciler,
		mockRemoteHandler:      mockRemoteHandler,
	}
}

func (h *MockRemoteReconcilerAction[k8sObject, apiObject, apiClient]) GetRemoteHandler(ctx context.Context, req reconcile.Request, o k8sObject, logger *logrus.Entry) (handler remote.RemoteExternalReconciler[k8sObject, apiObject, apiClient], res reconcile.Result, err error) {
	return h.mockRemoteHandler(ctx, req, o, logger)
}
