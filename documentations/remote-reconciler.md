# Remote Reconciler

> Use it when you need to manage external/remote resources via API calls (not K8s resources) from your own CRD.

## Overview

- Use case: your CRD manages external resources (Elasticsearch, DB, cloud API, etc.) from Kubernetes.
- The framework handles the reconciliation loop: finalizer management, 3-way diff, status tracking, and error handling.

```
                        +-----------+
                        | Get CRD   |
                        +-----+-----+
                              |
                    +---------v----------+
                    | Add Finalizer      |
                    | Track Status       |
                    +---------+----------+
                              |
                   +----------v-----------+
                   | GetRemoteHandler()   |
                   | (get API client)     |
                   +----------+-----------+
                              |
                   +----------v-----------+
                   | Configure()          |
                   | (optional init)      |
                   +----------+-----------+
                              |
                   +----------v-----------+
                   | Read()               |
                   | Build() + Get()      |
                   +----------+-----------+
                              |
                  +-----------v-----------+
                  | Is Deleting?          |
                  | -> Delete()           |
                  | -> Remove Finalizer   |
                  +-----------+-----------+
                              |
                   +----------v-----------+
                   | Diff()               |
                   | 3-way merge patch    |
                   +----+----------+------+
                        |          |
              +---------v--+  +---v---------+
              | NeedCreate |  | NeedUpdate   |
              | -> Create  |  | -> Update    |
              +-----+------+  +------+------+
                    |                |
                    +-------+--------+
                            |
                  +---------v---------+
                  | OnSuccess()       |
                  | Store LastApplied |
                  +---------+---------+
                            |
                         +--v--+
                         | END |
                         +-----+
```

## Sample: Elasticsearch Role Operator

The CRD `Role` manages Elasticsearch roles via the Elasticsearch REST API.

Full source code: `samples/elasticsearch-operator/`.

---

## 1. Define the CRD

### `api/v1alpha1/role_types.go`

```go
package v1alpha1

import (
	remoteapis "github.com/disaster37/operator-sdk-extra/v2/pkg/apis/remote"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RoleSpec struct {
	ElasticsearchRef  ElasticsearchRef                    `json:"elasticsearchRef"`
	Name              string                              `json:"name,omitempty"`
	Cluster           []string                            `json:"cluster,omitempty"`
	Indices           []RoleSpecIndicesPermissions        `json:"indices,omitempty"`
	Applications      []RoleSpecApplicationPrivileges     `json:"applications,omitempty"`
	RunAs             []string                            `json:"runAs,omitempty"`
	Global            string                              `json:"global,omitempty"`
	Metadata          string                              `json:"metadata,omitempty"`
	TransientMetadata string                              `json:"transientMetadata,omitempty"`
}

type RoleStatus struct {
	remoteapis.DefaultRemoteObjectStatus `json:",inline"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion
//+kubebuilder:printcolumn:name="Sync",type="boolean",JSONPath=".status.isSync"
//+kubebuilder:printcolumn:name="Error",type="boolean",JSONPath=".status.isOnError"
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Role struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   RoleSpec   `json:"spec,omitempty"`
	Status RoleStatus `json:"status,omitempty"`
}
```

Key points:
- `remoteapis.DefaultRemoteObjectStatus` embedded inline in the status provides `IsSync`, `LastAppliedConfiguration`, conditions, and error tracking.

### `api/v1alpha1/role_func.go`

```go
package v1alpha1

import "github.com/disaster37/operator-sdk-extra/v2/pkg/object"

func (o *Role) GetStatus() object.RemoteObjectStatus {
	return &o.Status
}

func (o *Role) GetExternalName() string {
	if o.Spec.Name == "" {
		return o.Name
	}
	return o.Spec.Name
}
```

Implements `object.RemoteObject`:
- `GetStatus()` returns the status object implementing `object.RemoteObjectStatus`.
- `GetExternalName()` returns the name used on the remote API (falls back to the K8s resource name).

---

## 2. Implement the external API client

### `controllers/role_external_reconciler.go`

```go
package controllers

import (
	"encoding/json"

	eshandler "github.com/disaster37/es-handler/v8"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/remote"
	elasticsearchapicrd "github.com/disaster37/operator-sdk-extra/v2/samples/elasticsearch-operator/api/v1alpha1"
)

type roleApiClient struct {
	remote.RemoteExternalReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
}

func newRoleApiClient(client eshandler.ElasticsearchHandler) remote.RemoteExternalReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler] {
	return &roleApiClient{
		RemoteExternalReconciler: remote.NewRemoteExternalReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler](client),
	}
}

func (h *roleApiClient) Build(o *elasticsearchapicrd.Role) (role *eshandler.XPackSecurityRole, err error) {
	role = &eshandler.XPackSecurityRole{
		Cluster: o.Spec.Cluster,
		RunAs:   o.Spec.RunAs,
	}
	// ... parse JSON fields (Global, Metadata, TransientMetadata, Applications, Indices)
	return role, nil
}

func (h *roleApiClient) Get(o *elasticsearchapicrd.Role) (*eshandler.XPackSecurityRole, error) {
	return h.Client().RoleGet(o.GetExternalName())
}

func (h *roleApiClient) Create(object *eshandler.XPackSecurityRole, o *elasticsearchapicrd.Role) error {
	return h.Client().RoleUpdate(o.GetExternalName(), object)
}

func (h *roleApiClient) Update(object *eshandler.XPackSecurityRole, o *elasticsearchapicrd.Role) error {
	return h.Client().RoleUpdate(o.GetExternalName(), object)
}

func (h *roleApiClient) Delete(o *elasticsearchapicrd.Role) error {
	return h.Client().RoleDelete(o.GetExternalName())
}
```

Key points:
- Embeds `remote.RemoteExternalReconciler[k8s, api, client]` which provides the default `Diff()` (3-way merge) and `Client()`.
- You must implement: `Build`, `Get`, `Create`, `Update`, `Delete`.
- Constructor: `remote.NewRemoteExternalReconciler[k8s, api, client](handler)`.

---

## 3. Implement the remote reconciler action

### `controllers/role_reconciler.go`

```go
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
	esClient, err := GetElasticsearchHandler(ctx, o, o.Spec.ElasticsearchRef, h.Client(), logger)
	if err != nil && o.DeletionTimestamp.IsZero() {
		return nil, res, err
	}
	if esClient == nil {
		return nil, reconcile.Result{RequeueAfter: 60 * time.Second}, nil
	}
	handler = newRoleApiClient(esClient)
	return handler, res, nil
}
```

Key points:
- Embeds `remote.RemoteReconcilerAction[k8s, api, client]` which provides default implementations for `Configure`, `Read`, `Create`, `Update`, `Delete`, `Diff`, `OnSuccess`, `OnError`.
- Override `GetRemoteHandler()` to instantiate and return the external API client.
- Signature: `GetRemoteHandler(ctx, req, o *Role, logger)` -- note that `o` is directly typed to your CRD.

---

## 4. Implement the main reconciler

### `controllers/role_controller.go`

```go
package controllers

import (
	"context"

	eshandler "github.com/disaster37/es-handler/v8"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/remote"
	elasticsearchapicrd "github.com/disaster37/operator-sdk-extra/v2/samples/elasticsearch-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	roleFinalizer shared.FinalizerName = "role.elasticsearchapi.k8s.webcenter.fr/finalizer"
	roleName      string               = "role"
)

type RoleReconciler struct {
	controller.Controller
	remote.RemoteReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
	reconcilerAction remote.RemoteReconcilerAction[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
	name             string
}

func NewRoleReconciler(c client.Client, logger *logrus.Entry, recorder record.EventRecorder) controller.Controller {
	return &RoleReconciler{
		Controller: controller.NewController(),
		RemoteReconciler: remote.NewRemoteReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler](
			c,
			roleName,
			roleFinalizer,
			logger,
			recorder,
		),
		reconcilerAction: newRoleReconciler(c, recorder),
		name:             roleName,
	}
}

func (r *RoleReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	o := &elasticsearchapicrd.Role{}
	data := map[string]any{}
	return r.RemoteReconciler.Reconcile(ctx, req, o, data, r.reconcilerAction)
}

func (r *RoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elasticsearchapicrd.Role{}).
		Complete(r)
}
```

Key points:
- Embeds `remote.RemoteReconciler[k8s, api, client]` as the orchestrator.
- Constructor: `remote.NewRemoteReconciler[k8s, api, client](client, name, finalizer, logger, recorder)`.
- The `data` map is passed through all action methods for sharing state between steps.

---

## 5. Wire up in main.go

```go
log := logrus.New()
log.SetLevel(logrus.DebugLevel)

roleReconciler := controllers.NewRoleReconciler(
    mgr.GetClient(),
    logrus.NewEntry(log),
    mgr.GetEventRecorderFor("role-controller"),
)
if err = roleReconciler.SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "Role")
    os.Exit(1)
}
```

---

## Reconciliation flow

1. **Get CRD** from Kubernetes, add finalizer if missing, track status.
2. **`GetRemoteHandler()`** -- obtain the external API client (e.g. Elasticsearch connection).
3. **`Configure()`** -- optional initialization (e.g. init conditions on status). Default is no-op.
4. **`Read()`** -- internally calls `handler.Build()` to build expected object, then `handler.Get()` to fetch current state from remote API.
5. **If deleting**: call `handler.Delete()`, remove finalizer, return.
6. **`Diff()`** -- 3-way merge using `lastAppliedConfiguration` as the original. Detects real spec changes without being polluted by remote state mutations.
7. **If `NeedCreate()`**: call `handler.Create()`, store `lastAppliedConfiguration`.
8. **If `NeedUpdate()`**: call `handler.Update()`, store `lastAppliedConfiguration`.
9. **`OnSuccess()`** -- called if no error. Set status conditions, update CRD status.
10. **`OnError()`** -- called on any error. Set error conditions on status.

---

## Key interfaces

| Interface | Description |
|---|---|
| `remote.RemoteReconciler[k8s, api, client]` | Main orchestrator. Manages finalizer, status tracking, and calls the action methods in order. |
| `remote.RemoteReconcilerAction[k8s, api, client]` | Defines the reconciliation steps: `GetRemoteHandler`, `Configure`, `Read`, `Create`, `Update`, `Delete`, `Diff`, `OnSuccess`, `OnError`, `GetIgnoresDiff`. |
| `remote.RemoteExternalReconciler[k8s, api, client]` | External API client wrapper. Methods: `Build`, `Get`, `Create`, `Update`, `Delete`, `Diff`, `Client`. |
| `remote.RemoteRead[api]` | Stores current and expected API objects after `Read()`. |
| `remote.RemoteDiff[api]` | Holds patch result. `NeedCreate()`, `NeedUpdate()`, `GetObjectToCreate()`, `GetObjectToUpdate()`, `Diff()`. |
| `object.RemoteObject` | CRD interface: `GetStatus() RemoteObjectStatus` + `GetExternalName() string`. |
| `object.RemoteObjectStatus` | CRD status interface: `GetIsSync()`, `SetIsSync()`, `GetLastAppliedConfiguration()`, `SetLastAppliedConfiguration()`. |

---

## Constructors

| Constructor | Description |
|---|---|
| `remote.NewRemoteReconciler[k8s, api, client](client, name, finalizer, logger, recorder)` | Main orchestrator with finalizer and status management. |
| `remote.NewRemoteReconcilerAction[k8s, api, client](client, recorder)` | Default action implementation. Override `GetRemoteHandler` at minimum. |
| `remote.NewRemoteExternalReconciler[k8s, api, client](handler)` | API client wrapper. Provides default `Diff()` via 3-way merge. You must implement `Build`, `Get`, `Create`, `Update`, `Delete`. |

---

## 3-way Diff and LastAppliedConfiguration

The framework uses a 3-way merge patch strategy (similar to `kubectl apply`):

- After a successful `Create` or `Update`, the expected object is serialized, compressed (zip+base64), and stored in `status.lastAppliedConfiguration`.
- On the next reconciliation, `Diff()` uses three inputs:
  - **current**: the object fetched from the remote API via `Get()`.
  - **expected**: the object built from the CRD spec via `Build()`.
  - **original**: the deserialized `lastAppliedConfiguration`.
- This detects real spec changes made by the user, ignoring mutations introduced by the remote API (e.g. default values, computed fields).
- If the diff is empty, no action is taken.
