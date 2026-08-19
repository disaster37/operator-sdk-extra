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
  The client-side 3-way diff is gone: diff is computed **only when asked** (opt-in
  `dryRun=true`) via an SSA dry-run apply. The old `Create()` / `Update()` step methods are
  replaced by a single `Apply()` method.
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
- Diff is only computed when you set `dryRun=true` (opt-in), via an SSA dry-run apply.

Step methods in your custom step builders:

```diff
-func (s *myStep) Create() error { ... }
-func (s *myStep) Update() error { ... }
+func (s *myStep) Apply() error { ... }
```

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
