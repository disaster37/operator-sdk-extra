# operator-sdk-extra: generic certificate-content customization for the existing TLS saga

**Target repository:** `/projects/operator-sdk-extra` (Go module `github.com/disaster37/operator-sdk-extra/v3`, Go 1.26).

**Goal:** give downstream operators a **generic, app-agnostic** way to customize the *content* of certificates produced by the **already-implemented** `pkg/controller/certificate/**` saga — subject fields (organization, organizational units, country, locality, …), common name, DNS SANs, IP SANs, key algorithm/size, key/extended-key usages, and validity — **without reimplementing certificate generation in each operator**. A secondary, optional, **generic** client-side TLS-config helper (`*tls.Config`/`*http.Transport` builder) is included but framed app-agnostically (any remote API), with **no** Elasticsearch/OpenSearch/Kibana-specific code or sections.

This plan is self-contained: a coder can implement it without reading any other document.

> **Scope ruling (resolved from rejected feedback):** the library stays **generic**. No dedicated code for Elasticsearch / OpenSearch / Kibana. The feature is a *global concept*: (1) extend the existing `TLSSpec` into a richer declarative certificate-content contract; (2) add an optional `CertificateCustomizer` compute hook with sane defaults; (3) make every existing backend (`selfmanaged`, `certmanager`, `byo`, `pernode`) and the `rotation` saga honor that content. The previously proposed client-side `pkg/tlsconfig` idea is retained **only** as a secondary, generic helper.

---

## 1. Grounding facts (verified against the repo)

- The certificate saga lives in `pkg/controller/certificate/**` and is **already generic** (no ES/OS/Kibana references):
  - `backend.go` — `TLSSpec` (plain struct, json tags), `TLSBackend[T]`, `TLSSpecProvider[T]` + `TLSSpecProviderFunc[T]`, `LeafManager[T]`, `NodeSetTLSBackend[T]`, `LeafChange`/`LeafChangeReason`, `GetValidLeafDays`/`GetValidCADays`/`GetValidRenewalDays`, curve/validity constants.
  - `selfmanaged/backend.go` — the only place that *generates* certs (Go `crypto/x509` + `crypto/ecdsa`): `BuildCA` (CA subject = `Organization: []string{spec.Organization}`, `CommonName: fmt.Sprintf("%s-ca", spec.CommonName)`, `KeyUsage: CertSign|CRLSign`), `SignLeaf` (leaf subject `Organization`/`CommonName`, `KeyUsage: DigitalSignature|KeyEncipherment`, `ExtKeyUsage: ServerAuth|ClientAuth`, `DNSNames`, `IPAddresses`), `ParseCA`, `CANeedsRenewal`, `DesiredObjects`, `DesiredLeafWithCA`, `LeafNeedsChange`.
  - `selfmanaged/pernode/backend.go` — per-node certs via `NodeSpecProvider[T]`; calls `selfmanaged.BuildCA`/`SignLeaf`.
  - `certmanager/backend.go` — emits `unstructured` Issuer/Certificate CRs; `buildCACertificate` **hardcodes** `subject.organizations: ["operator-sdk-extra"]`; `buildLeafCertificate(WithIssuer)` + `setCommonCertificateSpec` map `commonName`, `dnsNames`, `ipAddresses`, `renewBefore`, `duration`.
  - `byo/backend.go` — validates `SecretName` non-empty, emits nothing (no content).
  - `rotation/rotation.go` — `NewTLSStep(...)` saga step; `Read()` computes `spec := s.provider.TLSSpec(o)` once and threads it to every backend call; options via `With*`.
  - `rollout.go` — `ShouldRollout(policy, sig)` maps `LeafChangeReason` → rollout decision.
- **Key seams (exact insertion points for customization):**
  1. `TLSSpec` (the content contract) — `pkg/controller/certificate/backend.go:62-105`.
  2. CA subject/usage generation — `selfmanaged/backend.go:74-85` (`BuildCA`).
  3. Leaf subject/usage/SAN/key generation — `selfmanaged/backend.go:135-179` (`SignLeaf`), key curve via `curveFor` (`:354-365`).
  4. cert-manager CA subject — `certmanager/backend.go:117-138` (`buildCACertificate`, hardcoded org).
  5. cert-manager leaf spec mapping — `certmanager/backend.go:140-206` (`buildLeafCertificate`, `buildLeafCertificateWithIssuer`, `setCommonCertificateSpec`).
  6. Saga spec computation — `rotation/rotation.go:172` (`spec := s.provider.TLSSpec(o)`).
  7. Drift/rollout detection — `selfmanaged/backend.go:296-341` + `pernode/backend.go:97-162` (`LeafNeedsChange`), `rollout.go:47-78` (`ShouldRollout`).
  8. CA-renewal gating — `rotation/rotation.go:242-249` + `selfmanaged.CANeedsRenewal` (`:223-233`).
- **Downstream glue to reduce** (measured): `samples/elasticsearch-operator/controllers/helper.go` (`GetElasticsearchHandler`, ~87 lines hand-rolling `http.Transport` + `tls.Config` + client config) and `api/v1alpha1/role_types.go` (`ElasticsearchCaSecretRef`, CA-secret-only, no content). The remote reconciler (`pkg/controller/remote`) forces every operator to implement `GetRemoteHandler` and build the client.
- **Conventions** (from `CONTRIBUTING.md` + existing code): `emperror.dev/errors` (`errors.Wrapf`/`errors.Errorf`), no logging in library code, `testify` assert/require, `go.uber.org/mock` (mocks in `pkg/mock/`), controller-runtime fake client for step tests, external test packages (`pkg_test`), **100% coverage target on `pkg/`**, Dagger for all tasks. Library helpers return errors and never log.
- `TLSSpec` is documented as the "computed spec" operators supply via `TLSSpecProvider` (embedding a *slimmer* copy in their CRD and computing values). The customization hook must fit this model.

---

## 2. Design overview — the global concept

Three additive pieces, all in the existing saga:

1. **`TLSSpec` becomes the rich, declarative, json-tagged certificate-content contract** (single source of truth). New fields: a nested `Subject CertificateSubject` RDN block, `KeyAlgorithm`, `KeySize`, `Usages`, `KeyUsages`, `CACommonName`, `CASubject`. Resolver methods (`LeafSubject()`, `CASubject()`, `EffectiveKeyAlgorithm()`, `EffectiveKeySize()`, `EffectiveUsages()`, `EffectiveKeyUsages()`, `ValidateContent()`) centralize defaulting + fallback so backends never re-read raw fields.
2. **`CertificateCustomizer[T]`** — an optional Go hook (`CustomizeCertificate(o T, base TLSSpec) (TLSSpec, error)`) plus `CertificateCustomizerFunc[T]` adapter and an opt-in `DefaultCertificateCustomizer[T]`. Operators implement it **only** to compute content (org/SAN/IP/key/usage) that cannot be declared statically; the library does all generation. Applied once per reconcile in the `rotation` saga (one seam).
3. **Every backend honors the resolved content.** `selfmanaged` generalizes signing to `crypto.Signer` (ECDSA + RSA), `certmanager` maps subject/usages/privateKey to its CR spec, `pernode` inherits, `byo` is unaffected (no generation). `rotation` applies the customizer and detects CA/leaf content drift for regeneration + rollout.

Backward compatibility: zero-value of every new field reproduces today's exact behavior (P-256 ECDSA, `Organization` singular, ServerAuth+ClientAuth, 365/730/30 validities, `-ca` naming). Existing exported signatures are preserved (see §4.2 for the additive generalized variants).

---

## 3. Core package changes — `pkg/controller/certificate/backend.go`

### 3.1 New types & constants

```go
import "crypto/x509/pkix" // added to this package (stdlib only)

// CertificateSubject is the certificate RDN block beyond the legacy single
// CommonName/Organization fields. All fields optional; empty = unset.
type CertificateSubject struct {
	Organizations       []string `json:"organizations,omitempty"`
	OrganizationalUnits []string `json:"organizationalUnits,omitempty"`
	Countries           []string `json:"countries,omitempty"`
	Localities          []string `json:"localities,omitempty"`
	Provinces           []string `json:"provinces,omitempty"`
	StreetAddresses     []string `json:"streetAddresses,omitempty"`
	PostalCodes         []string `json:"postalCodes,omitempty"`
	SerialNumber        string   `json:"serialNumber,omitempty"`
}
```

Constants (usage vocabulary — single string set shared by all backends; mapped per backend):

```go
const (
	KeyAlgorithmECDSA = "ECDSA"
	KeyAlgorithmRSA   = "RSA"

	DefaultECDSAKeySize = 256 // P-256
	DefaultRSAKeySize   = 2048

	// ExtKeyUsage strings (leaf).
	UsageServerAuth      = "serverAuth"
	UsageClientAuth      = "clientAuth"
	UsageCodeSigning     = "codeSigning"
	UsageEmailProtection = "emailProtection"
	UsageTimestamping    = "timestamping"
	UsageOCSPSigning     = "ocspSigning"

	// KeyUsage strings.
	KeyUsageDigitalSignature  = "digitalSignature"
	KeyUsageContentCommitment = "contentCommitment"
	KeyUsageKeyEncipherment   = "keyEncipherment"
	KeyUsageDataEncipherment  = "dataEncipherment"
	KeyUsageKeyAgreement      = "keyAgreement"
	KeyUsageCertSign          = "certSign"
	KeyUsageCRLSign           = "crlSign"
)
```

### 3.2 `TLSSpec` extensions (additive; all `json:"...,omitempty"`)

Append to `TLSSpec` (keep every existing field and json tag untouched):

```go
	// Subject is the extended RDN block. When Subject.Organizations is empty,
	// the legacy Organization field is used (backward compatible).
	Subject CertificateSubject `json:"subject,omitempty"`

	// KeyAlgorithm is ECDSA (default) or RSA.
	KeyAlgorithm string `json:"keyAlgorithm,omitempty"`

	// KeySize is the key size in bits. 0 = default per algorithm
	// (ECDSA -> 256/P-256; RSA -> 2048). For ECDSA must be 256/384/521.
	KeySize int `json:"keySize,omitempty"`

	// Usages is the leaf ExtKeyUsage list (see Usage* constants).
	// Empty = [serverAuth, clientAuth].
	Usages []string `json:"usages,omitempty"`

	// KeyUsages is the leaf KeyUsage list (see KeyUsage* constants).
	// Empty = [digitalSignature, keyEncipherment].
	KeyUsages []string `json:"keyUsages,omitempty"`

	// CACommonName overrides the CA certificate CN. Default "<CommonName>-ca".
	CACommonName string `json:"caCommonName,omitempty"`

	// CASubject overrides the CA RDN block. Default = leaf subject with
	// CN = CACommonName.
	CASubject *CertificateSubject `json:"caSubject,omitempty"`
```

New `LeafChangeReason` values (append, keep existing):

```go
	LeafSubjectChanged LeafChangeReason = "SubjectChanged"
	LeafKeyChanged     LeafChangeReason = "KeyChanged"
	LeafUsagesChanged  LeafChangeReason = "UsagesChanged"
```

### 3.3 Resolver methods (single source of truth for defaulting)

```go
// Organizations returns the effective leaf organizations:
// Subject.Organizations if non-empty, else []string{Organization} (dropping "").
func (s TLSSpec) Organizations() []string

// LeafSubject returns the resolved leaf pkix.Name (CN + full RDN).
func (s TLSSpec) LeafSubject() pkix.Name

// CASubject returns the resolved CA pkix.Name. CN = CACommonName (default
// "<CommonName>-ca"); other RDN fields = CASubject if set, else leaf subject.
func (s TLSSpec) CASubject() pkix.Name

// EffectiveKeyAlgorithm returns KeyAlgorithm or KeyAlgorithmECDSA (default).
func (s TLSSpec) EffectiveKeyAlgorithm() string

// EffectiveKeySize resolves the key size: KeySize if > 0, else the size implied
// by Curve (P-256->256, P-384->384, P-521->521), else the algorithm default.
// Returns an error if KeySize and Curve disagree.
func (s TLSSpec) EffectiveKeySize() (int, error)

// EffectiveUsages returns the leaf ExtKeyUsage strings (defaults applied).
func (s TLSSpec) EffectiveUsages() []string

// EffectiveKeyUsages returns the leaf KeyUsage strings (defaults applied).
func (s TLSSpec) EffectiveKeyUsages() []string

// ValidateContent validates all customizable content fields. Returns the first
// error. Called by backends and by the rotation saga before generation.
func (s TLSSpec) ValidateContent() error
```

`ValidateContent` rules (fail fast):
- `KeyAlgorithm` must be `""` or one of `KeyAlgorithmECDSA`/`KeyAlgorithmRSA`; else error.
- ECDSA: `KeySize` must be 0/256/384/521, and must agree with `Curve` if both set; else error.
- RSA: `KeySize` must be 0 or >= 2048; else error.
- Every entry in `Usages` must be a known `Usage*`; every entry in `KeyUsages` a known `KeyUsage*`; else error (unknown usage).
- DNS entries: reject empty strings (non-empty only); IPs are validated by the existing `net.ParseIP` path in `selfmanaged` (no change).
- `Subject`/`CASubject` fields are free-form (no strict validation).

### 3.4 `CertificateCustomizer` + default

```go
// CertificateCustomizer lets an operator compute certificate content that
// cannot be declared statically (e.g., SANs/IPs from cluster state). It
// receives the reconciled object and the declared base TLSSpec and returns the
// final content. Return base unchanged to keep declared defaults. It has no
// ctx by design (matching TLSSpecProvider/NodeSpecProvider); capture a client
// in a closure if cluster reads are needed.
type CertificateCustomizer[T object.MultiPhaseObject] interface {
	CustomizeCertificate(o T, base TLSSpec) (TLSSpec, error)
}

type CertificateCustomizerFunc[T object.MultiPhaseObject] func(o T, base TLSSpec) (TLSSpec, error)

func (f CertificateCustomizerFunc[T]) CustomizeCertificate(o T, base TLSSpec) (TLSSpec, error) {
	return f(o, base)
}

// DefaultCertificateCustomizer returns a customizer that only fills CommonName
// with the object name when empty, and is otherwise the identity. Opt-in: pass
// it explicitly via rotation.WithCertificateCustomizer (no implicit behavior
// change for existing operators).
func DefaultCertificateCustomizer[T object.MultiPhaseObject]() CertificateCustomizer[T]
```

---

## 4. Backend integration

### 4.1 `pkg/controller/certificate/selfmanaged/backend.go`

Generalize the signing core to `crypto.Signer` (supports ECDSA + RSA) while **preserving** the existing exported ECDSA signatures.

New internal helpers (unexported, free to change):

```go
// generateKey returns a fresh private key per spec (ECDSA curve or RSA size).
func generateKey(spec certificate.TLSSpec) (crypto.Signer, error)

// marshalSignerKey returns the PEM block type ("EC PRIVATE KEY" | "RSA PRIVATE KEY")
// and DER for a signer.
func marshalSignerKey(signer crypto.Signer) (string, []byte, error)

// parseSignerFromPEM parses an EC or RSA private-key PEM block.
func parseSignerFromPEM(block *pem.Block) (crypto.Signer, error)

// keyUsageFor maps KeyUsage* strings to x509.KeyUsage.
func keyUsageFor(names []string) (x509.KeyUsage, error)

// extKeyUsageFor maps Usage* strings to x509.ExtKeyUsage.
func extKeyUsageFor(names []string) ([]x509.ExtKeyUsage, error)
```

New exported generalized functions (additive):

```go
func BuildCASigner(namespace, secretName string, spec certificate.TLSSpec) (*corev1.Secret, crypto.Signer, *x509.Certificate, error)
func SignLeafSigner(caCert *x509.Certificate, caSigner crypto.Signer, spec certificate.TLSSpec) (leafCertPEM, leafKeyPEM []byte, err error)
func ParseCASigner(caSecret *corev1.Secret) (*x509.Certificate, crypto.Signer, error)
```

Existing exported functions become thin ECDSA wrappers (unchanged signatures):

```go
func BuildCA(namespace, secretName string, spec certificate.TLSSpec) (*corev1.Secret, *ecdsa.PrivateKey, *x509.Certificate, error)
//   = BuildCASigner(...) then type-assert *ecdsa.PrivateKey (error if spec requests RSA).
func SignLeaf(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, spec certificate.TLSSpec) ([]byte, []byte, error)
//   = SignLeafSigner(caCert, caKey, spec).
func ParseCA(caSecret *corev1.Secret) (*x509.Certificate, *ecdsa.PrivateKey, error)
//   = ParseCASigner(...) then type-assert.
```

Behavior changes inside `BuildCASigner`/`SignLeafSigner`:
- Call `spec.ValidateContent()` first (fail fast).
- CA template subject = `spec.CASubject()`; leaf subject = `spec.LeafSubject()`.
- Key algorithm/size from `spec.EffectiveKeyAlgorithm()`/`EffectiveKeySize()`; `generateKey` replaces `curveFor`.
- Leaf `KeyUsage`/`ExtKeyUsage` from `spec.EffectiveKeyUsages()`/`spec.EffectiveUsages()` via `keyUsageFor`/`extKeyUsageFor` (defaults reproduce today's `DigitalSignature|KeyEncipherment` + `ServerAuth|ClientAuth`).
- CA `KeyUsage` stays `CertSign|CRLSign` (fixed; not customizable).
- CA Secret `ca.key` block type is now `EC PRIVATE KEY` **or** `RSA PRIVATE KEY` (from `marshalSignerKey`). Leaf `tls.key` likewise.

Update `DesiredObjects`/`DesiredLeafWithCA` to call the `*Signer` variants (so RSA works end-to-end). Add:

```go
// CAContentChanged reports whether the CA cert's subject or public-key
// algorithm differs from spec (used to trigger a CA saga on content change,
// not just expiry).
func CAContentChanged(caSecret *corev1.Secret, spec certificate.TLSSpec) (bool, error)
```

Extend `LeafNeedsChange` (§4.4) to compare full subject (beyond CN/Org), key algorithm/size, and usages against the resolved spec.

### 4.2 `pkg/controller/certificate/selfmanaged/pernode/backend.go`

- `buildLeafSecret` and `DesiredLeafWithCA` switch to `selfmanaged.ParseCASigner`/`SignLeafSigner` (no ECDSA-only assumption).
- `LeafNeedsChange` comparison extended (same subject/key/usage checks as single-leaf; per-node CN/Org already handled) — see §4.4.

### 4.3 `pkg/controller/certificate/certmanager/backend.go`

- `buildCACertificate`: replace hardcoded `subject.organizations: ["operator-sdk-extra"]` and `commonName: name+"-ca"` with `spec.CASubject()` (map organizations/organizationalUnits/countries/localities/provinces/streetAddresses/postalCodes/serialNumber). Keep `isCA: true`, `duration` from `GetValidCADays`.
- `buildLeafCertificate`/`buildLeafCertificateWithIssuer` + `setCommonCertificateSpec`:
  - `commonName` from `spec.LeafSubject().CommonName` (unchanged source).
  - `subject` block from `spec.LeafSubject()` (organizations + OUs + countries + localities + provinces + streetAddresses + postalCodes + serialNumber) — omit empty slices (preserve current "absent when empty" behavior).
  - `usages` (ExtKeyUsage) from `spec.EffectiveUsages()` mapped to cert-manager strings (e.g. `"server auth"`, `"client auth"`; a `certManagerUsageFor(name string) (string, bool)` helper).
  - `privateKey: {algorithm: ECDSA|RSA, size: N}` from `spec.EffectiveKeyAlgorithm()`/`EffectiveKeySize()`.
  - Keep `dnsNames`/`ipAddresses`/`renewBefore`/`duration` as today.
- `byo`: no change (no generation); `DesiredObjects` still only validates `SecretName`.

### 4.4 Drift detection & rollout — `LeafNeedsChange` + `ShouldRollout`

Extend `selfmanaged.LeafNeedsChange` and `pernode.LeafNeedsChange` precedence (first match wins) to:

```
expiring  -> LeafExpiring
CN differ -> LeafCNChanged
Org differ-> LeafOrgChanged            (use spec.Organizations() set-equality)
other RDN differ -> LeafSubjectChanged (compare spec.LeafSubject() vs cert.Subject minus CN/Org)
SANs differ -> LeafSANsChanged
IPs differ  -> LeafIPsChanged
key algorithm/size differ -> LeafKeyChanged   (cert.PublicKeyAlgorithm / bit size vs spec)
usages differ -> LeafUsagesChanged            (cert.KeyUsage/ExtKeyUsage vs spec)
else LeafNone
```

`ShouldRollout` (`rollout.go`): add `LeafSubjectChanged`, `LeafKeyChanged`, `LeafUsagesChanged` to the "rollout true" set (same bucket as `LeafCNChanged`/`LeafOrgChanged`/`LeafExpiring`/`LeafMissing`/`LeafForceRegen`). `LeafSANsChanged`/`LeafIPsChanged` keep their additive-only semantics.

### 4.5 Rotation saga — `rotation/rotation.go`

- Add option:

```go
// WithCertificateCustomizer injects the content customizer. Applied once per
// Read cycle; the customized spec is threaded to every backend call and to
// drift/renewal checks in that cycle.
func WithCertificateCustomizer[T object.MultiPhaseObject](c certificate.CertificateCustomizer[T]) Option[T]
```

- Add `customizer certificate.CertificateCustomizer[T]` to `tlsStep`; wire in `NewTLSStep`.
- In `Read()`, replace `spec := s.provider.TLSSpec(o)` (line 172) with:

```go
spec, err := s.computeSpec(ctx, o)
if err != nil { return nil, reconcile.Result{}, err }

func (s *tlsStep[T]) computeSpec(ctx context.Context, o T) (certificate.TLSSpec, error) {
	spec := s.provider.TLSSpec(o)
	if s.customizer != nil {
		var err error
		spec, err = s.customizer.CustomizeCertificate(o, spec)
		if err != nil { return certificate.TLSSpec{}, err }
	}
	if err := spec.ValidateContent(); err != nil { return certificate.TLSSpec{}, err }
	return spec, nil
}
```

- CA-need gating (lines 242-249): OR in content drift so a CA subject/key change triggers a full CA saga, not just expiry:

```go
caNeed := !caExists || forceAll
if caExists && !forceAll {
	if need, err := selfmanaged.CANeedsRenewal(currentCA, spec, now); err != nil { ... }
	else { caNeed = need }
	if !caNeed {
		changed, err := selfmanaged.CAContentChanged(currentCA, spec)
		if err != nil { ... }
		caNeed = changed
	}
}
```

---

## 5. Minimal-glue consumption story

### BEFORE (today's glue — per operator)

```go
// 1) CRD spec: only a CA secret ref (no content).
type ElasticsearchRef struct {
	ElasticsearchCaSecretRef *corev1.LocalObjectReference `json:"elasticsearchCASecretRef,omitempty"`
}

// 2) A TLSSpecProvider that hand-computes content.
func tlsSpec(o *MyCRD) certificate.TLSSpec {
	return certificate.TLSSpec{SecretName: o.Name + "-tls", CommonName: o.Name,
		DNSNames: []string{o.Name + "." + o.Namespace + ".svc"}, Organization: "my-org"}
}

// 3) Hand-rolled client transport (client-side, ~87 lines today).
func GetHandler(ctx, o, ...) { transport := &http.Transport{TLSClientConfig: &tls.Config{}}; ... }
```

### AFTER (minimal glue)

```go
// 1) CRD spec embeds the declarative content contract directly (json-tagged).
type MyCRDSpec struct {
	TLS certificate.TLSSpec `json:"tls,omitempty"` // secretName, subject, dnsNames, ipAddresses, keyAlgorithm, keySize, usages, ...
}

// 2) One embedded struct + one optional customizer (compute-only), no generation code.
step := rotation.NewTLSStep[*MyCRD](
	r.Client(), "tls", "TLSCertificatesReady", r.Recorder, "my-operator",
	selfmanaged.NewSelfManagedBackend[*MyCRD](),
	certificate.TLSSpecProviderFunc[*MyCRD](func(o *MyCRD) certificate.TLSSpec { return o.Spec.TLS }),
	rotation.WithCertificateCustomizer(
		certificate.CertificateCustomizerFunc[*MyCRD](func(o *MyCRD, base certificate.TLSSpec) (certificate.TLSSpec, error) {
			if len(base.DNSNames) == 0 {
				base.DNSNames = []string{o.Name + "." + o.Namespace + ".svc"}
			}
			return base, nil // library generates everything
		}),
	),
	rotation.WithConvergenceCheck(...),
)
```

Operators that can declare content statically skip the customizer entirely. Operators that only need the CN default use `certificate.DefaultCertificateCustomizer[*MyCRD]()`.

---

## 6. Secondary, optional, generic client-side TLS helper (`pkg/tlsconfig`)

Retained **only** as a generic, app-agnostic helper (secondary). It has **no** client-library imports and **no** ES/OS/Kibana sections.

- `pkg/tlsconfig` (stdlib + `emperror.dev/errors` only): serializable `CertificateOptions` (CA sources, optional client cert + key, `InsecureSkipVerify`, `ServerName`, `MinVersion`/`MaxVersion`), `Validate`, `Resolve` (path/inline only), `BuildTLSConfig`, `BuildHTTPTransport`, `WithBaseTLSConfig`/`WithBaseTransport`. Zero value → `(nil, nil)` (no customization).
- `pkg/tlsconfig/k8s` (imports controller-runtime `client.Client`): resolves Secret/ConfigMap sources; `Resolve`/`New`/`BuildHTTPTransport` with `(ctx, c, namespace, opts, options...)`.
- **Generic wiring note only**: any operator builds its HTTP client by passing the produced `*tls.Config`/`*http.Transport` into its client library's transport field (or CA bytes/`InsecureSkipVerify` where the client only supports those). No specific client library is named.

This component is a straight port of the previous plan's §3–§6 with all client-specific §9 content removed and wording generalized. It is fully additive and optional.

---

## 7. Edge cases (encode as tests)

| Case | Behavior |
|---|---|
| Zero-value new fields | Exact current behavior (P-256, `Organization` singular, ServerAuth+ClientAuth, 365/730/30, `<cn>-ca`). Existing tests must pass unchanged. |
| `Subject.Organizations` + legacy `Organization` both set | `Subject.Organizations` wins; `Organization` ignored. |
| Only `Subject.Organizations` set | Leaf org = that list; legacy `Organization` untouched. |
| Only `KeySize` set (ECDSA 384) | P-384; `Curve` empty. |
| `KeySize` + `Curve` disagree | `ValidateContent` error (fail fast). |
| `KeyAlgorithm=RSA` + `KeySize=0` | RSA 2048. |
| `KeyAlgorithm=RSA` + `KeySize=1024` | `ValidateContent` error (RSA must be >= 2048). |
| Unknown `KeyAlgorithm` / usage string | `ValidateContent` error. |
| `Usages`/`KeyUsages` empty | Defaults (leaf: ServerAuth+ClientAuth / DigitalSignature+KeyEncipherment). |
| Empty string in `DNSNames` | `ValidateContent` error. |
| Invalid IP SAN | existing `parseIPs` error (unchanged). |
| Duplicate SANs/IPs | Deduped (existing `stringSetDiff`/`ipSetDiff` semantics preserved for drift; generation emits as-given or deduped — dedupe in resolvers). |
| `CACommonName` empty | Default `<CommonName>-ca`. |
| `CASubject` set | CA RDN = CASubject; CN = `CACommonName`. |
| Customizer returns zero `TLSSpec` | Treated literally (subject/CN empty); library still generates a valid cert (matches today's empty-CN behavior). |
| Customizer returns error | `Read` returns it; reconciler OnError path (requeue/fail per operator). |
| CA subject/key change (not expiry) | `CAContentChanged` true → full CA saga (bundle + Rotate/Converge). |
| Leaf subject/key/usage change | `LeafNeedsChange` returns the new reason → leaf-only regen (or CA saga if CA also changed). |
| Rotation with nil customizer | Identical to today (backward compatible). |
| RSA cert round-trip | `ParseCASigner` parses `RSA PRIVATE KEY`; `ca.key`/`tls.key` emit `RSA PRIVATE KEY`. |
| cert-manager `Subject` empty | `spec.subject` absent (preserves "absent when empty" test). |
| cert-manager RSA | `spec.privateKey = {algorithm: RSA, size: 2048}`. |
| `byo` with content fields | Ignored (no generation); only `SecretName` validated. |

---

## 8. Error handling & validation policy

- **Library returns errors; never logs, never panics.** The saga step surfaces errors through the reconciler's `OnError` (existing path); it does not log.
- **Wrapping:** `emperror.dev/errors` — `errors.Wrapf(err, "...")` at each failure site; new validation errors via `errors.Errorf(...)`. Add exported sentinels only where programmatic checks are expected: `ErrUnsupportedKeyAlgorithm`, `ErrInvalidKeySize`, `ErrUnknownUsage` (wrapped with context at the failing site so `errors.Cause` still finds them).
- **Fail fast on misconfiguration** (bad algorithm/size/usage/empty DNS) in `ValidateContent`, called by backends and by the saga before generation. No silent fallback.
- **Defaults are data-level, not error-level:** empty/zero values fall through to documented defaults (preserves backward compatibility); only *invalid* values error.
- **Customizer errors propagate** (operator's compute logic owns that error; the library wraps with `"customize certificate content"` context).
- Transient vs permanent mirrors repo convention: content/validation errors are permanent (fail, no requeue); Secret NotFound stays transient (existing behavior unchanged).

---

## 9. Backward compatibility & migration

- **Entirely additive.** No existing exported field, constant, or signature is removed or renamed. `BuildCA`/`SignLeaf`/`ParseCA` keep their exact signatures (thin ECDSA wrappers); generalized `*Signer` variants are **new**.
- Zero-value of all new `TLSSpec` fields reproduces today's byte-equivalent certificate content (verified by keeping all existing `selfmanaged`/`certmanager`/`pernode`/`rotation` tests green).
- `certmanager` CA certificate loses the hardcoded `organizations: ["operator-sdk-extra"]` — this is a **bug fix** (CA org now follows `spec`). Default remains a single org `"operator-sdk-extra"` **only if** the operator sets `Organization: "operator-sdk-extra"`; otherwise it is omitted/empty. Note this explicitly in the changelog/doc; no operator depends on the hardcoded string being correct.
- Downstream operators regenerate deepcopy/CRD manifests after adding `TLSSpec` fields (standard kubebuilder/operator-sdk flow) — not this repo's concern.

---

## 10. Do / Do not (shared-library notes)

**Do:**
- Keep everything in `pkg/controller/certificate/**` (reuse the saga; no new cert-generation path).
- Keep `pkg/tlsconfig` free of controller-runtime and any client-library imports (stdlib + `emperror.dev/errors` only); keep the k8s resolver in a separate sub-package (mirrors the `certmanager` optional-subpackage isolation pattern).
- Keep `TLSSpec` the single declarative content contract (json-tagged, embeddable in CRDs).
- Centralize defaulting/fallback in `TLSSpec` resolver methods so backends never re-read raw fields.
- Return errors; never log; use `emperror.dev/errors`; fail fast on invalid values, default on empty.
- Preserve existing exported signatures; add generalized variants additively.
- Test 100% coverage on `pkg/` and keep all existing certificate tests green.

**Do not:**
- Do **not** add Elasticsearch/OpenSearch/Kibana-specific types, packages, or doc sections.
- Do **not** import `es-handler`, `go-kibana-rest`, `go-elasticsearch`, or opensearch into `pkg/`.
- Do **not** reimplement certificate generation in downstream operators (the whole point is to reuse this saga).
- Do **not** remove or repurpose the existing `TLSSpec` fields, backend interfaces, or `NewTLSStep` signature.
- Do **not** auto-apply `DefaultCertificateCustomizer` (opt-in only — preserves strict backward compatibility).
- Do **not** change CA key usage away from `CertSign|CRLSign`, nor change Secret naming/annotations (`<name>-ca`, `-ca-issuer`, `ca.crt`/`tls.crt`/`tls.key`/`ca.key`).
- Do **not** log inside `pkg/tlsconfig` (return errors; callers log).

---

## 11. Test strategy (per file, colocated `*_test.go`, external package, testify)

### 11.1 `pkg/controller/certificate/backend_test.go` (extend)
- `TestTLSSpecOrganizations_*`: Subject.Organizations set; only legacy Organization; both (precedence); both empty.
- `TestTLSSpecLeafSubject_*`: full RDN mapping (O/OU/C/L/ST/street/postal/serial) → `pkix.Name`; empty → CN only.
- `TestTLSSpecCASubject_*`: default CN `<cn>-ca`; explicit `CACommonName`; explicit `CASubject`.
- `TestTLSSpecEffectiveKeyAlgorithm_*`: empty→ECDSA; ECDSA; RSA.
- `TestTLSSpecEffectiveKeySize_*`: ECDSA defaults (256/384/521) from KeySize; from Curve; KeySize+Curve conflict error; RSA default 2048; RSA <2048 error.
- `TestTLSSpecEffectiveUsages_*` / `TestTLSSpecEffectiveKeyUsages_*`: defaults; custom lists.
- `TestTLSSpecValidateContent_*`: unknown algorithm; bad ECDSA size; bad RSA size; unknown usage; empty DNS string; valid all-fields; zero-value (no error).
- `TestDefaultCertificateCustomizer_*`: fills CN when empty; leaves non-empty CN; identity otherwise.

### 11.2 `pkg/controller/certificate/selfmanaged/backend_test.go` (extend)
- `TestBuildCASigner_*`: ECDSA default (asserts `ca.crt` subject CN/org + `EC PRIVATE KEY`); RSA (asserts `RSA PRIVATE KEY` + `PublicKeyAlgorithm == RSA`); custom `CASubject`/`CACommonName`; invalid content → error.
- `TestSignLeafSigner_*`: custom subject (O/OU/CN), custom `Usages`/`KeyUsages` reflected in parsed leaf (`ExtKeyUsage`/`KeyUsage`); RSA leaf; custom key size; IP/DNS SANs; invalid usage → error.
- `TestBuildCA`/`TestSignLeaf`/`TestParseCA` wrappers: ECDSA path unchanged; `BuildCA` returns error when spec requests RSA.
- `TestParseCASigner_*`: EC key; RSA key; mismatch (public key != cert) → error.
- `TestCAContentChanged_*`: nil/missing ca.crt → true; subject differs → true; algorithm differs → true; identical → false.
- `TestLeafNeedsChange_*`: subject-only change → `LeafSubjectChanged`; key change → `LeafKeyChanged`; usage change → `LeafUsagesChanged`; precedence order.

### 11.3 `pkg/controller/certificate/selfmanaged/pernode/backend_test.go` (extend)
- RSA end-to-end via `DesiredObjects`; subject/key/usage drift via `LeafNeedsChange` (new reasons); per-node CN/Org unchanged.

### 11.4 `pkg/controller/certificate/certmanager/backend_test.go` (extend)
- CA `subject` from `spec.CASubject()` (org/OU/…) and no longer hardcoded; `commonName` = CACommonName.
- Leaf `subject` full RDN present when set, absent when empty.
- `usages` mapping (serverAuth/clientAuth → cert-manager strings).
- `privateKey` = ECDSA/256 and RSA/2048.
- Existing `TestCertManagerBackendSubjectOrganizationsAbsent` still passes (empty subject → absent).

### 11.5 `pkg/controller/certificate/rollout_test.go` (extend)
- `ShouldRollout` returns true for `LeafSubjectChanged`/`LeafKeyChanged`/`LeafUsagesChanged` under `RolloutOnAdditive`; existing policies unchanged.

### 11.6 `pkg/controller/certificate/rotation/rotation_test.go` (extend)
- `WithCertificateCustomizer` mutates spec before `DesiredObjects`/`LeafNeedsChange` (assert via a recording customizer).
- Customizer error → `Read` returns error.
- Nil customizer → identical behavior to today.
- CA content change (not expiry) triggers `runCASaga` (`rotationRenewed=true`, Rotate phase).
- `computeSpec` calls `ValidateContent` (bad content → error).

### 11.7 `pkg/tlsconfig/**` (secondary, if implemented)
Port the previous plan's §12 tests (Validate/Resolve/BuildTLSConfig/New/BuildHTTPTransport/Option + k8s Resolve/New) with generic wording; 100% coverage.

---

## 12. Ordered implementation steps (file-by-file, with Definition-of-Done)

1. **`pkg/controller/certificate/backend.go`** (EDIT) — `CertificateSubject`, new `TLSSpec` fields, usage/key-algorithm constants, `LeafChangeReason` additions, resolver methods + `ValidateContent`, `CertificateCustomizer`/`CertificateCustomizerFunc`/`DefaultCertificateCustomizer`. *DoD:* `go build ./pkg/controller/certificate/...`; existing `backend_test.go` green.
2. **`pkg/controller/certificate/selfmanaged/backend.go`** (EDIT) — generalized `crypto.Signer` core, `BuildCASigner`/`SignLeafSigner`/`ParseCASigner`, ECDSA wrappers, `generateKey`/`marshalSignerKey`/`parseSignerFromPEM`/`keyUsageFor`/`extKeyUsageFor`, `CAContentChanged`, extended `LeafNeedsChange`, `DesiredObjects`/`DesiredLeafWithCA` on `*Signer` path. *DoD:* build green.
3. **`pkg/controller/certificate/selfmanaged/pernode/backend.go`** (EDIT) — use `*Signer` variants; extend `LeafNeedsChange`. *DoD:* build green.
4. **`pkg/controller/certificate/certmanager/backend.go`** (EDIT) — subject/usages/privateKey mapping; fix CA subject. *DoD:* build green.
5. **`pkg/controller/certificate/rollout.go`** (EDIT) — new reasons in `ShouldRollout`. *DoD:* build green.
6. **`pkg/controller/certificate/rotation/rotation.go`** (EDIT) — `WithCertificateCustomizer`, `computeSpec`, CA content drift. *DoD:* build green.
7. **Tests** (EDIT `*_test.go` per §11). *DoD:* 100% coverage on `pkg/controller/certificate/...`; all existing tests green.
8. **(Optional, secondary) `pkg/tlsconfig` + `pkg/tlsconfig/k8s`** (CREATE, generic only) + tests. *DoD:* 100% coverage; no client-lib imports in `pkg/`.
9. **Docs** — update `documentations/tls-and-workflow.md` with a "Certificate content customization" section (new `TLSSpec` fields table, `CertificateCustomizer` usage, defaults/precedence, minimal-glue example) and a short generic client-TLS note. *DoD:* doc renders.
10. **Full check** — `dagger call --src . ci`. *DoD:* green; 100% coverage maintained; no new non-stdlib/non-k8s deps in `pkg/controller/certificate`.

---

## 13. Validation commands

```bash
# Build the changed packages
go build ./pkg/controller/certificate/... ./pkg/tlsconfig/...

# Targeted tests (failures only)
dagger call --src . test --withGotestsum --path ./pkg/controller/certificate/ \
  2>&1 | grep -E "(FAIL|build failed|\.go:[0-9]+:|panic|--- FAIL)" | head -200

# Specific tests
dagger call --src . test --withGotestsum --run "TestTLSSpec"
dagger call --src . test --withGotestsum --run "TestSignLeafSigner"
dagger call --src . test --withGotestsum --run "TestCAContentChanged"
dagger call --src . test --withGotestsum --run "TestCertManagerBackend"
dagger call --src . test --withGotestsum --run "WithCertificateCustomizer"

# Coverage (100% target on pkg/)
dagger call --src . test --withGotestsum export --path cover.out
go tool cover -func cover.out | grep -E "certificate|tlsconfig"

# Full CI
dagger call --src . ci
```

---

## 14. Risks & open questions

- **Generalized signing type:** `crypto.Signer` return type on the *new* `*Signer` variants is idiomatic and keeps existing ECDSA signatures intact. The only churn is internal (`pernode` + `selfmanaged` method bodies). No breaking change to `TLSBackend`.
- **cert-manager CA org fix** is a behavior change (drops the hardcoded string). Call it out in docs; default remains empty unless the operator sets an org.
- **`CAContentChanged`** compares subject + algorithm (not key *size* for ECDSA, since the CA public-key size is fixed by the curve). Documented.
- **`pkix.Name` import in `certificate`** is stdlib-only and acceptable; it is the single resolved-subject representation used by both `selfmanaged` and `certmanager`.

**Open question (non-blocking):** whether to also expose an arbitrary `*x509.Certificate` template mutator (full escape hatch). **Recommendation: no** — keep `TLSSpec` + `CertificateCustomizer` as the contract; add an escape hatch only if a concrete operator requirement appears.
