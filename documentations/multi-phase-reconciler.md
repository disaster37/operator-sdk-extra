# Multi phase reconciler

 > Use it when you need to handle some K8s resources from your own CRD (configmap, deployment, ingress, etc.)

 In this scenario, we will deploy memcached from our custom CRD. It's the same sample like `operator-sdk`.
 Our CRD permit to set the number of replica and the port or memcached.
 The controller will create and handle one `configmap` and one `deployement` to illustrate the multi phase of reconcilation.

 The source code of the following sample is on `testdata/memcached-operator`. The sample is fully implemented compared to this documentation.

 ## How to do

 ### Bootstrap new operator

```bash
operator-sdk init --domain=example.com --repo=github.com/disaster37/operator-sdk-extra/testdata/memcached-operator

operator-sdk create api --group cache --version v1alpha1 --kind Memcached --resource --controller
```

### Implement CRD

You need to edit your memcached CRD to add the fields you need and to implement the interface  `object.MultiPhaseObject`

**api/v1alpha1/memcached_types.go**
```golang
// MemcachedSpec defines the desired state of Memcached
type MemcachedSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3
	// +kubebuilder:validation:ExclusiveMaximum=false

	// Size defines the number of Memcached instances
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Size int32 `json:"size,omitempty"`

	// Port defines the port that will be used to init the container with the image
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	ContainerPort int32 `json:"containerPort,omitempty"`
}

type MemcachedStatus struct {
	apis.BasicMultiPhaseObjectStatus `json:",inline"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1,memcached-deployment},{ConfigMap,v1,memcached-configmap}}
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Cluster deployment status"
// +kubebuilder:printcolumn:name="Error",type="boolean",JSONPath=".status.isOnError",description="Is on error"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='MemcachedReady')].status",description="Cluster health"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Memcached is the Schema for the memcacheds API
type Memcached struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MemcachedSpec   `json:"spec,omitempty"`
	Status MemcachedStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MemcachedList contains a list of Memcached
type MemcachedList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Memcached `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Memcached{}, &MemcachedList{})
}
```

Some explains:
- We expose for user 2 fields: size and containerPort
- We use regular status defined by framework and needed by the standard reconciler `apis.BasicMultiPhaseObjectStatus`
- We display some fields when you get memcached resource from kubectl with annotation `+kubebuilder:printcolumn:name`

**api/v1alpha1/memcached_func.go**
```golang
package v1alpha1

import "github.com/disaster37/operator-sdk-extra/pkg/object"

func (h *Memcached) GetStatus() object.MultiPhaseObjectStatus {
	return &h.Status
}

```

Some explains:
- We implement the interface `object.MultiPhaseObject` to get our Status object.


### Implement step reconcilers

Like we say, our controller will handle one configmap and one deployment resources. So we need to create 2 steps reconcilers. One for each resource type.

The step reconciler is a standard reconciler. So there are already standard struct that implement the interface `controller.MultiPhaseStepReconcilerAction`. In major of situation, you just need to overwrite the `Read` method.
It consist to read the existing K8s resource and to compute the expected K8s resources.

To have a clean code, we create resource builder to generate the expected resource.

#### Configmap step reconciler

We start to create the configMap builder to generate the expected configMap

**controllers/configmap_builder.go**
```golang
package controllers

import (
	"github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newConfigMapsBuilder(o *v1alpha1.Memcached) (configMaps []corev1.ConfigMap, err error) {
	configMaps = make([]corev1.ConfigMap, 0, 1)

	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      o.Name,
			Namespace: o.Namespace,
			Labels: map[string]string{
				"name":                          o.GetName(),
				v1alpha1.MemcachedAnnotationKey: "true",
			},
		},
		Data: map[string]string{
			"INSTANCE_NAME": o.Name,
		},
	}

	configMaps = append(configMaps, *cm)

	return configMaps, nil
}
```

> There are no specificity with the framework

Then, we  create the configMap reconciler that implement the `Read` method.

**controllers/configmap_reconciler.go**
```golang
package controllers

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/pkg/helper"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	"github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ConfigmapCondition shared.ConditionName = "ConfigmapReady"
	ConfigmapPhase     shared.PhaseName     = "Configmap"
)

type configMapReconciler struct {
	controller.MultiPhaseStepReconcilerAction
	controller.BaseReconciler
}

func newConfigMapReconciler(client client.Client, logger *logrus.Entry, recorder record.EventRecorder) (multiPhaseStepReconcilerAction controller.MultiPhaseStepReconcilerAction) {
	return &configMapReconciler{
		MultiPhaseStepReconcilerAction: controller.NewBasicMultiPhaseStepReconcilerAction(
			client,
			ConfigmapPhase,
			ConfigmapCondition,
			logger,
			recorder,
		),
		BaseReconciler: controller.BaseReconciler{
			Client:   client,
			Recorder: recorder,
			Log:      logger,
		},
	}
}

func (r *configMapReconciler) Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (read controller.MultiPhaseRead, res ctrl.Result, err error) {
	mc := o.(*v1alpha1.Memcached)
	cmList := &corev1.ConfigMapList{}
	read = controller.NewBasicMultiPhaseRead()

	// Read current configmaps
	labelSelectors, err := labels.Parse(fmt.Sprintf("name=%s,%s=true", o.GetName(), v1alpha1.MemcachedAnnotationKey))
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate label selector")
	}
	if err = r.Client.List(ctx, cmList, &client.ListOptions{Namespace: o.GetNamespace(), LabelSelector: labelSelectors}); err != nil {
		return read, res, errors.Wrapf(err, "Error when read configmaps")
	}

	read.SetCurrentObjects(helper.ToSliceOfObject(cmList.Items))

	// Generate expected configmaps
	expectedCms, err := newConfigMapsBuilder(mc)
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate expected configMaps")
	}
	read.SetExpectedObjects(helper.ToSliceOfObject(expectedCms))

	return read, res, nil
}

```

Some explains:
- The struct `configMapReconciler` need to implement the interface  `controller.MultiPhaseStepReconcilerAction`
- We create constructor to construct this step reconciler `newConfigMapReconciler`. It will call from the main reconciler.
- We use the standard step reconciler `controller.NewBasicMultiPhaseStepReconcilerAction()`.
- We implement the methode `Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (read controller.MultiPhaseRead, res ctrl.Result, err error)`. First, we read existing configMaps on K8s, then we generate expected configMaps.

#### Deployment step reconciler

We start to create the deployment builder to generate the expected deployment

**controllers/deployment_builder.go**
```golang
package controllers

import (
	"os"
	"strings"

	"github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	"github.com/thoas/go-funk"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newDeploymentsBuilder(memcached *v1alpha1.Memcached) (deployments []appsv1.Deployment, err error) {

	deployments = make([]appsv1.Deployment, 0, 1)
	ls := labelsForMemcached(memcached.Name)
	replicas := memcached.Spec.Size

	// Get the Operand image
	image, err := imageForMemcached()
	if err != nil {
		return nil, err
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      memcached.Name,
			Namespace: memcached.Namespace,
			Labels: funk.UnionStringMap(
				ls,
				memcached.Labels,
			),
			Annotations: memcached.GetAnnotations(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					// TODO(user): Uncomment the following code to configure the nodeAffinity expression
					// according to the platforms which are supported by your solution. It is considered
					// best practice to support multiple architectures. build your manager image using the
					// makefile target docker-buildx. Also, you can use docker manifest inspect <image>
					// to check what are the platforms supported.
					// More info: https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#node-affinity
					//Affinity: &corev1.Affinity{
					//	NodeAffinity: &corev1.NodeAffinity{
					//		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					//			NodeSelectorTerms: []corev1.NodeSelectorTerm{
					//				{
					//					MatchExpressions: []corev1.NodeSelectorRequirement{
					//						{
					//							Key:      "kubernetes.io/arch",
					//							Operator: "In",
					//							Values:   []string{"amd64", "arm64", "ppc64le", "s390x"},
					//						},
					//						{
					//							Key:      "kubernetes.io/os",
					//							Operator: "In",
					//							Values:   []string{"linux"},
					//						},
					//					},
					//				},
					//			},
					//		},
					//	},
					//},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						// IMPORTANT: seccomProfile was introduced with Kubernetes 1.19
						// If you are looking for to produce solutions to be supported
						// on lower versions you must remove this option.
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Image:           image,
						Name:            "memcached",
						ImagePullPolicy: corev1.PullIfNotPresent,
						// Ensure restrictive context for the container
						// More info: https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted
						SecurityContext: &corev1.SecurityContext{
							// WARNING: Ensure that the image used defines an UserID in the Dockerfile
							// otherwise the Pod will not run and will fail with "container has runAsNonRoot and image has non-numeric user"".
							// If you want your workloads admitted in namespaces enforced with the restricted mode in OpenShift/OKD vendors
							// then, you MUST ensure that the Dockerfile defines a User ID OR you MUST leave the "RunAsNonRoot" and
							// "RunAsUser" fields empty.
							RunAsNonRoot: &[]bool{true}[0],
							// The memcached image does not use a non-zero numeric user as the default user.
							// Due to RunAsNonRoot field being set to true, we need to force the user in the
							// container to a non-zero numeric user. We do this using the RunAsUser field.
							// However, if you are looking to provide solution for K8s vendors like OpenShift
							// be aware that you cannot run under its restricted-v2 SCC if you set this value.
							RunAsUser:                &[]int64{1001}[0],
							AllowPrivilegeEscalation: &[]bool{false}[0],
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
						},
						Ports: []corev1.ContainerPort{{
							ContainerPort: memcached.Spec.ContainerPort,
							Name:          "memcached",
						}},
						Command: []string{"memcached", "-m=64", "-o", "modern", "-v"},
						EnvFrom: []corev1.EnvFromSource{
							{
								ConfigMapRef: &corev1.ConfigMapEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: memcached.Name,
									},
								},
							},
						},
					}},
				},
			},
		},
	}

	deployments = append(deployments, *dep)
	return deployments, nil
}

// labelsForMemcached returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForMemcached(name string) map[string]string {
	var imageTag string
	image, err := imageForMemcached()
	if err == nil {
		imageTag = strings.Split(image, ":")[1]
	}
	return map[string]string{"app.kubernetes.io/name": "Memcached",
		"app.kubernetes.io/instance":    name,
		"app.kubernetes.io/version":     imageTag,
		"app.kubernetes.io/part-of":     "memcached-operator",
		"app.kubernetes.io/created-by":  "controller-manager",
		"name":                          name,
		v1alpha1.MemcachedAnnotationKey: "true",
	}
}

// imageForMemcached gets the Operand image which is managed by this controller
// from the MEMCACHED_IMAGE environment variable defined in the config/manager/manager.yaml
func imageForMemcached() (string, error) {
	var imageEnvVar = "MEMCACHED_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return "memcached:1.4.36-alpine", nil
	}
	return image, nil
}

```

> There are no specificity with the framework

Then, we  create the deployment reconciler that implement the `Read` method.

**controllers/deployment_reconciler.go**
```golang
package controllers

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/pkg/helper"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
	"github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	appv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DeploymentCondition shared.ConditionName = "DeploymentReady"
	DeploymentPhase     shared.PhaseName     = "Deployment"
)

type deploymentReconciler struct {
	controller.MultiPhaseStepReconcilerAction
	controller.BaseReconciler
}

func newDeploymentReconciler(client client.Client, logger *logrus.Entry, recorder record.EventRecorder) (multiPhaseStepReconcilerAction *deploymentReconciler) {

	return &deploymentReconciler{
		MultiPhaseStepReconcilerAction: controller.NewBasicMultiPhaseStepReconcilerAction(
			client,
			DeploymentPhase,
			DeploymentCondition,
			logger,
			recorder,
		),
		BaseReconciler: controller.BaseReconciler{
			Client:   client,
			Recorder: recorder,
			Log:      logger,
		},
	}
}

func (r *deploymentReconciler) Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (read controller.MultiPhaseRead, res ctrl.Result, err error) {
	mc := o.(*v1alpha1.Memcached)
	deploymentList := &appv1.DeploymentList{}
	read = controller.NewBasicMultiPhaseRead()

	// Read current configmaps
	labelSelectors, err := labels.Parse(fmt.Sprintf("name=%s,%s=true", o.GetName(), v1alpha1.MemcachedAnnotationKey))
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate label selector")
	}
	if err = r.Client.List(ctx, deploymentList, &client.ListOptions{Namespace: o.GetNamespace(), LabelSelector: labelSelectors}); err != nil {
		return read, res, errors.Wrapf(err, "Error when read deployments")
	}

	read.SetCurrentObjects(helper.ToSliceOfObject(deploymentList.Items))

	// Generate expected configmaps
	expectedDeployments, err := newDeploymentsBuilder(mc)
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate expected deployments")
	}
	read.SetExpectedObjects(helper.ToSliceOfObject(expectedDeployments))

	return read, res, nil
}

```

Some explains:
- The struct `DeploymentReconciler` need to implement the interface  `controller.MultiPhaseStepReconcilerAction`
- We create constructor to construct this step reconciler `newDeploymentReconciler`. It will call from the main reconciler.
- We use the standard step reconciler `controller.NewBasicMultiPhaseStepReconcilerAction()`.
- We implement the methode `Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (read controller.MultiPhaseRead, res ctrl.Result, err error)`. First, we read existing deployments on K8s, then we generate expected deployments.

### Implement the main reconciler

The goal of this reconciler is to get the object that wake up operator, maintain some standard status and call each step to reconcile resources.

It need to implement the `controller.MultiPhaseReconcilerAction` and `controller.MultiPhaseReconciler` interface. You can use standard MultiPhaseReconciler constructor.

**controllers/memcached_controller.go**:
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

package controllers

import (
	"context"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/disaster37/operator-sdk-extra/pkg/controller"
	cachecrd "github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
)

// MemcachedReconciler reconciles a Memcached object
type MemcachedReconciler struct {
	controller.Controller
	controller.MultiPhaseReconcilerAction
	controller.MultiPhaseReconciler
	controller.BaseReconciler
}

func NewMemcachedReconciler(client client.Client, logger *logrus.Entry, recorder record.EventRecorder) (multiPhaseReconciler controller.Controller) {

	return &MemcachedReconciler{
		Controller: controller.NewBasicController(),
		MultiPhaseReconcilerAction: controller.NewBasicMultiPhaseReconcilerAction(
			client,
			controller.ReadyCondition,
			logger,
			recorder,
		),
		MultiPhaseReconciler: controller.NewBasicMultiPhaseReconciler(
			client,
			"memcached",
			"memcached.cache.example.com/finalizer",
			logger,
			recorder,
		),
		BaseReconciler: controller.BaseReconciler{
			Client:   client,
			Recorder: recorder,
			Log:      logger,
		},
	}
}

//+kubebuilder:rbac:groups=cache.example.com,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.example.com,resources=memcacheds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.example.com,resources=memcacheds/finalizers,verbs=update
//+kubebuilder:rbac:groups="core",resources=events,verbs=patch;get;create
//+kubebuilder:rbac:groups="core",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Memcached object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	mc := &cachecrd.Memcached{}
	data := map[string]any{}

	return r.MultiPhaseReconciler.Reconcile(
		ctx,
		req,
		mc,
		data,
		r,
		newConfigMapReconciler(
			r.Client,
			r.Log,
			r.Recorder,
		),
		newDeploymentReconciler(
			r.Client,
			r.Log,
			r.Recorder,
		),
	)

}

// SetupWithManager sets up the controller with the Manager.
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachecrd.Memcached{}).
		Owns(&appv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}


```

Some explain:
- The struct `MemcachedReconciler` need to implement the interface `controller.MultiPhaseReconcilerAction` and `controller.MultiPhaseReconciler`. We use the standard implementation of this interface via `controller.NewBasicMultiPhaseReconcilerAction()` and `controller.NewBasicMultiPhaseReconciler`
-  We rewrite the main Reconcile methode to call the multi phase reconciler and use our custom step reconciler to reconcile configMap and Deployment.

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

	cachev1alpha1 "github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	"github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/controllers"
	"github.com/sirupsen/logrus"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(cachev1alpha1.AddToScheme(scheme))
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
		LeaderElectionID:       "1858d68a.example.com",
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

	memcachedReconciler := controllers.NewMemcachedReconciler(
		mgr.GetClient(),
		logrus.NewEntry(log),
		mgr.GetEventRecorderFor("memcached-controller"),
	)
	if err = memcachedReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Memcached")
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