# Auto-mode cleanup of legacy v2 `kubectl.kubernetes.io/last-applied-configuration` annotations

## Goal

When an operator built on operator-sdk-extra/v2 is upgraded to /v3, child K8s
resources (ConfigMaps, Services, StatefulSets, NetworkPolicies, ...) created
under v2 still carry the legacy client-side 3-way-diff annotation
`kubectl.kubernetes.io/last-applied-configuration` (set by
`github.com/disaster37/k8s-objectmatcher/patch.DefaultAnnotator.SetLastAppliedAnnotation`).
v3's multiphase and sentinel controllers use Server-Side Apply (SSA) and no
longer need it. This plan implements automatic ("auto mode") cleanup of the
annotation from managed child resources during normal reconciliation, with no
operator-developer intervention.

## Scope

**In scope:** multiphase and sentinel controllers' managed child resources
(objects returned by a step/sentinel `Read` as `currentObjects` that have a
matching `expectedObjects` entry).

**Out of scope (verified):**
- `pkg/controller/remote/` — stores its last-applied config in CRD status
  (`remote_action.go` `SetLastAppliedConfiguration`), not as a K8s annotation.
  No K8s annotation cleanup needed.
- `pkg/controller/helper.go` `EnsureNetworkPolicyForWebhook` — a standalone
  setup-time helper (called from operator `SetupWithManager`/webhook setup, not
  from per-resource reconcile; no sample calls it during reconcile). It still
  uses `patch.DefaultAnnotator` + `patch.DefaultPatchMaker` + `c.Update`. It
  manages a singleton NetworkPolicy that is NOT a multiphase/sentinel managed
  child, so the auto-cleanup below never touches it (no fight loop). Migrating
  it to SSA is a worthwhile follow-up but is **out of scope** for this plan
  (tracked as Open Question / follow-up).

## Key technical decision: cleanup mechanism

**Chosen: JSON merge patch (`client.RawPatch(types.MergePatchType, data)`)** that
sets `metadata.annotations["kubectl.kubernetes.io/last-applied-configuration"]`
to `null`, issued only when the live object still has the annotation.

**Why not SSA (the task's suggested follow-up SSA patch with remaining
annotations + ForceOwnership):** SSA can only delete a map key that the applying
field manager *owns*. The v3 field manager never owned the stale key (it was
set by v2's `patch.DefaultAnnotator`, a different field manager). An SSA apply
that *omits* the key leaves it in place (no conflict → no ForceOwnership
trigger → no deletion). An SSA apply that lists the remaining annotations
minus the stale key with `ForceOwnership` would steal ownership of the *other*
(third-party) annotation keys via ForceOwnership — undesirable — and still
would not delete the stale key (it is not listed, and the v3 manager never
owned it). An SSA apply with the key set to `null` + `ForceOwnership` *would*
work but requires building an `unstructured.Unstructured` with the correct GVK
(typed objects from `Read` often have empty `GetObjectKind()` GVK), adding
fragility. The JSON merge patch with `null` deletes the key **regardless of
field ownership**, is idempotent, does not claim ownership of other annotation
keys (preserves third-party annotations), and needs no GVK (the typed object's
Go type provides it via the scheme).

**Trade-off (accepted):** the merge patch records a transient managedFields
entry under a default patch field manager. This is a one-time migration
side-effect and harmless. The v3 field manager's normal SSA apply on the next
reconcile continues to own the fields it declares.

## Integration strategy: single choke point per reconciler (Read path)

Cleanup runs **once per reconcile, right after `Read` succeeds and before
`Diff`**, over the matched current objects. This single hook covers:

- objects that will be applied (creates + updates) in both simple and diff
  variants,
- **unchanged objects in the diff variant** (which are never passed to `Apply`
  and would otherwise never be cleaned),
- all managed children except orphans (current objects with no expected
  counterpart, which are about to be deleted and are skipped).

It uses the live annotations already present on `read.GetCurrentObjects()`
(fetched by the user's `Read`), so **no extra GET** is needed. It does not
modify `Apply`, so user-overridden `Apply` implementations are unaffected.

**Why not hook into `Apply` only:** `Apply` only receives objects in the
create/update lists. In the diff variant, unchanged objects are never applied,
so a stale annotation on a steady-state object would never be cleaned. The
Read-path hook covers them.

**Non-interference with Diff:** `pkg/helper/ssa/diff.go` `normalizeMap` already
strips the stale annotation before comparing, so removing it from the live
object does not change diff results. The in-memory `currentObjects` still hold
the annotation until the next `Read`, but `normalizeMap` ignores it. The
cleanup patch bumps the live `resourceVersion`, but `Diff` uses a dry-run apply
on the *expected* object (not the current `resourceVersion`) and `Apply`
clears `resourceVersion` on the expected object — so no conflict.

## Files to create / modify

### NEW `pkg/helper/ssa/cleanup.go`

Low-level per-object helper. Same package as `diff.go`, so it reuses the
unexported `lastAppliedConfigAnnotation` const (no new const needed).

```go
package ssa

import (
    "context"
    "encoding/json"

    "emperror.dev/errors"
    "github.com/sirupsen/logrus"
    "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// CleanupLastAppliedAnnotation removes the legacy
// kubectl.kubernetes.io/last-applied-configuration annotation from the live
// object represented by liveObj if present.
//
// It issues a JSON merge patch (MergePatchType) that sets the annotation key
// to null, which deletes the key regardless of which field manager owns it.
// This is necessary because SSA cannot delete a map key the applying field
// manager does not own, and the v3 field manager never owned this key.
//
// liveObj must reflect the current live server state: its annotations are
// inspected to decide whether a patch is needed. liveObj is not mutated.
//
// Idempotent: no-op (returns removed=false, nil) when the annotation is
// absent, when liveObj has a deletionTimestamp, or when liveObj has no
// annotations. Does not use SSA and does not require a fieldManager.
func CleanupLastAppliedAnnotation(
    ctx context.Context,
    c client.Client,
    liveObj client.Object,
    logger *logrus.Entry,
) (removed bool, err error) {
    if liveObj.GetDeletionTimestamp() != nil {
        return false, nil
    }
    if _, ok := liveObj.GetAnnotations()[lastAppliedConfigAnnotation]; !ok {
        return false, nil
    }

    patch := map[string]any{
        "metadata": map[string]any{
            "annotations": map[string]any{
                lastAppliedConfigAnnotation: nil,
            },
        },
    }
    data, err := json.Marshal(patch)
    if err != nil {
        return false, errors.Wrapf(err, "Error when marshalling cleanup patch for object '%s'", liveObj.GetName())
    }

    if err = c.Patch(ctx, liveObj, client.RawPatch(types.MergePatchType, data)); err != nil {
        return false, errors.Wrapf(err, "Error when removing last-applied-configuration annotation on object '%s'", liveObj.GetName())
    }

    logger.Debugf("Removed legacy last-applied-configuration annotation from object '%s'", liveObj.GetName())
    return true, nil
}
```

### NEW `pkg/controller/multiphase/cleanup.go`

Iteration helper over a `MultiPhaseRead`. Skips orphans (current objects with
no matching expected object by name), objects with deletionTimestamp, and
objects without the annotation. Best-effort: per-object errors are logged at
warn and do not abort the loop.

```go
package multiphase

import (
    "context"

    ssadiff "github.com/disaster37/operator-sdk-extra/v3/pkg/helper/ssa"
    "github.com/sirupsen/logrus"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// CleanupReadLastAppliedAnnotations removes the legacy
// kubectl.kubernetes.io/last-applied-configuration annotation from all current
// objects in read that have a matching expected object (managed children that
// are not orphans). Orphans are skipped because they are about to be deleted.
//
// Best-effort: per-object cleanup failures are logged at warn level and do not
// abort the loop or return an error. Returns the number of objects cleaned.
func CleanupReadLastAppliedAnnotations[T client.Object](
    ctx context.Context,
    c client.Client,
    read MultiPhaseRead[T],
    logger *logrus.Entry,
) (cleaned int) {
    expectedNames := make(map[string]struct{}, len(read.GetExpectedObjects()))
    for _, o := range read.GetExpectedObjects() {
        expectedNames[o.GetName()] = struct{}{}
    }

    for _, live := range read.GetCurrentObjects() {
        if _, ok := expectedNames[live.GetName()]; !ok {
            continue // orphan, will be deleted
        }
        removed, err := ssadiff.CleanupLastAppliedAnnotation(ctx, c, live, logger)
        if err != nil {
            logger.Warnf("Failed to clean last-applied-configuration annotation on object '%s': %s (continuing)", live.GetName(), err.Error())
            continue
        }
        if removed {
            cleaned++
        }
    }
    return cleaned
}
```

### MODIFY `pkg/controller/multiphase/multiphasestep_reconciler.go`

After the `Read` success block (after `logger.Debug("Call 'read' from step reconciler successfully")` and its `if res != (reconcile.Result{})` guard, before the `// Compute diff` block), insert:

```go
// Auto mode: remove legacy v2 kubectl last-applied-configuration annotations
// from managed child resources. Best-effort, idempotent, no-op once cleaned.
// Runs before Diff so unchanged objects (diff variant) are also covered.
// normalizeMap in pkg/helper/ssa strips this annotation, so cleanup does not
// affect diff results.
if cleaned := CleanupReadLastAppliedAnnotations(ctx, h.Client(), read, logger); cleaned > 0 {
    logger.Debugf("Cleaned legacy last-applied-configuration annotation from %d managed object(s)", cleaned)
}
```

### MODIFY `pkg/controller/sentinel/sentinel_reconciler.go`

After the `Read` success block (after `logger.Debug("Call 'read' from reconciler successfully")` and its `if res != (reconcile.Result{})` guard, before the `// Compute diff` block), insert:

```go
// Auto mode: remove legacy v2 kubectl last-applied-configuration annotations
// from managed child resources. Best-effort, idempotent, no-op once cleaned.
for objectType, r := range read.GetReads() {
    if cleaned := multiphase.CleanupReadLastAppliedAnnotations(ctx, h.Client(), r, logger); cleaned > 0 {
        logger.Debugf("Cleaned legacy last-applied-configuration annotation from %d managed object(s) of type '%s'", cleaned, objectType)
    }
}
```

(`sentinel_reconciler.go` already imports `github.com/disaster37/operator-sdk-extra/v3/pkg/controller/multiphase`.)

## Step-by-step algorithm (per reconcile, per managed object)

1. Reconciler calls `Read` → user returns `MultiPhaseRead` with `currentObjects`
   (live, with annotations) and `expectedObjects`.
2. Reconciler calls `CleanupReadLastAppliedAnnotations(ctx, client, read, logger)`.
3. For each `live` in `read.GetCurrentObjects()`:
   1. If `live.GetName()` not in expected names → skip (orphan, will be deleted).
   2. Call `ssadiff.CleanupLastAppliedAnnotation(ctx, client, live, logger)`:
      1. If `live.GetDeletionTimestamp() != nil` → return (false, nil).
      2. If `lastAppliedConfigAnnotation` not in `live.GetAnnotations()` → return (false, nil).
      3. Marshal `{"metadata":{"annotations":{"kubectl.kubernetes.io/last-applied-configuration":null}}}`.
      4. `client.Patch(ctx, live, client.RawPatch(types.MergePatchType, data))`.
      5. On success: `logger.Debugf(...)`, return (true, nil).
      6. On error: wrap with `emperror.dev/errors`, return (false, err).
   3. On error: `logger.Warnf(...)` and continue (do not abort).
4. Reconciler proceeds to `Diff` (unaffected — `normalizeMap` strips the
   annotation), `OnDiff`, `Apply`, `Delete`, `OnSuccess`.

## Edge cases

| Case | Handling |
|---|---|
| Annotation absent | No-op (step 3.2.2). No patch, no log. |
| Annotation is the only annotation key | Merge patch with `null` deletes the key; APIServer leaves `annotations: {}` or removes the map. Functionally identical; `normalizeMap` already treats empty map as absent. Harmless. |
| Object has `deletionTimestamp` | Skip (step 3.2.1). Avoids patching a deleting object. |
| Object is an orphan (current, no expected) | Skip (step 3.1). It will be deleted by `Delete` phase. |
| Object is a create (expected, no current) | Not in `currentObjects` → not iterated. Created by `Apply` via SSA without the annotation. |
| Unchanged object in diff variant | Covered: it is in `currentObjects` with a matching expected → cleaned. (This is the gap an `Apply`-only hook would miss.) |
| Third-party annotations present | Preserved: merge patch only nulls the stale key; other keys untouched. No ownership claimed over them. |
| `kubectl`/external actor re-adds the annotation | Next reconcile removes it again. This is intended enforcement of the v3 contract for managed children, not a bug. Documented behavior. |
| Webhook re-adds the annotation on patch | Same as above; would loop only if a webhook forcibly re-adds it on every patch. Rare; document. |
| Cluster-scoped child | `client.RawPatch` uses the object's namespace (empty) — works for cluster-scoped. |
| Namespaced child | Works (namespace set on object). |
| Diff variant dry-run | Cleanup is a real patch in the reconciler, independent of the diff variant's `DryRunApply`. No special handling. |
| Cleanup patch fails (transient API error) | `logger.Warnf`, continue. Next reconcile retries. Reconcile is NOT failed. |

## Error handling policy

**Best-effort, log-and-continue.** Cleanup is migration hygiene, not a
functional requirement. A cleanup failure must NOT fail the reconcile (which
would block SSA apply and status updates). Per-object failures: `logger.Warnf`
with object name + error, continue loop, no event. Successful removals:
`logger.Debugf` (quiet in steady state — after the first post-upgrade
reconcile, almost all objects have no annotation → no log). No `Recorder()`
events (cleanup is not user-facing critical). Errors wrapped with
`emperror.dev/errors` `errors.Wrapf` per repo convention.

## API / behavioral compatibility

- No changes to exported interfaces (`MultiPhaseStepReconcilerAction`,
  `SentinelReconcilerAction`, `MultiPhaseRead`, `MultiPhaseDiff`). New
  functions are additive.
- `CleanupLastAppliedAnnotation` and `CleanupReadLastAppliedAnnotations` are
  new exported helpers (callable by advanced users if desired).
- Behavior change: managed children lose the annotation on the first
  post-upgrade reconcile. This is the intended v3 contract (documented in
  `documentations/migration-v2-to-v3.md`).
- No new dependencies. `client.RawPatch`, `types.MergePatchType`,
  `encoding/json` are already transitively available (controller-runtime
  v0.19, apimachinery v0.32).

## Validation plan

### Unit tests

**NEW `pkg/helper/ssa/cleanup_test.go`** (fake client; pattern matches
`diff_test.go`):

- `TestCleanupLastAppliedAnnotation_RemovesWhenPresent` — ConfigMap with the
  annotation + a kept annotation; after call, GET shows stale key gone, kept
  annotation preserved; `removed == true`.
- `TestCleanupLastAppliedAnnotation_NoopWhenAbsent` — ConfigMap without the
  annotation; no patch issued (assert `removed == false`, nil err, and live
  object unchanged).
- `TestCleanupLastAppliedAnnotation_NoopWhenDeletionTimestamp` — object with
  `DeletionTimestamp` set and the annotation present; `removed == false`, nil
  err, annotation still present (no patch).
- `TestCleanupLastAppliedAnnotation_OnlyAnnotationKey` — object whose only
  annotation is the stale key; after call, stale key gone; `removed == true`.
- `TestCleanupLastAppliedAnnotation_PreservesThirdPartyAnnotations` — object
  with stale + `foo=bar` + `app.kubernetes.io/managed-by=other`; after call,
  only stale gone, others intact.

Note: fake client supports `client.RawPatch` with `MergePatchType` (unlike
`client.Apply`/SSA, which the existing tests skip). Verify; if fake client
does not honor null-deletion in merge patch, fall back to envtest for these
cases and keep only the no-op/skip cases as unit tests.

**NEW `pkg/controller/multiphase/cleanup_test.go`** (fake client + mock
`MultiPhaseRead`):

- `TestCleanupReadLastAppliedAnnotations_SkipsOrphans` — current objects
  `{a, b, orphan}`, expected `{a, b}`; only `a`/`b` cleaned, orphan untouched.
- `TestCleanupReadLastAppliedAnnotations_ContinuesOnError` — one object whose
  patch fails (use a client that errors on Patch, or a live object with no
  GVK to force an error) → loop continues, other objects still cleaned, no
  panic, returns count of successful.
- `TestCleanupReadLastAppliedAnnotations_NoopWhenNoAnnotation` — no objects
  have the annotation → `cleaned == 0`, no patches.

**MODIFY `pkg/controller/multiphase/multiphasestep_reconciler_unit_test.go`**
and **`pkg/controller/sentinel/sentinel_reconciler_unit_test.go`** — add a
test asserting the reconciler calls cleanup after Read (use the existing
`mockStepReconcilerAction` / sentinel mock with a `read` whose current objects
carry the annotation; assert the annotation is removed from the live fake
client after `Reconcile`). If the fake client cannot apply SSA (existing tests
skip `Apply`), scope this to just the cleanup phase by asserting the
annotation is gone after `Reconcile` returns (cleanup runs before `Apply`).

### Envtest (integration) tests

**MODIFY `pkg/controller/multiphase/controller_test.go`** — add a new test
step `doCleanupStep` (or extend `doCreateStep`'s `Check`) that:
1. Pre-seeds the managed ConfigMap with the legacy annotation (simulate a v2
   leftover) via `c.Patch`/`Update`.
2. Triggers a reconcile (e.g., update the parent spec to bump generation).
3. Asserts the ConfigMap no longer has the annotation, while other fields
   (data, labels, ownerReferences) are intact.

This uses the existing envtest suite (`suite_test.go`) and the real SSA path,
so it validates the full auto-mode behavior against a real apiserver.

### Commands

```bash
# Full test suite (per AGENTS.md)
dagger call --src . test --withGotestsum 2>&1 | grep -E "(FAIL|build failed|\.go:[0-9]+:|panic|--- FAIL)" | head -200

# Targeted
dagger call --src . test --withGotestsum --run "TestCleanup"
dagger call --src . test --withGotestsum --run "TestDefaultMultiPhaseStepReconciler"
dagger call --src . test --withGotestsum --run "TestDefaultSentinelReconciler"
dagger call --src . test --withGotestsum --run "TestControllerMultiphaseSuite"
```

### Lint / formatting

No `.golangci.yml` present in the repo. Follow existing conventions: `gofmt`,
`goimports` (group std / external / local `github.com/disaster37/operator-sdk-extra/v3`),
`emperror.dev/errors` wrapping, `//nolint` directives only where the existing
code uses them (e.g., `forcetypeassert` in `helper.go`). Run `go vet ./...`
and `go build ./...` before finalizing.

## Risks

1. **Fake client merge-patch null-deletion support** — controller-runtime's
   fake client may not honor `null` in a strategic/JSON merge patch for
   annotation deletion. Mitigation: verify; if unsupported, move the
   deletion-asserting unit tests to envtest and keep only no-op/skip cases as
   pure unit tests.
2. **Transient managedFields entry** from the merge patch field manager.
   Accepted; harmless. Documented.
3. **Re-add fighting** with external actors. Intended enforcement for managed
   children. Documented.
4. **`EnsureNetworkPolicyForWebhook` still sets the annotation** — out of
   scope here, but its NetworkPolicy is not a multiphase/sentinel managed
   child, so no fight loop with this cleanup. Follow-up: migrate it to SSA.

## Open questions / follow-ups

1. **`EnsureNetworkPolicyForWebhook` migration to SSA** — separate effort.
   This plan does not touch it. Should be tracked as a follow-up issue to
   fully remove `k8s-objectmatcher` usage from the controller package.
2. **Whether to also clean the annotation on the parent CR itself** — the
   parent CR is not a managed child of multiphase/sentinel (it is the
   reconciled object). v2 may have set the annotation on it via other means.
   Out of scope unless the parent is also a managed child of another
   controller. Not addressed here.
