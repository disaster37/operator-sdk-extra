// Package selfmanaged provides a TLSBackend that generates and manages
// CA and leaf certificates using Go's crypto/x509 standard library.
// It supports multi-cycle CA rotation via the WorkflowStep saga.
package selfmanaged

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"

	"emperror.dev/errors"
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

// BuildCA generates a fresh ECDSA CA key + self-signed CA certificate and
// returns the CA Secret (name secretName+CASecretSuffix), the CA private key,
// and the parsed CA certificate. When spec.GenerateCRL, ca.crl is added.
//
// This is the ECDSA wrapper around BuildCASigner; it errors if spec requests a
// non-ECDSA key algorithm.
func BuildCA(namespace, secretName string, spec certificate.TLSSpec) (*corev1.Secret, *ecdsa.PrivateKey, *x509.Certificate, error) {
	caSecret, signer, caCert, err := BuildCASigner(namespace, secretName, spec)
	if err != nil {
		return nil, nil, nil, err
	}
	ecKey, ok := signer.(*ecdsa.PrivateKey)
	if !ok {
		return nil, nil, nil, errors.Errorf("BuildCA requires an ECDSA key; spec requests %q", spec.EffectiveKeyAlgorithm())
	}
	return caSecret, ecKey, caCert, nil
}

// BuildCASigner generates a fresh CA key (ECDSA or RSA per spec) + self-signed
// CA certificate and returns the CA Secret, the CA crypto.Signer, and the
// parsed CA certificate. The CA subject is spec.ResolvedCASubject(); its key
// usage is fixed at CertSign|CRLSign.
func BuildCASigner(namespace, secretName string, spec certificate.TLSSpec) (*corev1.Secret, crypto.Signer, *x509.Certificate, error) {
	if err := spec.ValidateContent(); err != nil {
		return nil, nil, nil, err
	}

	caKey, err := generateKey(spec)
	if err != nil {
		return nil, nil, nil, err
	}

	serial, err := randomSerialNumber()
	if err != nil {
		return nil, nil, nil, err
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               spec.ResolvedCASubject(),
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(time.Duration(certificate.GetValidCADays(spec)) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caKey.Public(), caKey)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to create CA certificate")
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to parse CA certificate")
	}

	keyType, keyDER, err := marshalSignerKey(caKey)
	if err != nil {
		return nil, nil, nil, err
	}

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName + CASecretSuffix,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			CAKey:        pemEncode("CERTIFICATE", caDER),
			CAKeyPrivate: pemEncode(keyType, keyDER),
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
			return nil, nil, nil, errors.Wrap(err, "failed to create CRL")
		}
		caSecret.Data[CRLKey] = crlDER
	}

	return caSecret, caKey, caCert, nil
}

// SignLeaf generates a new leaf key + certificate signed by caKey/caCert and
// returns the PEM-encoded leaf cert and private key. The leaf key algorithm
// follows spec (ECDSA or RSA), not always ECDSA; this simply delegates to
// SignLeafSigner.
func SignLeaf(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, spec certificate.TLSSpec) (leafCertPEM, leafKeyPEM []byte, err error) {
	return SignLeafSigner(caCert, caKey, spec)
}

// SignLeafSigner generates a new leaf key (ECDSA or RSA per spec) + certificate
// signed by caCert/caSigner and returns the PEM-encoded leaf cert and private
// key. Subject, key algorithm/size, usages and SANs all come from spec.
func SignLeafSigner(caCert *x509.Certificate, caSigner crypto.Signer, spec certificate.TLSSpec) (leafCertPEM, leafKeyPEM []byte, err error) {
	if err := spec.ValidateContent(); err != nil {
		return nil, nil, err
	}

	leafKey, err := generateKey(spec)
	if err != nil {
		return nil, nil, err
	}

	ips, err := parseIPs(spec)
	if err != nil {
		return nil, nil, err
	}

	serial, err := randomSerialNumber()
	if err != nil {
		return nil, nil, err
	}

	keyUsage, err := keyUsageFor(spec.EffectiveKeyUsages())
	if err != nil {
		return nil, nil, err
	}
	extKeyUsage, err := extKeyUsageFor(spec.EffectiveUsages())
	if err != nil {
		return nil, nil, err
	}

	leafTemplate := &x509.Certificate{
		SerialNumber: serial,
		Subject:      spec.LeafSubject(),
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(time.Duration(certificate.GetValidLeafDays(spec)) * 24 * time.Hour),
		KeyUsage:     keyUsage,
		ExtKeyUsage:  extKeyUsage,
		DNSNames:     certificate.DedupStrings(spec.DNSNames),
		IPAddresses:  certificate.DedupIPs(ips),
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, leafKey.Public(), caSigner)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create leaf certificate")
	}

	keyType, keyDER, err := marshalSignerKey(leafKey)
	if err != nil {
		return nil, nil, err
	}

	return pemEncode("CERTIFICATE", leafDER), pemEncode(keyType, keyDER), nil
}

// ParseCA parses a CA Secret's ca.crt (first cert) and ca.key (EC private key).
// This is the ECDSA wrapper around ParseCASigner; it errors if the stored key
// is not an EC key.
func ParseCA(caSecret *corev1.Secret) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	cert, signer, err := ParseCASigner(caSecret)
	if err != nil {
		return nil, nil, err
	}
	ecKey, ok := signer.(*ecdsa.PrivateKey)
	if !ok {
		return nil, nil, errors.Errorf("CA secret %q %s is not an EC private key", caSecret.Name, CAKeyPrivate)
	}
	return cert, ecKey, nil
}

// ParseCASigner parses a CA Secret's ca.crt (first cert) and ca.key (EC or RSA
// private key) and returns the parsed CA certificate and crypto.Signer.
func ParseCASigner(caSecret *corev1.Secret) (*x509.Certificate, crypto.Signer, error) {
	if caSecret == nil {
		return nil, nil, errors.New("CA secret is nil")
	}
	rawCert, ok := caSecret.Data[CAKey]
	if !ok || len(rawCert) == 0 {
		return nil, nil, errors.Errorf("CA secret %q missing %s", caSecret.Name, CAKey)
	}
	certs, err := parsePEMCerts(rawCert)
	if err != nil {
		return nil, nil, err
	}
	caCert := certs[0]

	rawKey, ok := caSecret.Data[CAKeyPrivate]
	if !ok || len(rawKey) == 0 {
		return nil, nil, errors.Errorf("CA secret %q missing %s", caSecret.Name, CAKeyPrivate)
	}
	block, _ := pem.Decode(rawKey)
	if block == nil {
		return nil, nil, errors.Errorf("CA secret %q %s is not PEM", caSecret.Name, CAKeyPrivate)
	}
	signer, err := parseSignerFromPEM(block)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "CA secret %q %s parse", caSecret.Name, CAKeyPrivate)
	}
	// Fail closed on a tampered/mismatched CA secret: signing a leaf with a
	// key that does not match ca.crt would produce a certificate whose
	// signature does not verify against the advertised CA, silently breaking
	// every consumer that trusts ca.crt.
	pub, ok := signer.Public().(interface{ Equal(crypto.PublicKey) bool })
	if !ok {
		return nil, nil, errors.Errorf("CA secret %q %s has unsupported public key type %T", caSecret.Name, CAKeyPrivate, signer.Public())
	}
	if !pub.Equal(caCert.PublicKey) {
		return nil, nil, errors.Errorf("CA secret %q %s does not match %s", caSecret.Name, CAKeyPrivate, CAKey)
	}
	return caCert, signer, nil
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

// CAContentChanged reports whether the CA cert's subject, public-key algorithm
// or public-key size differs from spec (used to trigger a CA saga on content
// change, not just expiry). For ECDSA the public-key size is implied by the
// curve, and for RSA it is the modulus length; in both cases a requested
// key-strength change must rotate the CA so the operator's hardening intent is
// actually applied rather than silently ignored.
func CAContentChanged(caSecret *corev1.Secret, spec certificate.TLSSpec) (bool, error) {
	if caSecret == nil {
		return true, nil
	}
	raw, ok := caSecret.Data[CAKey]
	if !ok || len(raw) == 0 {
		return true, nil
	}
	certs, err := parsePEMCerts(raw)
	if err != nil {
		return false, err
	}
	caCert := certs[0]

	if !pkixNameEqual(spec.ResolvedCASubject(), caCert.Subject) {
		return true, nil
	}
	wantAlg := x509.ECDSA
	if spec.EffectiveKeyAlgorithm() == certificate.KeyAlgorithmRSA {
		wantAlg = x509.RSA
	}
	if caCert.PublicKeyAlgorithm != wantAlg {
		return true, nil
	}
	size, err := spec.EffectiveKeySize()
	if err != nil {
		return false, err
	}
	if publicKeyBitSize(caCert.PublicKey) != size {
		return true, nil
	}
	return false, nil
}

// DesiredObjects generates CA and leaf certificates and returns them as
// Kubernetes Secret objects for SSA reconciliation.
func (b *SelfManagedBackend[T]) DesiredObjects(ctx context.Context, o T, spec certificate.TLSSpec) ([]client.Object, error) {
	namespace := o.GetNamespace()

	caSecret, caSigner, caCert, err := BuildCASigner(namespace, spec.SecretName, spec)
	if err != nil {
		return nil, err
	}

	leafCertPEM, leafKeyPEM, err := SignLeafSigner(caCert, caSigner, spec)
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
	caCert, caSigner, err := ParseCASigner(caSecret)
	if err != nil {
		return nil, err
	}

	leafCertPEM, leafKeyPEM, err := SignLeafSigner(caCert, caSigner, spec)
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

	kc, err := KeyChanged(spec, cert)
	if err != nil {
		return chg, err
	}
	uc, err := UsagesChanged(spec, cert)
	if err != nil {
		return chg, err
	}

	// Reason precedence (first match wins).
	switch {
	case now.After(cert.NotAfter.Add(-window)):
		chg.Reason = certificate.LeafExpiring
	case cert.Subject.CommonName != spec.CommonName:
		chg.Reason = certificate.LeafCNChanged
	case !OrganizationsEqual(spec, cert):
		chg.Reason = certificate.LeafOrgChanged
	case !SubjectRestEqual(spec.LeafSubject(), cert.Subject):
		chg.Reason = certificate.LeafSubjectChanged
	case len(chg.SANsAdded) > 0 || len(chg.SANsRemoved) > 0:
		chg.Reason = certificate.LeafSANsChanged
	case len(chg.IPsAdded) > 0 || len(chg.IPsRemoved) > 0:
		chg.Reason = certificate.LeafIPsChanged
	case kc:
		chg.Reason = certificate.LeafKeyChanged
	case uc:
		chg.Reason = certificate.LeafUsagesChanged
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

// generateKey returns a fresh private key per spec (ECDSA curve or RSA size).
func generateKey(spec certificate.TLSSpec) (crypto.Signer, error) {
	if err := spec.ValidateContent(); err != nil {
		return nil, err
	}
	switch spec.EffectiveKeyAlgorithm() {
	case certificate.KeyAlgorithmECDSA:
		curve, err := curveForSpec(spec)
		if err != nil {
			return nil, err
		}
		key, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate ECDSA key")
		}
		return key, nil
	case certificate.KeyAlgorithmRSA:
		size, err := spec.EffectiveKeySize()
		if err != nil {
			return nil, err
		}
		key, err := rsa.GenerateKey(rand.Reader, size)
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate RSA key")
		}
		return key, nil
	default:
		return nil, errors.Wrapf(certificate.ErrUnsupportedKeyAlgorithm, "unsupported key algorithm %q", spec.EffectiveKeyAlgorithm())
	}
}

// curveForSpec resolves the ECDSA curve from spec, honoring the legacy Curve
// field (validated via curveFor) before falling back to the key size.
func curveForSpec(spec certificate.TLSSpec) (elliptic.Curve, error) {
	if spec.Curve != "" {
		return curveFor(spec)
	}
	size, err := spec.EffectiveKeySize()
	if err != nil {
		return nil, err
	}
	return curveForSize(size)
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
		return nil, errors.Errorf("unsupported curve %q: want P-256, P-384 or P-521", spec.Curve)
	}
}

// curveForSize maps an ECDSA key size to its elliptic.Curve.
func curveForSize(size int) (elliptic.Curve, error) {
	switch size {
	case 256:
		return elliptic.P256(), nil
	case 384:
		return elliptic.P384(), nil
	case 521:
		return elliptic.P521(), nil
	default:
		return nil, errors.Errorf("unsupported ECDSA key size %d: want 256, 384 or 521", size)
	}
}

// marshalSignerKey returns the PEM block type ("EC PRIVATE KEY" | "RSA PRIVATE
// KEY") and DER for a signer.
func marshalSignerKey(signer crypto.Signer) (string, []byte, error) {
	switch k := signer.(type) {
	case *ecdsa.PrivateKey:
		der, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return "", nil, errors.Wrap(err, "failed to marshal EC key")
		}
		return "EC PRIVATE KEY", der, nil
	case *rsa.PrivateKey:
		return "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(k), nil
	default:
		return "", nil, errors.Errorf("unsupported private key type %T", signer)
	}
}

// parseSignerFromPEM parses an EC or RSA private-key PEM block.
func parseSignerFromPEM(block *pem.Block) (crypto.Signer, error) {
	switch block.Type {
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.Wrap(err, "parse EC private key")
		}
		return key, nil
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.Wrap(err, "parse RSA private key")
		}
		return key, nil
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.Wrap(err, "parse PKCS#8 private key")
		}
		signer, ok := key.(crypto.Signer)
		if !ok {
			return nil, errors.Errorf("unsupported PKCS#8 private key type %T", key)
		}
		return signer, nil
	default:
		return nil, errors.Errorf("unsupported private key PEM type %q", block.Type)
	}
}

var keyUsageBits = map[string]x509.KeyUsage{
	certificate.KeyUsageDigitalSignature:  x509.KeyUsageDigitalSignature,
	certificate.KeyUsageContentCommitment: x509.KeyUsageContentCommitment,
	certificate.KeyUsageKeyEncipherment:   x509.KeyUsageKeyEncipherment,
	certificate.KeyUsageDataEncipherment:  x509.KeyUsageDataEncipherment,
	certificate.KeyUsageKeyAgreement:      x509.KeyUsageKeyAgreement,
}

// keyUsageFor maps KeyUsage* strings to x509.KeyUsage.
func keyUsageFor(names []string) (x509.KeyUsage, error) {
	var ku x509.KeyUsage
	for _, name := range names {
		bit, ok := keyUsageBits[name]
		if !ok {
			return 0, errors.Wrapf(certificate.ErrUnknownUsage, "unknown key usage %q", name)
		}
		ku |= bit
	}
	return ku, nil
}

var extKeyUsageBits = map[string]x509.ExtKeyUsage{
	certificate.UsageServerAuth:      x509.ExtKeyUsageServerAuth,
	certificate.UsageClientAuth:      x509.ExtKeyUsageClientAuth,
	certificate.UsageCodeSigning:     x509.ExtKeyUsageCodeSigning,
	certificate.UsageEmailProtection: x509.ExtKeyUsageEmailProtection,
	certificate.UsageTimestamping:    x509.ExtKeyUsageTimeStamping,
	certificate.UsageOCSPSigning:     x509.ExtKeyUsageOCSPSigning,
}

// extKeyUsageFor maps Usage* strings to x509.ExtKeyUsage.
func extKeyUsageFor(names []string) ([]x509.ExtKeyUsage, error) {
	usages := make([]x509.ExtKeyUsage, 0, len(names))
	for _, name := range names {
		u, ok := extKeyUsageBits[name]
		if !ok {
			return nil, errors.Wrapf(certificate.ErrUnknownUsage, "unknown extended key usage %q", name)
		}
		usages = append(usages, u)
	}
	return usages, nil
}

// KeyChanged reports whether cert's public-key algorithm or bit size differs
// from spec.
func KeyChanged(spec certificate.TLSSpec, cert *x509.Certificate) (bool, error) {
	wantAlg := x509.ECDSA
	if spec.EffectiveKeyAlgorithm() == certificate.KeyAlgorithmRSA {
		wantAlg = x509.RSA
	}
	if cert.PublicKeyAlgorithm != wantAlg {
		return true, nil
	}
	size, err := spec.EffectiveKeySize()
	if err != nil {
		return false, err
	}
	return publicKeyBitSize(cert.PublicKey) != size, nil
}

// UsagesChanged reports whether cert's KeyUsage/ExtKeyUsage differ from spec.
func UsagesChanged(spec certificate.TLSSpec, cert *x509.Certificate) (bool, error) {
	wantKU, err := keyUsageFor(spec.EffectiveKeyUsages())
	if err != nil {
		return false, err
	}
	wantEKU, err := extKeyUsageFor(spec.EffectiveUsages())
	if err != nil {
		return false, err
	}
	if cert.KeyUsage != wantKU {
		return true, nil
	}
	return !extKeyUsageSetEqual(cert.ExtKeyUsage, wantEKU), nil
}

// publicKeyBitSize returns the bit size of an ECDSA or RSA public key, or 0
// for unknown key types.
func publicKeyBitSize(pub any) int {
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		return k.Curve.Params().BitSize
	case *rsa.PublicKey:
		return k.N.BitLen()
	default:
		return 0
	}
}

// pkixNameEqual compares two pkix.Names field-by-field, treating empty-string
// RDNs as absent.
func pkixNameEqual(a, b pkix.Name) bool {
	return a.CommonName == b.CommonName &&
		stringSetEqual(dropEmpty(a.Organization), dropEmpty(b.Organization)) &&
		SubjectRestEqual(a, b)
}

// OrganizationsEqual reports whether cert's Organization RDN matches spec's
// effective leaf organizations (order-insensitive, empty treated as absent).
func OrganizationsEqual(spec certificate.TLSSpec, cert *x509.Certificate) bool {
	return stringSetEqual(dropEmpty(spec.Organizations()), dropEmpty(cert.Subject.Organization))
}

// SubjectRestEqual compares every RDN field except CommonName and Organization.
func SubjectRestEqual(a, b pkix.Name) bool {
	return stringSetEqual(dropEmpty(a.OrganizationalUnit), dropEmpty(b.OrganizationalUnit)) &&
		stringSetEqual(dropEmpty(a.Country), dropEmpty(b.Country)) &&
		stringSetEqual(dropEmpty(a.Locality), dropEmpty(b.Locality)) &&
		stringSetEqual(dropEmpty(a.Province), dropEmpty(b.Province)) &&
		stringSetEqual(dropEmpty(a.StreetAddress), dropEmpty(b.StreetAddress)) &&
		stringSetEqual(dropEmpty(a.PostalCode), dropEmpty(b.PostalCode)) &&
		a.SerialNumber == b.SerialNumber
}

// dropEmpty returns a copy of in with empty strings removed.
func dropEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// extKeyUsageSetEqual reports whether two ExtKeyUsage slices contain the same
// elements as a set (order-insensitive).
func extKeyUsageSetEqual(a, b []x509.ExtKeyUsage) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[x509.ExtKeyUsage]struct{}, len(a))
	for _, u := range a {
		set[u] = struct{}{}
	}
	for _, u := range b {
		if _, ok := set[u]; !ok {
			return false
		}
	}
	return true
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
		return nil, errors.Wrap(err, "failed to generate serial number")
	}
	// rand.Int returns values in [0, 2^128); RFC 5280 §4.1.2.2 requires the
	// serial number to be a positive integer, so guard against the
	// (cryptographically negligible) zero case rather than emit a cert whose
	// serial violates the profile.
	if serial.Sign() == 0 {
		serial.SetInt64(1)
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
			return nil, errors.Errorf("invalid IP SAN %q", s)
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
			return nil, errors.Wrap(err, "failed to parse certificate")
		}
		certs = append(certs, c)
	}
	if len(certs) == 0 {
		return nil, errors.New("no certificates found in PEM data")
	}
	return certs, nil
}

func pemEncode(blockType string, derBytes []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  blockType,
		Bytes: derBytes,
	})
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
