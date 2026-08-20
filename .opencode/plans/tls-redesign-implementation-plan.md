# TLS API Redesign — Implementation Plan

This is the implementation-ready plan for the TLS API redesign described in
`.kilo/plans/1787235611932-tls-redesign-plan.md`. It has been **verified and
corrected against the actual codebase**. A developer reading only this file can
implement the whole change.

Scope: `pkg/controller/certificate/**` + `documentations/tls-and-workflow.md`.
No other packages, no CRD schemas, no deepcopy/controller-gen changes.

---

## 0. Corrections to the `.kilo` plan (discrepancies found)

These are the places where the `.kilo` plan diverged from reality. The
implementation below resolves each one.

1. **`zz_generated.deepcopy.go` does NOT exist in `pkg/controller/certificate/`.**
   `TLSSpec` is a plain struct (not a `runtime.Object`) and the package has no
   `+k8s:deepcopy-gen=package` doc.go. Deepcopy files exist only under
   `pkg/apis/*`. `make generate` targets `./pkg/apis/...` only (see
   `Makefile`). `LayerSignals` and `LeafChange` live only in the volatile
   `data map[string]any` and are never persisted. **Resolution: drop the
   deepcopy-regeneration task entirely.** No controller-gen run is required.

2. **`NeedRenewal` is referenced more widely than "only rotation.go".** It is
   called by `rotation.go` (line 176), tested by 11 `TestNeedRenewal_*` tests in
   `selfmanaged/backend_test.go`, and referenced by
   `TestRead_Saga_NeedRenewalError` in `rotation_test.go`. It is also
   documented. **Resolution: remove `NeedRenewal`** (production + its 11 tests)
   and replace with `CANeedsRenewal` (CA expiry) + `LeafManager.LeafNeedsChange`
   (leaf expiry/drift). Keep the unexported `certsNeedRenewal`/`parsePEMCerts`
   helpers (reused by `CANeedsRenewal`). Rationale: v3 is already a breaking
   change; the combined leaf+CA semantics of `NeedRenewal` no longer fit the
   split model.

3. **`SelfSigned`/`CertManager` are dead in code but public API + documented.**
   They are only *defined* in `backend.go` and mentioned in
   `documentations/tls-and-workflow.md` and migration docs. No code reads them.
   Removing them is safe in-repo. `GetValidTLSDays`/`ValidityDays` are used by
   `selfmanaged/backend.go` and `certmanager/backend.go` (`spec.ValidityDays`
   directly) and tested in `backend_test.go`. All replaced below.

4. **cert-manager CA Certificate currently has NO duration.** `buildCACertificate`
   sets only `isCA/commonName/secretName/issuerRef/subject`. Adding
   `CAValidityDays` is *new* logic, not a rename. Also,
   `buildLeafCertificateWithIssuer` (existing-CA mode) currently sets **no**
   leaf duration at all (only `buildLeafCertificate` does) — an asymmetry the
   plan resolves by adding leaf duration to existing-CA mode too (flagged as a
   deliberate behavior fix).

5. **Per-node leaf-only drift needs a `LeafNodesChanged` reason.** The `.kilo`
   plan gates the leaf-only branch on `leafChg.Reason != LeafNone`, but its enum
   has no node-drift reason — so node add/remove (Reason would be empty) would
   never trigger the leaf-only regen. **Resolution: add `LeafNodesChanged` to the
   enum and an `LeafChange.IsZero()` helper; gate the leaf-only branch on
   `!leafChg.IsZero()`.**

6. **`NodeSpecProviderFunc` in the `.kilo` plan only adapts `NodeCertSpec`, not
   the whole `NodeSpecProvider` (which also has `ExpectedNodeNames`).**
   **Resolution: drop the `NodeSpecProviderFunc` adapter.** The constructor takes
   a `NodeSpecProvider[T]` interface; operators implement a two-method struct
   (exactly like the `.kilo` ES sketch's `esNodeSpecProvider{}`).

7. **Private-key leak if per-node publishes `tlsSecret`.** The per-node leaf
   Secret's private keys are `<node>.key` (dynamic names), not `tls.key`, so
   `sanitizedSecret(currentLeaf, selfmanaged.KeyKey)` would leave them in the
   data map. **Resolution: per-node steps (`NodeSetTLSBackend`) omit
   `data["tlsSecret"]` and `data["leafCert"]` entirely** (as the `.kilo` plan
   intends); `data["caSecret"]`/`data["caCert"]` are still published (CA secret
   uses `ca.key`, which `sanitizedSecret` already strips).

8. **`selfmanaged` helpers must be exported for reuse by the new
   `selfmanaged/pernode` package.** The current CA/leaf generation is inlined in
   `SelfManagedBackend.DesiredObjects`. The plan extracts exported helpers
   (see §5) and refactors `DesiredObjects` to use them.

9. **Force-annotation removal tests need the fake client to know `rotObject`.**
   `s.Client().Update(ctx, o)` operates on the CRD object, but the existing
   `rotation_test.go` fake client registers only `clientgoscheme` (Secrets). A
   scheme variant that registers `rotObject` is required (see §9 test helper).

10. **Validation commands confirmed.** `CONTRIBUTING.md` confirms
    `dagger call -m golang --src . lint` / `vulncheck` and
    `dagger call --src . test --withGotestsum` are the correct invocations (the
    `-m golang` references the `golang` dependency module). The `.kilo` plan's
    commands are correct; they are reproduced in §12.

11. **`client.Update` resourceVersion refresh is safe.** controller-runtime
    v0.19.3 (`go.mod`) typed `Update` deserializes the response into the object
    (`r.Into(obj)`), refreshing `resourceVersion`; the fake client does the
    same. The deferred `Status().Update` in the outer reconciler
    (`controller.DeferStatusUpdate`, `pkg/controller/helper.go`) therefore does
    not conflict after `OnSuccess` removes a force annotation. Still flagged as
    a low risk in §13.

---

## 1. Files to create / modify

| # | File | Action |
|---|---|---|
| 1 | `pkg/controller/certificate/backend.go` | MODIFY |
| 2 | `pkg/controller/certificate/rollout.go` | MODIFY |
| 3 | `pkg/controller/certificate/selfmanaged/backend.go` | MODIFY |
| 4 | `pkg/controller/certificate/selfmanaged/pernode/backend.go` | CREATE |
| 5 | `pkg/controller/certificate/certmanager/backend.go` | MODIFY |
| 6 | `pkg/controller/certificate/byo/backend.go` | NO CHANGE (verify only) |
| 7 | `pkg/controller/certificate/rotation/rotation.go` | MODIFY |
| 8 | `pkg/controller/certificate/backend_test.go` | MODIFY |
| 9 | `pkg/controller/certificate/rollout_test.go` | MODIFY |
| 10 | `pkg/controller/certificate/selfmanaged/backend_test.go` | MODIFY |
| 11 | `pkg/controller/certificate/selfmanaged/pernode/backend_test.go` | CREATE |
| 12 | `pkg/controller/certificate/certmanager/backend_test.go` | MODIFY |
| 13 | `pkg/controller/certificate/rotation/rotation_test.go` | MODIFY |
| 14 | `pkg/controller/certificate/rotation/rotation_internal_test.go` | NO CHANGE (verify only) |
| 15 | `documentations/tls-and-workflow.md` | MODIFY |

---

## 2. `pkg/controller/certificate/backend.go` (MODIFY)

Add imports `"time"` and `corev1 "k8s.io/api/core/v1"`.

### 2.1 `TLSSpec` (slimmed; remove `SelfSigned`, `CertManager`, `ValidityDays`)

```go
// TLSSpec defines the desired (computed) TLS configuration for a component.
// Operators embed a SLIMMED version in their CRD spec and pass computed values
// to the library via a TLSSpecProvider. Backend selection is an operator
// decision (which backend type to construct), NOT spec data — the old
// SelfSigned/CertManager booleans are removed.
type TLSSpec struct {
	// SecretName is the name of the Secret that will hold the certificate
	// and key. Consumers mount this Secret or hash it for rollout.
	SecretName string `json:"secretName,omitempty"`

	// IssuerRef references an existing Issuer or ClusterIssuer for the
	// cert-manager backend (existing-CA mode). When empty, a self-signed
	// Issuer is created (dedicated-CA mode).
	IssuerRef string `json:"issuerRef,omitempty"`

	// CommonName is the CN for the generated certificate.
	CommonName string `json:"commonName,omitempty"`

	// DNSNames lists the SAN DNS names for the certificate.
	DNSNames []string `json:"dnsNames,omitempty"`

	// Organization is the O field for the certificate.
	Organization string `json:"organization,omitempty"`

	// LeafValidityDays is the leaf certificate validity in days.
	// Defaults to 365 if not set (see GetValidLeafDays).
	LeafValidityDays int `json:"leafValidityDays,omitempty"`

	// CAValidityDays is the CA certificate validity in days.
	// Defaults to 2× LeafValidityDays when <= 0 (see GetValidCADays).
	CAValidityDays int `json:"caValidityDays,omitempty"`

	// Curve selects the ECDSA curve for the selfmanaged backend.
	// One of P-256 (default), P-384, P-521. Unknown values are rejected by
	// the selfmanaged backend. Ignored by cert-manager.
	Curve string `json:"curve,omitempty"`

	// IPAddresses lists the SAN IPs for the generated certificate.
	// Each entry must parse via net.ParseIP.
	IPAddresses []string `json:"ipAddresses,omitempty"`

	// RenewalDays is the window before expiry during which renewal is
	// triggered. Defaults to 30 (see GetValidRenewalDays).
	RenewalDays int `json:"renewalDays,omitempty"`

	// GenerateCRL, when true, adds a ca.crl key (DER-encoded revocation
	// list) to the CA Secret (selfmanaged only).
	GenerateCRL bool `json:"generateCRL,omitempty"`
}
```

### 2.2 Provider interface + adapter

```go
// TLSSpecProvider supplies the computed TLSSpec for an object. The operator
// implements this (or uses TLSSpecProviderFunc) instead of passing a bare
// func to NewTLSStep.
type TLSSpecProvider[T object.MultiPhaseObject] interface {
	TLSSpec(o T) TLSSpec
}

// TLSSpecProviderFunc adapts a func(o T) TLSSpec to TLSSpecProvider.
type TLSSpecProviderFunc[T object.MultiPhaseObject] func(o T) TLSSpec

func (f TLSSpecProviderFunc[T]) TLSSpec(o T) TLSSpec { return f(o) }
```

### 2.3 Validity helpers (replace `GetValidTLSDays`)

```go
// GetValidLeafDays returns the leaf certificate validity in days, defaulting
// to 365 when LeafValidityDays <= 0.
func GetValidLeafDays(spec TLSSpec) int {
	if spec.LeafValidityDays <= 0 {
		return 365
	}
	return spec.LeafValidityDays
}

// GetValidCADays returns the CA certificate validity in days, defaulting to
// 2× GetValidLeafDays when CAValidityDays <= 0.
func GetValidCADays(spec TLSSpec) int {
	if spec.CAValidityDays <= 0 {
		return 2 * GetValidLeafDays(spec)
	}
	return spec.CAValidityDays
}
```

**Remove** `GetValidTLSDays`. Keep `GetValidRenewalDays` unchanged.

### 2.4 Drift model

```go
// LeafChangeReason is the dominant reason a leaf needs regeneration.
type LeafChangeReason string

const (
	LeafNone         LeafChangeReason = ""
	LeafMissing      LeafChangeReason = "Missing"
	LeafExpiring     LeafChangeReason = "Expiring"
	LeafCNChanged    LeafChangeReason = "CNChanged"
	LeafOrgChanged   LeafChangeReason = "OrgChanged"
	LeafSANsChanged  LeafChangeReason = "SANsChanged"
	LeafIPsChanged   LeafChangeReason = "IPsChanged"
	LeafNodesChanged LeafChangeReason = "NodesChanged"
	LeafForceRegen   LeafChangeReason = "Forced"
)

// LeafChange describes leaf drift vs spec at `now`. Delta slices are always
// populated (independent of Reason) so consumers can compute additive-only
// rollout decisions.
type LeafChange struct {
	Reason       LeafChangeReason
	SANsAdded    []string
	SANsRemoved  []string
	IPsAdded     []string
	IPsRemoved   []string
	NodesAdded   []string
	NodesRemoved []string
}

// IsZero reports whether no regeneration is needed.
func (c LeafChange) IsZero() bool {
	return c.Reason == LeafNone &&
		len(c.SANsAdded) == 0 && len(c.SANsRemoved) == 0 &&
		len(c.IPsAdded) == 0 && len(c.IPsRemoved) == 0 &&
		len(c.NodesAdded) == 0 && len(c.NodesRemoved) == 0
}
```

### 2.5 Capability interfaces

```go
// LeafManager is the optional capability for saga backends that own their leaf
// Secret and can (a) regenerate it against an existing CA and (b) report drift.
// Implemented by selfmanaged (single-leaf) and selfmanaged/pernode. The
// rotation step type-asserts this; when absent (cert-manager/BYO) leaf-only
// regen is skipped and drift falls back to the full CA saga.
type LeafManager[T object.MultiPhaseObject] interface {
	// DesiredLeafWithCA re-issues the leaf signed by the CA in caSecret,
	// reusing that CA's key/cert (no new CA). The returned Secret's ca.crt
	// equals caSecret's ca.crt (single CA, no bundle).
	DesiredLeafWithCA(ctx context.Context, o T, spec TLSSpec, caSecret *corev1.Secret) (*corev1.Secret, error)

	// LeafNeedsChange reports whether the leaf Secret needs regeneration vs
	// spec at now. leafSecret may be nil (treated as LeafMissing).
	LeafNeedsChange(ctx context.Context, o T, leafSecret *corev1.Secret, spec TLSSpec, now time.Time) (LeafChange, error)
}

// NodeSetTLSBackend marks per-node (multi-cert) saga backends. Its presence
// changes rotation data publishing (omit tlsSecret/leafCert — see §7.3).
type NodeSetTLSBackend[T object.MultiPhaseObject] interface {
	TLSBackend[T]
	// ExpectedNodeNames returns the node names that should have a certificate.
	ExpectedNodeNames(o T) ([]string, error)
	// NodeSecretKeys returns the Data-key suffixes for a node's cert and key
	// (e.g. ".crt", ".key"); a node's Data keys are name+certSuffix and
	// name+keySuffix.
	NodeSecretKeys() (certSuffix, keySuffix string)
}
```

---

## 3. `pkg/controller/certificate/rollout.go` (MODIFY)

Add force-annotation constants (keep `AnnotationSecretHash`):

```go
const (
	// AnnotationSecretHash is the annotation key used to store a stable hash
	// of the certificate Secret contents on pod templates.
	AnnotationSecretHash = "operator-sdk-extra.webcenter.fr/certificate-hash"

	// AnnotationForceRegenerateAll triggers a full CA+leaf rotation + rollout.
	// Read as == "true" (the ignoreReconcile convention). Overridable via
	// rotation.WithForceRegenerateAllAnnotation.
	AnnotationForceRegenerateAll = "operator-sdk-extra.webcenter.fr/force-regenerate-tls"

	// AnnotationForceRegenerateLeaf triggers a leaf-only regen + rollout.
	AnnotationForceRegenerateLeaf = "operator-sdk-extra.webcenter.fr/force-regenerate-certificates"
)
```

Add signals + rollout policy:

```go
// LayerSignals is the per-layer change-delta signal a TLS step publishes under
// data["tls.<phaseName>"] for the STS step to gate rollout.
type LayerSignals struct {
	CARotated       bool        // a CA-saga started this cycle
	LeafRegenerated bool        // leaf regenerated (CA saga OR leaf-only)
	LeafChange      *LeafChange // drift/expiry detail (SAN/IP/node deltas)
	Forced          bool        // a force annotation was honored this cycle
}

// RolloutPolicy selects which change-delta signals trigger a rollout.
type RolloutPolicy string

const (
	RolloutAlways     RolloutPolicy = "Always"
	RolloutOnCAChange RolloutPolicy = "OnCAChange"
	RolloutOnAdditive RolloutPolicy = "OnAdditive" // recommended default
	RolloutNever      RolloutPolicy = "Never"
)

// ShouldRollout decides whether a pod rollout is needed for one layer's
// signals. Forced overrides everything, including RolloutNever.
func ShouldRollout(policy RolloutPolicy, sig *LayerSignals) bool {
	if sig == nil {
		return false
	}
	if sig.Forced {
		return true
	}
	switch policy {
	case RolloutAlways:
		return sig.CARotated || sig.LeafRegenerated
	case RolloutOnCAChange:
		return sig.CARotated
	case RolloutNever:
		return false
	default: // RolloutOnAdditive (unknown values treated as additive)
		if sig.CARotated {
			return true
		}
		lc := sig.LeafChange
		if lc == nil {
			return false
		}
		switch lc.Reason {
		case LeafExpiring, LeafCNChanged, LeafOrgChanged, LeafMissing, LeafForceRegen:
			return true
		case LeafSANsChanged, LeafIPsChanged:
			return len(lc.SANsAdded) > 0 || len(lc.IPsAdded) > 0
		default: // LeafNone, LeafNodesChanged
			return false
		}
	}
}

// RolloutAnnotation builds the pod-template annotation map entry.
//   - shouldRollout=true: fresh hash of secret (SecretHashAnnotation).
//   - shouldRollout=false and currentHash!="": keep currentHash (no restart).
//   - shouldRollout=false and currentHash=="" (first run): initialize hash.
func RolloutAnnotation(shouldRollout bool, secret *corev1.Secret, currentHash string) (map[string]string, error) {
	if !shouldRollout && currentHash != "" {
		return map[string]string{AnnotationSecretHash: currentHash}, nil
	}
	return SecretHashAnnotation(secret)
}
```

(Existing `SecretHash`, `SecretHashAnnotation`, `sortedKeys` unchanged.)

---

## 4. `pkg/controller/certificate/selfmanaged/backend.go` (MODIFY)

### 4.1 Exported shared helpers (new; reused by single-leaf and per-node)

```go
// BuildCA generates a fresh CA key + self-signed CA certificate and returns
// the CA Secret (name secretName+CASecretSuffix), the CA private key, and the
// parsed CA certificate. When spec.GenerateCRL, ca.crl is added to the Secret.
func BuildCA(namespace, secretName string, spec certificate.TLSSpec) (*corev1.Secret, *ecdsa.PrivateKey, *x509.Certificate, error)

// SignLeaf generates a new leaf key + certificate signed by caKey/caCert and
// returns the PEM-encoded leaf cert and private key. Validity uses
// GetValidLeafDays; SANs/CN/O come from spec.
func SignLeaf(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, spec certificate.TLSSpec) (leafCertPEM, leafKeyPEM []byte, err error)

// ParseCA parses a CA Secret's ca.crt (first cert) and ca.key (EC private key).
func ParseCA(caSecret *corev1.Secret) (*x509.Certificate, *ecdsa.PrivateKey, error)

// CANeedsRenewal reports whether the CA Secret's ca.crt needs renewal at now.
//   - nil secret -> true (missing CA).
//   - ca.crt absent/empty -> true.
//   - ca.crt malformed -> error.
//   - within GetValidRenewalDays(spec) of NotAfter -> true; else false.
func CANeedsRenewal(caSecret *corev1.Secret, spec certificate.TLSSpec, now time.Time) (bool, error)
```

Implementation notes:
- `BuildCA`/`SignLeaf` are extracted from the current `DesiredObjects` body (CA
  NotAfter uses `certificate.GetValidCADays(spec)`, leaf NotAfter uses
  `certificate.GetValidLeafDays(spec)`).
- `ParseCA` uses `pem.Decode` + `x509.ParseECPrivateKey` for the key and
  `parsePEMCerts` for the cert.
- `CANeedsRenewal` reuses `certsNeedRenewal(caSecret.Data[CAKey], now, window)`.

### 4.2 `DesiredObjects` (refactor to helpers; split validity)

`DesiredObjects` keeps its exact output shape (CA Secret + leaf Secret) but:
- calls `BuildCA` then `SignLeaf`;
- CA cert `NotAfter` = `GetValidCADays` days; leaf `NotAfter` =
  `GetValidLeafDays` days (previously both used `GetValidTLSDays`, CA = 2×).

### 4.3 Implement `LeafManager` on `*SelfManagedBackend[T]`

```go
func (b *SelfManagedBackend[T]) DesiredLeafWithCA(ctx context.Context, o T, spec certificate.TLSSpec, caSecret *corev1.Secret) (*corev1.Secret, error)

func (b *SelfManagedBackend[T]) LeafNeedsChange(ctx context.Context, o T, leafSecret *corev1.Secret, spec certificate.TLSSpec, now time.Time) (certificate.LeafChange, error)
```

**`DesiredLeafWithCA`**: `caCert, caKey := ParseCA(caSecret)`; error if `ca.key`
missing/unparseable (`"CA secret %q missing ca.key"`). `SignLeaf(caCert, caKey,
spec)`; build leaf Secret (name `spec.SecretName`, `Type: SecretTypeTLS`) with
`tls.crt`, `tls.key`, and `ca.crt = caSecret.Data[CAKey]` (single CA, no bundle).

**`LeafNeedsChange`** (single-cert semantics):
1. `leafSecret == nil` → `{Reason: LeafMissing}`.
2. `tls.crt` absent/empty → `{Reason: LeafMissing}`.
3. Parse first CERTIFICATE block of `tls.crt` (via `parsePEMCerts`, take
   `certs[0]`). Unparseable → **error** (not Missing).
4. Expiry: `now.After(cert.NotAfter.Add(-window))` → reason `LeafExpiring`.
5. CN drift: `cert.Subject.CommonName != spec.CommonName` → `LeafCNChanged`.
6. Org drift: compare `cert.Subject.Organization` (element set) to
   `[]string{spec.Organization}` → `LeafOrgChanged`.
7. SAN deltas: order-insensitive set diff of `cert.DNSNames` vs `spec.DNSNames`
   → `SANsAdded`/`SANsRemoved`. IP deltas: `parseIPs(spec)` (error propagates)
   vs `cert.IPAddresses` (as `net.IP` sets) → `IPsAdded`/`IPsRemoved`.
8. Reason precedence (first match wins): Missing > Expiring > CNChanged >
   OrgChanged > SANsChanged (SANsAdded/Removed non-empty) > IPsChanged
   (IPsAdded/Removed non-empty) > LeafNone. **All delta slices populated
   regardless of Reason.**

### 4.4 Remove `NeedRenewal`

Delete `NeedRenewal`. Keep `certsNeedRenewal`, `parsePEMCerts`, `pemEncode`,
`marshalECPrivateKey`, `curveFor`, `parseIPs` (unexported; reused).

---

## 5. `pkg/controller/certificate/selfmanaged/pernode/backend.go` (CREATE, NEW)

```go
// Package pernode provides a TLSBackend that keeps one certificate per node in
// a single Secret (multi-cert transport TLS, e.g. Elasticsearch transport).
package pernode

const (
	NodeCertSuffix = ".crt"
	NodeKeySuffix  = ".key"
)

// NodeSpecProvider supplies per-node certificate parameters. Operator-implemented.
type NodeSpecProvider[T object.MultiPhaseObject] interface {
	// ExpectedNodeNames returns the node names that should have a certificate.
	ExpectedNodeNames(o T) ([]string, error)
	// NodeCertSpec returns the CN/DNS/IP SANs for one node's certificate.
	NodeCertSpec(o T, nodeName string) (cn string, dnsNames []string, ips []string, err error)
}

// PerNodeBackend emits a CA Secret (<secretName>-ca) plus a single Opaque leaf
// Secret (<secretName>) with ca.crt and one <node>.crt/<node>.key pair per node.
type PerNodeBackend[T object.MultiPhaseObject] struct {
	provider NodeSpecProvider[T]
}

func NewPerNodeBackend[T object.MultiPhaseObject](provider NodeSpecProvider[T]) *PerNodeBackend[T]

// TLSBackend:
func (b *PerNodeBackend[T]) DesiredObjects(ctx context.Context, o T, spec certificate.TLSSpec) ([]client.Object, error)
func (b *PerNodeBackend[T]) CertificateSecretName(o T, spec certificate.TLSSpec) string // spec.SecretName
func (b *PerNodeBackend[T]) RequiresRotationSaga() bool                                 // true

// NodeSetTLSBackend:
func (b *PerNodeBackend[T]) ExpectedNodeNames(o T) ([]string, error)                    // delegate provider
func (b *PerNodeBackend[T]) NodeSecretKeys() (certSuffix, keySuffix string)             // (NodeCertSuffix, NodeKeySuffix)

// LeafManager:
func (b *PerNodeBackend[T]) DesiredLeafWithCA(ctx context.Context, o T, spec certificate.TLSSpec, caSecret *corev1.Secret) (*corev1.Secret, error)
func (b *PerNodeBackend[T]) LeafNeedsChange(ctx context.Context, o T, leafSecret *corev1.Secret, spec certificate.TLSSpec, now time.Time) (certificate.LeafChange, error)
```

**`DesiredObjects`**: `caSecret, caKey, caCert := selfmanaged.BuildCA(namespace,
spec.SecretName, spec)`. For each node from `provider.ExpectedNodeNames(o)`,
call `provider.NodeCertSpec`, build a node-specific `certificate.TLSSpec` (copy
of `spec` with `CommonName`/`DNSNames`/`IPAddresses` overridden), then
`selfmanaged.SignLeaf(caCert, caKey, nodeSpec)`. Leaf Secret (name
`spec.SecretName`, `Type: SecretTypeOpaque`): `Data["ca.crt"] = caSecret's
ca.crt`, plus `Data[nodeName+NodeCertSuffix]` and `Data[nodeName+NodeKeySuffix]`
for each node. Return `[]client.Object{caSecret, leafSecret}`. Error paths: no
nodes, or any provider/sign error.

**`DesiredLeafWithCA`**: `caCert, caKey := selfmanaged.ParseCA(caSecret)` (error
if `ca.key` missing). Re-issue **all** expected nodes' certs with the existing
CA (same node-loop as above), leaf `ca.crt = caSecret.Data[CAKey]`. Return the
Opaque leaf Secret.

**`LeafNeedsChange`**:
1. `leafSecret == nil` → `{Reason: LeafMissing}`.
2. `expected := provider.ExpectedNodeNames(o)`.
3. Existing node set = keys in `leafSecret.Data` matching `name+certSuffix`
   (prefix before `NodeCertSuffix`). `NodesAdded = expected - existing`;
   `NodesRemoved = existing - expected`.
4. For each existing node cert PEM (parse first cert; malformed → error), check
   expiry (→ `LeafExpiring`), CN vs `NodeCertSpec(...).cn`, Org vs `spec.Organization`.
5. Reason precedence: Missing > Expiring > CNChanged > OrgChanged >
   NodesChanged (NodesAdded/Removed non-empty) > LeafNone. `NodesAdded`/
   `NodesRemoved` always populated.

---

## 6. `pkg/controller/certificate/certmanager/backend.go` (MODIFY)

- `buildLeafCertificate`: replace `if spec.ValidityDays > 0 { ... ValidityDays*24 }`
  with `if spec.LeafValidityDays > 0 { specMap["duration"] = fmt.Sprintf("%dh", spec.LeafValidityDays*24) }`.
- `buildLeafCertificateWithIssuer`: **add** the same leaf-duration block (behavior
  fix — existing-CA mode previously omitted duration, so cert-manager used its
  90-day default).
- `buildCACertificate`: **add** `specMap["duration"] = fmt.Sprintf("%dh",
  certificate.GetValidCADays(spec)*24)` to the CA Certificate spec (new behavior;
  CA now tracks GetValidCADays instead of cert-manager's default). Flag in docs
  release notes.

No change to `byo/backend.go` (only reads `SecretName`).

---

## 7. `pkg/controller/certificate/rotation/rotation.go` (MODIFY)

### 7.1 Struct + options + constructor

```go
type tlsStep[T object.MultiPhaseObject] struct {
	*workflow.DefaultWorkflowStepReconcilerActionWithDiff[T, client.Object]
	backend              certificate.TLSBackend[T]
	provider             certificate.TLSSpecProvider[T]
	convergenceCheck     ConvergenceCheck[T]
	labelsDecorator      LabelsDecorator[T]
	annotationsDecorator AnnotationsDecorator[T]
	forceAllAnnotation   string
	forceLeafAnnotation  string
}
```

```go
// WithForceRegenerateAllAnnotation overrides the force-all annotation name.
func WithForceRegenerateAllAnnotation[T object.MultiPhaseObject](name string) Option[T] {
	return func(s *tlsStep[T]) { s.forceAllAnnotation = name }
}

// WithForceRegenerateLeafAnnotation overrides the force-leaf annotation name.
func WithForceRegenerateLeafAnnotation[T object.MultiPhaseObject](name string) Option[T] {
	return func(s *tlsStep[T]) { s.forceLeafAnnotation = name }
}
```

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
) workflow.WorkflowStepReconcilerActionWithDiff[T, client.Object] {
	s := &tlsStep[T]{
		DefaultWorkflowStepReconcilerActionWithDiff: workflow.NewWorkflowStepReconcilerActionWithDiff[T, client.Object](
			c, phaseName, conditionName, recorder, fieldManager,
		).(*workflow.DefaultWorkflowStepReconcilerActionWithDiff[T, client.Object]),
		backend:             backend,
		provider:            provider,
		forceAllAnnotation:  certificate.AnnotationForceRegenerateAll,
		forceLeafAnnotation: certificate.AnnotationForceRegenerateLeaf,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}
```

(Existing `WithConvergenceCheck`, `WithLabelsDecorator`, `WithAnnotationsDecorator`
unchanged. `specBuilder` field replaced by `provider`.)

### 7.2 Helpers

```go
func (s *tlsStep[T]) signalKey() string { return "tls." + s.GetPhaseName().String() }

// forceFlags reads the two force annotations (== "true"). force-all wins.
func (s *tlsStep[T]) forceFlags(o T) (forceAll, forceLeaf bool) {
	ann := o.GetAnnotations()
	if ann == nil {
		return false, false
	}
	forceAll = ann[s.forceAllAnnotation] == "true"
	forceLeaf = ann[s.forceLeafAnnotation] == "true"
	if forceAll && forceLeaf {
		forceLeaf = false
	}
	return forceAll, forceLeaf
}

// removeForceAnnotations deletes the honored annotation(s) and persists via
// client.Update. No requeue: the STS step still runs in this cycle.
func (s *tlsStep[T]) removeForceAnnotations(ctx context.Context, o T, forceAll, forceLeaf bool) error {
	ann := o.GetAnnotations()
	if len(ann) == 0 {
		return nil
	}
	if forceAll {
		delete(ann, s.forceAllAnnotation)
		delete(ann, s.forceLeafAnnotation) // force-all wins: clear both
	} else if forceLeaf {
		delete(ann, s.forceLeafAnnotation)
	} else {
		return nil
	}
	return s.Client().Update(ctx, o)
}
```

### 7.3 `Read` — phase `""` 3-way branch (rewrite)

Non-saga and Rotate/Converge/default branches are structurally unchanged except
for signal publishing (below). The phase `""` branch becomes:

```go
case "":
	now := time.Now()

	// 1. Force annotations (honored at phase "" only).
	forceAll, forceLeaf := s.forceFlags(o)

	// 2. CA need.
	caNeed := !caExists || forceAll
	if caExists && !forceAll {
		need, err := selfmanaged.CANeedsRenewal(currentCA, spec, now)
		if err != nil {
			return nil, reconcile.Result{}, err
		}
		caNeed = need
	}

	// 3. Leaf change (drift/expiry). currentLeaf is nil when !leafExists.
	leafChg := certificate.LeafChange{}
	if lm, ok := s.backend.(certificate.LeafManager[T]); ok {
		leafChg, err = lm.LeafNeedsChange(ctx, o, leafArg, spec, now)
		if err != nil {
			return nil, reconcile.Result{}, err
		}
	} else if !leafExists {
		leafChg = certificate.LeafChange{Reason: certificate.LeafMissing}
	}

	// 4. force-leaf overrides drift; fall back to full saga if CA missing.
	if forceLeaf && !forceAll {
		leafChg = certificate.LeafChange{Reason: certificate.LeafForceRegen}
	}
	if forceLeaf && caNeed {
		forceAll = true
		forceLeaf = false
	}

	switch {
	case caNeed:
		// FULL CA SAGA (unchanged bundle logic) + signals.
		objs, err := s.backend.DesiredObjects(ctx, o, spec)
		if err != nil { return nil, reconcile.Result{}, err }
		newCA, newLeaf, err := splitCAAndLeaf(objs, caName, leafName)
		if err != nil { return nil, reconcile.Result{}, err }
		if leafExists {
			if oldCA, ok := currentLeaf.Data[selfmanaged.CAKey]; ok && len(oldCA) > 0 {
				bundled := append([]byte{}, newCA.Data[selfmanaged.CAKey]...)
				bundled = append(bundled, oldCA...)
				newLeaf.Data[selfmanaged.CAKey] = bundled
			}
		}
		s.decorate(o, newCA); s.decorate(o, newLeaf)
		read.AddExpectedObject(newCA); read.AddExpectedObject(newLeaf)
		addCurrentObjects(read, currentCA, currentLeaf, caExists, leafExists)
		data["rotationRenewed"] = true
		data[s.signalKey()] = &certificate.LayerSignals{
			CARotated: true, LeafRegenerated: true, LeafChange: &leafChg, Forced: forceAll,
		}

	case !leafChg.IsZero():
		// LEAF-ONLY (single-leaf OR per-node). No phase writes.
		if lm, ok := s.backend.(certificate.LeafManager[T]); ok {
			newLeaf, err := lm.DesiredLeafWithCA(ctx, o, spec, currentCA)
			if err != nil { return nil, reconcile.Result{}, err }
			s.decorate(o, newLeaf)
			read.AddExpectedObject(newLeaf)
			read.AddCurrentObject(currentLeaf)
			read.AddExpectedObject(currentCA)
			read.AddCurrentObject(currentCA)
			data[s.signalKey()] = &certificate.LayerSignals{
				LeafRegenerated: true, LeafChange: &leafChg, Forced: forceLeaf,
			}
		} else {
			// Saga backend without LeafManager: fall back to full CA saga.
			// (Reuse the caNeed body above via a small helper or inline.)
			// ... identical to the caNeed branch ...
		}

	default:
		// STEADY (unchanged, incl. stale-bundle recovery to Rotate).
		if leafExists && caExists && staleBundle(currentLeaf, currentCA) {
			if _, err := s.AdvancePhase(ctx, o, PhaseRotate, logger); err != nil {
				return nil, reconcile.Result{}, fmt.Errorf("advance phase to %q: %w", PhaseRotate, err)
			}
		}
		addStableObjects(read, currentCA, currentLeaf, caExists, leafExists)
	}
```

`leafArg` = `currentLeaf` when `leafExists`, else `nil`.

**Data publishing (top of `Read`, after loading secrets):**

```go
_, isPerNode := s.backend.(certificate.NodeSetTLSBackend[T])
if leafExists && !isPerNode {
	data["tlsSecret"] = sanitizedSecret(currentLeaf, selfmanaged.KeyKey)
	if c := firstCert(currentLeaf.Data[selfmanaged.CertKey]); c != nil {
		data["leafCert"] = c
	}
}
if caExists {
	data["caSecret"] = sanitizedSecret(currentCA, selfmanaged.CAKeyPrivate)
	if c := firstCert(currentCA.Data[selfmanaged.CAKey]); c != nil {
		data["caCert"] = c
	}
}
```

(For per-node, `tlsSecret`/`leafCert` are omitted — see §0.7.)

**Non-saga branch (before phase switch) — force-unsupported handling:**

```go
if !s.backend.RequiresRotationSaga() {
	forceAll, forceLeaf := s.forceFlags(o)
	if forceAll || forceLeaf {
		s.Recorder().Event(o, corev1.EventTypeWarning, "TLSForceUnsupported",
			"force-regenerate annotations are not supported by this TLS backend; they were removed")
		if err := s.removeForceAnnotations(ctx, o, forceAll, forceLeaf); err != nil {
			return nil, reconcile.Result{}, err
		}
	}
	if err := s.emitDesired(ctx, o, spec, read); err != nil {
		return nil, reconcile.Result{}, err
	}
	return read, reconcile.Result{}, nil
}
```

(No `LayerSignals` published for non-saga — `ShouldRollout` sees nothing → false.)

### 7.4 `OnSuccess` — phase advance + force cleanup

```go
func (s *tlsStep[T]) OnSuccess(ctx context.Context, o T, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (reconcile.Result, error) {
	res, err := s.DefaultWorkflowStepReconcilerActionWithDiff.OnSuccess(ctx, o, data, diff, logger)
	if err != nil {
		return res, err
	}
	if !s.backend.RequiresRotationSaga() {
		return res, nil // non-saga force removal already happened in Read
	}

	// Remove honored force annotations (only reached after a successful Apply).
	if sig, _ := data[s.signalKey()].(*certificate.LayerSignals); sig != nil && sig.Forced {
		forceAll, forceLeaf := s.forceFlags(o) // annotations still present on o
		if forceAll || forceLeaf {
			if err := s.removeForceAnnotations(ctx, o, forceAll, forceLeaf); err != nil {
				return res, fmt.Errorf("remove force-regenerate annotation: %w", err)
			}
		}
	}

	startPhase, _ := data["rotationStartPhase"].(apworkflow.WorkflowPhase)
	switch startPhase {
	case "":
		if renewed, _ := data["rotationRenewed"].(bool); renewed {
			if _, err := s.AdvancePhase(ctx, o, PhaseRotate, logger); err != nil {
				return res, fmt.Errorf("advance phase to %q: %w", PhaseRotate, err)
			}
		}
	case PhaseConverge:
		if _, err := s.AdvancePhase(ctx, o, "", logger); err != nil {
			return res, fmt.Errorf("advance phase to empty: %w", err)
		}
	}
	return res, nil
}
```

`OnDiff` unchanged. `OnError` is inherited (annotation stays on Apply failure →
next cycle retries).

---

## 8. Behavioral specification (reference)

### 8.1 `ShouldRollout` truth table (`sig.Forced` short-circuits first)

| policy | CARotated | LeafRegenerated | LeafChange.Reason | result |
|---|---|---|---|---|
| any | * | * | any | `Forced` → **true** |
| Never | * | * | any | **false** |
| Always | true/— | true | any | **true** (CA or leaf regen) |
| Always | false | false | any | **false** |
| OnCAChange | true | * | any | **true** |
| OnCAChange | false | * | any | **false** |
| OnAdditive | true | * | any | **true** |
| OnAdditive | false | true | Expiring/CNChanged/OrgChanged/Missing/Forced | **true** |
| OnAdditive | false | true | SANsChanged/IPsChanged | **len(SANsAdded)>0 \|\| len(IPsAdded)>0** |
| OnAdditive | false | true | NodesChanged | **false** |
| OnAdditive | false | true | LeafNone | **false** |
| OnAdditive | false | * | nil | **false** |
| nil sig | — | — | — | **false** |

STS aggregation: OR `ShouldRollout(policy, sig)` across all `data["tls.<phase>"]`
layers present, then `RolloutAnnotation(rollout, secret, currentHash)`.

### 8.2 Force lifecycle (one-shot)

- Read at phase `""` only. `Rotate`/`Converge` ignore force annotations (left in
  place; honored once the saga returns to `""`).
- `forceAll && forceLeaf` → `forceLeaf=false` (force-all wins).
- `forceLeaf && caNeed` → `forceAll=true, forceLeaf=false` (fallback to full saga).
- `caNeed = !caExists || forceAll || CANeedsRenewal(currentCA, spec, now)`.
- `forceLeaf` (CA healthy) → leaf-only regen, `LayerSignals{LeafRegenerated, Forced}`.
- Removal in `OnSuccess` (saga) via `client.Update`, no requeue. Apply failure →
  `OnError`, annotation stays, retry.
- Non-saga → `TLSForceUnsupported` warning Event + annotation removal in `Read`,
  no regen, `Forced` not set.

### 8.3 Signal publication

Each saga TLS step publishes exactly one `*certificate.LayerSignals` under
`data["tls.<phaseName>"]` **only when a change occurs at phase `""`** (CA saga,
leaf-only, or force). Steady/Rotate/Converge publish no signal (→ no rollout).
`phaseName` doubles as the namespace so transport + API steps never collide.

---

## 9. Tests (exact names + assertions)

### 9.1 `backend_test.go` (MODIFY)
- Replace `TestGetValidTLSDays*` (4 tests) with:
  - `TestGetValidLeafDaysDefault` (365), `TestGetValidLeafDaysCustom` (90),
    `TestGetValidLeafDaysZero` (365), `TestGetValidLeafDaysNegative` (365).
  - `TestGetValidCADaysDefault` (`TLSSpec{LeafValidityDays: 0}` → 730),
    `TestGetValidCADaysCustom` (e.g. `CAValidityDays: 500` → 500),
    `TestGetValidCADaysZero` (`LeafValidityDays: 90, CAValidityDays: 0` → 180),
    `TestGetValidCADaysNegative` (`CAValidityDays: -1` → 2×leaf).
- `TestGetValidRenewalDays*` and `TestCurveConstants` unchanged.

### 9.2 `rollout_test.go` (MODIFY — add)
- `TestShouldRolloutNilSignal` → false.
- `TestShouldRolloutForcedOverridesNever` → `{Forced:true}` + `RolloutNever` → true.
- `TestShouldRolloutAlways` — CA-only, leaf-only, neither.
- `TestShouldRolloutOnCAChange` — CA→true, leaf-only→false.
- `TestShouldRolloutNever` — CA→false, Forced→true.
- `TestShouldRolloutOnAdditiveMatrix` — sub-cases: CARotated; Expiring;
  CNChanged; OrgChanged; Missing; LeafForceRegen; SANsAdded (rollout);
  SANsRemoved-only (no rollout); IPsAdded (rollout); IPsRemoved-only (no);
  SANsAdded+Removed same cycle (rollout, additive); NodesChanged-only (no);
  LeafNone (no); nil LeafChange (no).
- `TestRolloutAnnotationRollout` → fresh hash == `SecretHashAnnotation(secret)`.
- `TestRolloutAnnotationKeep` (`shouldRollout=false, currentHash="abc"`) →
  `{AnnotationSecretHash:"abc"}`.
- `TestRolloutAnnotationKeepButEmptyInitializes` (`currentHash=""`) → fresh hash.
- `TestForceAnnotationConstants` — assert the two constant strings.
- `TestLayerSignalsStruct` — trivial field assertions (optional; for coverage).

### 9.3 `selfmanaged/backend_test.go` (MODIFY)
- Update `ValidityDays` → `LeafValidityDays` in existing specs (lines 61, 344).
- Delete all 11 `TestNeedRenewal_*` tests.
- Add `TestBuildCASecret` — CA name `<name>-ca`, has `ca.crt`/`ca.key`, optional
  `ca.crl`; CA NotAfter ≈ now + `GetValidCADays`.
- Add `TestSignLeaf` — leaf CN/DNS/Org match spec; validity `GetValidLeafDays`;
  signed by CA (verify with `x509.Certificate.CheckSignatureFrom`).
- Add `TestParseCA` — round-trip a `BuildCA` secret; missing `ca.key` → error;
  malformed → error.
- Add `TestCANeedsRenewal_*` (mirror old expiry tests, CA-only): nil secret→true;
  missing `ca.crt`→true; empty→true; malformed→error; expired→true;
  within-window→true; outside-window→false; `RenewalDays` overflow clamped→true.
- Add `TestDesiredLeafWithCA` — build CA via `DesiredObjects`, call
  `DesiredLeafWithCA` with the CA secret; assert leaf `ca.crt` == CA `ca.crt`,
  new `tls.key` differs from old, leaf cert `CheckSignatureFrom(caCert)`, no
  bundle. Missing `ca.key` → error.
- Add `TestLeafNeedsChange_*`: `NilSecret`→Missing; `MissingTLSCrt`→Missing;
  `MalformedPEM`→error; `NoChange`→IsZero; `Expiring`→LeafExpiring;
  `CNChanged`→LeafCNChanged; `OrgChanged`→LeafOrgChanged; `SANAdded` (SANsAdded
  populated, Reason LeafSANsChanged); `SANRemoved` (SANsRemoved, Reason
  LeafSANsChanged); `SANAddAndRemove` (both, Reason LeafSANsChanged); `IPAdded`/
  `IPRemoved` (IPsAdded/IPsRemoved, Reason LeafIPsChanged); `MultiPEM` (uses
  first cert); `ExpiringDominatesSANRemoval` (Expiring reason, SANsRemoved also
  populated).

### 9.4 `selfmanaged/pernode/backend_test.go` (CREATE)
Use a fake object type mirroring `testSelfManagedObject` (embeds ObjectMeta,
`GetStatus() MultiPhaseObjectStatus`, `DeepCopyObject()`). A stub
`NodeSpecProvider` returning fixed `ExpectedNodeNames`/`NodeCertSpec`.
- `TestNewPerNodeBackend` → non-nil.
- `TestPerNodeDesiredObjects` — 2 objects; CA secret name `<name>-ca`; leaf secret
  `Type: Opaque`, has `ca.crt` + `<node>.crt`/`<node>.key` for each expected node.
- `TestPerNodeDesiredLeafWithCA` — re-sign all nodes against an existing CA
  (build CA first via `selfmanaged.BuildCA`); `ca.crt` == CA `ca.crt`; all node
  certs `CheckSignatureFrom(caCert)`; new keys differ from prior. Missing `ca.key`
  → error.
- `TestPerNodeLeafNeedsChange_*`: `NilSecret`→Missing; `NodeAdded` (NodesAdded,
  Reason NodesChanged); `NodeRemoved` (NodesRemoved); `NodeAddAndRemove` (both);
  `NoChange` (IsZero); `NodeExpiring` (LeafExpiring); `NodeCNOrgDrift`
  (LeafCNChanged/LeafOrgChanged); `MissingSecret`→Missing.
- `TestPerNodeExpectedNodeNames` / `TestPerNodeNodeSecretKeys` — delegate + suffixes.
- `TestPerNodeRequiresRotationSaga` → true; `TestPerNodeCertificateSecretName` → spec.SecretName.

### 9.5 `certmanager/backend_test.go` (MODIFY)
- Add `TestCertManagerBackendLeafValidityDays` — `LeafValidityDays: 90` →
  `spec.duration == "2160h"` in **both** dedicated and existing-CA modes.
- Add `TestCertManagerBackendCADuration` — `CAValidityDays`/default → CA
  Certificate `spec.duration == fmt.Sprintf("%dh", GetValidCADays*24)`.
- Add `TestCertManagerBackendLeafValidityAbsentByDefault` — `LeafValidityDays: 0`
  → `spec.duration` absent (preserve current `> 0` guard).

### 9.6 `rotation/rotation_test.go` (MODIFY)
- Change `testSpecBuilder` usage: `newStep` passes
  `certificate.TLSSpecProviderFunc[*rotObject](testSpecBuilder)`.
- Update `testSpecBuilder` to set `LeafValidityDays: 365` (not `ValidityDays`).
- Add `TestRead_Saga_CANeedsRenewalError` (replace `TestRead_Saga_NeedRenewalError`):
  CA secret with malformed `ca.crt` → error.
- Add leaf-only single-leaf tests: `TestRead_LeafOnly_SANAdd` (CA healthy + SAN
  add → leaf regenerated, expected objects = {newLeaf, currentCA}, NO
  `rotationRenewed`, phase stays "", `data["tls.tls"]` `*LayerSignals` with
  `LeafRegenerated=true, CARotated=false`); `TestRead_LeafOnly_SANRemove`
  (same, `SANsRemoved`, `ShouldRollout(OnAdditive)=false`).
- Add per-node step tests (stub `PerNodeBackend` via a fake `NodeSetTLSBackend`
  or the real pernode backend + a stub provider): `TestRead_PerNode_NodeAdded`
  (`NodesAdded`, no phase write, `ShouldRollout(OnAdditive)=false`);
  `TestRead_PerNode_NodeRemoved`; `TestRead_PerNode_Expiring` (leaf-only regen
  all nodes, `ShouldRollout=true`).
- Force tests: `TestRead_ForceAll_FullSaga` (phase "" → full saga, `Forced=true`,
  `CARotated=true`, `rotationRenewed=true`); `TestRead_ForceLeaf_LeafOnly` (CA
  healthy → leaf-only, `Forced=true`, no phase write); `TestRead_ForceLeaf_MissingCA_FallsBackToSaga`
  (`forceAll` path, both); `TestRead_ForceAllWins_OverForceLeaf`;
  `TestRead_ForceIgnored_MidSaga` (phase Rotate + force annotation → ignored,
  annotation left); `TestRead_NonSaga_ForceUnsupported` (byo + force →
  `TLSForceUnsupported` Event recorded on `record.FakeRecorder`, annotation
  removed, no regen); custom annotation names via `WithForceRegenerateAllAnnotation`.
- OnSuccess force removal: `TestOnSuccess_ForceAll_RemovesAnnotations` (data with
  `sig.Forced=true`, `rotationRenewed=true`; assert phase→Rotate and annotation
  removed from fake-client `rotObject`); `TestOnSuccess_ForceLeaf_RemovesAnnotation`
  (no phase advance, annotation removed). **Requires the scheme variant below.**
- Namespaced signals: `TestRead_TwoSteps_NamespacedSignals` (two steps with
  `phaseName` "transport"/"api" write `tls.transport`/`tls.api` without colliding;
  the flat `rotationStartPhase`/`rotationRenewed` are transient per-step).

**Test scheme helper for force-removal tests** (add to `rotation_test.go`):

```go
var rotGV = schema.GroupVersion{Group: "test.operator-sdk-extra", Version: "v1"}

func newFakeClientWithRot(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	scheme.AddKnownTypes(rotGV, &rotObject{})
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}
```

Seed the rotObject via `WithObjects(o)` (and set
`o.TypeMeta = metav1.TypeMeta{APIVersion: rotGV.String(), Kind: "RotObject"}`)
so `client.Update` resolves the GVK. The fake client refreshes `resourceVersion`
on Update, matching production behavior.

### 9.7 `rotation_internal_test.go`
No changes required (`splitCAAndLeaf`/`firstCert` untouched). Optionally add a
white-box `TestRemoveForceAnnotations` (package `rotation`) asserting annotation
deletion + `client.Update` invocation using `newFakeClientWithRot`.

---

## 10. Error handling (summary)

| Path | Return vs swallow | Event | Retry |
|---|---|---|---|
| leaf/CA Secret `Get` non-NotFound error | return error | via inherited `OnError` | yes |
| `LeafNeedsChange` malformed cert | return error | inherited | yes |
| `CANeedsRenewal` malformed CA | return error | inherited | yes |
| `DesiredObjects`/`DesiredLeafWithCA` error (curve/IP/key) | return error | inherited | yes |
| `splitCAAndLeaf` missing CA/leaf | return error | inherited | yes |
| `removeForceAnnotations` Update error | return error | inherited | yes |
| Apply failure (SSA) | inherited `OnError`; annotation stays | `ReconcilerStepActionError` | yes (force retried) |
| Non-saga force annotation | **swallow** (warning only) | `TLSForceUnsupported` Warning | n/a (no regen) |
| `ShouldRollout(nil)` | returns false (no error) | — | — |

`client.Update` after successful Apply does **not** requeue (STS step runs in the
same cycle via the shared `data` map; see §0.11).

---

## 11. Migration / renaming impacts (callers + tests)

All in-repo callers are inside `pkg/controller/certificate/**` (verified: no
`cmd/`/`samples/` usage). No CRD/`zz_generated.deepcopy.go` impact (see §0.1).

| Rename/removal | Callers/tests to update |
|---|---|
| `ValidityDays` → `LeafValidityDays` (+ `CAValidityDays`) | `selfmanaged/backend.go`, `certmanager/backend.go`, `backend_test.go`, `selfmanaged/backend_test.go` (lines 61, 344), `rotation_test.go` (`testSpecBuilder`) |
| `GetValidTLSDays` → `GetValidLeafDays`/`GetValidCADays` | `selfmanaged/backend.go`, `backend_test.go` |
| remove `SelfSigned`/`CertManager` | `backend.go` only (no code callers); docs update |
| `NewTLSStep` `specBuilder` → `provider TLSSpecProvider[T]` | `rotation_test.go` (`newStep`), docs |
| remove `NeedRenewal` | `selfmanaged/backend.go`, `selfmanaged/backend_test.go` (11 tests), `rotation.go`, `rotation_test.go` (`TestRead_Saga_NeedRenewalError`) |
| `documentations/tls-and-workflow.md` | TLSSpec table, provider interface, `NeedRenewal`→`CANeedsRenewal`+`LeafManager`, force annotations, per-node, RolloutPolicy + LayerSignals, migration notes |

---

## 12. Validation (exact commands)

```bash
# Lint
dagger call -m golang --src . lint

# Vulnerability check
dagger call -m golang --src . vulncheck

# Certificate package tests (failures only)
dagger call --src . test --withGotestsum --path ./pkg/controller/certificate/ \
  2>&1 | grep -E "(FAIL|build failed|\.go:[0-9]+:|panic|--- FAIL)" | head -200

# Specific new test (example)
dagger call --src . test --withGotestsum --run "TestShouldRolloutOnAdditiveMatrix"

# Coverage
dagger call --src . test --withGotestsum export --path cover.out
go tool cover -func cover.out

# Full CI (format -> lint -> vulncheck -> tests -> manifests)
dagger call --src . ci
```

Coverage expectation: 100% on `pkg/` (CONTRIBUTING.md). Every new branch in
`ShouldRollout`, the `Read` 3-way split, force lifecycle, and per-node paths must
have a test.

---

## 13. Risks / open items for the coder

- **Double-rotation on force-removal failure (low).** If `OnSuccess`'s annotation
  removal `client.Update` keeps failing through an entire CA saga, the next `""`
  cycle re-honors the force. Mid-saga phases ignore the annotation, so this is
  bounded; document it and rely on the standard reconcile retry. Do NOT move
  removal into `Read` (that would drop Apply-failure retry).
- **`client.Update` resourceVersion refresh** (verified for controller-runtime
  v0.19.3 + fake client). If the outer operator wraps the client in a way that
  does not refresh `resourceVersion`, the deferred `Status().Update` may conflict;
  standard practice is to tolerate `apierrors.IsConflict` on status update.
- **cert-manager duration changes** (`CAValidityDays`, existing-CA leaf duration)
  are behavior changes; call them out in `documentations/tls-and-workflow.md`
  migration notes.
- **Flat `data` keys (`rotationStartPhase`/`rotationRenewed`/`tlsSecret`/`caSecret`)
  remain flat.** They are safe because each step consumes them within its own
  lifecycle (steps run sequentially sharing `data`). Cross-step consumers (STS)
  must use the namespaced `data["tls.<phaseName>"]` LayerSignals only.
- **Per-node private-key leak:** never publish `tlsSecret`/`leafCert` for
  `NodeSetTLSBackend` steps (see §0.7); keep `caSecret`/`caCert`.
- **elasticsearch-operator migration** (referenced in the `.kilo` plan) is a
  **separate repo** (`/projects/elasticsearch-operator`), not present here. The
  migration notes stay in `documentations/tls-and-workflow.md` as guidance only;
  no code in this repo changes for it.

---

## 14. Ordered task list (file-by-file)

1. `pkg/controller/certificate/backend.go` — slim `TLSSpec`; add
   `TLSSpecProvider`/`TLSSpecProviderFunc`; add `GetValidLeafDays`/`GetValidCADays`;
   remove `GetValidTLSDays`; add `LeafChange`/`LeafChangeReason`/`IsZero`;
   add `LeafManager`/`NodeSetTLSBackend`. Add imports.
2. `pkg/controller/certificate/rollout.go` — add force-annotation constants,
   `LayerSignals`, `RolloutPolicy` (+consts), `ShouldRollout`, `RolloutAnnotation`.
3. `pkg/controller/certificate/selfmanaged/backend.go` — extract `BuildCA`/
   `SignLeaf`/`ParseCA`/`CANeedsRenewal`; refactor `DesiredObjects` (split
   validity); implement `LeafManager` (`DesiredLeafWithCA`, `LeafNeedsChange`);
   remove `NeedRenewal`.
4. `pkg/controller/certificate/selfmanaged/pernode/backend.go` (NEW) —
   `NodeSpecProvider`, `PerNodeBackend`, constants, all methods.
5. `pkg/controller/certificate/certmanager/backend.go` — leaf validity rename,
   existing-CA leaf duration, CA duration.
6. `pkg/controller/certificate/rotation/rotation.go` — `provider` field + options
   + constructor; rewrite `Read` phase `""` (3-way + force + signals); per-node
   data-publishing omit; non-saga force handling; `OnSuccess` force cleanup +
   phase advance; `signalKey`/`forceFlags`/`removeForceAnnotations` helpers.
7. Tests (files 8–14 in §1) — update renames + add all §9 tests.
8. `documentations/tls-and-workflow.md` — update TLSSpec table, provider
   interface, regeneration model (CA saga vs leaf-only), per-node section,
   RolloutPolicy + LayerSignals, force annotations, migration notes.
9. Run §12 validation; iterate until lint/vuln/test/coverage pass.
