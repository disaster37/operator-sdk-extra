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
    SecretName   string   // Name of the resulting Secret
    SelfSigned   bool     // Enable self-managed CA backend
    CertManager  bool     // Enable cert-manager backend
    IssuerRef    string   // Existing Issuer/ClusterIssuer name (cert-manager only)
    CommonName   string   // Certificate CN
    DNSNames     []string // SAN DNS names
    Organization string   // Certificate O field
    ValidityDays int      // Validity in days (default 365)
}
```

### Self-managed backend

The self-managed backend generates a CA + leaf certificate pair using Go's
`crypto/x509` with ECDSA P-256 keys. It returns two Secrets:

1. `<secretName>-ca` — contains `ca.crt` and `ca.key` (CA private key).
2. `<secretName>` — contains `tls.crt`, `tls.key`, and `ca.crt`
   (Type: `kubernetes.io/tls`).

The static `RequiresRotationSaga() == true` signals that the operator should
run the multi-cycle CA rotation workflow.

### BYO backend

The BYO backend validates that `TLSSpec.SecretName` is non-empty and returns
no child objects. It is the default when neither `SelfSigned` nor `CertManager`
is set. The user manages the secret lifecycle out of band.

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

## Testing

Unit tests cover all packages with the fake client and testify assertions.
Integration tests (envtest) are colocated in each package's `*_test.go` files.

Run tests for the new packages:

```bash
go test ./pkg/apis/workflow/... ./pkg/controller/workflow/... ./pkg/controller/certificate/...
```
