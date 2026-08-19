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
package certificate

import (
	"context"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
