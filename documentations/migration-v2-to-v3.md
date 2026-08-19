# Migration guide: v2 → v3

> Audience: human maintainers **and** LLM coding agents upgrading an operator built on
> `github.com/disaster37/operator-sdk-extra/v2` to the new `…/v3` release.
>
> This release is a **major, breaking** change. Read the [TL;DR](#tldr) first, then use the
> [mechanical rename table](#1-module-path-and-import-renames) and the per-pattern sections.

---

## TL;DR

- New **`/v3` module path** (Go semantic import versioning): every import and `go.mod`
  requirement must be updated.
- The **multiphase** and **sentinel** reconcilers now use **Kubernetes Server-Side Apply (SSA)**.
  The client-side 3-way diff is gone: the **diff** variants (`...WithDiff` constructors) compute
  the diff via an SSA dry-run apply, while the **simple** variants always-apply every expected
  object. The old `Create()` / `Update()` step methods are replaced by a single `Apply()` method.
- **TLS/certificate workflow** is now available directly in the library:
  `pkg/controller/certificate` (with `byo`, `selfmanaged` and `certmanager` backends),
  `pkg/apis/workflow` and the TLS workflow helpers.
- **Error handling**: no more truncated error strings — original errors are returned.
- `EnsureNetworkPolicyForWebhook` now takes a `context.Context` as first argument.
- Minimum Go version bumped to **1.26**.

---

## 1. Module path and import renames

### 1.1 `go.mod`

```diff
-require github.com/disaster37/operator-sdk-extra/v2 v2.x.x
+require github.com/disaster37/operator-sdk-extra/v3 v3.0.0
```

### 1.2 Import path table

Every `github.com/disaster37/operator-sdk-extra/v2/…` import becomes
`github.com/disaster37/operator-sdk-extra/v3/…`:

```diff
-	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/multiphase"
-	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/remote"
-	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/sentinel"
+	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/multiphase"
+	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/remote"
+	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/sentinel"
```

### 1.3 Module path of the samples

```diff
-module github.com/disaster37/operator-sdk-extra/v2/samples/memcached-operator
+module github.com/disaster37/operator-sdk-extra/v3/samples/memcached-operator
```

---

## 2. Multiphase and sentinel: Server-Side Apply

The reconciler now declares the *desired* object and `PATCH`es it with `client.Apply` +
a **field manager**:

- The API server owns conflict detection and field ownership.
- No more `kubectl.kubernetes.io/last-applied-configuration` annotation on managed K8s
  resources for these two patterns.
- SSA is idempotent: applying every expected object does not cause update loops.
- Diff is computed by the **diff** variant (`...WithDiff` constructors) via an SSA dry-run
  apply; the **simple** variant always-applies every expected object.

Step methods in your custom step builders:

```diff
-func (s *myStep) Create() error { ... }
-func (s *myStep) Update() error { ... }
+func (s *myStep) Apply() error { ... }
```

### 2.1 The `fieldManager` parameter

`NewMultiPhaseStepReconcilerAction` and `NewSentinelAction` now take a
`fieldManager string` parameter:

```go
multiphase.NewMultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap](
    c,
    ConfigmapPhase,
    ConfigmapCondition,
    recorder,
    "memcached-operator", // fieldManager
)
```

The old `dryRun bool` constructor parameter is gone. Instead, pick the variant you need:

- `NewMultiPhaseStepReconcilerAction` / `NewSentinelAction` — **simple** variant: always-apply
  classification, no `OnDiff` hook.
- `NewMultiPhaseStepReconcilerActionWithDiff` / `NewSentinelActionWithDiff` — **diff** variant:
  SSA dry-run classification plus an optional `OnDiff` hook (default no-op).

The reconcilers detect the `OnDiff` capability at runtime via an optional-interface type
assertion and only call it when the action implements the `...WithDiff` interface.

This string is passed as `client.FieldOwner(...)` on every SSA `Patch`. It identifies
**who owns the fields applied by your operator** and is recorded in each object's
`metadata.managedFields`. This is how the API server tracks per-field ownership and
detects conflicts with other actors (users via `kubectl`, HPA, other operators, ...).

What to put here:

- **One stable name for the whole operator, shared by all controllers.** All controllers
  run in the same binary and are a single manager from the API server's perspective.
  Using a different name per controller fragments `managedFields` and hides conflicts
  with truly external actors behind conflicts between your own controllers.
- Use the operator name (e.g. `"memcached-operator"`), ideally aligned with the
  controller-runtime manager name so all your SSA applies are attributed to a single
  manager.
- Keep it **constant**: changing it later orphans every field applied under the old
  name.

Note: this SDK always applies with `client.ForceOwnership`, so a wrong or duplicated name
never blocks an apply — but it still degrades conflict detection and pollutes
`managedFields`.

## 3. TLS / certificate workflow

TLS certificate management is now part of the library instead of being implemented in each
operator. See [tls-and-workflow.md](tls-and-workflow.md) for usage.

- `pkg/controller/certificate`: backend interface + `byo` (bring-your-own),
  `selfmanaged` (generated certs) and `certmanager` backends.
- `pkg/apis/workflow`: `WorkflowStatus` / `WorkflowStep` status helpers.

## 4. Other breaking changes

| Change | v2 | v3 |
|---|---|---|
| `EnsureNetworkPolicyForWebhook` | `(c, logger, namespace, labels, podSelecetors)` | `(ctx, c, logger, namespace, labels, podSelecetors)` |
| Error strings | truncated (max length) | original errors returned |
| Go version | 1.24 | 1.26 |
