# TLS Certificate Management & Workflow Orchestration

This document describes the reusable certificate management and multi-cycle
workflow abstractions added in `operator-sdk-extra/v3`. They were extracted
from patterns pioneered by the elasticsearch-operator and generalized for any
operator built on this library.

---

## Motivation

Operators that manage TLS certificates face a common set of problems:

1. **Certificate provisioning** — generating CA + leaf certs (self-managed),
   delegating to cert-manager, or referencing user-provided secrets.
2. **Rolling restart on cert change** — decoupling secret rotation from pod
   rollout without bespoke StatefulSet polling.
3. **Multi-cycle sagas** — orchestrating phased workflows that span many
   reconcile cycles (e.g., CA rotation: append new CA → wait pods trust it →
   issue new leaf → wait rollout → strip old CA).

The library provides three abstractions to solve these:

- **`TLSBackend`** — a pluggable certificate provider interface.
- **`WorkflowStepReconcilerAction`** — a multi-cycle saga abstraction with
  typed phase cursors and convergence gating.
- **Rollout helper** — computes a stable hash of a certificate Secret for
  pod-template annotations.

---

## Three communication channels

The abstractions use three distinct channels, each serving one purpose:

| Channel | Location | Purpose | Persistence |
|---|---|---|---|
| `data map[string]any` | Per-reconcile blackboard (unchanged from v1) | Ephemeral computed values per cycle | Volatile |
| `WorkflowStatus` sub-struct | Object status (`.status.workflow`) | Saga phase cursor | Durable (etcd) |
| Conditions (`status.conditions`) | Standard Kubernetes convention | User-facing readiness/degraded signals | Durable (etcd) |

> **Important**: Conditions must NOT be used as a workflow cursor. The
> `WorkflowStatus` sub-struct exists specifically to keep conditions clean
> for user-facing status reporting.

---

## TLSBackend (certificate provider)

### Interface

```go
type TLSBackend[T object.MultiPhaseObject] interface {
    DesiredObjects(ctx context.Context, o T, spec TLSSpec) ([]client.Object, error)
    CertificateSecretName(o T, spec TLSSpec) string
    RequiresRotationSaga() bool
}
```

### Backend comparison

| Backend | Package | Emits | `RequiresRotationSaga` | Renewal owner |
|---|---|---|---|---|
| **Self-managed** | `selfmanaged` | CA + leaf `Secret`s (EC P-256) | `true` | Your operator |
| **BYO** (bring-your-own) | `byo` | Nothing (validates Secret exists) | `false` | User |
| **cert-manager** | `certmanager` | `Issuer` + `Certificate` CRs | `false` | cert-manager |

### TLSSpec

```go
type TLSSpec struct {
    SecretName       string   // Name of the resulting Secret
    IssuerRef        string   // Existing Issuer/ClusterIssuer name (cert-manager only)
    CommonName       string   // Certificate CN
    DNSNames         []string // SAN DNS names
    Organization     string   // Certificate O field
    LeafValidityDays int      // Leaf validity in days (default 365)
    CAValidityDays   int      // CA validity in days (default 2× leaf)
    Curve            string   // ECDSA curve (P-256/P-384/P-521)
    IPAddresses      []string // SAN IP addresses
    RenewalDays      int      // Renewal window before NotAfter (default 30)
    GenerateCRL      bool     // Add ca.crl to the CA Secret (selfmanaged)
}
```

| Field | Type | Default | Applies to | Description |
|---|---|---|---|---|
| `LeafValidityDays` | int | 365 | selfmanaged, certmanager | Leaf validity; `GetValidLeafDays(spec)`. |
| `CAValidityDays` | int | 2× leaf | selfmanaged, certmanager | CA validity; `GetValidCADays(spec)`. |
| `Curve` | string | `P-256` | selfmanaged | ECDSA curve: `P-256`, `P-384`, `P-521`. Unknown → error. |
| `IPAddresses` | []string | none | selfmanaged, certmanager | SAN IPs (parsed via `net.ParseIP`). |
| `RenewalDays` | int | 30 | selfmanaged (renewal window), certmanager (renewBefore) | Renewal window before `NotAfter`. |
| `GenerateCRL` | bool | false | selfmanaged | Adds `ca.crl` (DER) to the CA Secret. |

Backend selection is an **operator decision** (which backend type to construct), not
spec data: the old `SelfSigned`/`CertManager` booleans have been removed. The
computed `TLSSpec` is supplied to the library via a `TLSSpecProvider`:

```go
type TLSSpecProvider[T object.MultiPhaseObject] interface {
    TLSSpec(o T) TLSSpec
}
// certificate.TLSSpecProviderFunc[T] adapts a bare func(o T) TLSSpec.
```

### Certificate content customization

Beyond the legacy `CommonName`/`Organization` fields, `TLSSpec` is now a rich,
declarative, json-tagged certificate-content contract. All new fields are
optional and backward compatible (zero value reproduces the previous behavior:
P-256 ECDSA, `Organization` singular, `ServerAuth`+`ClientAuth`, `<cn>-ca` CA
naming, 365/730/30-day validities).

| Field | Type | Default | Description |
|---|---|---|---|
| `subject` | `CertificateSubject` | — | Extended leaf RDN block: `organizations`, `organizationalUnits`, `countries`, `localities`, `provinces`, `streetAddresses`, `postalCodes`, `serialNumber`. |
| `keyAlgorithm` | string | `ECDSA` | `ECDSA` or `RSA`. |
| `keySize` | int | 256 (ECDSA) / 2048 (RSA) | Key size in bits; ECDSA accepts 256/384/521. |
| `usages` | []string | `[serverAuth, clientAuth]` | Leaf `ExtKeyUsage` strings (`Usage*` constants). |
| `keyUsages` | []string | `[digitalSignature, keyEncipherment]` | Leaf `KeyUsage` strings (`KeyUsage*` constants). |
| `caCommonName` | string | `<commonName>-ca` | CA certificate CN override. |
| `caSubject` | `*CertificateSubject` | leaf subject | CA RDN override. |

Defaulting and precedence are centralized in `TLSSpec` resolver methods —
`Organizations()`, `LeafSubject()`, `ResolvedCASubject()`,
`EffectiveKeyAlgorithm()`, `EffectiveKeySize()`, `EffectiveUsages()`,
`EffectiveKeyUsages()`, `ValidateContent()` — so backends never re-read raw
fields. `ValidateContent()` fails fast on invalid algorithm/size/usage/empty-DNS
values and is called by every backend and by the rotation saga before
generation. Exported sentinels (`ErrUnsupportedKeyAlgorithm`, `ErrInvalidKeySize`,
`ErrUnknownUsage`) allow programmatic checks.

For content that cannot be declared statically, operators supply an optional
`CertificateCustomizer` compute hook (applied once per reconcile by the rotation
saga):

```go
type CertificateCustomizer[T object.MultiPhaseObject] interface {
    CustomizeCertificate(o T, base TLSSpec) (TLSSpec, error)
}
```

`certificate.DefaultCertificateCustomizer[T]()` fills `CommonName` with the
object name when empty (opt-in; no implicit behavior change). Minimal glue:

```go
step := rotation.NewTLSStep[*MyCRD](
    r.Client(), "tls", "TLSCertificatesReady", r.Recorder, "my-operator",
    selfmanaged.NewSelfManagedBackend[*MyCRD](),
    certificate.TLSSpecProviderFunc[*MyCRD](func(o *MyCRD) certificate.TLSSpec { return o.Spec.TLS }),
    rotation.WithCertificateCustomizer(
        certificate.CertificateCustomizerFunc[*MyCRD](func(o *MyCRD, base certificate.TLSSpec) (certificate.TLSSpec, error) {
            if len(base.DNSNames) == 0 {
                base.DNSNames = []string{o.Name + "." + o.Namespace + ".svc"}
            }
            return base, nil // the library generates everything
        }),
    ),
)
```

The selfmanaged backend now signs with `crypto.Signer` (ECDSA and RSA), and the
cert-manager backend maps `subject`/`usages`/`privateKey` to its CR spec.
`selfmanaged.CAContentChanged` and the extended `LeafNeedsChange` (new reasons
`SubjectChanged`, `KeyChanged`, `UsagesChanged`) detect CA/leaf content drift so
a subject/key/usage change triggers regeneration (and rollout) even before
expiry.

> **Behavior note (v3)**: the cert-manager CA `Certificate` no longer hardcodes
> `subject.organizations: ["operator-sdk-extra"]`, and its `commonName` changed
> from `<secretName>-ca` to `<commonName>-ca` (unified with the selfmanaged
> backend); the CA subject now follows `TLSSpec` (default empty unless the
> operator sets an organization). The cert-manager leaf `Certificate` now also
> sets `spec.privateKey` (`ECDSA`/256 by default) and `spec.usages` (`server
> auth`, `client auth`) explicitly, where previously both were omitted and
> relied on cert-manager's defaults (may cause a one-time re-issuance/key-type
> change).

### Self-managed backend

The self-managed backend generates a CA + leaf certificate pair using Go's
`crypto/x509` with ECDSA keys. The curve is selectable via `TLSSpec.Curve`
(`P-256` by default, or `P-384` / `P-521`); unknown values are rejected. IP
SANs are added via `TLSSpec.IPAddresses` (each entry parsed with
`net.ParseIP`). An optional CRL is emitted when `TLSSpec.GenerateCRL` is set:
the CA Secret then also carries a `ca.crl` key (raw DER, parsed with
`x509.ParseRevocationList`). The CRL is reissued fresh on each renewal with an
empty revoked list — no incremental revocation maintenance is performed
(documented limitation).

It returns two Secrets:

1. `<secretName>-ca` — contains `ca.crt`, `ca.key` (CA private key), and
   (when `GenerateCRL`) `ca.crl`.
2. `<secretName>` — contains `tls.crt`, `tls.key`, and `ca.crt`
   (Type: `kubernetes.io/tls`).

The CA validity defaults to 2× the leaf validity (`certificate.GetValidCADays`,
leaf via `certificate.GetValidLeafDays`). Regeneration is split by layer:

- **CA renewal** is gated by `selfmanaged.CANeedsRenewal(caSecret, spec, now)`,
  using the renewal window `certificate.GetValidRenewalDays(spec)`.
- **Leaf drift/expiry** is reported by the optional `LeafManager` capability
  (`DesiredLeafWithCA` re-signs against an existing CA; `LeafNeedsChange`
  reports a `LeafChange`). The selfmanaged (single-leaf) and selfmanaged/pernode
  backends both implement it.

The static `RequiresRotationSaga() == true` signals that the operator should
run the multi-cycle CA rotation workflow.

### BYO backend

The BYO backend validates that `TLSSpec.SecretName` is non-empty and returns
no child objects. The user manages the secret lifecycle out of band.

### Per-node backend (`selfmanaged/pernode`)

The per-node backend keeps one certificate per node in a single Opaque leaf
Secret (`<secretName>`), alongside a CA Secret (`<secretName>-ca`). The leaf
Secret carries `ca.crt` plus one `<node>.crt` / `<node>.key` pair per expected
node (multi-cert transport TLS, e.g. Elasticsearch transport). Operators
supply a `NodeSpecProvider`:

```go
type NodeSpecProvider[T object.MultiPhaseObject] interface {
    ExpectedNodeNames(o T) ([]string, error)
    NodeCertSpec(o T, nodeName string) (cn string, dnsNames []string, ips []string, err error)
}
```

Per-node steps implement `NodeSetTLSBackend` and therefore never publish
`tlsSecret`/`leafCert` into the data map (their private keys use dynamic
`<node>.key` names); `caSecret`/`caCert` are still published.

### cert-manager backend (optional subpackage)

The cert-manager backend emits `Issuer` and `Certificate` CRs from the
`cert-manager.io/v1` API group. It operates in two modes:

- **Dedicated-CA**: Creates a self-signed `Issuer`, a CA `Certificate`,
  a CA `Issuer`, and leaf `Certificate`s. All cert-manager CRs are built as
  `unstructured.Unstructured` — no cert-manager Go API dependency is required
  at build time.
- **Existing-CA**: Creates only leaf `Certificate`s referencing an existing
  `Issuer` or `ClusterIssuer` specified by `TLSSpec.IssuerRef`.

Requires `cert-manager` to be installed in the cluster. The cert-manager
operator handles renewal; `RequiresRotationSaga() == false`.

> **Behavior note (v3)**: the leaf `Certificate` now sets `spec.duration` from
> `LeafValidityDays` in **both** dedicated-CA and existing-CA modes (existing-CA
> previously omitted it, so cert-manager used its 90-day default), and the CA
> `Certificate` sets `spec.duration` from `GetValidCADays(spec)` (previously
> omitted, so cert-manager used its default).

### Import isolation

The `certmanager` subpackage is optional. Core library packages do not import
it. Only operators that use the cert-manager backend need to import:

```go
import "github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/certmanager"
```

---

## WorkflowStep (saga orchestration)

### Interface

```go
type WorkflowStepReconcilerAction[k8sObject, k8sStepObject] interface {
    multiphase.MultiPhaseStepReconcilerAction[k8sObject, k8sStepObject]

    CurrentPhase(o k8sObject) workflow.WorkflowPhase
    AdvancePhase(ctx, o, to WorkflowPhase, logger) (reconcile.Result, error)
    IsPhase(o k8sObject, phase WorkflowPhase) bool
    IsPhaseEmpty(o k8sObject) bool
}
```

### Phase cursor lifecycle

The typed phase cursor (`.status.workflow.currentPhase`) drives the saga:

1. **Phase empty** → start the saga with the first phase.
2. **Current phase active** → perform phase-appropriate work in `Read()`,
   emit desired objects for SSA, then `AdvancePhase()` to the next phase.
3. **Wait for convergence** → call `WaitForOwnedObjects()` after mutating
   owned objects (e.g., updating a StatefulSet after a cert secret change).
   The function returns `RequeueAfter` when objects have not converged.
4. **Terminal phase** → the saga is complete; conditions reflect readiness.

### WorkflowStatus

```go
type WorkflowStatus struct {
    CurrentPhase    WorkflowPhase      `json:"currentPhase,omitempty"`
    PhaseConditions []metav1.Condition `json:"phaseConditions,omitempty"`
}
```

Embed `WorkflowStatus` in your CRD's status struct and implement the
`WorkflowStatusGetter` interface to enable phase tracking:

```go
func (s *MyStatus) GetWorkflowStatus() *workflow.WorkflowStatus {
    return &s.WorkflowStatus
}
```

### WaitForOwnedObjects

```go
func WaitForOwnedObjects[T client.Object](
    ctx, c, owner, list, predicate, listOpts...,
) (done bool, res reconcile.Result, err error)
```

This primitive generalises the hand-coded StatefulSet `CurrentReplicas` /
sequence-number polling. It lists owned objects matching the selector, applies
a predicate to each, and short-circuits with `RequeueAfter: 10s` when not all
objects have converged.

**Usage pattern**:

```go
// After updating a StatefulSet, wait for rollout:
done, res, err := workflow.WaitForOwnedObjects[*appv1.StatefulSet](
    ctx, r.Client(), o, &appv1.StatefulSetList{},
    func(sts *appv1.StatefulSet) bool {
        return sts.Status.CurrentReplicas == *sts.Spec.Replicas &&
            sts.Status.ObservedGeneration >= sts.Generation
    },
    client.InNamespace(o.GetNamespace()),
)
if err != nil { return res, err }
if !done { return res, nil } // Requeue after 10s
```

---

## Rotation saga (rotation package)

The `rotation` package provides a reusable multi-cycle TLS rotation saga step
built on `workflow.WorkflowStepReconcilerActionWithDiff`. It drives the CA
rotation for saga-capable backends (`RequiresRotationSaga() == true`; only
selfmanaged today).

### Phase diagram

```
""       -> Rotate   : emit new CA + leaf with ca.crt = newCA||oldCA bundle.
Rotate   -> Converge : hold desired state stable, gate on an operator-injected
                       convergence check, then advance.
Converge -> ""       : emit leaf with ca.crt = newCA only (strip old CA).
```

Phase advances happen in `OnSuccess` (after a successful Apply), except
`Rotate → Converge` which advances in `OnDiff` — safe because Apply is a no-op
during Rotate (expected == current). Advancing before Apply would persist a
phase whose desired state was never applied.

### Data-map contract

The `Read` method populates the per-reconcile blackboard (`data map[string]any`)
with the following keys:

| Key | Type | Meaning |
|---|---|---|
| `rotationStartPhase` | `workflow.WorkflowPhase` | Phase at cycle start (before any advance) |
| `rotationRenewed` | `bool` | `true` when phase `""` triggered a CA saga this cycle |
| `tlsSecret` | `*corev1.Secret` | Current leaf secret (if present, non-per-node) — sanitized copy |
| `caSecret` | `*corev1.Secret` | Current CA secret (if present) — sanitized copy |
| `leafCert` | `*x509.Certificate` | First cert parsed from the leaf's `tls.crt` (if parseable, non-per-node) |
| `caCert` | `*x509.Certificate` | First cert parsed from the CA secret's `ca.crt` (if parseable) |
| `tls.<phaseName>` | `*certificate.LayerSignals` | Change-delta signal for the STS step (only on a change at phase `""`) |

`tlsSecret` and `caSecret` are **sanitized deep copies** of the current
Secrets: the private-key entries (`tls.key` from the leaf copy, `ca.key` from
the CA copy) are removed from `Data`, while certificate material (`tls.crt`,
`ca.crt`, `ca.crl`) and all metadata (name, namespace, labels, annotations)
are retained. This lets injected convergence checks inspect certificates and
identities without exposing private-key material to the data map, which is
handed to `OnDiff`/`OnSuccess`/`OnError` and could be debug-logged. The
original Secrets are not mutated.

Per-node (`NodeSetTLSBackend`) steps omit `tlsSecret`/`leafCert` entirely: their
leaf private keys use dynamic `<node>.key` names that `sanitizedSecret` would
not strip.

Each saga TLS step publishes exactly one `*certificate.LayerSignals` under
`data["tls.<phaseName>"]` **only when a change occurs at phase `""`** (CA saga,
leaf-only, or force). Steady/Rotate/Converge publish no signal (→ no rollout).
The flat `rotationStartPhase`/`rotationRenewed`/`tlsSecret`/`caSecret` keys are
transient per-step; cross-step consumers (the STS step) must use only the
namespaced `tls.<phaseName>` signals.

### Regeneration model

At phase `""` the step evaluates three conditions:

1. **Force annotations** (honored at phase `""` only; see below).
2. **CA need**: `!caExists || forceAll || CANeedsRenewal(currentCA, spec, now)`.
3. **Leaf change**: `LeafManager.LeafNeedsChange(...)` (drift/expiry/missing).

- `caNeed` → **full CA saga** (bundle + Rotate/Converge phases) and
  `rotationRenewed=true`.
- otherwise a non-zero `LeafChange` → **leaf-only** regeneration via
  `LeafManager.DesiredLeafWithCA` (no phase writes, no `rotationRenewed`).
- otherwise → steady state (with stale-bundle recovery to Rotate).

### Force-regenerate annotations

Two one-shot annotations trigger regeneration at phase `""`:

| Annotation | Constant | Effect |
|---|---|---|
| `operator-sdk-extra.webcenter.fr/force-regenerate-tls` | `AnnotationForceRegenerateAll` | Full CA+leaf rotation. |
| `operator-sdk-extra.webcenter.fr/force-regenerate-certificates` | `AnnotationForceRegenerateLeaf` | Leaf-only regeneration (falls back to full saga if the CA is missing). |

Read as `== "true"`. force-all wins when both are set. The honored annotation
is removed in `OnSuccess` (saga backends) after a successful Apply; on Apply
failure it is left in place and retried next cycle. Non-saga backends record a
`TLSForceUnsupported` warning Event and remove the annotation without
regenerating. The annotation names are overridable via
`WithForceRegenerateAllAnnotation` / `WithForceRegenerateLeafAnnotation`.

### Constructor

```go
func NewTLSStep[T object.MultiPhaseObject](
    c client.Client,
    phaseName shared.PhaseName,
    conditionName shared.ConditionName,
    recorder record.EventRecorder,
    fieldManager string,
    backend certificate.TLSBackend[T],
    provider certificate.TLSSpecProvider[T],
    opts ...Option[T],
) workflow.WorkflowStepReconcilerActionWithDiff[T, client.Object]
```

Options: `WithConvergenceCheck`, `WithLabelsDecorator`,
`WithAnnotationsDecorator`, `WithForceRegenerateAllAnnotation`,
`WithForceRegenerateLeafAnnotation`, `WithCertificateCustomizer`.

```go
step := rotation.NewTLSStep[*MyCRD](
    r.Client(), "tls", "TLSCertificatesReady", r.Recorder, "my-operator",
    selfmanaged.NewSelfManagedBackend[*MyCRD](),
    certificate.TLSSpecProviderFunc[*MyCRD](func(o *MyCRD) certificate.TLSSpec { return o.Spec.TLS }),
    rotation.WithConvergenceCheck(func(ctx context.Context, o *MyCRD, data map[string]any) (bool, error) {
        return workflow.WaitForOwnedObjects[*appv1.StatefulSet](
            ctx, r.Client(), o, &appv1.StatefulSetList{},
            func(sts *appv1.StatefulSet) bool { return sts.Status.CurrentReplicas == *sts.Spec.Replicas },
            client.InNamespace(o.GetNamespace()), client.MatchingLabels{"app":"my-app"},
        )
    }),
    rotation.WithLabelsDecorator(func(o *MyCRD, obj client.Object) { /* set labels */ }),
)
```

Backends with `RequiresRotationSaga() == false` degrade to a single-cycle emit
of `DesiredObjects` with no phase writes (identical to the previous
operator-managed single-cycle flow).

> **Migration note**: operators' CRD status must embed
> `workflow.WorkflowStatus` (now DeepCopy-safe via the hand-written
> `zz_generated.deepcopy.go`) and implement
> `GetWorkflowStatus() *workflow.WorkflowStatus`.

## Rollout helper

The rollout helper computes a stable SHA-256 hash of a certificate Secret's
data and exposes it via the `operator-sdk-extra.webcenter.fr/certificate-hash`
annotation. Mount this annotation into your pod template to get automatic
rolling restarts when the certificate changes.

```go
hash, err := certificate.SecretHash(secret)
annotations, err := certificate.SecretHashAnnotation(secret)
// Merge into pod template annotations
```

For saga TLS steps, prefer the signal-driven helpers: aggregate
`ShouldRollout(policy, sig)` over every `data["tls.<phaseName>"]` layer, then
build the annotation with `RolloutAnnotation(shouldRollout, secret, currentHash)`.

```go
type LayerSignals struct {
    CARotated       bool
    LeafRegenerated bool
    LeafChange      *LeafChange // SAN/IP/node deltas, expiry, reason
    Forced          bool
}

type RolloutPolicy string // RolloutAlways | RolloutOnCAChange | RolloutOnAdditive | RolloutNever
```

`RolloutOnAdditive` (recommended default) rolls out on CA rotation, or on
leaf regeneration whose `LeafChange` is additive-only (missing/expiring/
CN/Org/Forced reasons, or added SANs/IPs) but **not** on pure removals or
node-set changes. `Forced` overrides everything, including `RolloutNever`.

This decouples "certificate changed" from "must restart" without bespoke
polling of `StatefulSet.Status.CurrentReplicas`.

---

## Reference migration (elasticsearch-operator)

The elasticsearch-operator's `tlsReconciler` uses a condition-based FSM
implemented via `status.conditions`. The mapping to the new abstractions:

| Old condition-based FSM | New `WorkflowStep` + `TLSBackend` |
|---|---|
| `TlsGeneratePki` | Phase: `GenerateCA` → `selfmanaged.DesiredObjects()` |
| `TlsPropagatePki` | Phase: `PropagateCA` → `WaitForOwnedObjects(predicate)` |
| `TlsGenerateCertificates` | Phase: `GenerateLeaf` → `selfmanaged.DesiredObjects()` |
| `TlsPropagateCertificates` | Phase: `PropagateLeaf` → `WaitForOwnedObjects(predicate)` |
| `TlsReady` / `TlsBlackout` | Removed; conditions now reflect user-facing readiness only |

The `TlsBlackout` condition becomes a phase check (skip cert operations during
blackout window) rather than a condition.

---

## Migration guide (v1 → v2 workflow style)

| Step | v1 approach | v2 approach |
|---|---|---|
| Phase cursor | Stored in `status.conditions` | `WorkflowStatus.CurrentPhase` |
| Convergence gate | Hand-coded StatefulSet polling | `WaitForOwnedObjects()` |
| Diff step | Default client-side `Diff` | SSA (no reliance on default Diff) |
| Cert generation | Embedded in controller | `TLSBackend.DesiredObjects()` |
| Rolling restart | Custom annotation logic | `SecretHashAnnotation()` |

---

## Client-side TLS helper (pkg/tlsconfig)

For operators that also need a client-side TLS configuration (any remote API),
`pkg/tlsconfig` provides a generic, app-agnostic builder with no
controller-runtime or client-library dependencies. `CertificateOptions` carries
CA sources (inline PEM or file paths), an optional client certificate + key,
`InsecureSkipVerify`, `ServerName`, and TLS version bounds, and exposes
`Validate`, `Resolve`, `BuildTLSConfig`, and `BuildHTTPTransport`
(`WithBaseTLSConfig`/`WithBaseTransport` options layer onto an existing base).
The zero value yields `(nil, nil)` — no customization.

```go
opts := &tlsconfig.CertificateOptions{InsecureSkipVerify: true}
tlsCfg, err := opts.BuildTLSConfig()      // *tls.Config
transport, err := opts.BuildHTTPTransport() // *http.Transport
```

`pkg/tlsconfig/k8s` (imports controller-runtime `client.Client`) resolves the
same material from Kubernetes Secrets:

```go
r := tlsk8s.New(c) // client.Client
transport, err := r.BuildHTTPTransport(ctx, namespace, tlsk8s.Options{
    CASecretName:       "my-tls-ca",
    ClientCertSecretName: "my-tls",
})
```

The produced `*tls.Config`/`*http.Transport` (or CA bytes/`InsecureSkipVerify`)
is wired into whichever HTTP client library the operator uses — no specific
client library is imported by this package.

> **Security notes**: `pkg/tlsconfig/k8s` `Options.Validate` fails closed and
> requires at least one of `CASecretName` or `ClientCertSecretName`; a
> skip-verify-only config (neither secret set) is rejected. When
> `InsecureSkipVerify` is `true`, Go's TLS stack skips verification entirely,
> so any CA certificates are ignored for verification (`RootCAs` is unused).
> Path-based sources (`CAPath`/`ClientCertPath`/`ClientKeyPath`) read arbitrary
> files and must only ever be populated from operator-controlled configuration,
> never untrusted input.

## Testing

Unit tests cover all packages with the fake client and testify assertions.
Integration tests (envtest) are colocated in each package's `*_test.go` files.

Run tests for the new packages:

```bash
go test ./pkg/apis/workflow/... ./pkg/controller/workflow/... ./pkg/controller/certificate/...
```

The `./pkg/controller/certificate/rotation/` package is included above. Its
tests call `Read`/`OnDiff`/`OnSuccess` directly rather than driving a full
`Reconcile`, because the controller-runtime fake client does not support the
SSA dry-run `Patch` that the inherited `Diff` step requires (the inherited
`Diff`/`Apply` are covered by the `multiphase` package tests).
