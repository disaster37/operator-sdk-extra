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

// BuildCA generates a fresh CA key + self-signed CA certificate and returns
// the CA Secret (name secretName+CASecretSuffix), the CA private key, and the
// parsed CA certificate. When spec.GenerateCRL, ca.crl is added to the Secret.
func BuildCA(namespace, secretName string, spec certificate.TLSSpec) (*corev1.Secret, *ecdsa.PrivateKey, *x509.Certificate, error) {
	curve, err := curveFor(spec)
	if err != nil {
		return nil, nil, nil, err
	}

	caKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate CA key: %w", err)
	}

	serial, err := randomSerialNumber()
	if err != nil {
		return nil, nil, nil, err
	}

	caTemplate := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{spec.Organization},
			CommonName:   fmt.Sprintf("%s-ca", spec.CommonName),
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(time.Duration(certificate.GetValidCADays(spec)) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName + CASecretSuffix,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			CAKey:        pemEncode("CERTIFICATE", caDER),
			CAKeyPrivate: pemEncode("EC PRIVATE KEY", marshalECPrivateKey(caKey)),
		},
	}

	if spec.GenerateCRL {
		now := time.Now()
		// Derive NextUpdate from the parsed CA's NotAfter rather than
		// recomputing `GetValidCADays(spec) * 24 * time.Hour`. This both
		// expresses the intent ("CRL valid for the CA's lifetime") and avoids
		// a separate time.Duration overflow path when CAValidityDays is large.
		crlTemplate := &x509.RevocationList{
			Issuer:     caCert.Subject,
			ThisUpdate: now,
			NextUpdate: caCert.NotAfter,
			Number:     big.NewInt(1),
			// RevokedCertificateEntries left nil (empty list).
		}
		crlDER, err := x509.CreateRevocationList(rand.Reader, crlTemplate, caCert, caKey)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create CRL: %w", err)
		}
		caSecret.Data[CRLKey] = crlDER
	}

	return caSecret, caKey, caCert, nil
}

// SignLeaf generates a new leaf key + certificate signed by caKey/caCert and
// returns the PEM-encoded leaf cert and private key. Validity uses
// GetValidLeafDays; SANs/CN/O come from spec.
func SignLeaf(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, spec certificate.TLSSpec) (leafCertPEM, leafKeyPEM []byte, err error) {
	curve, err := curveFor(spec)
	if err != nil {
		return nil, nil, err
	}

	leafKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate leaf key: %w", err)
	}

	ips, err := parseIPs(spec)
	if err != nil {
		return nil, nil, err
	}

	serial, err := randomSerialNumber()
	if err != nil {
		return nil, nil, err
	}

	leafTemplate := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{spec.Organization},
			CommonName:   spec.CommonName,
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(time.Duration(certificate.GetValidLeafDays(spec)) * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		DNSNames:    spec.DNSNames,
		IPAddresses: ips,
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create leaf certificate: %w", err)
	}

	return pemEncode("CERTIFICATE", leafDER), pemEncode("EC PRIVATE KEY", marshalECPrivateKey(leafKey)), nil
}

// ParseCA parses a CA Secret's ca.crt (first cert) and ca.key (EC private key).
func ParseCA(caSecret *corev1.Secret) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	if caSecret == nil {
		return nil, nil, fmt.Errorf("CA secret is nil")
	}
	rawCert, ok := caSecret.Data[CAKey]
	if !ok || len(rawCert) == 0 {
		return nil, nil, fmt.Errorf("CA secret %q missing %s", caSecret.Name, CAKey)
	}
	certs, err := parsePEMCerts(rawCert)
	if err != nil {
		return nil, nil, err
	}
	caCert := certs[0]

	rawKey, ok := caSecret.Data[CAKeyPrivate]
	if !ok || len(rawKey) == 0 {
		return nil, nil, fmt.Errorf("CA secret %q missing %s", caSecret.Name, CAKeyPrivate)
	}
	block, _ := pem.Decode(rawKey)
	if block == nil {
		return nil, nil, fmt.Errorf("CA secret %q %s is not PEM", caSecret.Name, CAKeyPrivate)
	}
	caKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("CA secret %q %s parse: %w", caSecret.Name, CAKeyPrivate, err)
	}
	// Fail closed on a tampered/mismatched CA secret: signing a leaf with a
	// key that does not match ca.crt would produce a certificate whose
	// signature does not verify against the advertised CA, silently breaking
	// every consumer that trusts ca.crt.
	if !caKey.PublicKey.Equal(caCert.PublicKey) {
		return nil, nil, fmt.Errorf("CA secret %q %s does not match %s", caSecret.Name, CAKeyPrivate, CAKey)
	}
	return caCert, caKey, nil
}

// CANeedsRenewal reports whether the CA Secret's ca.crt needs renewal at now.
//   - nil secret -> true (missing CA).
//   - ca.crt absent/empty -> true.
//   - ca.crt malformed -> error.
//   - within GetValidRenewalDays(spec) of NotAfter -> true; else false.
func CANeedsRenewal(caSecret *corev1.Secret, spec certificate.TLSSpec, now time.Time) (bool, error) {
	if caSecret == nil {
		return true, nil
	}
	raw, ok := caSecret.Data[CAKey]
	if !ok || len(raw) == 0 {
		return true, nil
	}
	window := time.Duration(certificate.GetValidRenewalDays(spec)) * 24 * time.Hour
	return certsNeedRenewal(raw, now, window)
}

// DesiredObjects generates CA and leaf certificates and returns them as
// Kubernetes Secret objects for SSA reconciliation.
func (b *SelfManagedBackend[T]) DesiredObjects(ctx context.Context, o T, spec certificate.TLSSpec) ([]client.Object, error) {
	namespace := o.GetNamespace()

	caSecret, caKey, caCert, err := BuildCA(namespace, spec.SecretName, spec)
	if err != nil {
		return nil, err
	}

	leafCertPEM, leafKeyPEM, err := SignLeaf(caCert, caKey, spec)
	if err != nil {
		return nil, err
	}

	leafSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.SecretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			CertKey: leafCertPEM,
			KeyKey:  leafKeyPEM,
			CAKey:   caSecret.Data[CAKey],
		},
	}

	return []client.Object{caSecret, leafSecret}, nil
}

// DesiredLeafWithCA re-issues the leaf signed by the CA in caSecret, reusing
// that CA's key/cert (no new CA). The returned Secret's ca.crt equals
// caSecret's ca.crt (single CA, no bundle).
func (b *SelfManagedBackend[T]) DesiredLeafWithCA(ctx context.Context, o T, spec certificate.TLSSpec, caSecret *corev1.Secret) (*corev1.Secret, error) {
	caCert, caKey, err := ParseCA(caSecret)
	if err != nil {
		return nil, err
	}

	leafCertPEM, leafKeyPEM, err := SignLeaf(caCert, caKey, spec)
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.SecretName,
			Namespace: o.GetNamespace(),
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			CertKey: leafCertPEM,
			KeyKey:  leafKeyPEM,
			CAKey:   caSecret.Data[CAKey],
		},
	}, nil
}

// LeafNeedsChange reports whether the leaf Secret needs regeneration vs spec
// at now, with single-cert semantics.
func (b *SelfManagedBackend[T]) LeafNeedsChange(ctx context.Context, o T, leafSecret *corev1.Secret, spec certificate.TLSSpec, now time.Time) (certificate.LeafChange, error) {
	chg := certificate.LeafChange{}

	if leafSecret == nil {
		chg.Reason = certificate.LeafMissing
		return chg, nil
	}
	rawCert, ok := leafSecret.Data[CertKey]
	if !ok || len(rawCert) == 0 {
		chg.Reason = certificate.LeafMissing
		return chg, nil
	}
	certs, err := parsePEMCerts(rawCert)
	if err != nil {
		return chg, err
	}
	cert := certs[0]

	window := time.Duration(certificate.GetValidRenewalDays(spec)) * 24 * time.Hour

	// Delta slices are always populated regardless of the dominant reason.
	chg.SANsAdded, chg.SANsRemoved = stringSetDiff(spec.DNSNames, cert.DNSNames)

	ips, err := parseIPs(spec)
	if err != nil {
		return chg, err
	}
	chg.IPsAdded, chg.IPsRemoved = ipSetDiff(ips, cert.IPAddresses)

	// Reason precedence (first match wins).
	switch {
	case now.After(cert.NotAfter.Add(-window)):
		chg.Reason = certificate.LeafExpiring
	case cert.Subject.CommonName != spec.CommonName:
		chg.Reason = certificate.LeafCNChanged
	case !stringSetEqual([]string{spec.Organization}, cert.Subject.Organization):
		chg.Reason = certificate.LeafOrgChanged
	case len(chg.SANsAdded) > 0 || len(chg.SANsRemoved) > 0:
		chg.Reason = certificate.LeafSANsChanged
	case len(chg.IPsAdded) > 0 || len(chg.IPsRemoved) > 0:
		chg.Reason = certificate.LeafIPsChanged
	default:
		chg.Reason = certificate.LeafNone
	}
	return chg, nil
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

// randomSerialNumber returns a cryptographically-random 128-bit serial number
// for an x509 certificate. RFC 5280 §4.1.2.2 requires serial numbers to be
// unique per issuer; deriving them from crypto/rand (rather than a fixed
// constant) ensures each leaf issued by a CA has a distinct serial so CRL
// revocation by (issuer, serial) can address a single certificate.
func randomSerialNumber() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}
	return serial, nil
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

// stringSetDiff computes the order-insensitive set difference between want
// (desired) and have (current). added = want - have, removed = have - want.
func stringSetDiff(want, have []string) (added, removed []string) {
	haveSet := make(map[string]struct{}, len(have))
	for _, s := range have {
		haveSet[s] = struct{}{}
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, s := range want {
		wantSet[s] = struct{}{}
	}
	for _, s := range want {
		if _, ok := haveSet[s]; !ok {
			added = append(added, s)
		}
	}
	for _, s := range have {
		if _, ok := wantSet[s]; !ok {
			removed = append(removed, s)
		}
	}
	return added, removed
}

// ipSetDiff computes the order-insensitive set difference between want and
// have net.IP sets, returned as their string forms.
func ipSetDiff(want, have []net.IP) (added, removed []string) {
	wantStr := make([]string, len(want))
	for i, ip := range want {
		wantStr[i] = ip.String()
	}
	haveStr := make([]string, len(have))
	for i, ip := range have {
		haveStr[i] = ip.String()
	}
	return stringSetDiff(wantStr, haveStr)
}

// stringSetEqual reports whether two string slices contain the same elements
// as a set (order-insensitive).
func stringSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, s := range a {
		set[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := set[s]; !ok {
			return false
		}
	}
	return true
}
