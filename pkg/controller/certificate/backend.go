// Package certificate provides pluggable TLSBackend abstractions for
// certificate management in Kubernetes operators.
//
// It supports three backends:
//   - selfmanaged: generates and rotates CA + leaf certificates using Go's
//     crypto/x509, returning them as Kubernetes Secrets for SSA apply.
//   - byo (bring-your-own): references an existing user-managed Secret,
//     emitting no child objects.
//   - certmanager (optional subpackage): emits cert-manager Issuer/Certificate
//     CRs and relies on cert-manager to produce the Secret.
//
// The selfmanaged backend supports ECDSA curve selection (P-256/P-384/P-521),
// IP SANs, an optional CA CRL, and a renewal window; the reusable rotation
// saga lives in pkg/controller/certificate/rotation.
package certificate

import (
	"context"
	"crypto/x509/pkix"
	"net"
	"slices"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

	// MaxRenewalDays is the largest accepted renewal window. Values above
	// this are clamped down to it. This prevents integer overflow when
	// computing `RenewalDays * 24 * time.Hour` (a time.Duration in int64
	// nanoseconds overflows around 106_752 days, which would produce a
	// negative window and silently disable renewal — or a huge positive
	// window causing perpetual renewal). 100 years is well below the
	// overflow threshold and far exceeds any legitimate cert validity.
	MaxRenewalDays = 36500

	// MaxValidityDays is the largest accepted certificate validity (leaf or
	// CA) in days. It mirrors MaxRenewalDays: `ValidityDays * 24 * time.Hour`
	// (a time.Duration in int64 nanoseconds) overflows around 106_752 days,
	// which would either fail certificate creation or wrap to a small negative
	// (already-expired) NotAfter and trigger a perpetual renewal loop. 100
	// years is safely below the overflow threshold and exceeds any legitimate
	// certificate validity.
	MaxValidityDays = 36500

	// KeyAlgorithmECDSA selects an ECDSA private key (P-256/P-384/P-521).
	KeyAlgorithmECDSA = "ECDSA"
	// KeyAlgorithmRSA selects an RSA private key.
	KeyAlgorithmRSA = "RSA"

	// DefaultECDSAKeySize is the default ECDSA key size in bits (P-256).
	DefaultECDSAKeySize = 256
	// DefaultRSAKeySize is the default RSA key size in bits.
	DefaultRSAKeySize = 2048

	// MaxRSAKeySize is the largest accepted RSA key size in bits. RSA key
	// generation is CPU- and memory-intensive and scales super-linearly with
	// the bit size. An unbounded KeySize would let a misconfiguration (or a
	// tampered CRD in a multi-tenant deployment) drive rsa.GenerateKey to
	// synthesize an enormous key and exhaust the controller's CPU/memory
	// (CWE-400/CWE-770). 8192 is a generous ceiling that covers even very
	// high-assurance deployments while keeping generation bounded.
	MaxRSAKeySize = 8192

	// ExtKeyUsage (leaf) string vocabulary, mapped per backend.
	UsageServerAuth      = "serverAuth"
	UsageClientAuth      = "clientAuth"
	UsageCodeSigning     = "codeSigning"
	UsageEmailProtection = "emailProtection"
	UsageTimestamping    = "timestamping"
	UsageOCSPSigning     = "ocspSigning"

	// KeyUsage string vocabulary, mapped per backend.
	KeyUsageDigitalSignature  = "digitalSignature"
	KeyUsageContentCommitment = "contentCommitment"
	KeyUsageKeyEncipherment   = "keyEncipherment"
	KeyUsageDataEncipherment  = "dataEncipherment"
	KeyUsageKeyAgreement      = "keyAgreement"
	KeyUsageCertSign          = "certSign"
	KeyUsageCRLSign           = "crlSign"
)

// ErrUnsupportedKeyAlgorithm is returned when TLSSpec.KeyAlgorithm is set to an
// unsupported value. It is wrapped with context at each failing site so
// errors.Cause still finds it.
var ErrUnsupportedKeyAlgorithm = errors.Sentinel("unsupported key algorithm")

// ErrInvalidKeySize is returned when TLSSpec.KeySize is invalid for the
// selected key algorithm (or disagrees with TLSSpec.Curve).
var ErrInvalidKeySize = errors.Sentinel("invalid key size")

// ErrUnknownUsage is returned when TLSSpec.Usages or TLSSpec.KeyUsages
// contains an unknown usage string.
var ErrUnknownUsage = errors.Sentinel("unknown usage")

// ErrUnknownCurve is returned when TLSSpec.Curve is set to a value other than
// CurveP256, CurveP384 or CurveP521.
var ErrUnknownCurve = errors.Sentinel("unknown curve")

// CertificateSubject is the certificate RDN block beyond the legacy single
// CommonName/Organization fields. All fields are optional; empty = unset.
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

// pkixName builds a pkix.Name from a CertificateSubject and a common name.
func (s CertificateSubject) pkixName(commonName string) pkix.Name {
	return pkix.Name{
		CommonName:         commonName,
		Organization:       s.Organizations,
		OrganizationalUnit: s.OrganizationalUnits,
		Country:            s.Countries,
		Locality:           s.Localities,
		Province:           s.Provinces,
		StreetAddress:      s.StreetAddresses,
		PostalCode:         s.PostalCodes,
		SerialNumber:       s.SerialNumber,
	}
}

// curveKeySize maps a Curve constant to its ECDSA bit size. The second return
// is false for unknown curve names.
func curveKeySize(curve string) (int, bool) {
	switch curve {
	case "", CurveP256:
		return 256, true
	case CurveP384:
		return 384, true
	case CurveP521:
		return 521, true
	default:
		return 0, false
	}
}

// isKnownUsage reports whether name is a recognized Usage* constant.
func isKnownUsage(name string) bool {
	switch name {
	case UsageServerAuth, UsageClientAuth, UsageCodeSigning, UsageEmailProtection, UsageTimestamping, UsageOCSPSigning:
		return true
	}
	return false
}

// isKnownKeyUsage reports whether name is a recognized KeyUsage* constant.
func isKnownKeyUsage(name string) bool {
	switch name {
	case KeyUsageDigitalSignature, KeyUsageContentCommitment, KeyUsageKeyEncipherment,
		KeyUsageDataEncipherment, KeyUsageKeyAgreement, KeyUsageCertSign, KeyUsageCRLSign:
		return true
	}
	return false
}

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

	// Curve selects the ECDSA curve. One of P-256 (default), P-384, P-521.
	// Unknown values are rejected by ValidateContent. It drives the effective
	// ECDSA key size (EffectiveKeySize) and therefore also cert-manager's
	// privateKey.size.
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
}

// TLSBackend defines the interface for pluggable certificate providers.
//
// Each backend returns the child objects it wants the reconciler to manage
// via SSA, the name of the resulting certificate Secret, and whether a
// multi-cycle CA-rotation saga is needed.
//
//   - selfmanaged backend: returns Secret objects (CA + leaf), needs saga.
//   - cert-manager backend: returns Issuer/Certificate CRs, no saga needed.
//   - BYO backend: returns nothing, no saga needed.
type TLSBackend[T object.MultiPhaseObject] interface {
	// DesiredObjects returns the child objects that the reconciler should
	// manage via SSA. These may be Secrets (self-managed) or cert-manager
	// CRs (cert-manager backend).
	DesiredObjects(ctx context.Context, o T, spec TLSSpec) ([]client.Object, error)

	// CertificateSecretName returns the name of the Secret that consumers
	// should mount or hash for rolling restart.
	CertificateSecretName(o T, spec TLSSpec) string

	// RequiresRotationSaga reports whether the multi-cycle CA-rotation
	// workflow is needed. Only the self-managed mutual-TLS backend returns
	// true; cert-manager and BYO backends return false.
	RequiresRotationSaga() bool
}

// ContentIgnoringBackend is an optional capability for backends that perform no
// certificate generation and therefore ignore TLSSpec content fields (e.g. the
// BYO backend, which only references an existing Secret). The rotation saga
// skips ValidateContent for such backends while still enforcing each backend's
// own structural validation (e.g. a non-empty SecretName).
type ContentIgnoringBackend interface {
	// IgnoresCertificateContent reports whether TLSSpec content fields
	// (subject, SANs, key algorithm/size, usages, validity) are ignored by
	// this backend.
	IgnoresCertificateContent() bool
}

// TLSSpecProvider supplies the computed TLSSpec for an object. The operator
// implements this (or uses TLSSpecProviderFunc) instead of passing a bare
// func to NewTLSStep.
type TLSSpecProvider[T object.MultiPhaseObject] interface {
	TLSSpec(o T) TLSSpec
}

// TLSSpecProviderFunc adapts a func(o T) TLSSpec to TLSSpecProvider.
type TLSSpecProviderFunc[T object.MultiPhaseObject] func(o T) TLSSpec

func (f TLSSpecProviderFunc[T]) TLSSpec(o T) TLSSpec { return f(o) }

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
// changes rotation data publishing (omit tlsSecret/leafCert).
type NodeSetTLSBackend[T object.MultiPhaseObject] interface {
	TLSBackend[T]
	// ExpectedNodeNames returns the node names that should have a certificate.
	ExpectedNodeNames(o T) ([]string, error)
	// NodeSecretKeys returns the Data-key suffixes for a node's cert and key
	// (e.g. ".crt", ".key"); a node's Data keys are name+certSuffix and
	// name+keySuffix.
	NodeSecretKeys() (certSuffix, keySuffix string)
}

// LeafChangeReason is the dominant reason a leaf needs regeneration.
type LeafChangeReason string

const (
	LeafNone           LeafChangeReason = ""
	LeafMissing        LeafChangeReason = "Missing"
	LeafExpiring       LeafChangeReason = "Expiring"
	LeafCNChanged      LeafChangeReason = "CNChanged"
	LeafOrgChanged     LeafChangeReason = "OrgChanged"
	LeafSubjectChanged LeafChangeReason = "SubjectChanged"
	LeafKeyChanged     LeafChangeReason = "KeyChanged"
	LeafUsagesChanged  LeafChangeReason = "UsagesChanged"
	LeafSANsChanged    LeafChangeReason = "SANsChanged"
	LeafIPsChanged     LeafChangeReason = "IPsChanged"
	LeafNodesChanged   LeafChangeReason = "NodesChanged"
	LeafForceRegen     LeafChangeReason = "Forced"
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

// GetValidLeafDays returns the leaf certificate validity in days, defaulting
// to 365 when LeafValidityDays <= 0. Values above MaxValidityDays are clamped
// down to it to prevent time.Duration overflow in downstream
// `LeafValidityDays * 24 * time.Hour` math (see MaxValidityDays).
func GetValidLeafDays(spec TLSSpec) int {
	if spec.LeafValidityDays <= 0 {
		return 365
	}
	if spec.LeafValidityDays > MaxValidityDays {
		return MaxValidityDays
	}
	return spec.LeafValidityDays
}

// GetValidCADays returns the CA certificate validity in days, defaulting to
// 2× GetValidLeafDays when CAValidityDays <= 0. Values above MaxValidityDays
// are clamped down to it (see MaxValidityDays).
func GetValidCADays(spec TLSSpec) int {
	days := spec.CAValidityDays
	if days <= 0 {
		days = 2 * GetValidLeafDays(spec)
	}
	if days > MaxValidityDays {
		return MaxValidityDays
	}
	return days
}

// GetValidRenewalDays returns the renewal window in days, defaulting to
// DefaultRenewalDays (30) when RenewalDays is not set or invalid (<= 0).
// Values above MaxRenewalDays are clamped to MaxRenewalDays to prevent
// time.Duration overflow in downstream `RenewalDays * 24 * time.Hour` math
// (see MaxRenewalDays).
func GetValidRenewalDays(spec TLSSpec) int {
	if spec.RenewalDays <= 0 {
		return DefaultRenewalDays
	}
	if spec.RenewalDays > MaxRenewalDays {
		return MaxRenewalDays
	}
	return spec.RenewalDays
}

// Organizations returns the effective leaf organizations:
// Subject.Organizations if non-empty, else []string{Organization} (dropping "").
func (s TLSSpec) Organizations() []string {
	if len(s.Subject.Organizations) > 0 {
		return slices.Clone(s.Subject.Organizations)
	}
	if s.Organization == "" {
		return nil
	}
	return []string{s.Organization}
}

// LeafSubject returns the resolved leaf pkix.Name (CN + full RDN).
func (s TLSSpec) LeafSubject() pkix.Name {
	name := s.Subject.pkixName(s.CommonName)
	name.Organization = s.Organizations()
	return name
}

// ResolvedCASubject returns the resolved CA pkix.Name. CN = CACommonName
// (default "<CommonName>-ca"); other RDN fields = CASubject if set, else leaf
// subject. It is a method rather than a field because the exported
// `CASubject *CertificateSubject` field already occupies that identifier.
func (s TLSSpec) ResolvedCASubject() pkix.Name {
	cn := s.CACommonName
	if cn == "" {
		cn = s.CommonName + "-ca"
	}
	if s.CASubject != nil {
		return s.CASubject.pkixName(cn)
	}
	name := s.LeafSubject()
	name.CommonName = cn
	return name
}

// EffectiveKeyAlgorithm returns KeyAlgorithm or KeyAlgorithmECDSA (default).
func (s TLSSpec) EffectiveKeyAlgorithm() string {
	if s.KeyAlgorithm == "" {
		return KeyAlgorithmECDSA
	}
	return s.KeyAlgorithm
}

// EffectiveKeySize resolves the key size: KeySize if > 0, else the size implied
// by Curve (P-256->256, P-384->384, P-521->521), else the algorithm default.
// Returns an error if KeySize and Curve disagree.
func (s TLSSpec) EffectiveKeySize() (int, error) {
	if s.KeySize > 0 {
		if err := s.keySizeCurveConflict(); err != nil {
			return 0, err
		}
		return s.KeySize, nil
	}
	if s.EffectiveKeyAlgorithm() == KeyAlgorithmECDSA {
		if size, ok := curveKeySize(s.Curve); ok {
			return size, nil
		}
		return DefaultECDSAKeySize, nil
	}
	return DefaultRSAKeySize, nil
}

// keySizeCurveConflict returns an error when an explicit KeySize disagrees with
// an explicitly selected ECDSA Curve; nil otherwise.
func (s TLSSpec) keySizeCurveConflict() error {
	if s.KeySize == 0 || s.EffectiveKeyAlgorithm() != KeyAlgorithmECDSA || s.Curve == "" {
		return nil
	}
	if size, ok := curveKeySize(s.Curve); ok && size != s.KeySize {
		return errors.Wrapf(ErrInvalidKeySize, "key size %d disagrees with curve %s", s.KeySize, s.Curve)
	}
	return nil
}

// EffectiveUsages returns the leaf ExtKeyUsage strings (defaults applied).
func (s TLSSpec) EffectiveUsages() []string {
	if len(s.Usages) == 0 {
		return []string{UsageServerAuth, UsageClientAuth}
	}
	return slices.Clone(s.Usages)
}

// EffectiveKeyUsages returns the leaf KeyUsage strings (defaults applied).
func (s TLSSpec) EffectiveKeyUsages() []string {
	if len(s.KeyUsages) == 0 {
		return []string{KeyUsageDigitalSignature, KeyUsageKeyEncipherment}
	}
	return slices.Clone(s.KeyUsages)
}

// DedupStrings returns a copy of in with duplicate entries removed, preserving
// the order of first occurrence.
func DedupStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// DedupIPs returns a copy of in with duplicate IPs removed, preserving the
// order of first occurrence. Duplicates are detected by each IP's canonical
// String() form (so "2001:db8::1" and "2001:db8:0:0:0:0:0:1" dedupe).
func DedupIPs(in []net.IP) []net.IP {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]net.IP, 0, len(in))
	for _, ip := range in {
		s := ip.String()
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, ip)
	}
	return out
}

// ValidateContent validates all customizable content fields. Returns the first
// error. Called by backends and by the rotation saga before generation.
func (s TLSSpec) ValidateContent() error {
	switch s.KeyAlgorithm {
	case "", KeyAlgorithmECDSA:
		if s.KeySize != 0 {
			if s.KeySize != 256 && s.KeySize != 384 && s.KeySize != 521 {
				return errors.Wrapf(ErrInvalidKeySize, "invalid ECDSA key size %d: want 256, 384 or 521", s.KeySize)
			}
			if err := s.keySizeCurveConflict(); err != nil {
				return err
			}
		}
		if s.Curve != "" {
			if _, ok := curveKeySize(s.Curve); !ok {
				return errors.Wrapf(ErrUnknownCurve, "unsupported curve %q: want P-256, P-384 or P-521", s.Curve)
			}
		}
	case KeyAlgorithmRSA:
		if s.KeySize != 0 && (s.KeySize < DefaultRSAKeySize || s.KeySize > MaxRSAKeySize) {
			return errors.Wrapf(ErrInvalidKeySize, "invalid RSA key size %d: must be between %d and %d", s.KeySize, DefaultRSAKeySize, MaxRSAKeySize)
		}
	default:
		return errors.Wrapf(ErrUnsupportedKeyAlgorithm, "unsupported key algorithm %q: want %q or %q", s.KeyAlgorithm, KeyAlgorithmECDSA, KeyAlgorithmRSA)
	}

	for _, u := range s.Usages {
		if !isKnownUsage(u) {
			return errors.Wrapf(ErrUnknownUsage, "unknown extended key usage %q", u)
		}
	}
	for _, u := range s.KeyUsages {
		if !isKnownKeyUsage(u) {
			return errors.Wrapf(ErrUnknownUsage, "unknown key usage %q", u)
		}
		if u == KeyUsageCertSign || u == KeyUsageCRLSign {
			return errors.Errorf("leaf key usage %q is reserved for the CA", u)
		}
	}
	for _, d := range s.DNSNames {
		if strings.TrimSpace(d) == "" {
			return errors.Errorf("DNS name must not be empty")
		}
	}
	for _, ip := range s.IPAddresses {
		if net.ParseIP(ip) == nil {
			return errors.Errorf("invalid IP SAN %q", ip)
		}
	}
	return nil
}

// CertificateCustomizer lets an operator compute certificate content that
// cannot be declared statically (e.g., SANs/IPs from cluster state). It
// receives the reconciled object and the declared base TLSSpec and returns the
// final content. Return base unchanged to keep declared defaults. It has no
// ctx by design (matching TLSSpecProvider/NodeSpecProvider); capture a client
// in a closure if cluster reads are needed.
type CertificateCustomizer[T object.MultiPhaseObject] interface {
	CustomizeCertificate(o T, base TLSSpec) (TLSSpec, error)
}

// CertificateCustomizerFunc adapts a func to CertificateCustomizer.
type CertificateCustomizerFunc[T object.MultiPhaseObject] func(o T, base TLSSpec) (TLSSpec, error)

// CustomizeCertificate implements CertificateCustomizer.
func (f CertificateCustomizerFunc[T]) CustomizeCertificate(o T, base TLSSpec) (TLSSpec, error) {
	return f(o, base)
}

// DefaultCertificateCustomizer returns a customizer that only fills CommonName
// with the object name when empty, and is otherwise the identity. Opt-in: pass
// it explicitly via rotation.WithCertificateCustomizer (no implicit behavior
// change for existing operators).
func DefaultCertificateCustomizer[T object.MultiPhaseObject]() CertificateCustomizer[T] {
	return CertificateCustomizerFunc[T](func(o T, base TLSSpec) (TLSSpec, error) {
		if base.CommonName == "" {
			base.CommonName = o.GetName()
		}
		return base, nil
	})
}
