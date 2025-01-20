package remote_test

import (
	"context"

	eshandler "github.com/disaster37/es-handler/v8"
	"github.com/disaster37/generic-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/remote"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	name      string               = "test"
	finalizer shared.FinalizerName = "test.operator.webcenter.fr/finalizer"
)

/*************
* Controller
 */
type TestReconciler struct {
	controller.Controller
	remote.RemoteReconciler[*RemoteObject, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
	remote.RemoteReconcilerAction[*RemoteObject, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
	name string
}

func NewTestReconciler(client client.Client, logger *logrus.Entry, recorder record.EventRecorder) controller.Controller {
	return &TestReconciler{
		Controller: controller.NewController(),
		RemoteReconciler: remote.NewRemoteReconciler[*RemoteObject, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler](
			client,
			name,
			finalizer,
			logger,
			recorder,
		),
		RemoteReconcilerAction: newTestReconciler(client, recorder),
		name:                   name,
	}
}

func (r *TestReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	o := &RemoteObject{}
	data := map[string]any{}

	return r.RemoteReconciler.Reconcile(
		ctx,
		req,
		o,
		data,
		r.RemoteReconcilerAction,
	)
}

func (r *TestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&RemoteObject{}).
		Complete(r)
}

/************
* remote reconciler
 */
type testReconciler struct {
	remote.RemoteReconcilerAction[*RemoteObject, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
}

func newTestReconciler(c client.Client, recorder record.EventRecorder) remote.RemoteReconcilerAction[*RemoteObject, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler] {
	return &testReconciler{
		RemoteReconcilerAction: remote.NewRemoteReconcilerAction[*RemoteObject, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler](
			c,
			recorder,
		),
	}
}

func (h *testReconciler) GetRemoteHandler(ctx context.Context, req reconcile.Request, o *RemoteObject, logger *logrus.Entry) (handler remote.RemoteExternalReconciler[*RemoteObject, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler], res reconcile.Result, err error) {
	return newRemoteObjectApiClient(nil), res, nil
}

/**************
* External API handler
 */

type remoteObjectApiClient struct {
	remote.RemoteExternalReconciler[*RemoteObject, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
}

func newRemoteObjectApiClient(client eshandler.ElasticsearchHandler) remote.RemoteExternalReconciler[*RemoteObject, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler] {
	return &remoteObjectApiClient{
		RemoteExternalReconciler: remote.NewRemoteExternalReconciler[*RemoteObject, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler](
			client,
		),
	}
}

func (h *remoteObjectApiClient) Build(o *RemoteObject) (role *eshandler.XPackSecurityRole, err error) {
	return &eshandler.XPackSecurityRole{}, nil
}

func (h *remoteObjectApiClient) Create(role *eshandler.XPackSecurityRole, o *RemoteObject) (err error) {
	return h.Client().RoleUpdate(o.Name, role)
}

func (h *remoteObjectApiClient) Update(role *eshandler.XPackSecurityRole, o *RemoteObject) (err error) {
	return h.Client().RoleUpdate(o.Name, role)
}

func (h *remoteObjectApiClient) Delete(o *RemoteObject) (err error) {
	return h.Client().RoleDelete(o.Name)
}

func (h *remoteObjectApiClient) Get(o *RemoteObject) (role *eshandler.XPackSecurityRole, err error) {
	return h.Client().RoleGet(o.Name)
}

func (h *remoteObjectApiClient) Diff(currentOject *eshandler.XPackSecurityRole, expectedObject *eshandler.XPackSecurityRole, originalObject *eshandler.XPackSecurityRole, k8sO *RemoteObject, ignoresDiff ...patch.CalculateOption) (patchResult *patch.PatchResult, err error) {
	return h.Client().RoleDiff(currentOject, expectedObject, originalObject)
}
