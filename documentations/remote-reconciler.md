# Remote reconciler

 > Use it when you need to handle some external resources from your own CRD (remote API call).

 In this scenario, we will create operator that permit to create role on Elasticsearch cluster from our CRD. To do that, it will use the Elasticsearch API to reconcile the role on Elasticsearch.

  The source code of the following sample is on `testdata/elasticsearch-operator`. The sample is fully implemented compared to this documentation.

 ## How to do

 ### Bootstrap new operator

```bash
operator-sdk init --domain=example.com --repo=github.com/disaster37/operator-sdk-extra/v2/testdata/elasticsearch-operator

operator-sdk create api --group elasticsearchapi --version v1alpha1 --kind Role --resource --controller
```

### Implement CRD

You need to edit your role CRD to add the fields you need and to implement the interface  `object.RemoteObject`

**api/v1alpha1/role_types.go**
```golang
/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RoleSpec defines the desired state of Role
// +k8s:openapi-gen=true
type RoleSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ElasticsearchRef is the Elasticsearch ref to connect on.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	ElasticsearchRef ElasticsearchRef `json:"elasticsearchRef"`

	// Name is the custom role name
	// If empty, it use the ressource name
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	Name string `json:"name,omitempty"`

	// Cluster is a list of cluster privileges
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	Cluster []string `json:"cluster,omitempty"`

	// Indices is the list of indices permissions
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	Indices []RoleSpecIndicesPermissions `json:"indices,omitempty"`

	// Applications is the list of application privilege
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	Applications []RoleSpecApplicationPrivileges `json:"applications,omitempty"`

	// RunAs is the list of users that the owners of this role can impersonate
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	RunAs []string `json:"runAs,omitempty"`

	// Global  defining global privileges
	// JSON string
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	Global string `json:"global,omitempty"`

	// Metadata is optional meta-data
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// JSON string
	// +optional
	Metadata string `json:"metadata,omitempty"`

	// TransientMetadata
	// JSON string
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	TransientMetadata string `json:"transientMetadata,omitempty"`
}

// ElasticsearchRoleSpecApplicationPrivileges is the application privileges object
type RoleSpecApplicationPrivileges struct {

	// Application
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Application string `json:"application"`

	// Privileges
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	Privileges []string `json:"privileges,omitempty"`

	// Resources
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	Resources []string `json:"resources,omitempty"`
}

// RoleSpecIndicesPermissions is the indices permission object
type RoleSpecIndicesPermissions struct {

	// Names
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Names []string `json:"names"`

	// Privileges
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Privileges []string `json:"privileges"`

	// FieldSecurity
	// JSON string
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	FieldSecurity string `json:"fieldSecurity,omitempty"`

	// Query
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	Query string `json:"query,omitempty"`

	// Allow to manage restricted index
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	AllowRestrictedIndices bool `json:"allowRestrictedIndices,omitempty"`
}

type ElasticsearchRef struct {

	// ManagedElasticsearchRef is the managed Elasticsearch cluster by operator
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	ManagedElasticsearchRef *ElasticsearchManagedRef `json:"managed,omitempty"`

	// ExternalElasticsearchRef is the external Elasticsearch cluster not managed by operator
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	ExternalElasticsearchRef *ElasticsearchExternalRef `json:"external,omitempty"`

	// ElasticsearchCaSecretRef is the secret that store your custom CA certificate to connect on Elasticsearch API.
	// It need to have the following keys: ca.crt
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	ElasticsearchCaSecretRef *corev1.LocalObjectReference `json:"elasticsearchCASecretRef,omitempty"`
}

type ElasticsearchManagedRef struct {

	// Name is the Elasticsearch cluster deployed by operator
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Name string `json:"name"`

	// Namespace is the namespace where Elasticsearch is deployed by operator
	// No need to set if Kibana is deployed on the same namespace
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// TargetNodeGroup is the target Elasticsearch node group to use as service to connect on Elasticsearch
	// Default, it use the global service
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	TargetNodeGroup string `json:"targetNodeGroup,omitempty"`
}

type ElasticsearchExternalRef struct {

	// Addresses is the list of Elasticsearch addresses
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Addresses []string `json:"addresses"`

	// SecretName is the secret that contain the setting to connect on Elasticsearch that is not managed by ECK.
	// It need to contain the keys `username` and `password`.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SecretRef *corev1.LocalObjectReference `json:"secretRef"`
}

// IsManaged permit to know if Elasticsearch is managed by operator
func (h ElasticsearchRef) IsManaged() bool {
	return h.ManagedElasticsearchRef != nil && h.ManagedElasticsearchRef.Name != ""
}

// IsExternal permit to know if Elasticsearch is external (not managed by operator)
func (h ElasticsearchRef) IsExternal() bool {
	return h.ExternalElasticsearchRef != nil && len(h.ExternalElasticsearchRef.Addresses) > 0
}


// RoleStatus defines the observed state of Role
type RoleStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	apis.BasicRemoteObjectStatus `json:",inline"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

// Role is the Schema for the roles API
// +kubebuilder:printcolumn:name="Sync",type="boolean",JSONPath=".status.isSync"
// +kubebuilder:printcolumn:name="Error",type="boolean",JSONPath=".status.isOnError",description="Is on error"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status",description="health"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Role struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoleSpec   `json:"spec,omitempty"`
	Status RoleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RoleList contains a list of Role
type RoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Role `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Role{}, &RoleList{})
}

```

Some explains:
- We expose for user some fields to configure expected Elasticsearch role (RoleSpec)
- We use regular status defined by framework and needed by the standard reconciler `apis.BasicRemoteObjectStatus`
- We display some fields when you get role resource from kubectl with annotation `+kubebuilder:printcolumn:name`

**api/v1alpha1/role_func.go**
```golang
package v1alpha1

import "github.com/disaster37/operator-sdk-extra/v2/pkg/object"

// GetStatus return the status object
func (o *Role) GetStatus() object.RemoteObjectStatus {
	return &o.Status
}

// GetExternalName return the role name
// If name is empty, it use the ressource name
func (o *Role) GetExternalName() string {
	if o.Spec.Name == "" {
		return o.Name
	}

	return o.Spec.Name
}


```

Some explains:
- We implement the interface `object.RemoteObject` to get our Status object.


### Implement Remote Controller

Like we say, our controller need to reconcile expected role on Elasticsearch cluster. To to that, we need to create, update, delete, and get role on Elasticsearch from API.

Then we need to orchestrate the reconcilation:
  - Generate expected role
  - Get the current role
  - Diff the expected and current role
  - Apply actions if needed: create, update or delete

So we need to implement
  - the `controller.RemoteExternalReconciler` interface
  - the `controller.RemoteReconcilerAction` interface
  - the `controller.RemoteReconciler` interface

#### Remote External reconciler

We start to create the remoteExternalReconciler dediceted to reconcile role on Elasticsearch API. The goal is to speak with Elasticsearch API


**controllers/role_external_reconciler.go**
```golang
package controllers

import (
	"encoding/json"

	eshandler "github.com/disaster37/es-handler/v8"
	"github.com/disaster37/generic-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	elasticsearchapicrd "github.com/disaster37/operator-sdk-extra/v2/testdata/elasticsearch-operator/api/v1alpha1"
)

type roleApiClient struct {
	*controller.BasicRemoteExternalReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
}

func newRoleApiClient(client eshandler.ElasticsearchHandler) controller.RemoteExternalReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler] {
	return &roleApiClient{
		BasicRemoteExternalReconciler: controller.NewBasicRemoteExternalReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler](client),
	}
}

func (h *roleApiClient) Build(o *elasticsearchapicrd.Role) (role *eshandler.XPackSecurityRole, err error) {

	role = &eshandler.XPackSecurityRole{
		Cluster: o.Spec.Cluster,
		RunAs:   o.Spec.RunAs,
	}

	if o.Spec.Global != "" {
		global := make(map[string]any)
		if err := json.Unmarshal([]byte(o.Spec.Global), &global); err != nil {
			return nil, err
		}
		role.Global = global
	}

	if o.Spec.Metadata != "" {
		meta := make(map[string]any)
		if err := json.Unmarshal([]byte(o.Spec.Metadata), &meta); err != nil {
			return nil, err
		}
		role.Metadata = meta
	}

	if o.Spec.TransientMetadata != "" {
		tm := make(map[string]any)
		if err := json.Unmarshal([]byte(o.Spec.TransientMetadata), &tm); err != nil {
			return nil, err
		}
		role.TransientMetadata = tm
	}

	if o.Spec.Applications != nil {
		role.Applications = make([]eshandler.XPackSecurityApplicationPrivileges, 0, len(o.Spec.Applications))
		for _, application := range o.Spec.Applications {
			role.Applications = append(role.Applications, eshandler.XPackSecurityApplicationPrivileges{
				Application: application.Application,
				Privileges:  application.Privileges,
				Resources:   application.Resources,
			})
		}
	}

	if o.Spec.Indices != nil {
		role.Indices = make([]eshandler.XPackSecurityIndicesPermissions, 0, len(o.Spec.Indices))
		for _, indice := range o.Spec.Indices {
			i := eshandler.XPackSecurityIndicesPermissions{
				Names:                  indice.Names,
				Privileges:             indice.Privileges,
				Query:                  indice.Query,
				AllowRestrictedIndices: indice.AllowRestrictedIndices,
			}
			if indice.FieldSecurity != "" {
				fs := make(map[string]any)
				if err := json.Unmarshal([]byte(indice.FieldSecurity), &fs); err != nil {
					return nil, err
				}
				i.FieldSecurity = fs
			}
			role.Indices = append(role.Indices, i)
		}
	}

	return role, nil
}

func (h *roleApiClient) Get(o *elasticsearchapicrd.Role) (object *eshandler.XPackSecurityRole, err error) {
	return h.Client().RoleGet(o.GetExternalName())
}

func (h *roleApiClient) Create(object *eshandler.XPackSecurityRole, o *elasticsearchapicrd.Role) (err error) {
	return h.Client().RoleUpdate(o.GetExternalName(), object)
}

func (h *roleApiClient) Update(object *eshandler.XPackSecurityRole, o *elasticsearchapicrd.Role) (err error) {
	return h.Client().RoleUpdate(o.GetExternalName(), object)

}

func (h *roleApiClient) Delete(o *elasticsearchapicrd.Role) (err error) {
	return h.Client().RoleDelete(o.GetExternalName())

}

```

Some explains:
  - The `Build` method permit to generate expected Elasticsearch role
  - The `Get` method permit to read the actual role from Elasticsearch
  - The `Create` method permit to create the role on Elasticsearch
  - The `Update` method permit to update the role on Elasticsearch
  - The `Delete` method permit to delete the role on Elasticsearch
  - The `Diff` is not specified is this sample because we use the stand diff that are already implemented by `controller.BasicRemoteExternalReconciler`


### Implement the remote reconciler action

> It's standard reconciler. The major part of code is already implemented by  `controller.BasicRemoteReconcilerAction`.

**controllers/role_reconciler.go**
```golang
package controllers

import (
	"context"
	"time"

	eshandler "github.com/disaster37/es-handler/v8"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/object"
	elasticsearchapicrd "github.com/disaster37/operator-sdk-extra/v2/testdata/elasticsearch-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type roleReconciler struct {
	controller.RemoteReconcilerAction[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
	controller.BaseReconciler
}

func newRoleReconciler(client client.Client, logger *logrus.Entry, recorder record.EventRecorder) controller.RemoteReconcilerAction[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler] {
	return &roleReconciler{
		RemoteReconcilerAction: controller.NewRemoteReconcilerAction[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler](
			client,
			logger,
			recorder,
		),
		BaseReconciler: controller.BaseReconciler{
			Client:   client,
			Log:      logger,
			Recorder: recorder,
		},
	}
}

func (h *roleReconciler) GetRemoteHandler(ctx context.Context, req ctrl.Request, o object.RemoteObject) (handler controller.RemoteExternalReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler], res ctrl.Result, err error) {
	role := o.(*elasticsearchapicrd.Role)
	esClient, err := GetElasticsearchHandler(ctx, role, role.Spec.ElasticsearchRef, h.BaseReconciler.Client, h.BaseReconciler.Log)
	if err != nil && role.DeletionTimestamp.IsZero() {
		return nil, res, err
	}

	// Elastic not ready
	if esClient == nil {
		return nil, ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	handler = newRoleApiClient(esClient)

	return handler, res, nil
}

```

Some explain:
  - The `GetRemoteHandler` method permit to get handler that permit to call with Elasticsearch API and instantiate the the external reconsiler with it.

### Implement the main reconciler

The goal of this reconciler is to get the object that wake up operator, maintain some standard status and reconcile resource on Elasticsearch cluster.

It need to implement the `controller.RemoteReconciler` and `controller.RemoteReconcilerAction` interface. You can use standard RemoteReconciler constructor.

**controllers/role_controller.go**:
```golang
//*
Copyright 2022.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"

	eshandler "github.com/disaster37/es-handler/v8"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	elasticsearchapicrd "github.com/disaster37/operator-sdk-extra/v2/testdata/elasticsearch-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	roleName string = "role"
)

// RoleReconciler reconciles a Role object
type RoleReconciler struct {
	controller.Controller
	controller.RemoteReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
	reconcilerAction controller.RemoteReconcilerAction[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
	name             string
}

func NewRoleReconciler(client client.Client, logger *logrus.Entry, recorder record.EventRecorder) controller.Controller {

	r := &RoleReconciler{
		Controller: controller.NewBasicController(),
		RemoteReconciler: controller.NewBasicRemoteReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler](
			client,
			roleName,
			"role.elasticsearchapi.k8s.webcenter.fr/finalizer",
			logger,
			recorder,
		),
		reconcilerAction: newRoleReconciler(
			client,
			logger,
			recorder,
		),
		name: roleName,
	}

	return r
}

//+kubebuilder:rbac:groups=elasticsearchapi.k8s.webcenter.fr,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=elasticsearchapi.k8s.webcenter.fr,resources=roles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=elasticsearchapi.k8s.webcenter.fr,resources=roles/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=patch;get;create
//+kubebuilder:rbac:groups="elasticsearch.k8s.webcenter.fr",resources=elasticsearches,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the License object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *RoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	role := &elasticsearchapicrd.Role{}
	data := map[string]any{}

	return r.RemoteReconciler.Reconcile(
		ctx,
		req,
		role,
		data,
		r.reconcilerAction,
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elasticsearchapicrd.Role{}).
		Complete(r)
}

```

Some explain:
- The struct `RoleReconciler` need to implement the interface `controller.RemoteReconciler` and  `controller.Controller`. We use the standard implementation of this interface via `controller.NewBasicController` and `controller.NewBasicRemoteReconciler`
- We instanciate our `controller.RemoteReconcilerAction` with `newRoleReconciler`.

### Call reconciler from main

We just need to create our custom main multiphase reconciler from the main and call the setup manager.

**main.go**:
```golang
/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	elasticsearchapiv1alpha1 "github.com/disaster37/operator-sdk-extra/v2/testdata/elasticsearch-operator/api/v1alpha1"
	"github.com/disaster37/operator-sdk-extra/v2/testdata/elasticsearch-operator/controllers"
	"github.com/sirupsen/logrus"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(elasticsearchapiv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "7d8bc9de.example.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	memcachedReconciler := controllers.NewRoleReconciler(
		mgr.GetClient(),
		logrus.NewEntry(log),
		mgr.GetEventRecorderFor("role-controller"),
	)
	if err = memcachedReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Role")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}


```