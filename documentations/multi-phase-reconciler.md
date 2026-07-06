# Multi-phase Reconciler

> Use it when you need to manage multiple K8s resources from your own CRD (ConfigMaps, Deployments, Services, etc.).

## Overview

The multi-phase reconciler orchestrates the lifecycle of multiple Kubernetes resources owned by a single CRD. Each managed resource type is handled by a dedicated **step reconciler** that reads the current state, diffs it against the expected state, and applies the necessary create/update/delete operations.

```
Get CRD object
    |
    v
Add Finalizer (if not present)
    |
    v
Track status changes (deep-copy + deferred update)
    |
    v
Check ignoreReconcile annotation
    |
    v
Configure()  -- init conditions
    |
    v
Read()       -- user logic (main action)
    |
    v
Is deleting? -- yes --> Delete(), remove finalizer, return
    |
    no
    v
For each step reconciler:
    Configure -> Read -> Diff -> Create/Update/Delete -> OnSuccess
    |
    v
OnSuccess()  -- main action
```

## Sample: Memcached Operator

This sample deploys Memcached via a CRD that manages one `ConfigMap` and one `Deployment`. The CRD exposes `size` (replica count) and `containerPort`.

Complete source code: `samples/memcached-operator/`

---

## 1. Define the CRD

### `api/v1alpha1/memcached_types.go`

```go
package v1alpha1

import (
	multiphase "github.com/disaster37/operator-sdk-extra/v2/pkg/apis/multiphase"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MemcachedAnnotationKey = "cache.example.com"
)

type MemcachedSpec struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3
	// +kubebuilder:validation:ExclusiveMaximum=false
	Size          int32 `json:"size,omitempty"`
	ContainerPort int32 `json:"containerPort,omitempty"`
}

type MemcachedStatus struct {
	multiphase.DefaultMultiPhaseObjectStatus `json:",inline"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1,memcached-deployment},{ConfigMap,v1,memcached-configmap}}
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Cluster deployment status"
// +kubebuilder:printcolumn:name="Error",type="boolean",JSONPath=".status.isOnError",description="Is on error"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='MemcachedReady')].status",description="Cluster health"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

type Memcached struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MemcachedSpec   `json:"spec,omitempty"`
	Status MemcachedStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type MemcachedList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Memcached `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Memcached{}, &MemcachedList{})
}
```

Key points:
- `multiphase.DefaultMultiPhaseObjectStatus` (package `pkg/apis/multiphase/`) provides `PhaseName`, `Conditions`, `IsOnError`, `LastErrorMessage`, and `ObservedGeneration`.
- Print columns expose phase, error flag, readiness condition, and age via `kubectl get`.

### `api/v1alpha1/memcached_func.go`

```go
package v1alpha1

import "github.com/disaster37/operator-sdk-extra/v2/pkg/object"

func (h *Memcached) GetStatus() object.MultiPhaseObjectStatus {
	return &h.Status
}
```

- Implements `object.MultiPhaseObject` by returning the status pointer.
- The framework uses `GetStatus()` to read/write conditions, phase, and error state during reconciliation.

---

## 2. Build expected resources (builders)

Builders are plain functions that produce the desired K8s objects from the CRD spec. No framework-specific interfaces required.

### `controllers/configmap_builder.go`

```go
package controllers

import (
	"github.com/disaster37/operator-sdk-extra/v2/samples/memcached-operator/api/v1alpha1"
	"github.com/thoas/go-funk"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newConfigMapsBuilder(o *v1alpha1.Memcached) (configMaps []corev1.ConfigMap, err error) {
	configMaps = make([]corev1.ConfigMap, 0, 1)

	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      o.Name,
			Namespace: o.Namespace,
			Labels: funk.UnionStringMap(
				map[string]string{
					"name":                          o.GetName(),
					v1alpha1.MemcachedAnnotationKey: "true",
				},
				o.Labels,
			),
		},
		Data: map[string]string{
			"INSTANCE_NAME": o.Name,
		},
	}

	configMaps = append(configMaps, *cm)

	return configMaps, nil
}
```

The deployment builder (`controllers/deployment_builder.go`) follows the same pattern, producing an `appsv1.Deployment` from the CRD spec.

---

## 3. Implement step reconcilers

Each step reconciler handles one resource type. In most cases you only need to override the `Read()` method; the default implementation already provides `Configure()`, `Diff()`, `Create()`, `Update()`, `Delete()`, `OnError()`, and `OnSuccess()`.

### `controllers/configmap_reconciler.go`

```go
package controllers

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/multiphase"
	cachecrd "github.com/disaster37/operator-sdk-extra/v2/samples/memcached-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	ConfigmapCondition shared.ConditionName = "ConfigmapReady"
	ConfigmapPhase     shared.PhaseName     = "Configmap"
)

type configMapReconciler struct {
	multiphase.MultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap]
}

func newConfigMapReconciler(c client.Client, recorder record.EventRecorder) multiphase.MultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap] {
	return &configMapReconciler{
		MultiPhaseStepReconcilerAction: multiphase.NewMultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap](
			c,
			ConfigmapPhase,
			ConfigmapCondition,
			recorder,
		),
	}
}

func (r *configMapReconciler) Read(ctx context.Context, o *cachecrd.Memcached, data map[string]any, logger *logrus.Entry) (read multiphase.MultiPhaseRead[*corev1.ConfigMap], res reconcile.Result, err error) {
	cmList := &corev1.ConfigMapList{}
	read = multiphase.NewMultiPhaseRead[*corev1.ConfigMap]()

	labelSelectors, err := labels.Parse(fmt.Sprintf("name=%s,%s=true", o.GetName(), cachecrd.MemcachedAnnotationKey))
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate label selector")
	}
	if err = r.Client().List(ctx, cmList, &client.ListOptions{Namespace: o.GetNamespace(), LabelSelector: labelSelectors}); err != nil {
		return read, res, errors.Wrapf(err, "Error when read configmaps")
	}

	for i := range cmList.Items {
		read.AddCurrentObject(&cmList.Items[i])
	}

	expectedCms, err := newConfigMapsBuilder(o)
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate expected configMaps")
	}
	for i := range expectedCms {
		read.AddExpectedObject(&expectedCms[i])
	}

	return read, res, nil
}
```

Key points:
- `multiphase.MultiPhaseStepReconcilerAction[*Memcached, *ConfigMap]` -- generic, type-safe; no type assertions needed.
- `Read()` receives `logger *logrus.Entry` as a parameter (not stored in a struct field).
- `multiphase.NewMultiPhaseRead[*corev1.ConfigMap]()` replaces the old `controller.NewBasicMultiPhaseRead()`.
- `read.AddCurrentObject()` / `read.AddExpectedObject()` work with the concrete type (`*corev1.ConfigMap`), no `helper.ToSliceOfObject()` needed.

### `controllers/deployment_reconciler.go`

Follows the same pattern with `*appv1.Deployment` instead of `*corev1.ConfigMap`. Uses phase `"Deployment"` and condition `"DeploymentReady"`.

---

## 4. Implement the main reconciler

### `controllers/memcached_controller.go`

```go
package controllers

import (
	"context"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8scontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/multiphase"
	cachecrd "github.com/disaster37/operator-sdk-extra/v2/samples/memcached-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
)

const (
	mainFinalizer shared.FinalizerName = "memcached.cache.example.com/finalizer"
)

type MemcachedReconciler struct {
	controller.Controller
	multiphase.MultiPhaseReconciler[*cachecrd.Memcached]
	multiphase.MultiPhaseReconcilerAction[*cachecrd.Memcached]
	name            string
	stepReconcilers []multiphase.MultiPhaseStepReconcilerAction[*cachecrd.Memcached, client.Object]
}

func NewMemcachedReconciler(c client.Client, logger *logrus.Entry, recorder record.EventRecorder) controller.Controller {
	configMapStep := newConfigMapReconciler(c, recorder)
	deploymentStep := newDeploymentReconciler(c, recorder)

	return &MemcachedReconciler{
		Controller: controller.NewController(),
		MultiPhaseReconciler: multiphase.NewMultiPhaseReconciler[*cachecrd.Memcached](
			c,
			"memcached",
			mainFinalizer,
			logger,
			recorder,
		),
		MultiPhaseReconcilerAction: multiphase.NewMultiPhaseReconcilerAction[*cachecrd.Memcached](
			c,
			controller.ReadyCondition,
			recorder,
		),
		name: "memcached",
		stepReconcilers: []multiphase.MultiPhaseStepReconcilerAction[*cachecrd.Memcached, client.Object]{
			multiphase.NewObjectMultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap, client.Object](configMapStep),
			multiphase.NewObjectMultiPhaseStepReconcilerAction[*cachecrd.Memcached, *appv1.Deployment, client.Object](deploymentStep),
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

func (h *MemcachedReconciler) Client() client.Client {
	return h.MultiPhaseReconcilerAction.Client()
}

func (h *MemcachedReconciler) Recorder() record.EventRecorder {
	return h.MultiPhaseReconcilerAction.Recorder()
}

func (r *MemcachedReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	o := &cachecrd.Memcached{}
	data := map[string]any{}

	return r.MultiPhaseReconciler.Reconcile(ctx, req, o, data, r, r.stepReconcilers...)
}

func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachecrd.Memcached{}).
		Owns(&appv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		WithOptions(k8scontroller.Options{
			RateLimiter: controller.DefaultControllerRateLimiter[reconcile.Request](),
		}).
		Complete(r)
}
```

Key points:
- `multiphase.MultiPhaseReconciler[*Memcached]` -- orchestrates finalizer, status tracking, and step execution.
- `multiphase.MultiPhaseReconcilerAction[*Memcached]` -- provides default `Configure()`, `Read()`, `Delete()`, `OnError()`, `OnSuccess()`.
- `controller.NewController()` -- replaces the old `controller.NewBasicController()`.
- `multiphase.NewMultiPhaseReconciler[*Memcached](...)` -- replaces the old `controller.NewBasicMultiPhaseReconciler(...)`.
- `multiphase.NewMultiPhaseReconcilerAction[*Memcached](...)` -- replaces the old `controller.NewBasicMultiPhaseReconcilerAction(...)`.
- Step reconcilers use specific types (`*ConfigMap`, `*Deployment`) but are wrapped with `multiphase.NewObjectMultiPhaseStepReconcilerAction` to convert them to `client.Object` for the orchestrator.
- Step reconcilers are created in the constructor and stored in the struct, not in the `Reconcile()` method.
- `reconcile.Request` is used (from `sigs.k8s.io/controller-runtime/pkg/reconcile`), not `ctrl.Request`.
- `controller.DefaultControllerRateLimiter[reconcile.Request]()` provides a default rate limiter for the controller options.

---

## 5. Wire up in main.go

```go
package main

import (
	"flag"
	"os"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	cachev1alpha1 "github.com/disaster37/operator-sdk-extra/v2/samples/memcached-operator/api/v1alpha1"
	"github.com/disaster37/operator-sdk-extra/v2/samples/memcached-operator/controllers"
	"github.com/sirupsen/logrus"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(cachev1alpha1.AddToScheme(scheme))
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
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{BindAddress: metricsAddr},
		WebhookServer:          webhook.NewServer(webhook.Options{Port: 9443}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "1858d68a.example.com",
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

- The reconciler constructor takes `client.Client`, `*logrus.Entry`, and `record.EventRecorder`.
- `SetupWithManager()` registers the CRD as the primary resource and `Owns()` for each managed child type.

---

## Reconciliation flow

The `DefaultMultiPhaseReconciler.Reconcile()` method (`pkg/controller/multiphase/multiphase_reconciler.go`) executes these steps in order:

1. **Get** the CRD object from the K8s API. Return early if not found.
2. **Add finalizer** if not already present. Requeue immediately after adding.
3. **Track status** -- deep-copy the current status; defer a status update if it changed.
4. **Check ignoreReconcile** annotation (`operator-sdk-extra.webcenter.fr/ignoreReconcile=true`). Skip reconciliation if set.
5. **Configure()** on the main action -- initializes the ready condition.
6. **Read()** on the main action -- optional user logic before step reconciliation.
7. **Delete path** -- if `DeletionTimestamp` is set: call `Delete()`, remove finalizer, return.
8. **For each step reconciler**: Configure -> Read -> Diff -> Create/Update/Delete -> OnSuccess. Each step sets the phase name on the status.
9. **OnSuccess()** on the main action -- sets the ready condition to `True`, phase to `"running"`, clears error state, updates `ObservedGeneration`.

If any step returns an error, `OnError()` is called to update the status condition and error message.

---

## Key interfaces

| Interface | Package | Purpose |
|---|---|---|
| `multiphase.MultiPhaseReconciler[k8sObject]` | `pkg/controller/multiphase/` | Orchestrator: manages finalizer, status tracking, and step execution |
| `multiphase.MultiPhaseReconcilerAction[k8sObject]` | same | User-facing hooks: `Configure`, `Read`, `Delete`, `OnError`, `OnSuccess` |
| `multiphase.MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]` | same | Per-resource step: `Read`, `Diff`, `Create`, `Update`, `Delete` |
| `multiphase.MultiPhaseRead[k8sStepObject]` | same | Holds current and expected objects for a step |
| `multiphase.MultiPhaseDiff[k8sStepObject]` | same | Holds create/update/delete lists produced by the diff; `GetObjectsToApply()` returns the union of create + update |
| `object.MultiPhaseObject` | `pkg/object/` | CRD must implement `GetStatus() MultiPhaseObjectStatus` |
| `object.MultiPhaseObjectStatus` | `pkg/object/` | `GetConditions`, `SetConditions`, `GetPhaseName`, `SetPhaseName`, `GetIsOnError`, `SetIsOnError`, `GetLastErrorMessage`, `SetLastErrorMessage`, `GetObservedGeneration`, `SetObservedGeneration` |

---

## Constructors

| Constructor | Description |
|---|---|
| `multiphase.NewMultiPhaseReconciler[k8sObject](client, name, finalizer, logger, recorder)` | Main orchestrator with finalizer and status tracking |
| `multiphase.NewMultiPhaseReconcilerAction[k8sObject](client, conditionName, recorder)` | Default action: initializes ready condition, provides `OnError` / `OnSuccess` |
| `multiphase.NewMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject](client, phaseName, conditionName, recorder, fieldManager, dryRun)` | Default step action: provides `Configure`, `Read`, `Diff`, `Apply`, `Delete`, `OnError`, `OnSuccess`, `OnDiff`. `fieldManager` identifies this controller for SSA. `dryRun` enables SSA dry-run diff detection (opt-in). |
| `multiphase.NewObjectMultiPhaseStepReconcilerAction[k8sObject, src, dst](stepReconciler)` | Wraps a typed step reconciler to convert from a specific type (e.g. `*ConfigMap`) to `client.Object` |
| `multiphase.NewMultiPhaseRead[k8sStepObject]()` | Creates an empty read result for collecting current and expected objects |
| `controller.NewController()` | Base controller (no-op scaffolding) |

---

## Server-Side Apply (SSA)

The multi-phase reconciler uses **Server-Side Apply** exclusively. SSA delegates conflict detection and field ownership to the Kubernetes API server, eliminating client-side diff computation and the `kubectl.kubernetes.io/last-applied-configuration` annotation.

### Benefits

- **No update loops**: the API server only persists changes when owned fields actually differ.
- **Native conflict detection**: field ownership is tracked by the API server via managed fields.
- **Simpler `Read()` implementation**: `currentObjects` is only needed for orphan detection, not for diff computation.
- **Single API call for create and update**: `Apply` handles both cases.

### Reconciliation Flow

```
Configure -> Read -> Diff -> OnDiff -> Apply/Delete -> OnSuccess
```

- **Diff()** classifies objects into create, update, and delete lists. When `dryRun=true`, uses SSA dry-run to detect actual changes. When `dryRun=false`, all expected objects with current counterparts go to the update list (legacy always-apply).
- **OnDiff()** is a pre-apply hook called between Diff and Apply. Use it for pre-tasks like draining nodes before a StatefulSet update.
- **Apply()** uses `client.Patch(obj, client.Apply, client.FieldOwner(...), client.ForceOwnership)` for each object, handling both creation and updates in a single call.
- **Delete()** removes orphaned objects (current objects that are no longer expected).

### Constructor

The `fieldManager` and `dryRun` are required parameters when constructing the step reconciler action:

```go
func newConfigMapReconciler(c client.Client, recorder record.EventRecorder) multiphase.MultiPhaseStepReconcilerAction[*Memcached, *corev1.ConfigMap] {
    return &configMapReconciler{
        MultiPhaseStepReconcilerAction: multiphase.NewMultiPhaseStepReconcilerAction[*Memcached, *corev1.ConfigMap](
            c,
            ConfigmapPhase,
            ConfigmapCondition,
            recorder,
            "memcached-operator",  // fieldManager — required
            false,                  // dryRun — false means all objects always-applied
        ),
    }
}
```

### Detecting changes with SSA (dry-run diff)

When `dryRun=true`, the Diff() step performs an SSA dry-run (`client.Apply` + `client.DryRunAll`) for each existing object to predict what the API server would produce. It normalizes both the predicted and current object (stripping `resourceVersion`, `generation`, `managedFields`, `status`, etc.) and compares them. Objects are then classified:

- **Create**: no current counterpart exists
- **Update**: current counterpart exists and the normalized diff is non-empty
- **Unchanged**: current counterpart exists and the normalized diff is empty — **skipped** (no API call)
- **Delete**: current object with no expected counterpart (orphan)

**How to enable**: pass `dryRun=true` to `NewMultiPhaseStepReconcilerAction`. Note this adds one extra dry-run API call per existing object.

**OnDiff pre-task pattern**: override `OnDiff` to run logic before Apply, keyed on `diff.NeedUpdate()` / `diff.GetObjectsToUpdate()`:

```go
func (r *deploymentReconciler) OnDiff(ctx context.Context, o *Memcached, data map[string]any, diff multiphase.MultiPhaseDiff[*appv1.Deployment], logger *logrus.Entry) (reconcile.Result, error) {
    if diff.NeedUpdate() {
        logger.Infof("Deployment update detected for %d object(s)", len(diff.GetObjectsToUpdate()))
        // Drain nodes, scale down, or other pre-update tasks here
    }
    return reconcile.Result{}, nil
}
```

**Disabled-diff contract**: when `dryRun=false`, the default `OnDiff` returns `controller.ErrDiffDisabled`. Controllers that do not need pre-tasks must either:
- Enable diff with `dryRun=true`, or
- Override `OnDiff` to return `reconcile.Result{}, nil` (safe no-op)

This ensures no accidental silent no-op when pre-tasks are configured but diff detection is left disabled.

**Caveat**: mutating webhooks may modify objects during dry-run apply, causing false-positive updates (predicted differs from live despite identical input). Webhook-noise filtering is documented for a future enhancement.

### Requirements

1. **Kubernetes 1.22+** — SSA is stable since this version.
2. **TypeMeta must be set** on expected objects — SSA requires `apiVersion` and `kind`:

```go
read.AddExpectedObject(&corev1.ConfigMap{
    TypeMeta: metav1.TypeMeta{
        APIVersion: "v1",
        Kind:       "ConfigMap",
    },
    ObjectMeta: metav1.ObjectMeta{
        Name:      o.Name,
        Namespace: o.Namespace,
    },
    Data: map[string]string{"key": "value"},
})
```

3. **Deterministic names** — SSA does not support `generateName`. All expected objects must have a fixed `Name`.
4. **No status fields** in expected objects — SSA apply objects should only contain spec-level fields owned by the operator.

---

## Status Tracking

`multiphase.DefaultMultiPhaseObjectStatus` (package `pkg/apis/multiphase/`) embeds `apis.DefaultObjectStatus` and adds `PhaseName`. It provides:

| Field | Type | Description |
|---|---|---|
| `PhaseName` | `shared.PhaseName` | Current phase (e.g. `"Configmap"`, `"Deployment"`, `"running"`) |
| `Conditions` | `[]metav1.Condition` | Standard K8s conditions list |
| `IsOnError` | `*bool` | Whether the reconciler is stuck on an error |
| `LastErrorMessage` | `string` | Last error message (truncated to 5000 chars) |
| `ObservedGeneration` | `int64` | Last successfully reconciled generation |
