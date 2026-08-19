# Plan: Split OnDiff into a `...WithDiff` interface variant (multiphase / sentinel / workflow)

## Goal

Eliminate the `dryRun` constructor parameter and the `controller.ErrDiffDisabled` trap by splitting each action interface into two type-safe variants:

- **Base variant** — no `OnDiff` concept; `Diff` always uses always-apply classification (`dryRun=false`).
- **`WithDiff` variant** — embeds the base, adds `OnDiff`; `Diff` always uses SSA dry-run classification (`dryRun=true`); default `OnDiff` returns `nil` (safe no-op, no trap because dry-run is guaranteed).

The reconcilers detect the optional `OnDiff` capability with the standard Go optional-interface idiom (type assertion against the `...WithDiff` interface) and call `OnDiff` only when implemented.

This is a **clean breaking change** (v3 not yet released). No backward-compat wrappers, no deprecated symbols.

## Locked design decisions (do not revisit)

1. Two interfaces per pattern: base (no `OnDiff`) + `...WithDiff` (embeds base, adds `OnDiff`).
2. Reconcilers use optional-interface assertion to call `OnDiff` only when implemented.
3. No `dryRun` parameter in any public constructor. Diff variant ⇒ dry-run always on; simple variant ⇒ dry-run always off.
4. Default `OnDiff` in the diff variant returns `nil` (safe no-op).
5. `Diff` stays in the **base** interface (both variants classify objects; `OnSuccess` still receives a diff in both). `ClassifyObjects` keeps its `dryRun bool` parameter; the two default types pass `false` / `true` respectively.
6. Remove `controller.ErrDiffDisabled`. Keep `controller.ErrWhenCallOnDiffFromReconciler` (still used when an implemented `OnDiff` errors).

---

## New API surface (signatures with type parameters)

### multiphase (`pkg/controller/multiphase/multiphasestep_action.go`)

```go
// Base interface — NO OnDiff.
type MultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] interface {
    controller.ReconcilerAction
    Configure(ctx context.Context, req reconcile.Request, o k8sObject, logger *logrus.Entry) (res reconcile.Result, err error)
    Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (read MultiPhaseRead[k8sStepObject], res reconcile.Result, err error)
    Apply(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObject, logger *logrus.Entry) (res reconcile.Result, err error)
    Delete(ctx context.Context, o k8sObject, data map[string]any, objects []k8sStepObject, logger *logrus.Entry) (res reconcile.Result, err error)
    OnError(ctx context.Context, o k8sObject, data map[string]any, currentErr error, logger *logrus.Entry) (res reconcile.Result, err error)
    OnSuccess(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObject], logger *logrus.Entry) (res reconcile.Result, err error)
    Diff(ctx context.Context, o k8sObject, read MultiPhaseRead[k8sStepObject], data map[string]any, logger *logrus.Entry) (diff MultiPhaseDiff[k8sStepObject], res reconcile.Result, err error)
    GetPhaseName() shared.PhaseName
}

// Diff variant — embeds base, adds OnDiff.
type MultiPhaseStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] interface {
    MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]
    OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObject], logger *logrus.Entry) (res reconcile.Result, err error)
}

// Base default — no dryRun field; Diff calls ClassifyObjects(..., false).
type DefaultMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] struct {
    controller.ReconcilerAction
    phaseName    shared.PhaseName
    fieldManager string
}

// Diff default — embeds base default, overrides Diff (dryRun=true), adds OnDiff returning nil.
type DefaultMultiPhaseStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] struct {
    *DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]
}

func NewMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object](client client.Client, phaseName shared.PhaseName, conditionName shared.ConditionName, recorder record.EventRecorder, fieldManager string) MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]

func NewMultiPhaseStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObject client.Object](client client.Client, phaseName shared.PhaseName, conditionName shared.ConditionName, recorder record.EventRecorder, fieldManager string) MultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]
```

`DefaultMultiPhaseStepReconcilerActionWithDiff.Diff` overrides the embedded `Diff`:
```go
func (h *DefaultMultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]) Diff(ctx context.Context, o k8sObject, read MultiPhaseRead[k8sStepObject], data map[string]any, logger *logrus.Entry) (diff MultiPhaseDiff[k8sStepObject], res reconcile.Result, err error) {
    diff = NewMultiPhaseDiff[k8sStepObject]()
    creates, updates, deletes, diffStrs, err := ClassifyObjects(ctx, h.Client(), read.GetExpectedObjects(), read.GetCurrentObjects(), h.fieldManager, true)
    if err != nil { return diff, res, err }
    PopulateDiff(diff, creates, updates, deletes, diffStrs)
    return diff, res, nil
}
func (h *DefaultMultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]) OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObject], logger *logrus.Entry) (res reconcile.Result, err error) {
    return res, nil
}
```
`NewMultiPhaseStepReconcilerActionWithDiff` constructs a `DefaultMultiPhaseStepReconcilerActionWithDiff` embedding a `NewMultiPhaseStepReconcilerAction(...)` result (cast to `*DefaultMultiPhaseStepReconcilerAction`), mirroring the existing workflow constructor pattern.

### Object wrapper (same file)

```go
// Simple wrapper — NO OnDiff method (so reconciler assertion fails). Returns base interface.
type ObjectMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object] struct {
    controller.ReconcilerAction
    in MultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc]
}
func NewObjectMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object](in MultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc]) MultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectDst]

// Diff wrapper — HAS OnDiff (forwards to inner.OnDiff). Returns WithDiff interface.
type ObjectMultiPhaseStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object] struct {
    controller.ReconcilerAction
    in MultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObjectSrc]
}
func NewObjectMultiPhaseStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object](in MultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObjectSrc]) MultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObjectDst]
```

Both wrappers forward `Configure/Read/Apply/Delete/OnError/OnSuccess/Diff/GetPhaseName` to `h.in` (with the existing `NewObjectMultiphaseRead` / `NewObjectMultiphaseDiff` conversions). The diff wrapper additionally forwards `OnDiff`:
```go
func (h *ObjectMultiPhaseStepReconcilerActionWithDiff[...]) OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff MultiPhaseDiff[k8sStepObjectDst], logger *logrus.Entry) (res reconcile.Result, err error) {
    return h.in.OnDiff(ctx, o, data, NewObjectMultiphaseDiff[k8sStepObjectDst, k8sStepObjectSrc](diff), logger)
}
```
**Critical**: the simple wrapper must NOT define `OnDiff` — otherwise the reconciler's optional-interface assertion would succeed and call a non-existent inner method. Removing the method is what makes the assertion fail for simple steps.

### sentinel (`pkg/controller/sentinel/sentinel_action.go`)

```go
type SentinelReconcilerAction[k8sObject client.Object] interface { /* base, no OnDiff */ }
type SentinelReconcilerActionWithDiff[k8sObject client.Object] interface {
    SentinelReconcilerAction[k8sObject]
    OnDiff(ctx context.Context, o k8sObject, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (res reconcile.Result, err error)
}

type DefaultSentinelAction[k8sObject client.Object] struct {
    controller.ReconcilerAction
    fieldManager string
}
type DefaultSentinelActionWithDiff[k8sObject client.Object] struct {
    *DefaultSentinelAction[k8sObject]
}

func NewSentinelAction[k8sObject client.Object](client client.Client, recorder record.EventRecorder, fieldManager string) SentinelReconcilerAction[k8sObject]
func NewSentinelActionWithDiff[k8sObject client.Object](client client.Client, recorder record.EventRecorder, fieldManager string) SentinelReconcilerActionWithDiff[k8sObject]
```
`DefaultSentinelAction.Diff` calls `multiphase.ClassifyObjects(..., false)`; `DefaultSentinelActionWithDiff.Diff` overrides to call `ClassifyObjects(..., true)` and adds `OnDiff` returning `nil`. `NewSentinelActionWithDiff` embeds a `NewSentinelAction(...)` result cast to `*DefaultSentinelAction`.

### workflow (`pkg/controller/workflow/workflow_step_action.go`)

```go
type WorkflowStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] interface {
    multiphase.MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]
    CurrentPhase(o k8sObject) workflow.WorkflowPhase
    AdvancePhase(ctx context.Context, o k8sObject, to workflow.WorkflowPhase, logger *logrus.Entry) (res reconcile.Result, err error)
    IsPhase(o k8sObject, phase workflow.WorkflowPhase) bool
    IsPhaseEmpty(o k8sObject) bool
}
type WorkflowStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] interface {
    multiphase.MultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]
    CurrentPhase(o k8sObject) workflow.WorkflowPhase
    AdvancePhase(ctx context.Context, o k8sObject, to workflow.WorkflowPhase, logger *logrus.Entry) (res reconcile.Result, err error)
    IsPhase(o k8sObject, phase workflow.WorkflowPhase) bool
    IsPhaseEmpty(o k8sObject) bool
}

type DefaultWorkflowStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] struct {
    *multiphase.DefaultMultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]
}
type DefaultWorkflowStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObject client.Object] struct {
    *multiphase.DefaultMultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]
}

func NewWorkflowStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObject client.Object](c client.Client, phaseName shared.PhaseName, conditionName shared.ConditionName, recorder record.EventRecorder, fieldManager string) WorkflowStepReconcilerAction[k8sObject, k8sStepObject]
func NewWorkflowStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObject client.Object](c client.Client, phaseName shared.PhaseName, conditionName shared.ConditionName, recorder record.EventRecorder, fieldManager string) WorkflowStepReconcilerActionWithDiff[k8sObject, k8sStepObject]
```
Workflow has **no own reconciler** calling `OnDiff` — it reuses `multiphase.DefaultMultiPhaseStepReconciler.Reconcile`, whose optional-interface assertion against `MultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]` succeeds for `*DefaultWorkflowStepReconcilerActionWithDiff` (via embedded `OnDiff`). The phase-management methods (`getWorkflowStatus`, `CurrentPhase`, `AdvancePhase`, `IsPhase`, `IsPhaseEmpty`) are identical on both default types — extract a shared unexported helper or duplicate (small); recommend duplicating onto both default types to keep embedding clean.

---

## File-by-file changes

### 1. `pkg/controller/reconciler.go`
- **Remove** the `ErrDiffDisabled` sentinel (line 26).
- Keep `ErrWhenCallOnDiffFromReconciler` (line 20) — still used by reconcilers when an implemented `OnDiff` errors.

### 2. `pkg/controller/multiphase/multiphasestep_action.go`
- Split `MultiPhaseStepReconcilerAction` → base (remove `OnDiff` method) + `MultiPhaseStepReconcilerActionWithDiff` (embeds base, adds `OnDiff`). Update doc comments: remove all `dryRun`/`ErrDiffDisabled` mentions; state that the diff variant always uses SSA dry-run classification and the simple variant always uses always-apply classification.
- `DefaultMultiPhaseStepReconcilerAction`: remove `dryRun` field. `Diff` calls `ClassifyObjects(..., false)`. **Delete** the `OnDiff` method entirely.
- Add `DefaultMultiPhaseStepReconcilerActionWithDiff` embedding `*DefaultMultiPhaseStepReconcilerAction`; override `Diff` (pass `true`); add `OnDiff` returning `nil`.
- `NewMultiPhaseStepReconcilerAction`: drop `dryRun bool` param; return `MultiPhaseStepReconcilerAction[...]` (base).
- Add `NewMultiPhaseStepReconcilerActionWithDiff`: same params (no `dryRun`); return `MultiPhaseStepReconcilerActionWithDiff[...]`, embedding a `NewMultiPhaseStepReconcilerAction(...)` result cast to `*DefaultMultiPhaseStepReconcilerAction`.
- `ObjectMultiPhaseStepReconcilerAction` (simple wrapper): **remove** its `OnDiff` method. Keep `Diff` forwarding. Constructor `NewObjectMultiPhaseStepReconcilerAction` unchanged signature (takes base, returns base).
- Add `ObjectMultiPhaseStepReconcilerActionWithDiff` + `NewObjectMultiPhaseStepReconcilerActionWithDiff` (takes `MultiPhaseStepReconcilerActionWithDiff[src]`, returns `MultiPhaseStepReconcilerActionWithDiff[dst]`); forward all base methods + `OnDiff`.

### 3. `pkg/controller/multiphase/multiphasestep_reconciler.go`
- Replace the unconditional `OnDiff` call (lines 83–91) with optional-interface detection:
  ```go
  if diffAction, ok := reconcilerAction.(MultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]); ok {
      res, err = diffAction.OnDiff(ctx, o, data, diff, logger)
      if err != nil {
          logger.Errorf("Error when call 'onDiff' from step reconciler: %s", err.Error())
          return reconcilerAction.OnError(ctx, o, data, errors.Wrap(err, controller.ErrWhenCallOnDiffFromReconciler.Error()), logger)
      }
      if res != (reconcile.Result{}) {
          return res, nil
      }
  }
  ```
- Generics note: the assertion target `MultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]` uses the same type params as the receiver — valid Go generic interface assertion.

### 4. `pkg/controller/multiphase/multiphase_reconciler.go`
- No structural change. The variadic `...MultiPhaseStepReconcilerAction[k8sObject, client.Object]` keeps the **base** type; `...WithDiff` steps satisfy it via embedding. Verify the existing `h.reconcilerStep.Reconcile(...)` call still type-checks (it does — `MultiPhaseStepReconciler[k8sObject, client.Object]` takes the base interface, which both variants satisfy).

### 5. `pkg/controller/multiphase/multiphase_diff.go`
- No code change. `ClassifyObjects` keeps `dryRun bool`. Update only the doc comment if it references removed concepts (it references `dryRun` parameter — keep, still accurate).

### 6. `pkg/controller/sentinel/sentinel_action.go`
- Split `SentinelReconcilerAction` → base (remove `OnDiff`) + `SentinelReconcilerActionWithDiff` (embeds, adds `OnDiff`). Update doc comments.
- `DefaultSentinelAction`: remove `dryRun` field; `Diff` calls `ClassifyObjects(..., false)`; **delete** `OnDiff` method.
- Add `DefaultSentinelActionWithDiff` embedding `*DefaultSentinelAction`; override `Diff` (pass `true`); add `OnDiff` returning `nil`.
- `NewSentinelAction`: drop `dryRun` param; return base.
- Add `NewSentinelActionWithDiff`: no `dryRun`; return `SentinelReconcilerActionWithDiff`, embedding `NewSentinelAction(...)` cast to `*DefaultSentinelAction`.

### 7. `pkg/controller/sentinel/sentinel_reconciler.go`
- Replace unconditional `OnDiff` call (lines 123–131) with optional-interface detection against `SentinelReconcilerActionWithDiff[k8sObject]`, same pattern as multiphase.

### 8. `pkg/controller/workflow/workflow_step_action.go`
- Split `WorkflowStepReconcilerAction` → base (embeds `multiphase.MultiPhaseStepReconcilerAction`) + `WorkflowStepReconcilerActionWithDiff` (embeds `multiphase.MultiPhaseStepReconcilerActionWithDiff`); both keep the 4 phase methods.
- `DefaultWorkflowStepReconcilerAction`: embed `*multiphase.DefaultMultiPhaseStepReconcilerAction`; phase methods as today.
- Add `DefaultWorkflowStepReconcilerActionWithDiff`: embed `*multiphase.DefaultMultiPhaseStepReconcilerActionWithDiff`; duplicate the phase methods (`getWorkflowStatus`, `CurrentPhase`, `AdvancePhase`, `IsPhase`, `IsPhaseEmpty`) — identical bodies.
- `NewWorkflowStepReconcilerAction`: drop `dryRun` param; return base; embed `multiphase.NewMultiPhaseStepReconcilerAction(...)` cast to `*multiphase.DefaultMultiPhaseStepReconcilerAction`.
- Add `NewWorkflowStepReconcilerActionWithDiff`: no `dryRun`; return `WorkflowStepReconcilerActionWithDiff`; embed `multiphase.NewMultiPhaseStepReconcilerActionWithDiff(...)` cast to `*multiphase.DefaultMultiPhaseStepReconcilerActionWithDiff`.

### 9. `samples/memcached-operator/controllers/configmap_reconciler.go`
- `configMapReconciler` embeds `multiphase.MultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap]` (base — unchanged type).
- `newConfigMapReconciler` calls `multiphase.NewMultiPhaseStepReconcilerAction[...]` **without** `dryRun` (was `false`).
- **Delete** the `OnDiff` stub method (lines 68–70).

### 10. `samples/memcached-operator/controllers/deployment_reconciler.go`
- `deploymentReconciler` embeds `multiphase.MultiPhaseStepReconcilerActionWithDiff[*cachecrd.Memcached, *appv1.Deployment]`.
- `newDeploymentReconciler` returns `multiphase.MultiPhaseStepReconcilerActionWithDiff[...]` and calls `multiphase.NewMultiPhaseStepReconcilerActionWithDiff[...]` (no `dryRun`).
- Keep the `OnDiff` override (it uses `diff.NeedUpdate()` / `diff.GetObjectsToUpdate()`).

### 11. `samples/memcached-operator/controllers/memcached_controller.go`
- `stepReconcilers` slice type stays `[]multiphase.MultiPhaseStepReconcilerAction[*cachecrd.Memcached, client.Object]` (base).
- `configMapStep` (simple) → `multiphase.NewObjectMultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap, client.Object](configMapStep)` (unchanged call).
- `deploymentStep` (diff) → `multiphase.NewObjectMultiPhaseStepReconcilerActionWithDiff[*cachecrd.Memcached, *appv1.Deployment, client.Object](deploymentStep)`.

### 12. `samples/ingress-sentinel-operator/controllers/ingress_sentinel_controller.go`
- `ingressSentinelAction` embeds `sentinel.SentinelReconcilerAction[k8sObject]` (base — unchanged).
- `newIngressSentinelAction` calls `sentinel.NewSentinelAction[k8sObject](c, recorder, "ingress-sentinel-operator")` (drop `false`).

### 13. `pkg/controller/multiphase/controller_sample_test.go` (test fixture, not a sample)
- `configMapReconciler` embeds `MultiPhaseStepReconcilerAction[*MultiPhaseObject, *corev1.ConfigMap]` — currently uses `dryRun=true` with no `OnDiff` override. Convert to the **diff** variant to preserve dry-run classification behavior:
  - embed `MultiPhaseStepReconcilerActionWithDiff[*MultiPhaseObject, *corev1.ConfigMap]`.
  - `newConfiMapReconciler` calls `NewMultiPhaseStepReconcilerActionWithDiff[...]` (drop `true`); returns `MultiPhaseStepReconcilerActionWithDiff[...]`.
- `NewTestReconciler` step slice: wrap with `NewObjectMultiPhaseStepReconcilerActionWithDiff[*MultiPhaseObject, *corev1.ConfigMap, client.Object](...)`.

### 14. `pkg/controller/sentinel/controller_sample_test.go` (test fixture)
- `templateAnnotationsReconciler` uses `dryRun=true`, no `OnDiff` override → convert to diff variant:
  - embed `SentinelReconcilerActionWithDiff[k8sObject]`.
  - `newTemplateAnnotationsReconciler` calls `NewSentinelActionWithDiff[k8sObject](c, recorder, "test-sentinel-operator")` (drop `true`); returns `SentinelReconcilerActionWithDiff[k8sObject]`.
- `TestReconciler.SentinelReconcilerAction` field type → `SentinelReconcilerActionWithDiff[*corev1.Namespace]`.

### 15. Documentation
- `documentations/multi-phase-reconciler.md`: rewrite the "Constructors" table row (drop `dryRun`), the "Reconciliation Flow" / "Constructor" / "Detecting changes" / "OnDiff pre-task pattern" / "Disabled-diff contract" sections. Replace with: two variants explanation, `NewMultiPhaseStepReconcilerAction` (simple, always-apply) vs `NewMultiPhaseStepReconcilerActionWithDiff` (dry-run + optional `OnDiff` no-op default), optional-interface detection semantics, `NewObjectMultiPhaseStepReconcilerActionWithDiff` wrapper.
- `documentations/sentinel-reconciler.md`: same rewrite for sentinel (`NewSentinelAction` / `NewSentinelActionWithDiff`).
- `documentations/migration-v2-to-v3.md`: update §2.1 code sample (drop `dryRun` arg), §TL;DR "Diff is only computed when you set `dryRun=true`" → "Diff variant (`...WithDiff`) uses SSA dry-run; simple variant always-applies". Remove `ErrDiffDisabled` references.
- `documentations/migration-v1-to-v2.md`: this is a v1→v2 historical doc. **Out of scope** to rewrite history, but add a short note at top or in a v3 addendum? Decision: **leave as-is** (it documents a past migration; v2→v3 doc covers the current break). Mark out of scope.
- `documentations/tls-and-workflow.md`: grep found no `dryRun`/`OnDiff`/`ErrDiffDisabled` references → no change.

---

## Test plan

### Updated existing tests (constructor call sites — drop `dryRun` arg)

- `pkg/controller/multiphase/action_unit_test.go`: lines 111, 361, 373–374, 388, 458–459 — remove the trailing `false` arg. Where the test exercises `Diff` behavior, decide variant: these tests use `false` (simple) and assert create/update/delete classification with always-apply. Keep as **simple** variant (`NewMultiPhaseStepReconcilerAction`). The `TestNewObjectMultiPhaseStepReconcilerAction` and `TestObjectMultiPhaseStepReconcilerActionImplementations` use simple inner + simple wrapper — keep simple.
- `pkg/controller/multiphase/multiphasestep_action_unit_test.go`: lines 107, 144, 184, 224, 277 — drop `false`; keep as simple variant.
- `pkg/controller/sentinel/sentinel_action_unit_test.go`: lines 71, 87, 115, 146, 177, 210, 236 — drop `false`; keep as simple variant (`NewSentinelAction`). `TestDefaultSentinelAction_Diff` asserts always-apply classification (apply list of 3) — stays valid for simple variant.
- `pkg/controller/workflow/workflow_step_action_test.go`: lines 84–91, 104–106, 130–132, 152–154, 176–178 — drop `true`; switch to `NewWorkflowStepReconcilerActionWithDiff` (these tests only check phase methods, not diff; either variant works — use WithDiff to also cover that constructor). Update the 5 call sites.
- `pkg/controller/sentinel/sentinel_reconciler_unit_test.go`: `mockSentinelReconcilerAction` embeds `SentinelReconcilerAction[*corev1.Pod]` (base) **and** defines `OnDiff`. After the split, the reconciler's assertion to `SentinelReconcilerActionWithDiff[*corev1.Pod]` **succeeds** (mock has matching `OnDiff`). Existing "successful reconcile" test still calls `OnDiff` (returns nil) — fine. **No change required** to the mock for existing tests, but see new tests below.

### New tests to add (100% coverage target)

**`pkg/controller/multiphase/multiphasestep_action_unit_test.go`** (or a new `multiphasestep_action_withdiff_unit_test.go`):
1. `TestNewMultiPhaseStepReconcilerActionWithDiff` — construct, assert non-nil, `GetPhaseName` correct.
2. `TestDefaultMultiPhaseStepReconcilerActionWithDiff_OnDiff` — default `OnDiff` returns `(reconcile.Result{}, nil)`.
3. `TestDefaultMultiPhaseStepReconcilerActionWithDiff_Diff` — with a current object that matches expected, assert the object is **not** in the update list when SSA dry-run says unchanged (use envtest fake client or skip if fake client lacks dry-run; alternatively assert the dry-run code path is invoked by checking `diff.IsDiff()` is false for identical objects — note: fake client may not support `DryRunAll`; if so, mark with `t.Skip` like the existing `Apply` test, but still cover the `OnDiff` nil path and the constructor).
4. `TestNewObjectMultiPhaseStepReconcilerActionWithDiff` — construct with a WithDiff inner, assert returned type satisfies `MultiPhaseStepReconcilerActionWithDiff[dst]`; assert `OnDiff` forwards to inner (use a mock inner that records the call).
5. `TestObjectMultiPhaseStepReconcilerAction_SimpleDoesNotImplementOnDiff` — compile-time / runtime check that the simple wrapper does **not** satisfy `MultiPhaseStepReconcilerActionWithDiff[dst]` (assertion returns `ok=false`).

**`pkg/controller/multiphase/multiphasestep_reconciler_unit_test.go`** (new file — there is currently no unit test for `DefaultMultiPhaseStepReconciler.Reconcile`):
6. `TestDefaultMultiPhaseStepReconciler_Reconcile_SimpleAction_OnDiffNotCalled` — mock step action implementing only the **base** interface (no `OnDiff`); set a flag if `OnDiff` would be called (it can't, since not implemented); assert full Reconcile completes and `OnSuccess` called with the diff. Covers the `ok=false` branch.
7. `TestDefaultMultiPhaseStepReconciler_Reconcile_WithDiffAction_OnDiffCalled` — mock step action implementing `MultiPhaseStepReconcilerActionWithDiff`; assert `OnDiff` is invoked with the computed diff and that an `OnDiff` error propagates through `OnError` (covers `err != nil` branch and `ErrWhenCallOnDiffFromReconciler`).
8. `TestDefaultMultiPhaseStepReconciler_Reconcile_WithDiffAction_OnDiffRequeue` — `OnDiff` returns `reconcile.Result{Requeue:true}`; assert short-circuit.

**`pkg/controller/sentinel/sentinel_action_unit_test.go`**:
9. `TestNewSentinelActionWithDiff` / `TestDefaultSentinelActionWithDiff_OnDiff` (nil) / `TestDefaultSentinelActionWithDiff_Diff` (dry-run path; skip if fake client lacks dry-run).

**`pkg/controller/sentinel/sentinel_reconciler_unit_test.go`**:
10. `TestDefaultSentinelReconciler_Reconcile_SimpleAction_OnDiffNotCalled` — add a `mockSentinelReconcilerActionSimple` that does **not** implement `OnDiff`; assert reconcile completes without calling OnDiff (covers `ok=false` branch). The existing `mockSentinelReconcilerAction` (with `OnDiff`) already covers the `ok=true` path including the nil-return success case.
11. `TestDefaultSentinelReconciler_Reconcile_WithDiffAction_OnDiffError` — `OnDiff` returns an error; assert it propagates via `OnError` (covers `ErrWhenCallOnDiffFromReconciler` for sentinel).
12. `TestDefaultSentinelReconciler_Reconcile_WithDiffAction_OnDiffRequeue` — `OnDiff` returns requeue; assert short-circuit.

**`pkg/controller/workflow/workflow_step_action_test.go`**:
13. `TestNewWorkflowStepReconcilerActionWithDiff` — construct WithDiff variant; assert phase methods work and that it satisfies `multiphase.MultiPhaseStepReconcilerActionWithDiff`.
14. `TestDefaultWorkflowStepReconcilerActionWithDiff_OnDiff` — default `OnDiff` returns nil.

### Coverage notes
- The optional-interface assertion has two branches (`ok=true`, `ok=false`); both must be exercised in each reconciler (multiphase + sentinel) to keep 100% coverage.
- The `OnDiff` error branch and requeue branch must be exercised in each reconciler.
- Default `OnDiff` returning nil must be covered for all three diff default types.
- If envtest fake client cannot do SSA dry-run (`client.DryRunAll`), the `Diff` dry-run classification path of the diff defaults is already covered by integration tests (`controller_test.go` uses real envtest). For unit tests, skip with a clear message (consistent with existing `TestDefaultSentinelAction_Apply` skip pattern) — but still cover the `OnDiff` and constructor paths.

### Verification command
```bash
dagger call --src . test --withGotestsum 2>&1 | grep -E "(FAIL|build failed|\.go:[0-9]+:|panic|--- FAIL)" | head -200
```
Also run targeted:
```bash
dagger call --src . test --withGotestsum --path ./pkg/controller/multiphase/
dagger call --src . test --withGotestsum --path ./pkg/controller/sentinel/
dagger call --src . test --withGotestsum --path ./pkg/controller/workflow/
```
And full CI (format + lint + vulncheck + tests + manifests):
```bash
dagger call --src . ci
```

---

## Implementation ordering

1. **`pkg/controller/reconciler.go`** — remove `ErrDiffDisabled`. (Do first so downstream files can't reference it.)
2. **`pkg/controller/multiphase/multiphasestep_action.go`** — split interfaces, default types, constructors, object wrappers. (Core change.)
3. **`pkg/controller/multiphase/multiphasestep_reconciler.go`** — optional-interface assertion for `OnDiff`.
4. **`pkg/controller/multiphase/multiphase_reconciler.go`** — verify compiles (no structural change expected).
5. **`pkg/controller/sentinel/sentinel_action.go`** — split interfaces, default types, constructors.
6. **`pkg/controller/sentinel/sentinel_reconciler.go`** — optional-interface assertion for `OnDiff`.
7. **`pkg/controller/workflow/workflow_step_action.go`** — split interfaces, default types, constructors.
8. **Update test fixtures** (`pkg/controller/multiphase/controller_sample_test.go`, `pkg/controller/sentinel/controller_sample_test.go`) — switch to WithDiff variants where `dryRun=true` was used.
9. **Update existing unit tests** — drop `dryRun` args across multiphase/sentinel/workflow `*_test.go`.
10. **Add new unit tests** (steps 6–14 in test plan) — including the new `multiphasestep_reconciler_unit_test.go` for the step reconciler.
11. **Update samples** (`samples/memcached-operator/controllers/{configmap,deployment,memcached_controller}.go`, `samples/ingress-sentinel-operator/controllers/ingress_sentinel_controller.go`).
12. **Update documentation** (`multi-phase-reconciler.md`, `sentinel-reconciler.md`, `migration-v2-to-v3.md`).
13. **Run** `dagger call --src . ci` and fix any failures; iterate until green and 100% coverage maintained.

---

## Risks & edge cases

- **Generic optional-interface assertion**: the assertion target must use the *exact* type parameters of the receiver (`MultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObject]`). A mismatched type arg (e.g. asserting against `client.Object` when the action is typed `*ConfigMap`) would silently fail. The reconciler always asserts with its own type params, so this is safe — but the object wrapper must preserve the dst type so the assertion against `client.Object` succeeds for diff wrappers. Verified in design above.
- **Simple wrapper must not define `OnDiff`**: if a stale `OnDiff` method remains on `ObjectMultiPhaseStepReconcilerAction`, the reconciler would call it for simple steps (which have no inner `OnDiff`). This is the single most important correctness invariant — double-check the simple wrapper has no `OnDiff` method.
- **Embedding shadowing**: `DefaultMultiPhaseStepReconcilerActionWithDiff.Diff` must shadow the embedded `Diff`. Confirm via a test that the diff default's `Diff` uses dry-run (e.g. unchanged object not classified as update) and the simple default's `Diff` classifies it as update.
- **`OnSuccess` still receives a diff** in both variants (structural classification always runs). Sample/doc code that reads `diff` in `OnSuccess` keeps working.
- **Fake client dry-run**: `sigs.k8s.io/controller-runtime` fake client historically does not honor `client.DryRunAll`. Dry-run `Diff` classification is covered by envtest integration tests; unit tests for the diff defaults' `Diff` may need `t.Skip` (consistent with existing `TestDefaultSentinelAction_Apply` skip). Do not block the plan on this — cover `OnDiff`/constructor paths in unit tests and rely on integration tests for dry-run classification.
- **`pkg/mock/`**: only contains `remote_reconciler.go` (remote pattern) — **not affected**. No multiphase/sentinel/workflow mocks exist; the new tests use hand-written mocks in the test files (matching existing convention).
- **`samples/elasticsearch-operator/`**: uses the `remote` pattern only — **not affected** (verified via grep: no `NewSentinelAction`/`NewMultiPhaseStepReconcilerAction`/`NewWorkflowStepReconcilerAction`/`dryRun`/`OnDiff` references).
- **`documentations/migration-v1-to-v2.md`**: historical v1→v2 doc; out of scope (do not rewrite past migrations).

## Out of scope

- `pkg/controller/remote/*` (uses 3-way merge, no `dryRun`/`OnDiff`).
- `pkg/controller/certificate/*`.
- `documentations/migration-v1-to-v2.md`.
- Any change to `ClassifyObjects` signature or `MultiPhaseDiff`/`MultiPhaseRead` interfaces.
