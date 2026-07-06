# Sentinel Reconciler

> Watch standard K8s resources you do NOT own (Ingresses, Secrets, ConfigMaps, Namespaces, etc.) and derive new resources from their annotations or labels.

## Overview

- No finalizer
- No CRD required (watches standard Kubernetes resources)
- No `Delete()` — Kubernetes GC via owner references
- Use case: annotations on existing objects → create ConfigMaps, Secrets, etc.

## Sample: Namespace Sentinel

The official framework test shows a sentinel on `*corev1.Namespace` that creates a ConfigMap and a Secret inside the namespace designated by the annotation `test.operator.webcenter.fr/sentinel`.

## Typical use case examples

- Ingress with annotation `sentinel.example.com/target-config` → create ConfigMap (see `samples/ingress-sentinel-operator/`)
- Namespace with annotation → create ConfigMap + Secret inside the namespace (see framework test)

## 1. Implement the action (the logic)

### `templateAnnotationsReconciler[k8sObject]` (from the framework test)

```go
type templateAnnotationsReconciler[k8sObject client.Object] struct {
    sentinel.SentinelReconcilerAction[k8sObject]
}

func newTemplateAnnotationsReconciler[k8sObject client.Object](c client.Client, recorder record.EventRecorder) sentinel.SentinelReconcilerAction[k8sObject] {
    return &templateAnnotationsReconciler[k8sObject]{
        SentinelReconcilerAction: sentinel.NewSentinelAction[k8sObject](
            c,
            recorder,
            "template-sentinel",
            false,
        ),
    }
}
```

**`Read()` is the only method you need to override** — the default implementation handles Configure, Diff, Create, Update, Delete, OnError, OnSuccess.

```go
func (h *templateAnnotationsReconciler[k8sObject]) Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (read sentinel.SentinelRead, res reconcile.Result, err error) {
    read = sentinel.NewSentinelRead(h.Client().Scheme())
    var (
        cm *corev1.ConfigMap
        s  *corev1.Secret
    )

    if o.GetAnnotations() != nil && o.GetAnnotations()[annotation] != "" {
        // Compute expecting objects
        read.AddExpectedObject(&corev1.ConfigMap{
            ObjectMeta: v1.ObjectMeta{
                Name:      o.GetName(),
                Namespace: o.GetName(),
            },
            Data: map[string]string{
                "val": o.GetAnnotations()[annotation],
            },
        })

        read.AddExpectedObject(&corev1.Secret{
            ObjectMeta: v1.ObjectMeta{
                Name:      o.GetName(),
                Namespace: o.GetName(),
            },
            Data: map[string][]byte{
                "val": []byte(o.GetAnnotations()[annotation]),
            },
        })
    }

    // Read current objects
    cm = &corev1.ConfigMap{}
    if err = h.Client().Get(ctx, types.NamespacedName{Namespace: o.GetName(), Name: o.GetName()}, cm); err != nil {
        if !k8serrors.IsNotFound(err) {
            return nil, res, err
        }
        cm = nil
    }
    read.AddCurrentObject(cm)

    s = &corev1.Secret{}
    if err = h.Client().Get(ctx, types.NamespacedName{Namespace: o.GetName(), Name: o.GetName()}, s); err != nil {
        if !k8serrors.IsNotFound(err) {
            return nil, res, err
        }
        s = nil
    }
    read.AddCurrentObject(s)

    return read, res, nil
}
```

## 2. Implement the controller

```go
type TestReconciler struct {
    controller.Controller
    sentinel.SentinelReconciler[*corev1.Namespace]
    sentinel.SentinelReconcilerAction[*corev1.Namespace]
    name string
}

func NewTestReconciler(c client.Client, logger *logrus.Entry, recorder record.EventRecorder) (multiPhaseReconciler controller.Controller) {
    return &TestReconciler{
        Controller: controller.NewController(),
        SentinelReconciler: sentinel.NewSentinelReconciler[*corev1.Namespace](
            c,
            name,
            logger,
            recorder,
        ),
        SentinelReconcilerAction: newTemplateAnnotationsReconciler[*corev1.Namespace](
            c,
            recorder,
        ),
        name: name,
    }
}

func (r *TestReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    o := &corev1.Namespace{}
    data := map[string]any{}

    return r.SentinelReconciler.Reconcile(
        ctx,
        req,
        o,
        data,
        r.SentinelReconcilerAction,
    )
}

func (h *TestReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&corev1.Namespace{}).
        Owns(&corev1.ConfigMap{}).
        Owns(&corev1.Secret{}).
        WithOptions(k8scontroller.Options{
            RateLimiter: controller.DefaultControllerRateLimiter[reconcile.Request](),
        }).
        WithOptions(k8scontroller.TypedOptions[reconcile.Request]{
            MaxConcurrentReconciles: 1,
        }).
        Complete(h)
}
```

## Reconciliation flow

1. Get watched object (e.g. Namespace, Ingress)
2. Track status if status subresource exists
3. Check `operator-sdk-extra.webcenter.fr/ignoreReconcile` annotation
4. Call `Configure()` — optional
5. Call `Read()` — user reads current, builds expected, groups by type via `SentinelRead`
6. Call `Diff()` — per-type comparison, classifies into create/update/delete. When `dryRun=true`, uses SSA dry-run to detect actual changes.
7. Call `OnDiff()` — pre-apply hook (errors if diff disabled and not overridden)
8. `Apply()`, `Delete()` as needed
9. Call `OnSuccess()` — optional

## Owner references & GC

`ctrl.SetControllerReference()` is called on every expected object before diff, linking child to parent. The Kubernetes garbage collector deletes children automatically when the parent is deleted. No finalizer needed.

## Key interfaces

| Interface | Purpose |
|---|---|
| `sentinel.SentinelReconciler[k8sObject]` | Orchestrator (no finalizer, no delete handling) |
| `sentinel.SentinelReconcilerAction[k8sObject]` | User-defined: Configure, Read, Diff, Create, Update, Delete, OnError, OnSuccess |
| `sentinel.SentinelRead` | Stores current and expected objects grouped by resource type (group/version/kind) |

## Constructors

| Constructor | Description |
|---|---|
| `sentinel.NewSentinelReconciler[k8sObject](client, name, logger, recorder)` | Creates the orchestrator |
| `sentinel.NewSentinelAction[k8sObject](client, recorder, fieldManager, dryRun)` | Creates default action — you only need to override `Read()`. `fieldManager` identifies this controller for SSA. `dryRun` enables SSA dry-run diff detection (opt-in, one extra API call per existing object). |
| `sentinel.NewSentinelRead(scheme)` | Creates a read result to collect objects |

## Helpers

`sentinel.GetObjectType(o.GetObjectKind())` returns a `"group/version/kind"` string used as the key in `SentinelRead`. Objects of different types are diffed independently.

## Detecting changes with SSA (dry-run diff)

When `dryRun=true` is passed to `NewSentinelAction`, the Diff() step performs an SSA dry-run for each existing object to predict what the API server would produce. Objects are classified into **create**, **update**, and **delete** lists. Unchanged objects are skipped (no API call).

**OnDiff pre-task pattern**: override `OnDiff` to run logic before Apply, keyed on `diff.NeedUpdate()` / `diff.GetObjectsToUpdate()`.

**Disabled-diff contract**: when `dryRun=false`, the default `OnDiff` returns `controller.ErrDiffDisabled`. Override `OnDiff` to `return reconcile.Result{}, nil` if you do not need pre-tasks.
