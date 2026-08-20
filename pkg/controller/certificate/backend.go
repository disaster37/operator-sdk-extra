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

	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
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
)

// TLSSpec defines the desired TLS configuration for a component.
// Operators embed this in their CRD spec and pass it to the TLSBackend.
type TLSSpec struct {
	// SecretName is the name of the Secret that will hold the certificate
	// and key. Consumers mount this Secret or hash it for rollout.
	SecretName string `json:"secretName,omitempty"`

	// SelfSigned, when true, enables the self-managed CA backend.
	// When false (default), the BYO backend is used.
	SelfSigned bool `json:"selfSigned,omitempty"`

	// CertManager, when true, enables the cert-manager backend.
	// Mutually exclusive with SelfSigned.
	CertManager bool `json:"certManager,omitempty"`

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

	// ValidityDays is the certificate validity in days.
	// Defaults to 365 if not set.
	ValidityDays int `json:"validityDays,omitempty"`

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

// GetValidTLSDays returns the certificate validity in days, defaulting to 365.
func GetValidTLSDays(spec TLSSpec) int {
	if spec.ValidityDays <= 0 {
		return 365
	}
	return spec.ValidityDays
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
