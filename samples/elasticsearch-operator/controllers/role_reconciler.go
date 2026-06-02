package controllers

import (
	"context"
	"time"

	eshandler "github.com/disaster37/es-handler/v8"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/remote"
	elasticsearchapicrd "github.com/disaster37/operator-sdk-extra/v2/samples/elasticsearch-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type roleReconciler struct {
	remote.RemoteReconcilerAction[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
}

func newRoleReconciler(c client.Client, recorder record.EventRecorder) remote.RemoteReconcilerAction[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler] {
	return &roleReconciler{
		RemoteReconcilerAction: remote.NewRemoteReconcilerAction[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler](
			c,
			recorder,
		),
	}
}

func (h *roleReconciler) GetRemoteHandler(ctx context.Context, req reconcile.Request, o *elasticsearchapicrd.Role, logger *logrus.Entry) (handler remote.RemoteExternalReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler], res reconcile.Result, err error) {
	role := o
	esClient, err := GetElasticsearchHandler(ctx, role, role.Spec.ElasticsearchRef, h.Client(), logger)
	if err != nil && role.DeletionTimestamp.IsZero() {
		return nil, res, err
	}

	if esClient == nil {
		return nil, reconcile.Result{RequeueAfter: 60 * time.Second}, nil
	}

	handler = newRoleApiClient(esClient)
	return handler, res, nil
}
