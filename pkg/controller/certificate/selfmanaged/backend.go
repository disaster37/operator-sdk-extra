// Package selfmanaged provides a TLSBackend that generates and manages
// CA and leaf certificates using Go's crypto/x509 standard library.
// It supports multi-cycle CA rotation via the WorkflowStep saga.
package selfmanaged

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// CASecretSuffix is appended to the SecretName for the CA certificate Secret.
	CASecretSuffix = "-ca"

	// CAKey is the key in the Secret data for the CA certificate PEM.
	CAKey = "ca.crt"

	// CertKey is the key in the Secret data for the leaf certificate PEM.
	CertKey = "tls.crt"

	// KeyKey is the key in the Secret data for the private key PEM.
	KeyKey = "tls.key"

	// CAKeyPrivate is the key for the CA private key PEM (kept in a separate Secret).
	CAKeyPrivate = "ca.key"

	// CRLKey is the key in the CA Secret data for the DER-encoded CRL.
	CRLKey = "ca.crl"
)

// SelfManagedBackend is a TLSBackend that generates CA and leaf certificates
// using Go's crypto/x509 standard library.
type SelfManagedBackend[T object.MultiPhaseObject] struct{}

// NewSelfManagedBackend creates a new self-managed backend.
func NewSelfManagedBackend[T object.MultiPhaseObject]() *SelfManagedBackend[T] {
	return &SelfManagedBackend[T]{}
}

// DesiredObjects generates CA and leaf certificates and returns them as
// Kubernetes Secret objects for SSA reconciliation.
func (b *SelfManagedBackend[T]) DesiredObjects(ctx context.Context, o T, spec certificate.TLSSpec) ([]client.Object, error) {
	validity := certificate.GetValidTLSDays(spec)

	curve, err := curveFor(spec)
	if err != nil {
		return nil, err
	}

	caKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{spec.Organization},
			CommonName:   fmt.Sprintf("%s-ca", spec.CommonName),
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(time.Duration(validity*2) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	leafKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate leaf key: %w", err)
	}

	ips, err := parseIPs(spec)
	if err != nil {
		return nil, err
	}

	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{spec.Organization},
			CommonName:   spec.CommonName,
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(time.Duration(validity) * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		DNSNames:    spec.DNSNames,
		IPAddresses: ips,
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caTemplate, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create leaf certificate: %w", err)
	}

	var crlDER []byte
	if spec.GenerateCRL {
		parsedCA, parseErr := x509.ParseCertificate(caDER)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse CA certificate for CRL: %w", parseErr)
		}
		now := time.Now()
		// Derive NextUpdate from the parsed CA's NotAfter rather than
		// recomputing `validity*2 * 24 * time.Hour`. This both expresses the
		// intent ("CRL valid for the CA's lifetime") and avoids a separate
		// time.Duration overflow path when ValidityDays is very large.
		crlTemplate := &x509.RevocationList{
			Issuer:     parsedCA.Subject,
			ThisUpdate: now,
			NextUpdate: parsedCA.NotAfter,
			Number:     big.NewInt(1),
			// RevokedCertificateEntries left nil (empty list).
		}
		crlDER, err = x509.CreateRevocationList(rand.Reader, crlTemplate, parsedCA, caKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create CRL: %w", err)
		}
	}

	namespace := o.GetNamespace()

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.SecretName + CASecretSuffix,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			CAKey:        pemEncode("CERTIFICATE", caDER),
			CAKeyPrivate: pemEncode("EC PRIVATE KEY", marshalECPrivateKey(caKey)),
		},
	}
	if crlDER != nil {
		caSecret.Data[CRLKey] = crlDER
	}

	leafSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.SecretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			CertKey: pemEncode("CERTIFICATE", leafDER),
			KeyKey:  pemEncode("EC PRIVATE KEY", marshalECPrivateKey(leafKey)),
			CAKey:   pemEncode("CERTIFICATE", caDER),
		},
	}

	return []client.Object{caSecret, leafSecret}, nil
}

// CertificateSecretName returns the name of the leaf certificate Secret.
func (b *SelfManagedBackend[T]) CertificateSecretName(o T, spec certificate.TLSSpec) string {
	return spec.SecretName
}

// RequiresRotationSaga returns true for the self-managed backend.
func (b *SelfManagedBackend[T]) RequiresRotationSaga() bool {
	return true
}

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

// NeedRenewal reports whether the certificate(s) in secret should be renewed
// at now, given spec's renewal window.
//   - nil secret → true (not yet provisioned).
//   - tls.crt absent or empty → true (not ready).
//   - tls.crt present but unparseable → error.
//   - ca.crt, if present, is also checked; absent ca.crt is skipped (no trigger),
//     malformed ca.crt → error.
//
// Renewal is triggered when now is within GetValidRenewalDays(spec) of any
// parsed cert's NotAfter (i.e. now > NotAfter - window), which also covers
// already-expired certs.
func NeedRenewal(secret *corev1.Secret, spec certificate.TLSSpec, now time.Time) (bool, error) {
	if secret == nil {
		return true, nil
	}
	window := time.Duration(certificate.GetValidRenewalDays(spec)) * 24 * time.Hour

	// tls.crt: absent/empty → true; malformed → error.
	raw, ok := secret.Data[CertKey]
	if !ok || len(raw) == 0 {
		return true, nil
	}
	need, err := certsNeedRenewal(raw, now, window)
	if err != nil {
		return false, err
	}
	if need {
		return true, nil
	}

	// ca.crt: optional. Present → check; absent → skip.
	if raw, ok = secret.Data[CAKey]; ok && len(raw) > 0 {
		need, err = certsNeedRenewal(raw, now, window)
		if err != nil {
			return false, err
		}
		if need {
			return true, nil
		}
	}
	return false, nil
}

// certsNeedRenewal parses PEM-encoded certs from pemBytes and reports whether
// any of them is within window of expiry (or already expired).
func certsNeedRenewal(pemBytes []byte, now time.Time, window time.Duration) (bool, error) {
	certs, err := parsePEMCerts(pemBytes)
	if err != nil {
		return false, err
	}
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
		if block == nil {
			break
		}
		rest = r
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificate: %w", err)
		}
		certs = append(certs, c)
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates found in PEM data")
	}
	return certs, nil
}

func pemEncode(blockType string, derBytes []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  blockType,
		Bytes: derBytes,
	})
}

func marshalECPrivateKey(key *ecdsa.PrivateKey) []byte {
	b, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal EC key: %v", err))
	}
	return b
}
