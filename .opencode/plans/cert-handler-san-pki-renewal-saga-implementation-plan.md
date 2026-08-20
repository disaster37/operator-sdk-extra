# Cert handler: SAN, real PKI (curve + CRL), renewal window, rotation saga — Implementation Plan

Source spec: `.kilo/plans/1787215632602-cert-handler-san-pki-renewal-saga.md` (user-approved).
This plan is self-contained: a coder can implement it without reading any other document.

Module path: `github.com/disaster37/operator-sdk-extra/v3`. Go 1.26.

---

## 0. Key codebase facts (grounding)

- `certificate.TLSBackend[T]` already declares `RequiresRotationSaga() bool` (`pkg/controller/certificate/backend.go:63-77`). All three backends already implement it. **No interface change is needed.**
- `workflow.WorkflowStepReconcilerActionWithDiff[k8sObject, k8sStepObject]` exists (`pkg/controller/workflow/workflow_step_action.go:52-66`) with phase helpers `CurrentPhase/AdvancePhase/IsPhase/IsPhaseEmpty` provided by the embedded `workflowPhaseManager`.
- `workflow.NewWorkflowStepReconcilerActionWithDiff[T, client.Object](c, phaseName, conditionName, recorder, fieldManager)` returns the interface; concrete type is `*workflow.DefaultWorkflowStepReconcilerActionWithDiff[T, client.Object]` (embeds `*multiphase.DefaultMultiPhaseStepReconcilerActionWithDiff` → embeds `*multiphase.DefaultMultiPhaseStepReconcilerAction` which has `Read` (panics), `OnDiff` (no-op), `OnSuccess` (sets condition True), `Apply`, `Diff` (SSA dry-run), `Client()`, `Recorder()`, `Condition()`).
- The multiphase step reconciler order is: `Configure → Read → Diff → OnDiff → Apply → Delete → OnSuccess` (`pkg/controller/multiphase/multiphasestep_reconciler.go`). `OnDiff` runs **before** `Apply`; returning a non-zero `reconcile.Result` from `Read` or `OnDiff` short-circuits before `Apply`.
- `multiphase.NewMultiPhaseRead[client.Object]()` gives a `MultiPhaseRead[client.Object]` with `AddExpectedObject`/`AddCurrentObject` (nil-rejecting). The default `Diff` (SSA dry-run) classifies expected vs current by `GetName()`.
- `workflow.WaitForOwnedObjects` exists (`pkg/controller/workflow/wait.go`) with `DefaultRequeueAfter = 10 * time.Second`. The rotation step does **not** use it directly (convergence is operator-injected); we reuse the constant.
- `selfmanaged.CASecretSuffix = "-ca"`, `selfmanaged.CAKey = "ca.crt"`, `selfmanaged.CertKey = "tls.crt"`, `selfmanaged.KeyKey = "tls.key"`, `selfmanaged.CAKeyPrivate = "ca.key"`.
- **Fake client limitation**: `ssadiff.DryRunApply` issues a `Patch(... DryRunAll ...)` which the controller-runtime fake client does **not** support (existing `TestDefaultMultiPhaseStepReconcilerActionWithDiff_Diff` skips on this error). Therefore rotation tests **cannot** drive the full `Reconcile`/`Diff`/`Apply` path with the fake client. Tests call `Read`, `OnDiff`, `OnSuccess` directly (unit style). The inherited `Diff`/`Apply` are already covered by the `multiphase` package tests.
- `pkg/apis/workflow/` has NO `zz_generated.deepcopy.go` today. `doc.go` carries `// +k8s:deepcopy-gen=package,register`, but controller-gen does not emit for `WorkflowStatus` in this repo and CI does not run deepcopy-gen. We hand-write the file.
- Test conventions: `*_test.go` colocated, external test package (`foo_test`), testify `assert`/`require`, fake client via `sigs.k8s.io/controller-runtime/pkg/client/fake`. See `pkg/controller/certificate/selfmanaged/backend_test.go` and `pkg/controller/workflow/workflow_step_action_test.go`.

---

## 1. `pkg/controller/certificate/backend.go` (modify)

### 1.1 Add constants (after the imports, before `TLSSpec`)

```go
const (
	// CurveP256 is the default ECDSA curve (secp256r1).
	CurveP256 = "P-256"
	// CurveP384 selects ECDSA P-384 (secp384r1).
	CurveP384 = "P-384"
	// CurveP521 selects ECDSA P-521 (secp521r1).
	CurveP521 = "P-521"

	// DefaultRenewalDays is the renewal window before NotAfter when renewal
	// is triggered, used when TLSSpec.RenewalDays is not set.
	DefaultRenewalDays = 30
)
```

### 1.2 Extend `TLSSpec` (additive fields, keep existing fields & json tags)

```go
	// Curve selects the ECDSA curve for the selfmanaged backend.
	// One of P-256 (default), P-384, P-521. Unknown values are rejected by
	// the selfmanaged backend. Ignored by cert-manager (manages its own keys).
	Curve string `json:"curve,omitempty"`

	// IPAddresses lists the SAN IPs for the generated certificate.
	// Applied to the selfmanaged leaf and to the cert-manager Certificate
	// (ipAddresses). Each entry must parse via net.ParseIP.
	IPAddresses []string `json:"ipAddresses,omitempty"`

	// RenewalDays is the window before expiry during which renewal is
	// triggered. Defaults to 30 (see GetValidRenewalDays). For cert-manager,
	// a value > 0 maps to renewBefore.
	RenewalDays int `json:"renewalDays,omitempty"`

	// GenerateCRL, when true, adds a ca.crl key (DER-encoded revocation
	// list) to the CA Secret (selfmanaged only). No incremental revocation
	// maintenance: the CRL is reissued fresh on each renewal with an empty
	// revoked list.
	GenerateCRL bool `json:"generateCRL,omitempty"`
```

### 1.3 Add helper (after `GetValidTLSDays`)

```go
// GetValidRenewalDays returns the renewal window in days, defaulting to
// DefaultRenewalDays (30) when RenewalDays is not set or invalid (<= 0).
func GetValidRenewalDays(spec TLSSpec) int {
	if spec.RenewalDays <= 0 {
		return DefaultRenewalDays
	}
	return spec.RenewalDays
}
```

### 1.4 Update the package doc comment

Append to the existing doc comment a sentence: "The selfmanaged backend supports ECDSA curve selection (P-256/P-384/P-521), IP SANs, an optional CA CRL, and a renewal window; the reusable rotation saga lives in pkg/controller/certificate/rotation."

### 1.5 Tests — `pkg/controller/certificate/backend_test.go` (modify, append)

- `TestGetValidRenewalDaysDefault` — `TLSSpec{}` → 30.
- `TestGetValidRenewalDaysCustom` — `TLSSpec{RenewalDays: 45}` → 45.
- `TestGetValidRenewalDaysZero` — `RenewalDays: 0` → 30.
- `TestGetValidRenewalDaysNegative` — `RenewalDays: -5` → 30.
- `TestCurveConstants` — assert `CurveP256=="P-256"`, `CurveP384=="P-384"`, `CurveP521=="P-521"`, `DefaultRenewalDays==30`.

---

## 2. `pkg/controller/certificate/selfmanaged/backend.go` (modify)

### 2.1 New constant

```go
	// CRLKey is the key in the CA Secret data for the DER-encoded CRL.
	CRLKey = "ca.crl"
```

### 2.2 New imports

Add `"net"`, `"time"` (already present), and keep existing. (No new third-party imports.)

### 2.3 New private helpers

```go
// curveFor resolves the ECDSA curve from spec.Curve. Empty or P-256 → P-256.
func curveFor(spec certificate.TLSSpec) (elliptic.Curve, error) {
	switch spec.Curve {
	case "", certificate.CurveP256:
		return elliptic.P256(), nil
	case certificate.CurveP384:
		return elliptic.P384(), nil
	case certificate.CurveP521:
		return elliptic.P521(), nil
	default:
		return nil, fmt.Errorf("unsupported curve %q: want P-256, P-384 or P-521", spec.Curve)
	}
}

// parseIPs parses spec.IPAddresses into net.IP. Returns error on any invalid entry.
func parseIPs(spec certificate.TLSSpec) ([]net.IP, error) {
	if len(spec.IPAddresses) == 0 {
		return nil, nil
	}
	ips := make([]net.IP, 0, len(spec.IPAddresses))
	for _, s := range spec.IPAddresses {
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP SAN %q", s)
		}
		ips = append(ips, ip)
	}
	return ips, nil
}
```

### 2.4 Modify `DesiredObjects`

- Replace `ecdsa.GenerateKey(elliptic.P256(), rand.Reader)` for the CA key with:
  ```go
  curve, err := curveFor(spec)
  if err != nil { return nil, err }
  caKey, err := ecdsa.GenerateKey(curve, rand.Reader)
  ```
  Same for the leaf key (reuse the same `curve`).
- After `parseIPs`:
  ```go
  ips, err := parseIPs(spec)
  if err != nil { return nil, err }
  ```
  Set `leafTemplate.IPAddresses = ips` (alongside the existing `DNSNames: spec.DNSNames`).
- CRL: after `caDER` is created, when `spec.GenerateCRL`:
  ```go
  var crlDER []byte
  if spec.GenerateCRL {
      now := time.Now()
      crlTemplate := &x509.RevocationList{
          Issuer:       caTemplate.Subject,
          ThisUpdate:    now,
          NextUpdate:    now.Add(time.Duration(validity*2) * 24 * time.Hour), // CA validity
          Number:        big.NewInt(1),
          // RevokedCertificateEntries left nil (empty list).
      }
      crlDER, err = x509.CreateRevocationList(rand.Reader, crlTemplate, caTemplate, caKey)
      if err != nil { return nil, fmt.Errorf("failed to create CRL: %w", err) }
  }
  ```
  Then in the CA secret `Data`, when `crlDER != nil`, add `CRLKey: crlDER` (raw DER, **not** PEM-encoded — CRLs are consumed as DER by `x509.ParseRevocationList`). Keep `CAKey` and `CAKeyPrivate` as today.
- Leaf secret unchanged (still `tls.crt`, `tls.key`, `ca.crt` = new CA PEM).
- Return order unchanged: `[]client.Object{caSecret, leafSecret}`.

### 2.5 New exported `NeedRenewal`

```go
// NeedRenewal reports whether the certificate(s) in secret should be renewed
// at now, given spec's renewal window.
//   - nil secret → true (not yet provisioned).
//   - tls.crt absent or empty → true (not ready).
//   - tls.crt present but unparseable → error.
//   - ca.crt, if present, is also checked; absent ca.crt is skipped (no trigger),
//     malformed ca.crt → error.
// Renewal is triggered when now is within GetValidRenewalDays(spec) of any
// parsed cert's NotAfter (i.e. now > NotAfter - window), which also covers
// already-expired certs.
func NeedRenewal(secret *corev1.Secret, spec certificate.TLSSpec, now time.Time) (bool, error) {
	if secret == nil {
		return true, nil
	}
	window := time.Duration(certificate.GetValidRenewalDays(spec)) * 24 * time.Hour

	// tls.crt: absent/empty → true; malformed → error.
	if raw, ok := secret.Data[CertKey]; !ok || len(raw) == 0 {
		return true, nil
	} else {
		need, err := certsNeedRenewal(raw, now, window)
		if err != nil { return false, err }
		if need { return true, nil }
	}

	// ca.crt: optional. Present → check; absent → skip.
	if raw, ok := secret.Data[CAKey]; ok && len(raw) > 0 {
		need, err := certsNeedRenewal(raw, now, window)
		if err != nil { return false, err }
		if need { return true, nil }
	}
	return false, nil
}

// certsNeedRenewal parses PEM-encoded certs from pemBytes and reports whether
// any of them is within window of expiry (or already expired).
func certsNeedRenewal(pemBytes []byte, now time.Time, window time.Duration) (bool, error) {
	certs, err := parsePEMCerts(pemBytes)
	if err != nil { return false, err }
	for _, c := range certs {
		if now.After(c.NotAfter.Add(-window)) {
			return true, nil
		}
	}
	return false, nil
}

// parsePEMCerts decodes all CERTIFICATE PEM blocks and parses each. Returns
// an error if no certificate block was found.
func parsePEMCerts(pemBytes []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := pemBytes
	for {
		block, r := pem.Decode(rest)
		if block == nil { break }
		rest = r
		if block.Type != "CERTIFICATE" { continue }
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil { return nil, fmt.Errorf("failed to parse certificate: %w", err) }
		certs = append(certs, c)
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates found in PEM data")
	}
	return certs, nil
}
```

### 2.6 Tests — `pkg/controller/certificate/selfmanaged/backend_test.go` (modify, append)

Reuse the existing `testSelfManagedObject` fixture. Add:

- `TestSelfManagedBackendCurveP384` — `Curve: P-384`; parse leaf `tls.crt` via `pem.Decode`+`x509.ParseCertificate`; assert `cert.PublicKey.(*ecdsa.PublicKey).Curve == elliptic.P384()`. Also assert `ca.key` PEM block type is `EC PRIVATE KEY`.
- `TestSelfManagedBackendCurveP521` — same with P-521.
- `TestSelfManagedBackendCurveDefault` — existing golden test already covers P-256; add an explicit assertion that the parsed leaf key curve is `elliptic.P256()` when `Curve` is empty.
- `TestSelfManagedBackendCurveUnknown` — `Curve: "P-999"` → error containing "unsupported curve".
- `TestSelfManagedBackendIPSANs` — `IPAddresses: []string{"10.0.0.1","192.168.1.1"}`; parse leaf cert; assert `cert.IPAddresses` equals the parsed IPs. Also assert DNS names still present.
- `TestSelfManagedBackendIPSANsInvalid` — `IPAddresses: []string{"not-an-ip"}` → error containing "invalid IP SAN".
- `TestSelfManagedBackendCRL` — `GenerateCRL: true`; assert CA secret `Data["ca.crl"]` is non-empty and `x509.ParseRevocationList(...)` succeeds; assert parsed CRL `Number.Int64()==1`, `RevokedCertificateEntries` is empty, `ThisUpdate`/`NextUpdate` non-zero. Assert leaf secret has no `ca.crl`.
- `TestSelfManagedBackendCRLAbsent` — `GenerateCRL: false` (default); assert CA secret has no `ca.crl` key.
- `TestNeedRenewal_NilSecret` → true, nil err.
- `TestNeedRenewal_MissingTLSCrt` — secret with no `tls.crt` → true.
- `TestNeedRenewal_EmptyTLSCrt` — `tls.crt: []byte{}` → true.
- `TestNeedRenewal_MalformedPEM` — `tls.crt: []byte("not pem")` → error.
- `TestNeedRenewal_Expired` — generate a cert via the backend with `ValidityDays: 1`, then call `NeedRenewal` with `now = NotAfter + time.Hour` → true.
- `TestNeedRenewal_WithinWindow` — set `RenewalDays: 30`, generate cert, call with `now = NotAfter - 10*24*time.Hour` → true.
- `TestNeedRenewal_OutsideWindow` — `RenewalDays: 30`, `now = NotAfter - 100*24*time.Hour` → false.
- `TestNeedRenewal_CANearExpiry` — craft a secret whose `tls.crt` is far from expiry but `ca.crt` is near expiry (build manually with `x509.CreateCertificate` using short CA validity) → true. (Covers the ca.crt branch.)
- `TestNeedRenewal_CABadButAbsent` — secret with valid `tls.crt`, no `ca.crt` → false (ca.crt absent is skipped, not an error).
- `TestNeedRenewal_MalformedCA` — secret with valid `tls.crt` and garbage `ca.crt` → error.

---

## 3. `pkg/controller/certificate/certmanager/backend.go` (modify)

### 3.1 `buildLeafCertificate`

After the existing `dnsNames` block, add:

```go
	if len(spec.IPAddresses) > 0 {
		ips := make([]interface{}, len(spec.IPAddresses))
		for i, ip := range spec.IPAddresses {
			ips[i] = ip
		}
		specMap["ipAddresses"] = ips
	}
	if spec.RenewalDays > 0 {
		specMap["renewBefore"] = fmt.Sprintf("%dh", spec.RenewalDays*24)
	}
```

### 3.2 `buildLeafCertificateWithIssuer`

Same additions (ipAddresses + renewBefore) after the existing `dnsNames` block. Note: this builder currently does not set `subject.organizations` or `duration`; do **not** add those (out of scope) — only ipAddresses and renewBefore.

### 3.3 Tests — `pkg/controller/certificate/certmanager/backend_test.go` (modify, append)

- `TestCertManagerBackendIPSANsDedicatedCA` — `IPAddresses: []string{"10.0.0.1"}`; assert leaf `spec.ipAddresses` == `["10.0.0.1"]` via `unstructured.NestedStringSlice`.
- `TestCertManagerBackendIPSANsExistingCA` — same with `IssuerRef` set; assert leaf `spec.ipAddresses`.
- `TestCertManagerBackendIPSANsAbsentByDefault` — no `IPAddresses`; assert `spec.ipAddresses` not present (`_, found, _ := NestedStringSlice(...); !found`).
- `TestCertManagerBackendRenewBefore` — `RenewalDays: 20`; assert leaf `spec.renewBefore == "480h"` (both modes).
- `TestCertManagerBackendRenewBeforeAbsent` — `RenewalDays: 0`; assert `spec.renewBefore` not present.
- Existing tests (`TestCertManagerBackendDedicatedCA`, `TestCertManagerBackendExistingCA`, `TestCertManagerBackendEmptySecretName`, `TestCertManagerCertificateSecretName`, `TestCertManagerRequiresRotationSaga`) must stay green.

---

## 4. `pkg/controller/certificate/rotation/` (NEW package)

Single file `pkg/controller/certificate/rotation/rotation.go` plus `pkg/controller/certificate/rotation/rotation_test.go`.

### 4.1 Package doc

```go
// Package rotation provides a reusable multi-cycle TLS rotation saga step
// built on workflow.WorkflowStepReconcilerActionWithDiff.
//
// The saga phases are:
//   ""       -> Rotate   : emit new CA + leaf with ca.crt = newCA||oldCA bundle.
//   Rotate   -> Converge : hold desired state stable, gate on an operator-injected
//                          convergence check, then advance.
//   Converge -> ""       : emit leaf with ca.crt = newCA only (strip old CA).
//
// Backends with RequiresRotationSaga() == false degrade to a single-cycle emit
// of DesiredObjects with no phase writes.
package rotation
```

### 4.2 Imports

```go
import (
	"context"
	"fmt"
	"time"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/workflow"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/selfmanaged"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/multiphase"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/workflow"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)
```

(`crypto/x509` is also imported for the parsed-cert data publication — see 4.7.)

### 4.3 Phase constants

```go
const (
	// PhaseRotate is the saga phase during which the new CA + bundled leaf
	// have been applied and the operator waits for convergence.
	PhaseRotate workflow.WorkflowPhase = "Rotate"
	// PhaseConverge is the saga phase that strips the old CA from the leaf.
	PhaseConverge workflow.WorkflowPhase = "Converge"
)
```

### 4.4 Data-map keys (per-reconcile blackboard)

| Key | Type | Set by | Meaning |
|---|---|---|---|
| `rotationStartPhase` | `workflow.WorkflowPhase` | Read | phase at cycle start (before any advance) |
| `rotationRenewed` | `bool` | Read | true when phase "" triggered a renewal this cycle |
| `tlsSecret` | `*corev1.Secret` | Read | current leaf secret (if present) — SANITIZED deep copy with `tls.key` removed; certs + metadata retained |
| `caSecret` | `*corev1.Secret` | Read | current CA secret (if present) — SANITIZED deep copy with `ca.key` removed; certs + metadata retained |
| `leafCert` | `*x509.Certificate` | Read | first cert parsed from current leaf `tls.crt` (if parseable) |
| `caCert` | `*x509.Certificate` | Read | first cert parsed from current CA `ca.crt` (if parseable) |

### 4.5 Types

```go
// ConvergenceCheck is invoked during the Rotate phase to decide whether the
// system has absorbed the new CA bundle (e.g., all pods have rolled). Return
// true to advance to Converge; return false to requeue.
type ConvergenceCheck[T object.MultiPhaseObject] func(ctx context.Context, o T, data map[string]any) (bool, error)

// LabelsDecorator mutates labels on each expected object.
type LabelsDecorator[T object.MultiPhaseObject] func(o T, obj client.Object)

// AnnotationsDecorator mutates annotations on each expected object.
type AnnotationsDecorator[T object.MultiPhaseObject] func(o T, obj client.Object)

// Option configures a TLS saga step.
type Option[T object.MultiPhaseObject] func(*tlsStep[T])

// WithConvergenceCheck injects the Rotate-phase convergence gate. If not set,
// the Rotate phase advances to Converge immediately (no waiting).
func WithConvergenceCheck[T object.MultiPhaseObject](f ConvergenceCheck[T]) Option[T] {
	return func(s *tlsStep[T]) { s.convergenceCheck = f }
}

// WithLabelsDecorator applies labels to every expected object.
func WithLabelsDecorator[T object.MultiPhaseObject](f LabelsDecorator[T]) Option[T] {
	return func(s *tlsStep[T]) { s.labelsDecorator = f }
}

// WithAnnotationsDecorator applies annotations to every expected object.
func WithAnnotationsDecorator[T object.MultiPhaseObject](f AnnotationsDecorator[T]) Option[T] {
	return func(s *tlsStep[T]) { s.annotationsDecorator = f }
}

// tlsStep is the concrete saga step. It embeds the default workflow WithDiff
// action (for Diff/Apply/Configure/OnError/phase helpers) and overrides
// Read, OnDiff, OnSuccess.
type tlsStep[T object.MultiPhaseObject] struct {
	*workflow.DefaultWorkflowStepReconcilerActionWithDiff[T, client.Object]
	backend             certificate.TLSBackend[T]
	specBuilder         func(o T) certificate.TLSSpec
	convergenceCheck    ConvergenceCheck[T]
	labelsDecorator     LabelsDecorator[T]
	annotationsDecorator AnnotationsDecorator[T]
}
```

### 4.6 Constructor

```go
// NewTLSStep creates a reusable TLS rotation saga step.
//
//   - phaseName/conditionName/recorder/fieldManager: standard step wiring.
//   - backend: the TLSBackend (selfmanaged → saga; byo/certmanager → single cycle).
//   - specBuilder: extracts the TLSSpec from the reconciled object.
//   - opts: WithConvergenceCheck, WithLabelsDecorator, WithAnnotationsDecorator.
//
// The returned step is a WorkflowStepReconcilerActionWithDiff[T, client.Object].
func NewTLSStep[T object.MultiPhaseObject](
	c client.Client,
	phaseName shared.PhaseName,
	conditionName shared.ConditionName,
	recorder record.EventRecorder,
	fieldManager string,
	backend certificate.TLSBackend[T],
	specBuilder func(o T) certificate.TLSSpec,
	opts ...Option[T],
) workflow.WorkflowStepReconcilerActionWithDiff[T, client.Object] {
	s := &tlsStep[T]{
		DefaultWorkflowStepReconcilerActionWithDiff: workflow.NewWorkflowStepReconcilerActionWithDiff[T, client.Object](
			c, phaseName, conditionName, recorder, fieldManager,
		).(*workflow.DefaultWorkflowStepReconcilerActionWithDiff[T, client.Object]),
		backend:     backend,
		specBuilder: specBuilder,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}
```

### 4.7 `Read`

```go
func (s *tlsStep[T]) Read(ctx context.Context, o T, data map[string]any, logger *logrus.Entry) (multiphase.MultiPhaseRead[client.Object], reconcile.Result, error) {
	read := multiphase.NewMultiPhaseRead[client.Object]()
	spec := s.specBuilder(o)
	namespace := o.GetNamespace()
	leafName := s.backend.CertificateSecretName(o, spec)
	caName := leafName + selfmanaged.CASecretSuffix

	// Load current secrets (NotFound is acceptable).
	currentLeaf := &corev1.Secret{}
	leafErr := s.Client().Get(ctx, client.ObjectKey{Namespace: namespace, Name: leafName}, currentLeaf)
	if leafErr != nil && !apierrors.IsNotFound(leafErr) {
		return nil, reconcile.Result{}, fmt.Errorf("read leaf secret %q: %w", leafName, leafErr)
	}
	currentCA := &corev1.Secret{}
	caErr := s.Client().Get(ctx, client.ObjectKey{Namespace: namespace, Name: caName}, currentCA)
	if caErr != nil && !apierrors.IsNotFound(caErr) {
		return nil, reconcile.Result{}, fmt.Errorf("read CA secret %q: %w", caName, caErr)
	}
	leafExists := leafErr == nil
	caExists := caErr == nil

	// Publish current data (best-effort parse).
	if leafExists {
		data["tlsSecret"] = currentLeaf
		if c := firstCert(currentLeaf.Data[selfmanaged.CertKey]); c != nil {
			data["leafCert"] = c
		}
	}
	if caExists {
		data["caSecret"] = currentCA
		if c := firstCert(currentCA.Data[selfmanaged.CAKey]); c != nil {
			data["caCert"] = c
		}
	}

	// Non-saga backend: single-cycle emit, no phase logic.
	if !s.backend.RequiresRotationSaga() {
		objs, err := s.backend.DesiredObjects(ctx, o, spec)
		if err != nil {
			return nil, reconcile.Result{}, err
		}
		for _, obj := range objs {
			s.decorate(o, obj)
			read.AddExpectedObject(obj)
		}
		if caExists { read.AddCurrentObject(currentCA) }
		if leafExists { read.AddCurrentObject(currentLeaf) }
		return read, reconcile.Result{}, nil
	}

	startPhase := s.CurrentPhase(o)
	data["rotationStartPhase"] = startPhase

	switch startPhase {
	case "":
		needRenewal := !leafExists || !caExists
		if leafExists && caExists && !needRenewal {
			need, err := selfmanaged.NeedRenewal(currentLeaf, spec, time.Now())
			if err != nil {
				return nil, reconcile.Result{}, err
			}
			needRenewal = need
		}
		if needRenewal {
			objs, err := s.backend.DesiredObjects(ctx, o, spec)
			if err != nil {
				return nil, reconcile.Result{}, err
			}
			newCA, newLeaf, err := splitCAAndLeaf(objs, caName, leafName)
			if err != nil {
				return nil, reconcile.Result{}, err
			}
			// Bundle ca.crt = newCA || oldCA (chain preserved during transition).
			if leafExists {
				if oldCA, ok := currentLeaf.Data[selfmanaged.CAKey]; ok && len(oldCA) > 0 {
					bundled := make([]byte, 0, len(newCA.Data[selfmanaged.CAKey])+len(oldCA))
					bundled = append(bundled, newCA.Data[selfmanaged.CAKey]...)
					bundled = append(bundled, oldCA...)
					newLeaf.Data[selfmanaged.CAKey] = bundled
				}
			}
			s.decorate(o, newCA)
			s.decorate(o, newLeaf)
			read.AddExpectedObject(newCA)
			read.AddExpectedObject(newLeaf)
			if caExists { read.AddCurrentObject(currentCA) }
			if leafExists { read.AddCurrentObject(currentLeaf) }
			data["rotationRenewed"] = true
		} else {
			// Steady state: expected == current (SSA no-op), data already published.
			if caExists { read.AddExpectedObject(currentCA); read.AddCurrentObject(currentCA) }
			if leafExists { read.AddExpectedObject(currentLeaf); read.AddCurrentObject(currentLeaf) }
		}

	case PhaseRotate:
		// Stable: do NOT regenerate. Expected == current (already applied in "").
		if caExists { read.AddExpectedObject(currentCA); read.AddCurrentObject(currentCA) }
		if leafExists { read.AddExpectedObject(currentLeaf); read.AddCurrentObject(currentLeaf) }

	case PhaseConverge:
		// Clean leaf: ca.crt = current CA only (strip old CA). Do NOT regenerate.
		if caExists && leafExists {
			cleanLeaf := currentLeaf.DeepCopy()
			cleanLeaf.Data[selfmanaged.CAKey] = currentCA.Data[selfmanaged.CAKey]
			s.decorate(o, cleanLeaf)
			read.AddExpectedObject(cleanLeaf)
			read.AddCurrentObject(currentLeaf)
			read.AddExpectedObject(currentCA)
			read.AddCurrentObject(currentCA)
		} else {
			// Should not happen (we applied both in ""), but degrade gracefully:
			// fall back to a fresh DesiredObjects emit.
			objs, err := s.backend.DesiredObjects(ctx, o, spec)
			if err != nil {
				return nil, reconcile.Result{}, err
			}
			for _, obj := range objs {
				s.decorate(o, obj)
				read.AddExpectedObject(obj)
			}
			if caExists { read.AddCurrentObject(currentCA) }
			if leafExists { read.AddCurrentObject(currentLeaf) }
		}

	default:
		// Unknown phase: treat as "" (re-evaluate).
		s.AdvancePhase(ctx, o, "", logger)
		// Recurse-free: just set expected == current and let next cycle handle.
		if caExists { read.AddExpectedObject(currentCA); read.AddCurrentObject(currentCA) }
		if leafExists { read.AddExpectedObject(currentLeaf); read.AddCurrentObject(currentLeaf) }
	}

	return read, reconcile.Result{}, nil
}
```

Helpers:

```go
// splitCAAndLeaf finds the CA and leaf Secret in objs by name.
func splitCAAndLeaf(objs []client.Object, caName, leafName string) (*corev1.Secret, *corev1.Secret, error) {
	var ca, leaf *corev1.Secret
	for _, obj := range objs {
		sec, ok := obj.(*corev1.Secret)
		if !ok {
			continue
		}
		switch sec.Name {
		case caName:
			ca = sec
		case leafName:
			leaf = sec
		}
	}
	if ca == nil || leaf == nil {
		return nil, nil, fmt.Errorf("saga backend must return CA secret %q and leaf secret %q", caName, leafName)
	}
	return ca, leaf, nil
}

// firstCert parses the first CERTIFICATE PEM block. Returns nil on any failure
// (best-effort publication).
func firstCert(pemBytes []byte) *x509.Certificate {
	if len(pemBytes) == 0 {
		return nil
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	return c
}

func (s *tlsStep[T]) decorate(o T, obj client.Object) {
	if s.labelsDecorator != nil {
		s.labelsDecorator(o, obj)
	}
	if s.annotationsDecorator != nil {
		s.annotationsDecorator(o, obj)
	}
}
```

(Add `"encoding/pem"` and `"crypto/x509"` to imports.)

### 4.8 `OnDiff`

```go
func (s *tlsStep[T]) OnDiff(ctx context.Context, o T, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (reconcile.Result, error) {
	if !s.backend.RequiresRotationSaga() {
		return reconcile.Result{}, nil
	}
	startPhase, _ := data["rotationStartPhase"].(workflow.WorkflowPhase)
	if startPhase != PhaseRotate {
		return reconcile.Result{}, nil
	}
	if s.convergenceCheck == nil {
		// No gate injected: advance immediately.
		s.AdvancePhase(ctx, o, PhaseConverge, logger)
		return reconcile.Result{}, nil
	}
	converged, err := s.convergenceCheck(ctx, o, data)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !converged {
		return reconcile.Result{RequeueAfter: workflow.DefaultRequeueAfter}, nil
	}
	s.AdvancePhase(ctx, o, PhaseConverge, logger)
	return reconcile.Result{}, nil
}
```

### 4.9 `OnSuccess`

```go
func (s *tlsStep[T]) OnSuccess(ctx context.Context, o T, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (reconcile.Result, error) {
	// Inherited condition handling (sets condition True on success).
	res, err := s.DefaultWorkflowStepReconcilerActionWithDiff.OnSuccess(ctx, o, data, diff, logger)
	if err != nil {
		return res, err
	}
	if !s.backend.RequiresRotationSaga() {
		return res, nil
	}
	startPhase, _ := data["rotationStartPhase"].(workflow.WorkflowPhase)
	switch startPhase {
	case "":
		if renewed, _ := data["rotationRenewed"].(bool); renewed {
			s.AdvancePhase(ctx, o, PhaseRotate, logger)
		}
	case PhaseConverge:
		s.AdvancePhase(ctx, o, "", logger)
	}
	return res, nil
}
```

### 4.10 Phase state machine summary & safety rationale

```
Cycle N, startPhase "":
  Read: renewal needed? -> expected = [newCA, newLeaf(bundled ca.crt)], data[renewed]=true
        else            -> expected = current (no-op), data published
  Diff/Apply: applies bundled (if renewed)
  OnDiff: startPhase != Rotate -> no-op
  OnSuccess: renewed -> AdvancePhase(Rotate)   [AFTER Apply succeeded]

Cycle N+1, startPhase Rotate:
  Read: expected = current (stable, no regenerate)
  Diff: no-op
  OnDiff: convergenceCheck -> false: RequeueAfter(10s) [Apply skipped, safe]
                            -> true:  AdvancePhase(Converge)
  OnSuccess: startPhase == Rotate -> no advance

Cycle N+2, startPhase Converge:
  Read: expected = [currentCA, cleanLeaf(ca.crt = newCA only)]
  Diff/Apply: applies clean leaf
  OnDiff: no-op
  OnSuccess: AdvancePhase("")   [AFTER Apply succeeded]
```

**Why advances happen in OnSuccess (not Read/OnDiff):** advancing before `Apply` would persist a phase whose desired state was never applied; on an Apply failure the next cycle would re-read with the new phase and skip the apply (e.g., Converge→"" before applying the clean leaf would leave the bundled `ca.crt` forever). Advancing in `OnSuccess` guarantees the phase only progresses after a successful Apply. The single exception is `Rotate → Converge` in `OnDiff`, which is safe because `Apply` is a no-op in Rotate (expected == current).

**No-saga degradation:** `RequiresRotationSaga() == false` → `Read` emits `DesiredObjects` once, `OnDiff`/`OnSuccess` early-return (only the inherited condition handling runs). No phase writes. Identical to today's operator-managed single-cycle flow.

**Initial issuance (no current secrets):** the saga still runs `"" → Rotate → Converge → ""`. The bundle step is skipped (no old CA). Converge is a no-op (leaf already clean). Two extra cycles, correct behavior. Documented limitation.

### 4.11 Tests — `pkg/controller/certificate/rotation/rotation_test.go` (new)

Test fixture (mirror `pkg/controller/workflow/workflow_step_action_test.go`):

```go
type rotStatus struct {
	Conditions []metav1.Condition
	PhaseName  shared.PhaseName
	IsOnError  *bool
	LastError  string
	ObsGen     int64
	Ws         *workflow.WorkflowStatus
}
// implement object.MultiPhaseObjectStatus + workflow.WorkflowStatusGetter (GetWorkflowStatus returns &s.Ws, lazily allocating)

type rotObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Status rotStatus
}
// implement object.MultiPhaseObject (GetStatus) + DeepCopyObject
```

Use `selfmanaged.NewSelfManagedBackend[*rotObject]()` for saga tests and `byo.NewBYOBackend[*rotObject]()` for the non-saga test. `specBuilder` returns a fixed `certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", DNSNames: []string{"test.example.com"}, ValidityDays: 365, RenewalDays: 30}`. Build the fake client with `fake.NewClientBuilder().WithScheme(scheme).WithObjects(...).Build()` where scheme = `runtime.NewScheme()` + `clientgoscheme.AddToScheme`. Logger = `logrus.NewEntry(logrus.New())`.

Tests (call `Read`/`OnDiff`/`OnSuccess` directly; do **not** call full `Reconcile` — fake client can't SSA dry-run):

1. `TestNewTLSStep` — construct with selfmanaged backend; assert non-nil; assert it satisfies `workflow.WorkflowStepReconcilerActionWithDiff[*rotObject, client.Object]`; assert `GetPhaseName()` returns the given phase.
2. `TestRead_NoSagaBackend_SingleCycle` — byo backend, empty fake client; `Read`; assert expected has 0 objects (byo returns nil), `data["rotationStartPhase"]` absent, no phase set.
3. `TestRead_SteadyState_NoRenewal` — pre-seed fake client with a fresh leaf+CA secret (generate via backend, set `ResourceVersion=""`, add via `WithObjects`); `Read`; assert expected == current (2 objects), `data["rotationRenewed"]` absent/false, `data["tlsSecret"]`/`data["caSecret"]` set, `data["leafCert"]`/`data["caCert"]` non-nil, phase unchanged ("").
4. `TestRead_RenewalTriggered_MissingSecrets` — empty fake client; `Read`; assert expected has 2 objects (newCA, newLeaf), leaf `ca.crt` == newCA `ca.crt` (no bundle since no old leaf), `data["rotationRenewed"]==true`, `data["rotationStartPhase"]==""`.
5. `TestRead_RenewalTriggered_WithOldSecret` — pre-seed an old leaf+CA (old CA cert near expiry or just a different cert); `Read`; assert expected leaf `ca.crt` == `newCA.ca.crt + oldLeaf.ca.crt` (bundle), `data["rotationRenewed"]==true`.
6. `TestRead_Rotate_Stable` — set `o.Status.Ws.CurrentPhase = PhaseRotate`; pre-seed bundled secrets; `Read`; assert expected == current (no new generation), `data["rotationStartPhase"]==PhaseRotate`.
7. `TestRead_Converge_CleanLeaf` — set `CurrentPhase = PhaseConverge`; pre-seed bundled leaf + CA; `Read`; assert expected leaf `ca.crt` == `currentCA.ca.crt` only (no old CA), expected CA == current CA.
8. `TestRead_Converge_MissingSecretsFallback` — `CurrentPhase = PhaseConverge`, empty client; `Read`; assert expected == DesiredObjects (fallback path), no panic.
9. `TestRead_UnknownPhase` — `CurrentPhase = "Weird"`; `Read`; assert phase advanced to "" and expected == current (or empty).
10. `TestRead_GetError` — use a fake client that returns NotFound (covered) and a non-NotFound error is hard to inject with fake; instead test the leaf-error path by... (skip if not feasible) — cover via the NotFound path which is the realistic one. (If 100% requires the non-NotFound branch, inject via a custom client wrapper implementing `client.Client` that returns a sentinel error on Get.) **Add a `failingClient` wrapper** to force a non-NotFound Get error and assert `Read` returns that error (covers the two error-return branches).
11. `TestOnDiff_NoSaga` — byo backend; any data; assert `reconcile.Result{}`, nil.
12. `TestOnDiff_PhaseEmpty` — selfmanaged, `data["rotationStartPhase"]=""`; assert no-op.
13. `TestOnDiff_PhaseConverge` — selfmanaged, `data["rotationStartPhase"]=PhaseConverge`; assert no-op.
14. `TestOnDiff_Rotate_NotConverged` — inject `WithConvergenceCheck` returning `(false, nil)`; assert `RequeueAfter == workflow.DefaultRequeueAfter`, phase unchanged (still Rotate).
15. `TestOnDiff_Rotate_Converged` — check returning `(true, nil)`; assert phase advanced to `PhaseConverge`.
16. `TestOnDiff_Rotate_NilCheck` — no `WithConvergenceCheck`; assert phase advanced to `PhaseConverge` immediately.
17. `TestOnDiff_Rotate_CheckError` — check returning `(false, sentinelErr)`; assert error propagated, phase unchanged.
18. `TestOnSuccess_PhaseEmpty_Renewed` — `data["rotationStartPhase"]=""`, `data["rotationRenewed"]=true`; assert phase advanced to `PhaseRotate` and condition set True.
19. `TestOnSuccess_PhaseEmpty_NotRenewed` — `data["rotationRenewed"]` absent; assert phase unchanged.
20. `TestOnSuccess_Converge` — `data["rotationStartPhase"]=PhaseConverge`; assert phase advanced to `""`.
21. `TestOnSuccess_NoSaga` — byo backend; assert no phase advance, condition True set (inherited).
22. `TestOnSuccess_InheritedError` — call `OnSuccess` with a diff whose inherited `OnSuccess`... (the inherited OnSuccess never errors in practice; cover by asserting `res == reconcile.Result{}` and nil err). Skip error branch if unreachable.
23. `TestDecorators` — `WithLabelsDecorator` setting `{"app":"x"}` and `WithAnnotationsDecorator` setting `{"rot":"1"}`; `Read` with renewal triggered; assert both expected secrets carry the labels and annotations.
24. `TestFullProgression` — simulate the three cycles by calling `Read`→`OnDiff`→`OnSuccess` in sequence (skip `Diff`/`Apply`; manually reflect applied state into the fake client between cycles by `Create`/`Update`-ing the expected secrets so the next `Read` sees them as current). Assert phase transitions `"" → Rotate → Converge → ""` and that the leaf `ca.crt` is bundled after cycle 1 and clean after cycle 3. (This is the integration-style test that ties the saga together without SSA.)
25. `TestSplitCAAndLeaf` — direct unit test: pass `[leaf, ca]` in wrong order, assert correct split by name; pass missing leaf → error; pass non-Secret object → ignored.
26. `TestFirstCert` — valid PEM → cert; empty → nil; non-PEM → nil; non-CERTIFICATE block → nil; bad DER → nil.

> Coverage note: the inherited `Diff`/`Apply`/`Configure`/`OnError` are **not** re-tested here (covered by `pkg/controller/multiphase` and `pkg/controller/workflow` tests). The 100% target applies to the **new/changed code in this package** (`Read`, `OnDiff`, `OnSuccess`, helpers, constructor, options). The `default` branch of the `Read` switch and the `Converge` fallback branch are covered by tests 8 and 9.

---

## 5. `pkg/apis/workflow/zz_generated.deepcopy.go` (NEW, hand-written)

```go
//go:build !ignore_autogenerated

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

// Hand-written deepcopy for WorkflowStatus.
//
// controller-gen does not generate deepcopy methods for WorkflowStatus in this
// repository (it is not a runtime.Object and is not referenced by any CRD
// generated here), and CI does not run deepcopy-gen. This file is therefore
// maintained BY HAND: when fields are added to WorkflowStatus, update the
// DeepCopyInto below manually. Do not delete this file expecting regeneration.

package workflow

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeepCopyInto copies the receiver, writing into out. in must be non-nil.
func (in *WorkflowStatus) DeepCopyInto(out *WorkflowStatus) {
	*out = *in
	if in.PhaseConditions != nil {
		in, out := &in.PhaseConditions, &out.PhaseConditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy returns a deep copy of the receiver, or nil if in is nil.
func (in *WorkflowStatus) DeepCopy() *WorkflowStatus {
	if in == nil {
		return nil
	}
	out := new(WorkflowStatus)
	in.DeepCopyInto(out)
	return out
}
```

Notes:
- `CurrentPhase` is a `WorkflowPhase` (string) → shallow copy via `*out = *in` is sufficient.
- `PhaseConditions` is `[]metav1.Condition` → deep-copied element-wise via `metav1.Condition.DeepCopyInto` (each Condition has pointer/slice fields like `LastTransitionTime`).
- The build tag `//go:build !ignore_autogenerated` matches sibling `zz_generated.deepcopy.go` files.

### 5.1 Tests — `pkg/apis/workflow/workflow_status_test.go` (modify, append)

- `TestWorkflowStatusDeepCopyNil` — `(*WorkflowStatus)(nil).DeepCopy()` → nil.
- `TestWorkflowStatusDeepCopyEmpty` — `&WorkflowStatus{}.DeepCopy()` → non-nil, equal, empty PhaseConditions (nil preserved).
- `TestWorkflowStatusDeepCopyPopulated` — build `&WorkflowStatus{CurrentPhase: "Rotate", PhaseConditions: []metav1.Condition{{Type:"Ready", Status: metav1.ConditionTrue, ...}}}`; `DeepCopy()`; assert equal; **mutate the copy's PhaseConditions[0].Type** and assert the original is unchanged (deep isolation); assert the copy's slice header is independent (append to copy, original length unchanged).
- `TestWorkflowStatusDeepCopyInto` — call `DeepCopyInto` into a pre-allocated `&WorkflowStatus{}` and assert populated.

---

## 6. `documentations/tls-and-workflow.md` (modify)

### 6.1 TLSSpec table (replace the Go block at lines 70-81)

Add the four new fields to the struct listing and add a field table beneath it:

| Field | Type | Default | Applies to | Description |
|---|---|---|---|---|
| `Curve` | string | `P-256` | selfmanaged | ECDSA curve: `P-256`, `P-384`, `P-521`. Unknown → error. |
| `IPAddresses` | []string | none | selfmanaged, certmanager | SAN IPs (parsed via `net.ParseIP`). |
| `RenewalDays` | int | 30 | selfmanaged (NeedRenewal), certmanager (renewBefore) | Renewal window before `NotAfter`. |
| `GenerateCRL` | bool | false | selfmanaged | Adds `ca.crl` (DER) to the CA Secret. |

### 6.2 Self-managed backend section (replace lines 83-93)

Document: ECDSA curve selection via `Curve`; IP SANs via `IPAddresses`; optional CRL via `GenerateCRL` (reissued fresh on each renewal, empty revoked list — documented limitation); renewal window via `RenewalDays` + `certificate.GetValidRenewalDays(spec)` + `selfmanaged.NeedRenewal(secret, spec, now)`. CA validity remains 2× leaf validity. CA Secret keys: `ca.crt`, `ca.key`, and (when `GenerateCRL`) `ca.crl`.

### 6.3 New "Rotation saga (rotation package)" section (insert after the WorkflowStep section, before "Rollout helper")

Include:
- Phase diagram: `"" → Rotate → Converge → ""` with the description from §4.10.
- `NewTLSStep` signature and a usage snippet:
  ```go
  step := rotation.NewTLSStep[*MyCRD](
      r.Client(), "tls", "TLSCertificatesReady", r.Recorder, "my-operator",
      selfmanaged.NewSelfManagedBackend[*MyCRD](),
      func(o *MyCRD) certificate.TLSSpec { return o.Spec.TLS },
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
- Note: backends with `RequiresRotationSaga() == false` degrade to a single-cycle emit.
- Migration note: operators' CRD status must embed `workflow.WorkflowStatus` (now DeepCopy-safe) and implement `GetWorkflowStatus() *workflow.WorkflowStatus`.

### 6.4 Testing section (replace lines 251-260)

Add `./pkg/controller/certificate/rotation/` to the test command list and note the fake-client SSA limitation (rotation tests call `Read`/`OnDiff`/`OnSuccess` directly).

---

## 7. Validation commands

```bash
# Format
dagger call -m golang --src . format export --path .

# Lint
dagger call -m golang --src . lint

# Tests (per package, filtered to failures)
dagger call --src . test --withGotestsum --path ./pkg/controller/certificate/ 2>&1 | grep -E "(FAIL|build failed|\.go:[0-9]+:|panic|--- FAIL)" | head -200
dagger call --src . test --withGotestsum --path ./pkg/apis/workflow/         2>&1 | grep -E "(FAIL|build failed|\.go:[0-9]+:|panic|--- FAIL)" | head -200

# Specific tests
dagger call --src . test --withGotestsum --run "TestNeedRenewal"
dagger call --src . test --withGotestsum --run "TestRead_RenewalTriggered"
dagger call --src . test --withGotestsum --run "TestWorkflowStatusDeepCopy"

# Coverage check (changed packages)
dagger call --src . test --withGotestsum --path ./pkg/controller/certificate/ export --path cover.out
go tool cover -func=cover.out | grep -E "backend.go|selfmanaged|certmanager|rotation"

# Full CI pipeline (includes vulncheck)
dagger call --src . ci
```

---

## 8. Design decisions made by the architect (NOT in the input spec)

1. **Phase advances happen in `OnSuccess`, not `Read`/`OnDiff`** (except `Rotate → Converge` in `OnDiff`). The spec said "Read ... then AdvancePhase(Rotate)" and "Phase Converge: ... AdvancePhase(\"\")". Implementing those advances in `Read` would persist a phase before `Apply` runs; on an `Apply` failure the next cycle would re-read with the new phase and skip the apply (e.g., the bundled `ca.crt` would never be cleaned). Advancing in `OnSuccess` (after successful Apply) is safe. `Rotate → Converge` in `OnDiff` is safe because `Apply` is a no-op in Rotate. This is the single most important deviation from the literal spec wording, made for correctness.

2. **`startPhase` is captured in `Read` and stored in `data["rotationStartPhase"]`** so `OnDiff`/`OnSuccess` reason about the phase at cycle start, not the (possibly already-mutated) current phase. This avoids ambiguous same-cycle transitions.

3. **CA/leaf identification by name, not by return order.** `splitCAAndLeaf` finds the CA secret (`leafName + selfmanaged.CASecretSuffix`) and leaf secret (`leafName`) by name in `DesiredObjects`'s return. This decouples the rotation step from the backend's return ordering and makes the saga robust to future backend changes. The step imports `selfmanaged.CASecretSuffix` (acceptable coupling: the saga is specifically for saga-capable backends, and only selfmanaged returns `true` today).

4. **`Converge` does NOT regenerate.** It builds the clean leaf from the **current** leaf + current CA secret (`cleanLeaf.Data["ca.crt"] = currentCA.Data["ca.crt"]`), preserving the exact CA/leaf applied during `Rotate`. Regenerating in `Converge` would produce a different CA/key pair and break the rotation. The spec's "do NOT regenerate — stable across cycles" applies to `Rotate`; we extend the same principle to `Converge` for the same reason.

5. **`Converge` fallback when current secrets are missing** (should not happen, but defensive): emit a fresh `DesiredObjects`. Covered by a test. Avoids a nil-pointer/empty-diff deadlock.

6. **`WithConvergenceCheck == nil` → advance immediately** (treated as always-converged). The spec didn't define the nil case. Immediate advance is the least-surprising default and lets operators skip the gate when they don't need it.

7. **Decorator style: mutators `func(o T, obj client.Object)`** applied to each expected object in `Read`. The spec said "WithLabelsDecorator / WithAnnotationsDecorator (default: none)" without defining the signature. Mutators are the most flexible (per-object labels) and easiest to test.

8. **Data publication keys** `tlsSecret`, `caSecret`, `leafCert`, `caCert` (best-effort parse via `firstCert`). The spec said "tlsSecret, caSecret, parsed certs" without naming the parsed keys; these names are chosen here.

9. **CRL stored as raw DER** (not PEM) under `ca.crl`, consumed via `x509.ParseRevocationList`. PEM-wrapping a CRL is non-standard; DER is the conventional on-wire encoding.

10. **Rotation tests call `Read`/`OnDiff`/`OnSuccess` directly** (not full `Reconcile`) because the controller-runtime fake client does not support SSA dry-run `Patch`, which the inherited `Diff` requires. The inherited `Diff`/`Apply` are already covered by `pkg/controller/multiphase` tests. A `failingClient` wrapper is introduced to cover the non-NotFound `Get` error branches in `Read`.

11. **`zz_generated.deepcopy.go` is hand-written with an explicit "maintained BY HAND" header** overriding the usual `DO NOT EDIT` line, because controller-gen does not emit for `WorkflowStatus` here and CI does not run deepcopy-gen. Risk: if a future CI step starts running controller-gen with `+k8s:deepcopy-gen=package,register` (already in `doc.go`), it could regenerate/overwrite this file; mitigated by the header comment and by the fact that the generated output would be functionally identical.

---

## 9. Risks & assumptions

- **Additive `TLSSpec` change**: no breaking change for existing operators. `Curve` defaults to P-256 so existing selfmanaged certs stay byte-stable until their next renewal.
- **`GenerateCRL` changes `SecretHash`**: operators opting in get one extra rolling restart (expected, documented).
- **CRL has no incremental revocation maintenance**: reissued fresh on each renewal with an empty revoked list (documented limitation).
- **Saga requires CRD status to embed `workflow.WorkflowStatus`** (now DeepCopy-safe). Migration note added to docs.
- **`TLSBackend` interface is untouched**; `DesiredObjects` stays pure desired-state. Renewal gating lives in `selfmanaged.NeedRenewal` + the `rotation` step.
- **Fake-client SSA limitation** constrains rotation test strategy to direct method calls (see decision 10).
- **Initial issuance runs the full saga** (two extra cycles, no bundle). Correct, slightly wasteful; documented.
- **The rotation step assumes saga backends return exactly two `*corev1.Secret` objects** named `<leafName>` and `<leafName>-ca`. Enforced by `splitCAAndLeaf` returning a clear error otherwise.

---

## 10. Open questions

None remaining — all decisions resolved in §8.
