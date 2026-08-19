# Migration guide: v1 → v2 (Server-Side Apply)

> Audience: human maintainers **and** LLM coding agents upgrading an operator built on
> `github.com/disaster37/operator-sdk-extra` to the new `…/v2` release.
>
> This release is a **major, breaking** change. Read the [TL;DR](#tldr) first, then use the
> [mechanical rename table](#1-module-path-and-import-renames) and the per-pattern sections.

---

## TL;DR

The headline change is that the **multiphase** and **sentinel** reconcilers no longer compute a
client-side 3-way diff. They now use **Kubernetes Server-Side Apply (SSA)**:

- The operator declares the *desired* object and `PATCH`es it with `client.Apply` + a
  **field manager**. The API server owns conflict detection and field ownership.
- **No more `kubectl.kubernetes.io/last-applied-configuration` annotation** on managed K8s
  resources, and no more `generic-objectmatcher` diff for these two patterns.
- Diff is now **only computed when you ask for it** (opt-in `dryRun=true`), using an SSA dry-run
  apply. When `dryRun=false`, the reconciler simply applies every expected object and lets the API
  server no-op unchanged fields (SSA is idempotent, so this does not cause update loops).
- The old `Create()` / `Update()` step methods are replaced by a single `Apply()` method.

The **remote** reconciler is unchanged in strategy: it still uses a client-side **3-way merge**
via `generic-objectmatcher` and still stores a compressed last-applied-configuration in the CRD
status. Only its import path and type constraints changed.

Other breaking changes shipped in the same release:

| Change | Old | New |
|---|---|---|
| Module version | `github.com/disaster37/operator-sdk-extra` | `github.com/disaster37/operator-sdk-extra/v2` |
| Package layout | flat `pkg/controller`, flat `pkg/apis` | split into `multiphase/`, `remote/`, `sentinel/`, `shared/` sub-packages |
| Naming convention | `Basic*` (`NewBasicController`, `BasicObjectStatus`, …) | `Default*` (`NewController`, `DefaultObjectStatus`, …) |
| Generics | interface args (`object.MultiPhaseObject`, `client.Object`) | type parameters (`[*MyCRD]`, `[*MyCRD, *corev1.ConfigMap]`) |
| Removed | `StdReconciler`, `StdK8sReconciler`, `K8sReconciler`, `K8sPhaseReconciler` | use multiphase/sentinel patterns instead |
| Request type | `ctrl.Request` in signatures | `reconcile.Request` |

---

## 1. Module path and import renames

### 1.1 `go.mod`

```diff
-require github.com/disaster37/operator-sdk-extra v1.x.x
+require github.com/disaster37/operator-sdk-extra/v2 v2.0.0
```

The `/v2` suffix is mandatory (Go module semantic import versioning). Every import must be updated.

### 1.2 Import path table

| Concern | v1 import | v2 import |
|---|---|---|
| Base controller / reconciler / helpers | `.../operator-sdk-extra/pkg/controller` | `.../operator-sdk-extra/v2/pkg/controller` |
| Multiphase pattern | `.../pkg/controller` | `.../operator-sdk-extra/v2/pkg/controller/multiphase` |
| Remote pattern | `.../pkg/controller` | `.../operator-sdk-extra/v2/pkg/controller/remote` |
| Sentinel pattern | `.../pkg/controller` | `.../operator-sdk-extra/v2/pkg/controller/sentinel` |
| Object interfaces | `.../pkg/object` | `.../operator-sdk-extra/v2/pkg/object` |
| Base status type | `.../pkg/apis` | `.../operator-sdk-extra/v2/pkg/apis` |
| Multiphase status | `.../pkg/apis` | `.../operator-sdk-extra/v2/pkg/apis/multiphase` |
| Remote status | `.../pkg/apis` | `.../operator-sdk-extra/v2/pkg/apis/remote` |
| Shared enums (phase/condition/finalizer) | `.../pkg/apis/shared` | `.../operator-sdk-extra/v2/pkg/apis/shared` |
| Test framework | `.../pkg/test` | `.../operator-sdk-extra/v2/pkg/test` |

> **Agent hint:** a good first pass is a repo-wide replace of the module prefix, then fix the
> pattern-specific sub-package imports (`multiphase` / `remote` / `sentinel`) reported by the
> compiler.

---

## 2. Symbol rename table (`Basic*` → `Default*` and constructors)

The whole library moved from the `Basic` prefix to `Default` for implementations, and dropped
`Basic` from constructors.

### 2.1 Status types (`pkg/apis`)

| v1 | v2 |
|---|---|
| `apis.BasicObjectStatus` | `apis.DefaultObjectStatus` |
| `apis.BasicMultiPhaseObjectStatus` | `multiphase.DefaultMultiPhaseObjectStatus` (moved to `pkg/apis/multiphase`) |
| `apis.BasicRemoteObjectStatus` | `remote.DefaultRemoteObjectStatus` (moved to `pkg/apis/remote`) |

### 2.2 Controller / base

| v1 | v2 |
|---|---|
| `controller.NewBasicController()` | `controller.NewController()` |
| `controller.BasicController` | `controller.DefaultController` |
| `controller.NewBasicReconciler(...)` | `controller.NewReconciler(...)` |
| `controller.NewBasicReconcilerAction(...)` | `controller.NewReconcilerAction(...)` |
| `controller.NewBaseReconciler(...)` | `controller.NewBaseReconciler(...)` (unchanged) |

> Behaviour change: `DefaultController.SetupWithManager` / `Reconcile` now **panic** with
> "You need implement it" if not overridden (v1 returned an error). This is intentional — these are
> abstract methods you must implement.

### 2.3 Multiphase (`pkg/controller/multiphase`)

| v1 | v2 |
|---|---|
| `controller.NewBasicMultiPhaseReconciler(...)` | `multiphase.NewMultiPhaseReconciler[*MyCRD](...)` |
| `controller.NewBasicMultiPhaseReconcilerAction(...)` | `multiphase.NewMultiPhaseReconcilerAction[*MyCRD](...)` |
| `controller.NewBasicMultiPhaseStepReconciler(...)` | `multiphase.NewMultiPhaseStepReconciler[*MyCRD, client.Object](...)` |
| `controller.NewBasicMultiPhaseStepReconcilerAction(...)` | `multiphase.NewMultiPhaseStepReconcilerAction[*MyCRD, *Child](client, phase, condition, recorder, fieldManager, dryRun)` |
| `controller.NewBasicMultiPhaseRead()` | `multiphase.NewMultiPhaseRead[*Child]()` |
| `controller.NewBasicMultiPhaseDiff()` | `multiphase.NewMultiPhaseDiff[*Child]()` |
| — (new) | `multiphase.NewObjectMultiPhaseStepReconcilerAction[*MyCRD, *Child, client.Object](step)` |

### 2.4 Remote (`pkg/controller/remote`)

| v1 | v2 |
|---|---|
| `controller.NewBasicRemoteReconciler[...](...)` | `remote.NewRemoteReconciler[*MyCRD, ApiObj, ApiClient](...)` |
| `controller.NewRemoteReconcilerAction[...](...)` | `remote.NewRemoteReconcilerAction[*MyCRD, ApiObj, ApiClient](...)` |
| `controller.NewBasicRemoteExternalReconciler[...](...)` | `remote.NewRemoteExternalReconciler[*MyCRD, ApiObj, ApiClient](...)` |
| `controller.NewBasicRemoteRead[T]()` | `remote.NewRemoteRead[ApiObj]()` |
| `controller.NewBasicRemoteDiff[T]()` | `remote.NewRemoteDiff[ApiObj]()` |

> Type-constraint change: the first type parameter changed from `comparable` to
> `object.RemoteObject`. Pass your concrete CRD pointer type (e.g. `*Role`) instead of the old
> comparable placeholder.

### 2.5 Sentinel (`pkg/controller/sentinel`)

| v1 | v2 |
|---|---|
| `controller.NewBasicSentinelReconciler(...)` | `sentinel.NewSentinelReconciler[*MyWatchedObj](...)` |
| `controller.NewBasicSentinelAction(...)` | `sentinel.NewSentinelAction[*MyWatchedObj](client, recorder, fieldManager, dryRun)` |
| `controller.NewBasicSentinelRead()` | `sentinel.NewSentinelRead(scheme)` (now needs a `runtime.ObjectTyper`) |
| `controller.NewBasicSentinelDiff()` | removed — sentinel now reuses `multiphase.MultiPhaseDiff[client.Object]` |

### 2.6 Removed entirely

`StdReconciler`, `NewStdReconciler`, `StdK8sReconciler`, `NewStdK8sReconciler`, `K8sReconciler`,
`K8sPhaseReconciler`, and the old flat `pkg/controller/k8s_reconciler.go` / `external_reconciler.go`
/ `common.go` are gone. Re-model any operator that used these on the **multiphase** (owns K8s
resources) or **sentinel** (watches foreign K8s resources) pattern.

---

## 3. Migrating a Multiphase operator (the big SSA change)

This is where most of the behavioural work is. See the full walkthrough in
[`multi-phase-reconciler.md`](multi-phase-reconciler.md); this section focuses on *what changed*.

### 3.1 Status type

```diff
 type MemcachedStatus struct {
-    apis.BasicMultiPhaseObjectStatus `json:",inline"`
+    multiphase.DefaultMultiPhaseObjectStatus `json:",inline"`
 }
```

```diff
-func (h *Memcached) GetStatus() object.MultiPhaseObjectStatus { return &h.Status }
+func (h *Memcached) GetStatus() object.MultiPhaseObjectStatus { return &h.Status }  // unchanged
```

### 3.2 Step reconciler: generics + `Read()` only

The step action interface is now generic over the CRD type **and** the child object type, so you no
longer type-assert `client.Object`:

```diff
-type configMapReconciler struct {
-    controller.MultiPhaseStepReconcilerAction
-}
+type configMapReconciler struct {
+    multiphase.MultiPhaseStepReconcilerAction[*Memcached, *corev1.ConfigMap]
+}

-func newConfigMapReconciler(c client.Client, recorder record.EventRecorder) controller.MultiPhaseStepReconcilerAction {
-    return &configMapReconciler{
-        MultiPhaseStepReconcilerAction: controller.NewBasicMultiPhaseStepReconcilerAction(
-            c, ConfigmapPhase, ConfigmapCondition, recorder,
-        ),
-    }
-}
+func newConfigMapReconciler(c client.Client, recorder record.EventRecorder) multiphase.MultiPhaseStepReconcilerAction[*Memcached, *corev1.ConfigMap] {
+    return &configMapReconciler{
+        MultiPhaseStepReconcilerAction: multiphase.NewMultiPhaseStepReconcilerAction[*Memcached, *corev1.ConfigMap](
+            c,
+            ConfigmapPhase,
+            ConfigmapCondition,
+            recorder,
+            "memcached-operator", // fieldManager — REQUIRED (SSA owner identity)
+            false,                // dryRun — see §3.4
+        ),
+    }
+}
```

`Read()` now returns a **typed** read and drops the stored logger:

```diff
-func (r *configMapReconciler) Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any, logger *logrus.Entry) (read controller.MultiPhaseRead, res ctrl.Result, err error) {
-    read = controller.NewBasicMultiPhaseRead()
-    ...
-    read.SetCurrentObjects(helper.ToSliceOfObject(currentList))
-    read.SetExpectedObjects(helper.ToSliceOfObject(expectedList))
+func (r *configMapReconciler) Read(ctx context.Context, o *Memcached, data map[string]any, logger *logrus.Entry) (read multiphase.MultiPhaseRead[*corev1.ConfigMap], res reconcile.Result, err error) {
+    read = multiphase.NewMultiPhaseRead[*corev1.ConfigMap]()
+    ...
+    read.AddCurrentObject(&cmList.Items[i]) // concrete type, no helper.ToSliceOfObject
+    read.AddExpectedObject(&expected[i])
 }
```

### 3.3 `Create()`/`Update()` → `Apply()`

The single biggest code change. If you overrode `Create` or `Update`, replace them with `Apply`:

```diff
-func (r *stepReconciler) Update(ctx context.Context, o ..., objects []client.Object, logger *logrus.Entry) (ctrl.Result, error) {
-    for _, obj := range objects {
-        if err := r.Client().Update(ctx, obj); err != nil { ... }
-    }
-}
+func (r *stepReconciler) Apply(ctx context.Context, o *Memcached, data map[string]any, objects []*corev1.ConfigMap, logger *logrus.Entry) (reconcile.Result, error) {
+    // Usually you do NOT override this. The default Apply() does:
+    //   ctrl.SetControllerReference(o, child, scheme)
+    //   child.SetManagedFields(nil); child.SetResourceVersion("")
+    //   client.Patch(child, client.Apply, client.FieldOwner(fieldManager), client.ForceOwnership)
+    return reconcile.Result{}, nil
+}
```

The reconcile flow changed from `Diff → Create/Update/Delete` to:

```
Configure → Read → Diff → OnDiff → Apply (create+update) / Delete (orphans) → OnSuccess
```

- `Apply()` is called once with `diff.GetObjectsToApply()` (the union of create + update lists).
- `Delete()` is called with orphans (`diff.GetObjectsToDelete()`).
- `OnDiff()` is a **new** pre-apply hook (see §3.4).

### 3.4 Diff is now opt-in (`dryRun`)

`Diff()` no longer computes a client-side patch by default:

- **`dryRun=false` (default):** every expected object that has a current counterpart goes to the
  *update* list; the reconciler applies them all. SSA makes this a no-op server-side when nothing
  changed, so there is no update loop. This is the simplest, cheapest mode (no extra API calls).
- **`dryRun=true`:** for each existing object the reconciler performs an **SSA dry-run apply**
  (`client.Apply` + `client.DryRunAll`), normalizes both predicted and current objects (strips
  `resourceVersion`, `generation`, `managedFields`, `creationTimestamp`, `uid`, `status`, and the
  `last-applied-configuration` annotation) and compares them with `cmp.Diff`. Only genuinely
  changed objects land in the update list; unchanged objects are **skipped** (still no write, but it
  cost one dry-run call). This is what you want when you need a real human-readable diff or a
  pre-update task.

**`OnDiff()` contract (important):** the default `OnDiff` returns `controller.ErrDiffDisabled` when
`dryRun=false`. So if you rely on `OnDiff` for pre-apply work you must either enable `dryRun=true`
**or** override `OnDiff` to return `(reconcile.Result{}, nil)`. This guard prevents a silent no-op
where a configured pre-task never runs because diff detection was left off.

Pre-task example (drain before a StatefulSet/Deployment update):

```go
func (r *deploymentReconciler) OnDiff(ctx context.Context, o *Memcached, data map[string]any, diff multiphase.MultiPhaseDiff[*appv1.Deployment], logger *logrus.Entry) (reconcile.Result, error) {
    if diff.NeedUpdate() {
        logger.Infof("Update detected for %d object(s), draining first", len(diff.GetObjectsToUpdate()))
        // drain / scale-down / backup here
    }
    return reconcile.Result{}, nil
}
```

### 3.5 SSA requirements you MUST satisfy on expected objects

SSA is stricter than the old client-side apply. When building expected objects in `Read()`:

1. **Kubernetes 1.22+** is required (SSA is stable since then).
2. **Set `TypeMeta`** (`apiVersion` + `kind`) on every expected object — SSA rejects objects without
   it:
   ```go
   &corev1.ConfigMap{
       TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
       ObjectMeta: metav1.ObjectMeta{Name: o.Name, Namespace: o.Namespace},
       Data:       map[string]string{"k": "v"},
   }
   ```
3. **Deterministic `Name`** — `generateName` is not supported by SSA.
4. **No status fields** in expected objects — apply only spec-level fields your operator owns.
5. **Owner reference:** the default `Apply()` sets the controller reference for you; do not set it in
   the builder as well.

> **Caveat (both human and agent):** mutating webhooks can rewrite objects during the dry-run apply,
> producing false-positive diffs (predicted ≠ live even though your input is identical). If you see
> spurious updates with `dryRun=true`, that is the likely cause. Webhook-noise filtering is a
> documented future enhancement.

### 3.6 Main reconciler wiring

```diff
 type MemcachedReconciler struct {
-    controller.Controller
-    controller.MultiPhaseReconciler
-    controller.MultiPhaseReconcilerAction
+    controller.Controller
+    multiphase.MultiPhaseReconciler[*Memcached]
+    multiphase.MultiPhaseReconcilerAction[*Memcached]
     name            string
-    stepReconcilers []controller.MultiPhaseStepReconcilerAction
+    stepReconcilers []multiphase.MultiPhaseStepReconcilerAction[*Memcached, client.Object]
 }
```

Wrap each typed step with `NewObjectMultiPhaseStepReconcilerAction` so the orchestrator can treat
them uniformly as `client.Object`:

```go
stepReconcilers: []multiphase.MultiPhaseStepReconcilerAction[*Memcached, client.Object]{
    multiphase.NewObjectMultiPhaseStepReconcilerAction[*Memcached, *corev1.ConfigMap, client.Object](configMapStep),
    multiphase.NewObjectMultiPhaseStepReconcilerAction[*Memcached, *appv1.Deployment, client.Object](deploymentStep),
},
```

`Reconcile` now uses `reconcile.Request` (not `ctrl.Request`):

```go
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    o := &cachecrd.Memcached{}
    return r.MultiPhaseReconciler.Reconcile(ctx, req, o, map[string]any{}, r, r.stepReconcilers...)
}
```

### 3.7 Data cleanup after upgrade

Because managed resources no longer store `kubectl.kubernetes.io/last-applied-configuration`, the
old annotation on already-deployed child objects is now dead metadata. SSA will simply take over
field ownership on the next apply; the stale annotation is harmless and is stripped from diff
computation. No manual cleanup is strictly required, but you may remove it for tidiness. The
`cmd/crd` `clean-crd` tool remains available for CRD YAML cleanup.

---

## 4. Migrating a Sentinel operator

Sentinel followed the same SSA migration as multiphase.

- Constructor gains `fieldManager` and `dryRun`:
  `sentinel.NewSentinelAction[*MyObj](client, recorder, "my-operator", false)`.
- `NewSentinelRead` now requires a `runtime.ObjectTyper` (pass `mgr.GetScheme()` /
  `client.Scheme()`): `sentinel.NewSentinelRead(r.Client().Scheme())`.
- `Create()` + `Update()` are replaced by a single `Apply()` (default does SSA patch, same as
  multiphase §3.3).
- The sentinel-specific `SentinelDiff` type is gone; sentinel now reuses
  `multiphase.MultiPhaseDiff[client.Object]` and `multiphase.ClassifyObjects`.
- Same `OnDiff` / `dryRun` contract as multiphase (§3.4), and the same SSA object requirements
  (§3.5): TypeMeta, deterministic names, no status.
- Sentinel still uses **no finalizer** — child cleanup relies on owner references + GC.

Full walkthrough: [`sentinel-reconciler.md`](sentinel-reconciler.md).

---

## 5. Migrating a Remote operator (mostly mechanical)

The remote pattern **keeps its 3-way merge** (client-side, via `generic-objectmatcher`, with the
compressed last-applied-configuration stored in the CRD status). There is **no SSA** here. Changes
are limited to package/type renames:

- Status: `apis.BasicRemoteObjectStatus` → `remote.DefaultRemoteObjectStatus`
  (`pkg/apis/remote`). It still exposes `IsSync`, `LastAppliedConfiguration`.
- Constructors move to the `remote` package and drop `Basic` (see §2.4).
- Type constraint of the first generic parameter changed from `comparable` to
  `object.RemoteObject`; pass your concrete CRD pointer (`*Role`).
- `Reconcile` signature uses `reconcile.Request`.
- The action interface is unchanged in spirit: `GetRemoteHandler`, `Configure`, `Read`, `Create`,
  `Update`, `Delete`, `Diff`, `OnError`, `OnSuccess`, `GetIgnoresDiff`. `Diff` still takes
  `...patch.CalculateOption` and does the current/expected/last-applied 3-way comparison.

Full walkthrough: [`remote-reconciler.md`](remote-reconciler.md).

---

## 6. Test framework changes (`pkg/test`)

The integration test helpers were generified and expanded:

- `NewTestCase` is now generic and no longer takes the object instance:
  `test.NewTestCase[*MyCRD](t, c, key, wait, data)` (v1: `NewTestCase(t, c, key, o, wait, data)`).
- `TestCase[T]` / `TestStep[T]` are generic over the CRD type.
- New reusable steps: `test.NewCreateStep[T](builder)`, `test.NewDeleteStep[T]()`.
- New builder: `test.NewCR[T](name, namespace)` fluent CR builder.
- New assertions: `AssertReadyCondition`, `AssertErrorCondition`, `AssertIsSync`,
  `AssertIsOnError`, `AssertHasPhase`, `AssertOwnerReference`, `AssertManagedByOperator`.
- New wait helpers: `WaitForReadyCondition`, `WaitForGenerationIncrement`,
  `WaitForResourceVersionChange`.
- New `NewInterceptorClient(c)` to inject failures around a `client.Client` for error-path tests.
- `RunWithTimeout` is unchanged.

---

## 7. Helper / API surface changes worth noting

- `apis.BasicObjectStatus` → `apis.DefaultObjectStatus` (fields unchanged: `IsOnError`,
  `Conditions`, `LastErrorMessage`, `ObservedGeneration`).
- New `controller.ErrDiffDisabled` sentinel error (returned by default `OnDiff` when `dryRun=false`).
- New SSA helper package `pkg/helper/ssa`: `DryRunApply`, `IsObjectDiff`, `Normalize`.
- New `controller.UserFacingError(err, maxLen)` + `MaxConditionMessage` / `MaxEventMessage` /
  `MaxStatusMessage` constants for consistent, root-cause-preserving message truncation
  (replaces the old blunt truncation). The historical `ShortenError` constant is deprecated.
- New `controller.EnsureNetworkPolicyForWebhook(...)` helper for webhook network policies.
- New `helper.MustInjectTypeMeta(src, dst)` and generic slice helpers
  (`ToObject`, `ToSliceOfObject`, `ToSlice`, `ToSlicePtr`, generic `DeleteItemFromSlice`).
- `controller.DefaultControllerRateLimiter[reconcile.Request]()` for controller options.

---

## 8. Suggested migration checklist

For each operator, in order:

1. [ ] Bump the module import to `…/operator-sdk-extra/v2` everywhere.
2. [ ] Split imports into `multiphase` / `remote` / `sentinel` / `shared` sub-packages.
3. [ ] Rename `Basic*` → `Default*` status types and `NewBasic*` → `New*` constructors (§2).
4. [ ] Convert reconciler/step types to the generic form (`[*MyCRD]`, `[*MyCRD, *Child]`).
5. [ ] (multiphase/sentinel) Add `fieldManager` + `dryRun` to step/action constructors.
6. [ ] (multiphase/sentinel) Replace `Create()`/`Update()` overrides with `Apply()`.
7. [ ] (multiphase/sentinel) Ensure expected objects set `TypeMeta`, fixed `Name`, no status.
8. [ ] (multiphase/sentinel) Decide `dryRun`; if you use `OnDiff`, honor the `ErrDiffDisabled`
       contract (§3.4).
9. [ ] Swap `ctrl.Request` → `reconcile.Request` in `Reconcile` signatures.
10. [ ] Update tests to the generic `pkg/test` API (§6).
11. [ ] `go build ./...` then run the suite:
        `dagger call --src . test --withGotestsum` (see `AGENTS.md` for filtered output).
12. [ ] Verify no update loops in a live cluster (SSA idempotency) and that orphan deletion still
        works.

---

## 9. Quick decision guide (for LLM agents)

- **"It owns/creates K8s resources from a CRD"** → multiphase pattern, SSA, follow §3.
- **"It watches foreign K8s resources and derives children"** → sentinel pattern, SSA, follow §4.
- **"It talks to an external (non-K8s) API"** → remote pattern, 3-way merge kept, follow §5.
- **Compiler says `undefined: NewBasicX`** → it moved to a sub-package and dropped `Basic` (§2).
- **Compiler says a step needs 6 args** → you're missing `fieldManager` + `dryRun` (§3.2).
- **`OnDiff` returns `ErrDiffDisabled` at runtime** → set `dryRun=true` or override `OnDiff` (§3.4).
- **SSA `PATCH` fails "must specify apiVersion/kind"** → set `TypeMeta` on expected objects (§3.5).
- **SSA `PATCH` fails on `generateName`** → give expected objects a deterministic `Name` (§3.5).
