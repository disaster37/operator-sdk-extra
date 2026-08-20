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
	"time"

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
)

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
