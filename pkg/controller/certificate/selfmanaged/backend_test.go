package selfmanaged_test

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
	"testing"
	"time"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/selfmanaged"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type testSelfManagedObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (o *testSelfManagedObject) GetStatus() object.MultiPhaseObjectStatus {
	return nil
}

func (o *testSelfManagedObject) DeepCopyObject() runtime.Object {
	return &testSelfManagedObject{
		TypeMeta:   o.TypeMeta,
		ObjectMeta: *o.DeepCopy(),
	}
}

func TestNewSelfManagedBackend(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	require.NotNil(t, backend)
}

func TestSelfManagedBackendDesiredObjects(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "test.example.com",
		DNSNames:         []string{"test.example.com", "test-alt.example.com"},
		Organization:     "TestOrg",
		LeafValidityDays: 90,
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 2, "expected CA secret and leaf secret")
}

func TestSelfManagedBackendDesiredObjectsDefaults(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 2)
}

func TestSelfManagedBackendGoldenFiles(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com"},
		Organization: "TestOrg",
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 2)

	// CA secret
	caSecret, ok := objects[0].(*corev1.Secret)
	require.True(t, ok, "first object should be a Secret")
	assert.Equal(t, "test-tls-ca", caSecret.Name)
	assert.Equal(t, "default", caSecret.Namespace)
	assert.Contains(t, string(caSecret.Data["ca.crt"]), "BEGIN CERTIFICATE")
	assert.Contains(t, string(caSecret.Data["ca.key"]), "BEGIN EC PRIVATE KEY")

	// Leaf secret
	leafSecret, ok := objects[1].(*corev1.Secret)
	require.True(t, ok, "second object should be a Secret")
	assert.Equal(t, "test-tls", leafSecret.Name)
	assert.Equal(t, "default", leafSecret.Namespace)
	assert.Equal(t, corev1.SecretTypeTLS, leafSecret.Type)
	assert.Contains(t, string(leafSecret.Data["tls.crt"]), "BEGIN CERTIFICATE")
	assert.Contains(t, string(leafSecret.Data["tls.key"]), "BEGIN EC PRIVATE KEY")
	assert.Contains(t, string(leafSecret.Data["ca.crt"]), "BEGIN CERTIFICATE")
}

func TestCertificateSecretName(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	spec := certificate.TLSSpec{SecretName: "my-tls"}
	name := backend.CertificateSecretName(o, spec)
	assert.Equal(t, "my-tls", name)
}

func TestRequiresRotationSaga(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	assert.True(t, backend.RequiresRotationSaga())
}

func TestSelfManagedBackendWithDNSNames(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com", "*.example.com"},
		Organization: "TestOrg",
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 2)
}

func testSelfManagedObjects(t *testing.T, spec certificate.TLSSpec) (*corev1.Secret, *corev1.Secret) {
	t.Helper()
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 2)

	caSecret, ok := objects[0].(*corev1.Secret)
	require.True(t, ok)
	leafSecret, ok := objects[1].(*corev1.Secret)
	require.True(t, ok)
	return caSecret, leafSecret
}

func parseLeafCert(t *testing.T, leaf *corev1.Secret) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(leaf.Data[selfmanaged.CertKey])
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return cert
}

func TestSelfManagedBackendCurveP384(t *testing.T) {
	spec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		Curve:      certificate.CurveP384,
	}

	caSecret, leafSecret := testSelfManagedObjects(t, spec)

	cert := parseLeafCert(t, leafSecret)
	ecKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	require.True(t, ok, "leaf public key should be ECDSA")
	assert.Equal(t, elliptic.P384(), ecKey.Curve)

	caKeyBlock, _ := pem.Decode(caSecret.Data[selfmanaged.CAKeyPrivate])
	require.NotNil(t, caKeyBlock)
	assert.Equal(t, "EC PRIVATE KEY", caKeyBlock.Type)
}

func TestSelfManagedBackendCurveP521(t *testing.T) {
	spec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		Curve:      certificate.CurveP521,
	}

	_, leafSecret := testSelfManagedObjects(t, spec)

	cert := parseLeafCert(t, leafSecret)
	ecKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	require.True(t, ok, "leaf public key should be ECDSA")
	assert.Equal(t, elliptic.P521(), ecKey.Curve)
}

func TestSelfManagedBackendCurveDefault(t *testing.T) {
	spec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
	}

	_, leafSecret := testSelfManagedObjects(t, spec)

	cert := parseLeafCert(t, leafSecret)
	ecKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	require.True(t, ok, "leaf public key should be ECDSA")
	assert.Equal(t, elliptic.P256(), ecKey.Curve)
}

func TestSelfManagedBackendCurveUnknown(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		Curve:      "P-999",
	}

	_, err := backend.DesiredObjects(context.Background(), o, spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported curve")
	assert.ErrorIs(t, err, certificate.ErrUnknownCurve)
}

func TestSelfManagedBackendIPSANs(t *testing.T) {
	spec := certificate.TLSSpec{
		SecretName:  "test-tls",
		CommonName:  "test.example.com",
		DNSNames:    []string{"test.example.com"},
		IPAddresses: []string{"10.0.0.1", "192.168.1.1"},
	}

	_, leafSecret := testSelfManagedObjects(t, spec)

	cert := parseLeafCert(t, leafSecret)
	require.Len(t, cert.IPAddresses, 2)
	assert.True(t, cert.IPAddresses[0].Equal(net.ParseIP("10.0.0.1")))
	assert.True(t, cert.IPAddresses[1].Equal(net.ParseIP("192.168.1.1")))
	assert.Equal(t, []string{"test.example.com"}, cert.DNSNames)
}

func TestSelfManagedBackendIPSANsInvalid(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:  "test-tls",
		CommonName:  "test.example.com",
		IPAddresses: []string{"not-an-ip"},
	}

	_, err := backend.DesiredObjects(context.Background(), o, spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid IP SAN")
}

func TestSelfManagedBackendCRL(t *testing.T) {
	spec := certificate.TLSSpec{
		SecretName:  "test-tls",
		CommonName:  "test.example.com",
		GenerateCRL: true,
	}

	caSecret, leafSecret := testSelfManagedObjects(t, spec)

	crlDER, ok := caSecret.Data[selfmanaged.CRLKey]
	require.True(t, ok, "ca.crl should be present")
	assert.NotEmpty(t, crlDER)

	crl, err := x509.ParseRevocationList(crlDER)
	require.NoError(t, err)
	assert.Equal(t, int64(1), crl.Number.Int64())
	assert.Empty(t, crl.RevokedCertificateEntries)
	assert.False(t, crl.ThisUpdate.IsZero())
	assert.False(t, crl.NextUpdate.IsZero())

	_, ok = leafSecret.Data[selfmanaged.CRLKey]
	assert.False(t, ok, "leaf secret should not have ca.crl")
}

func TestSelfManagedBackendCRLAbsent(t *testing.T) {
	spec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
	}

	caSecret, _ := testSelfManagedObjects(t, spec)

	_, ok := caSecret.Data[selfmanaged.CRLKey]
	assert.False(t, ok, "ca.crl should be absent when GenerateCRL is false")
}

// TestSelfManagedBackendCRLNextUpdateMatchesCA verifies the CRL NextUpdate is
// derived from the CA cert's NotAfter (not recomputed via a separate
// `validity*2*24*time.Hour` multiplication that could overflow). The CRL
// must outlive ThisUpdate and track the CA lifetime.
func TestSelfManagedBackendCRLNextUpdateMatchesCA(t *testing.T) {
	spec := certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "test.example.com",
		GenerateCRL:      true,
		LeafValidityDays: 90,
	}

	caSecret, _ := testSelfManagedObjects(t, spec)

	caCertBlock, _ := pem.Decode(caSecret.Data[selfmanaged.CAKey])
	require.NotNil(t, caCertBlock)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	require.NoError(t, err)

	crl, err := x509.ParseRevocationList(caSecret.Data[selfmanaged.CRLKey])
	require.NoError(t, err)
	assert.True(t, crl.NextUpdate.After(crl.ThisUpdate), "CRL NextUpdate must be after ThisUpdate")
	assert.True(t, crl.NextUpdate.Equal(caCert.NotAfter) || crl.NextUpdate.Before(caCert.NotAfter.Add(time.Second)),
		"CRL NextUpdate should track CA NotAfter, got CRL=%s CA=%s", crl.NextUpdate, caCert.NotAfter)
}

func makeTestCert(t *testing.T, cn string, notBefore, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestBuildCASecret(t *testing.T) {
	spec := certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "test.example.com",
		LeafValidityDays: 90,
	}
	caSecret, _, caCert, err := selfmanaged.BuildCA("default", spec.SecretName, spec)
	require.NoError(t, err)
	assert.Equal(t, "test-tls-ca", caSecret.Name)
	assert.Equal(t, "default", caSecret.Namespace)
	assert.Contains(t, caSecret.Data, selfmanaged.CAKey)
	assert.Contains(t, caSecret.Data, selfmanaged.CAKeyPrivate)
	_, ok := caSecret.Data[selfmanaged.CRLKey]
	assert.False(t, ok, "ca.crl should be absent when GenerateCRL is false")

	// CA NotAfter ≈ now + GetValidCADays (2× leaf = 180 days).
	want := time.Now().Add(time.Duration(certificate.GetValidCADays(spec)) * 24 * time.Hour)
	assert.WithinDuration(t, want, caCert.NotAfter, 5*time.Minute)
}

func TestBuildCASecretWithCRL(t *testing.T) {
	spec := certificate.TLSSpec{
		SecretName:  "test-tls",
		CommonName:  "test.example.com",
		GenerateCRL: true,
	}
	caSecret, _, _, err := selfmanaged.BuildCA("default", spec.SecretName, spec)
	require.NoError(t, err)
	assert.Contains(t, caSecret.Data, selfmanaged.CRLKey)
}

func TestSignLeaf(t *testing.T) {
	spec := certificate.TLSSpec{
		CommonName:       "test.example.com",
		DNSNames:         []string{"test.example.com"},
		Organization:     "TestOrg",
		LeafValidityDays: 90,
		IPAddresses:      []string{"10.0.0.1"},
	}
	_, caKey, caCert, err := selfmanaged.BuildCA("default", "test-tls", spec)
	require.NoError(t, err)

	certPEM, keyPEM, err := selfmanaged.SignLeaf(caCert, caKey, spec)
	require.NoError(t, err)
	assert.Contains(t, string(keyPEM), "BEGIN EC PRIVATE KEY")

	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	leafCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, "test.example.com", leafCert.Subject.CommonName)
	assert.Equal(t, []string{"TestOrg"}, leafCert.Subject.Organization)
	assert.Equal(t, []string{"test.example.com"}, leafCert.DNSNames)
	require.Len(t, leafCert.IPAddresses, 1)
	assert.True(t, leafCert.IPAddresses[0].Equal(net.ParseIP("10.0.0.1")))

	// Signed by the CA.
	require.NoError(t, leafCert.CheckSignatureFrom(caCert))

	// Validity ≈ now + GetValidLeafDays.
	want := time.Now().Add(time.Duration(certificate.GetValidLeafDays(spec)) * 24 * time.Hour)
	assert.WithinDuration(t, want, leafCert.NotAfter, 5*time.Minute)
}

func TestSignLeaf_UniqueSerialNumbers(t *testing.T) {
	spec := certificate.TLSSpec{
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com"},
		Organization: "TestOrg",
	}
	_, caKey, caCert, err := selfmanaged.BuildCA("default", "test-tls", spec)
	require.NoError(t, err)
	require.NotNil(t, caCert)
	require.NotZero(t, caCert.SerialNumber.Sign(), "CA serial must be positive and non-zero")

	certPEM1, _, err := selfmanaged.SignLeaf(caCert, caKey, spec)
	require.NoError(t, err)
	certPEM2, _, err := selfmanaged.SignLeaf(caCert, caKey, spec)
	require.NoError(t, err)

	parseSerial := func(t *testing.T, pemBytes []byte) *big.Int {
		t.Helper()
		block, _ := pem.Decode(pemBytes)
		require.NotNil(t, block)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		return cert.SerialNumber
	}

	s1 := parseSerial(t, certPEM1)
	s2 := parseSerial(t, certPEM2)
	assert.NotZero(t, s1.Sign())
	assert.NotZero(t, s2.Sign())
	assert.NotEqual(t, s1, s2, "each leaf issued by a CA must have a unique serial number")
}

func TestParseCA(t *testing.T) {
	caSecret, caKey, caCert, err := selfmanaged.BuildCA("default", "test-tls", certificate.TLSSpec{CommonName: "test.example.com"})
	require.NoError(t, err)

	parsedCert, parsedKey, err := selfmanaged.ParseCA(caSecret)
	require.NoError(t, err)
	assert.Equal(t, caCert.Raw, parsedCert.Raw)
	assert.True(t, caKey.PublicKey.Equal(&parsedKey.PublicKey))
}

func TestParseCA_MismatchedKeyCert(t *testing.T) {
	caSecret, _, _, err := selfmanaged.BuildCA("default", "test-tls", certificate.TLSSpec{CommonName: "test.example.com"})
	require.NoError(t, err)

	// Replace ca.key with a different (valid) EC key so the key no longer
	// matches ca.crt. ParseCA must fail closed rather than sign broken certs.
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalECPrivateKey(otherKey)
	require.NoError(t, err)
	caSecret.Data[selfmanaged.CAKeyPrivate] = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

	_, _, err = selfmanaged.ParseCA(caSecret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
}

func TestParseCA_MissingCAKey(t *testing.T) {
	caSecret, _, _, err := selfmanaged.BuildCA("default", "test-tls", certificate.TLSSpec{CommonName: "test.example.com"})
	require.NoError(t, err)
	delete(caSecret.Data, selfmanaged.CAKeyPrivate)

	_, _, err = selfmanaged.ParseCA(caSecret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing ca.key")
}

func TestParseCA_Malformed(t *testing.T) {
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-tls-ca", Namespace: "default"},
		Data: map[string][]byte{
			selfmanaged.CAKey:        []byte("garbage"),
			selfmanaged.CAKeyPrivate: []byte("garbage"),
		},
	}
	_, _, err := selfmanaged.ParseCA(caSecret)
	require.Error(t, err)
}

func TestCANeedsRenewal_NilSecret(t *testing.T) {
	need, err := selfmanaged.CANeedsRenewal(nil, certificate.TLSSpec{}, time.Now())
	require.NoError(t, err)
	assert.True(t, need)
}

func TestCANeedsRenewal_MissingCACrt(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{}}
	need, err := selfmanaged.CANeedsRenewal(secret, certificate.TLSSpec{}, time.Now())
	require.NoError(t, err)
	assert.True(t, need)
}

func TestCANeedsRenewal_EmptyCACrt(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CAKey: {}}}
	need, err := selfmanaged.CANeedsRenewal(secret, certificate.TLSSpec{}, time.Now())
	require.NoError(t, err)
	assert.True(t, need)
}

func TestCANeedsRenewal_Malformed(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CAKey: []byte("garbage")}}
	_, err := selfmanaged.CANeedsRenewal(secret, certificate.TLSSpec{}, time.Now())
	require.Error(t, err)
}

func TestCANeedsRenewal_Expired(t *testing.T) {
	now := time.Now()
	certPEM := makeTestCert(t, "test.example.com-ca", now.Add(-48*time.Hour), now.Add(-24*time.Hour))
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CAKey: certPEM}}

	need, err := selfmanaged.CANeedsRenewal(secret, certificate.TLSSpec{RenewalDays: 30}, now)
	require.NoError(t, err)
	assert.True(t, need)
}

func TestCANeedsRenewal_WithinWindow(t *testing.T) {
	now := time.Now()
	certPEM := makeTestCert(t, "test.example.com-ca", now.Add(-1*time.Hour), now.Add(5*24*time.Hour))
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CAKey: certPEM}}

	need, err := selfmanaged.CANeedsRenewal(secret, certificate.TLSSpec{RenewalDays: 30}, now)
	require.NoError(t, err)
	assert.True(t, need)
}

func TestCANeedsRenewal_OutsideWindow(t *testing.T) {
	now := time.Now()
	certPEM := makeTestCert(t, "test.example.com-ca", now.Add(-1*time.Hour), now.Add(365*24*time.Hour))
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CAKey: certPEM}}

	need, err := selfmanaged.CANeedsRenewal(secret, certificate.TLSSpec{RenewalDays: 30}, now)
	require.NoError(t, err)
	assert.False(t, need)
}

func TestCANeedsRenewal_RenewalDaysOverflowClamped(t *testing.T) {
	now := time.Now()
	certPEM := makeTestCert(t, "test.example.com-ca", now.Add(-1*time.Hour), now.Add(1*time.Hour))
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CAKey: certPEM}}

	need, err := selfmanaged.CANeedsRenewal(secret, certificate.TLSSpec{RenewalDays: 1 << 62}, now)
	require.NoError(t, err)
	assert.True(t, need, "clamped renewal window must still trigger renewal for a near-expiry CA")
}

func TestDesiredLeafWithCA(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}
	spec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com"},
		Organization: "TestOrg",
	}
	caSecret, oldLeaf := testSelfManagedObjects(t, spec)

	newLeaf, err := backend.DesiredLeafWithCA(context.Background(), o, spec, caSecret)
	require.NoError(t, err)
	assert.Equal(t, "test-tls", newLeaf.Name)
	assert.Equal(t, "default", newLeaf.Namespace)
	assert.Equal(t, corev1.SecretTypeTLS, newLeaf.Type)
	// ca.crt is a single CA (no bundle).
	assert.Equal(t, caSecret.Data[selfmanaged.CAKey], newLeaf.Data[selfmanaged.CAKey])
	// Fresh leaf key differs from the old leaf's key.
	assert.NotEqual(t, oldLeaf.Data[selfmanaged.KeyKey], newLeaf.Data[selfmanaged.KeyKey])

	caCert, _, err := selfmanaged.ParseCA(caSecret)
	require.NoError(t, err)
	leafCert := parseLeafCert(t, newLeaf)
	require.NoError(t, leafCert.CheckSignatureFrom(caCert))
}

func TestDesiredLeafWithCA_MissingCAKey(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com"}

	caSecret, _, _, err := selfmanaged.BuildCA("default", spec.SecretName, spec)
	require.NoError(t, err)
	delete(caSecret.Data, selfmanaged.CAKeyPrivate)

	_, err = backend.DesiredLeafWithCA(context.Background(), o, spec, caSecret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing ca.key")
}

func leafNeedsChange(t *testing.T, leafSecret *corev1.Secret, spec certificate.TLSSpec, now time.Time) certificate.LeafChange {
	t.Helper()
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}
	chg, err := backend.LeafNeedsChange(context.Background(), o, leafSecret, spec, now)
	require.NoError(t, err)
	return chg
}

func TestLeafNeedsChange_NilSecret(t *testing.T) {
	chg := leafNeedsChange(t, nil, certificate.TLSSpec{}, time.Now())
	assert.Equal(t, certificate.LeafMissing, chg.Reason)
}

func TestLeafNeedsChange_MissingTLSCrt(t *testing.T) {
	chg := leafNeedsChange(t, &corev1.Secret{Data: map[string][]byte{}}, certificate.TLSSpec{}, time.Now())
	assert.Equal(t, certificate.LeafMissing, chg.Reason)
}

func TestLeafNeedsChange_MalformedPEM(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}
	leaf := &corev1.Secret{Data: map[string][]byte{selfmanaged.CertKey: []byte("not pem")}}
	_, err := backend.LeafNeedsChange(context.Background(), o, leaf, certificate.TLSSpec{}, time.Now())
	require.Error(t, err)
}

func TestLeafNeedsChange_NoChange(t *testing.T) {
	spec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com"},
		Organization: "TestOrg",
	}
	_, leaf := testSelfManagedObjects(t, spec)
	chg := leafNeedsChange(t, leaf, spec, time.Now())
	assert.True(t, chg.IsZero())
	assert.Equal(t, certificate.LeafNone, chg.Reason)
}

func TestLeafNeedsChange_Expiring(t *testing.T) {
	spec := certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "test.example.com",
		DNSNames:         []string{"test.example.com"},
		Organization:     "TestOrg",
		LeafValidityDays: 365,
	}
	_, leaf := testSelfManagedObjects(t, spec)
	chg := leafNeedsChange(t, leaf, spec, time.Now().Add(340*24*time.Hour))
	assert.Equal(t, certificate.LeafExpiring, chg.Reason)
}

func TestLeafNeedsChange_CNChanged(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com"},
		Organization: "TestOrg",
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.CommonName = "other.example.com"
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafCNChanged, chg.Reason)
}

func TestLeafNeedsChange_OrgChanged(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com"},
		Organization: "TestOrg",
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.Organization = "OtherOrg"
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafOrgChanged, chg.Reason)
}

func TestLeafNeedsChange_SANAdded(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com"},
		Organization: "TestOrg",
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.DNSNames = []string{"test.example.com", "new.example.com"}
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafSANsChanged, chg.Reason)
	assert.Equal(t, []string{"new.example.com"}, chg.SANsAdded)
	assert.Empty(t, chg.SANsRemoved)
}

func TestLeafNeedsChange_SANRemoved(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com", "extra.example.com"},
		Organization: "TestOrg",
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.DNSNames = []string{"test.example.com"}
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafSANsChanged, chg.Reason)
	assert.Empty(t, chg.SANsAdded)
	assert.Equal(t, []string{"extra.example.com"}, chg.SANsRemoved)
}

func TestLeafNeedsChange_SANAddAndRemove(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"a.example.com", "b.example.com"},
		Organization: "TestOrg",
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.DNSNames = []string{"b.example.com", "c.example.com"}
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafSANsChanged, chg.Reason)
	assert.Equal(t, []string{"c.example.com"}, chg.SANsAdded)
	assert.Equal(t, []string{"a.example.com"}, chg.SANsRemoved)
}

func TestLeafNeedsChange_IPAdded(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com"},
		Organization: "TestOrg",
		IPAddresses:  []string{"10.0.0.1"},
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.IPAddresses = []string{"10.0.0.1", "10.0.0.2"}
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafIPsChanged, chg.Reason)
	assert.Equal(t, []string{"10.0.0.2"}, chg.IPsAdded)
	assert.Empty(t, chg.IPsRemoved)
}

func TestLeafNeedsChange_IPRemoved(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com"},
		Organization: "TestOrg",
		IPAddresses:  []string{"10.0.0.1", "10.0.0.2"},
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.IPAddresses = []string{"10.0.0.1"}
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafIPsChanged, chg.Reason)
	assert.Empty(t, chg.IPsAdded)
	assert.Equal(t, []string{"10.0.0.2"}, chg.IPsRemoved)
}

func TestLeafNeedsChange_MultiPEM(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com"},
		Organization: "TestOrg",
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)

	// Append a second, unrelated certificate. LeafNeedsChange must use the
	// first CERTIFICATE block only.
	extra := makeTestCert(t, "unrelated.example.com", time.Now().Add(-1*time.Hour), time.Now().Add(24*time.Hour))
	leaf.Data[selfmanaged.CertKey] = append(leaf.Data[selfmanaged.CertKey], extra...)

	chg := leafNeedsChange(t, leaf, buildSpec, time.Now())
	assert.True(t, chg.IsZero())
}

func TestLeafNeedsChange_ExpiringDominatesSANRemoval(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "test.example.com",
		DNSNames:         []string{"test.example.com", "extra.example.com"},
		Organization:     "TestOrg",
		LeafValidityDays: 365,
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.DNSNames = []string{"test.example.com"}

	chg := leafNeedsChange(t, leaf, driftSpec, time.Now().Add(340*24*time.Hour))
	assert.Equal(t, certificate.LeafExpiring, chg.Reason)
	assert.Equal(t, []string{"extra.example.com"}, chg.SANsRemoved)
}

func TestBuildCASigner_ECDSADefault(t *testing.T) {
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com"}
	caSecret, signer, caCert, err := selfmanaged.BuildCASigner("default", spec.SecretName, spec)
	require.NoError(t, err)
	assert.Equal(t, "test-tls-ca", caSecret.Name)
	assert.Equal(t, "test.example.com-ca", caCert.Subject.CommonName)
	assert.Empty(t, caCert.Subject.Organization)

	block, _ := pem.Decode(caSecret.Data[selfmanaged.CAKeyPrivate])
	require.NotNil(t, block)
	assert.Equal(t, "EC PRIVATE KEY", block.Type)

	_, ok := signer.(*ecdsa.PrivateKey)
	assert.True(t, ok, "default signer should be ECDSA")
}

func TestBuildCASigner_RSA(t *testing.T) {
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", KeyAlgorithm: certificate.KeyAlgorithmRSA}
	caSecret, signer, caCert, err := selfmanaged.BuildCASigner("default", spec.SecretName, spec)
	require.NoError(t, err)
	assert.Equal(t, x509.RSA, caCert.PublicKeyAlgorithm)

	block, _ := pem.Decode(caSecret.Data[selfmanaged.CAKeyPrivate])
	require.NotNil(t, block)
	assert.Equal(t, "RSA PRIVATE KEY", block.Type)

	_, ok := signer.(*rsa.PrivateKey)
	assert.True(t, ok, "RSA signer should be *rsa.PrivateKey")
}

func TestBuildCASigner_CustomCASubject(t *testing.T) {
	spec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		CACommonName: "my-ca",
		CASubject: &certificate.CertificateSubject{
			Organizations: []string{"ca-org"},
			Countries:     []string{"FR"},
		},
	}
	_, _, caCert, err := selfmanaged.BuildCASigner("default", spec.SecretName, spec)
	require.NoError(t, err)
	assert.Equal(t, "my-ca", caCert.Subject.CommonName)
	assert.Equal(t, []string{"ca-org"}, caCert.Subject.Organization)
	assert.Equal(t, []string{"FR"}, caCert.Subject.Country)
}

func TestBuildCASigner_InvalidContent(t *testing.T) {
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "x", KeyAlgorithm: "bogus"}
	_, _, _, err := selfmanaged.BuildCASigner("default", spec.SecretName, spec)
	require.Error(t, err)
}

func TestBuildCA_RSAReturnsError(t *testing.T) {
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", KeyAlgorithm: certificate.KeyAlgorithmRSA}
	_, _, _, err := selfmanaged.BuildCA("default", spec.SecretName, spec)
	require.Error(t, err)
}

func signLeafSignerHelper(t *testing.T, spec certificate.TLSSpec) ([]byte, []byte, *x509.Certificate, crypto.Signer) {
	t.Helper()
	caSpec := certificate.TLSSpec{CommonName: "ca.example.com"}
	_, caSigner, caCert, err := selfmanaged.BuildCASigner("default", "test-tls", caSpec)
	require.NoError(t, err)
	certPEM, keyPEM, err := selfmanaged.SignLeafSigner(caCert, caSigner, spec)
	require.NoError(t, err)
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	leaf, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return certPEM, keyPEM, leaf, caSigner
}

func TestSignLeafSigner_CustomSubject(t *testing.T) {
	spec := certificate.TLSSpec{
		CommonName: "leaf.example.com",
		Subject: certificate.CertificateSubject{
			Organizations:       []string{"org1", "org2"},
			OrganizationalUnits: []string{"ou"},
		},
	}
	_, _, leaf, _ := signLeafSignerHelper(t, spec)
	assert.Equal(t, "leaf.example.com", leaf.Subject.CommonName)
	assert.Equal(t, []string{"org1", "org2"}, leaf.Subject.Organization)
	assert.Equal(t, []string{"ou"}, leaf.Subject.OrganizationalUnit)
}

func TestSignLeafSigner_CustomUsages(t *testing.T) {
	spec := certificate.TLSSpec{
		CommonName: "leaf.example.com",
		Usages:     []string{certificate.UsageServerAuth, certificate.UsageCodeSigning},
		KeyUsages:  []string{certificate.KeyUsageDigitalSignature},
	}
	_, _, leaf, _ := signLeafSignerHelper(t, spec)
	assert.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageCodeSigning}, leaf.ExtKeyUsage)
	assert.Equal(t, x509.KeyUsageDigitalSignature, leaf.KeyUsage)
}

func TestSignLeafSigner_RSA(t *testing.T) {
	spec := certificate.TLSSpec{CommonName: "leaf.example.com", KeyAlgorithm: certificate.KeyAlgorithmRSA}
	_, keyPEM, leaf, _ := signLeafSignerHelper(t, spec)
	assert.Equal(t, x509.RSA, leaf.PublicKeyAlgorithm)
	assert.Contains(t, string(keyPEM), "BEGIN RSA PRIVATE KEY")
}

func TestSignLeafSigner_CustomKeySize(t *testing.T) {
	spec := certificate.TLSSpec{CommonName: "leaf.example.com", KeySize: 384}
	_, _, leaf, _ := signLeafSignerHelper(t, spec)
	ecKey, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	require.True(t, ok)
	assert.Equal(t, elliptic.P384(), ecKey.Curve)
}

func TestSignLeafSigner_SANs(t *testing.T) {
	spec := certificate.TLSSpec{
		CommonName:  "leaf.example.com",
		DNSNames:    []string{"leaf.example.com", "alt.example.com"},
		IPAddresses: []string{"10.0.0.1"},
	}
	_, _, leaf, _ := signLeafSignerHelper(t, spec)
	assert.Equal(t, []string{"leaf.example.com", "alt.example.com"}, leaf.DNSNames)
	require.Len(t, leaf.IPAddresses, 1)
	assert.True(t, leaf.IPAddresses[0].Equal(net.ParseIP("10.0.0.1")))
}

func TestSignLeafSigner_DedupSANs(t *testing.T) {
	spec := certificate.TLSSpec{
		CommonName:  "leaf.example.com",
		DNSNames:    []string{"a.example.com", "b.example.com", "a.example.com"},
		IPAddresses: []string{"10.0.0.1", "10.0.0.2", "10.0.0.1"},
	}
	_, _, leaf, _ := signLeafSignerHelper(t, spec)
	assert.Equal(t, []string{"a.example.com", "b.example.com"}, leaf.DNSNames)
	require.Len(t, leaf.IPAddresses, 2)
	assert.True(t, leaf.IPAddresses[0].Equal(net.ParseIP("10.0.0.1")))
	assert.True(t, leaf.IPAddresses[1].Equal(net.ParseIP("10.0.0.2")))
}

func TestSignLeafSigner_InvalidUsage(t *testing.T) {
	spec := certificate.TLSSpec{CommonName: "leaf.example.com", Usages: []string{"bogus"}}
	caSpec := certificate.TLSSpec{CommonName: "ca.example.com"}
	_, caSigner, caCert, err := selfmanaged.BuildCASigner("default", "test-tls", caSpec)
	require.NoError(t, err)
	_, _, err = selfmanaged.SignLeafSigner(caCert, caSigner, spec)
	require.Error(t, err)
}

func TestParseCASigner_EC(t *testing.T) {
	caSecret, _, caCert, err := selfmanaged.BuildCASigner("default", "test-tls", certificate.TLSSpec{CommonName: "test.example.com"})
	require.NoError(t, err)
	parsedCert, signer, err := selfmanaged.ParseCASigner(caSecret)
	require.NoError(t, err)
	assert.Equal(t, caCert.Raw, parsedCert.Raw)
	_, ok := signer.(*ecdsa.PrivateKey)
	assert.True(t, ok)
}

func TestParseCASigner_RSA(t *testing.T) {
	caSecret, _, caCert, err := selfmanaged.BuildCASigner("default", "test-tls", certificate.TLSSpec{CommonName: "test.example.com", KeyAlgorithm: certificate.KeyAlgorithmRSA})
	require.NoError(t, err)
	parsedCert, signer, err := selfmanaged.ParseCASigner(caSecret)
	require.NoError(t, err)
	assert.Equal(t, caCert.Raw, parsedCert.Raw)
	_, ok := signer.(*rsa.PrivateKey)
	assert.True(t, ok)
}

func TestParseCASigner_MismatchedKeyCert(t *testing.T) {
	caSecret, _, _, err := selfmanaged.BuildCASigner("default", "test-tls", certificate.TLSSpec{CommonName: "test.example.com"})
	require.NoError(t, err)

	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalECPrivateKey(otherKey)
	require.NoError(t, err)
	caSecret.Data[selfmanaged.CAKeyPrivate] = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

	_, _, err = selfmanaged.ParseCASigner(caSecret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
}

func TestCAContentChanged(t *testing.T) {
	t.Run("nil secret", func(t *testing.T) {
		changed, err := selfmanaged.CAContentChanged(nil, certificate.TLSSpec{})
		require.NoError(t, err)
		assert.True(t, changed)
	})
	t.Run("missing ca.crt", func(t *testing.T) {
		changed, err := selfmanaged.CAContentChanged(&corev1.Secret{Data: map[string][]byte{}}, certificate.TLSSpec{})
		require.NoError(t, err)
		assert.True(t, changed)
	})
	t.Run("subject differs", func(t *testing.T) {
		caSecret, _, _, err := selfmanaged.BuildCASigner("default", "test-tls", certificate.TLSSpec{CommonName: "test.example.com"})
		require.NoError(t, err)
		changed, err := selfmanaged.CAContentChanged(caSecret, certificate.TLSSpec{CommonName: "other.example.com"})
		require.NoError(t, err)
		assert.True(t, changed)
	})
	t.Run("algorithm differs", func(t *testing.T) {
		caSecret, _, _, err := selfmanaged.BuildCASigner("default", "test-tls", certificate.TLSSpec{CommonName: "test.example.com"})
		require.NoError(t, err)
		changed, err := selfmanaged.CAContentChanged(caSecret, certificate.TLSSpec{CommonName: "test.example.com", KeyAlgorithm: certificate.KeyAlgorithmRSA})
		require.NoError(t, err)
		assert.True(t, changed)
	})
	t.Run("ECDSA curve change", func(t *testing.T) {
		caSecret, _, _, err := selfmanaged.BuildCASigner("default", "test-tls", certificate.TLSSpec{CommonName: "test.example.com"})
		require.NoError(t, err)
		changed, err := selfmanaged.CAContentChanged(caSecret, certificate.TLSSpec{CommonName: "test.example.com", KeySize: 384})
		require.NoError(t, err)
		assert.True(t, changed, "an ECDSA curve change must trigger CA rotation, not be silently ignored")
	})
	t.Run("RSA key size change", func(t *testing.T) {
		spec := certificate.TLSSpec{CommonName: "test.example.com", KeyAlgorithm: certificate.KeyAlgorithmRSA, KeySize: 2048}
		caSecret, _, _, err := selfmanaged.BuildCASigner("default", "test-tls", spec)
		require.NoError(t, err)
		upgraded := spec
		upgraded.KeySize = 4096
		changed, err := selfmanaged.CAContentChanged(caSecret, upgraded)
		require.NoError(t, err)
		assert.True(t, changed, "an RSA key size change must trigger CA rotation, not be silently ignored")
	})
	t.Run("identical", func(t *testing.T) {
		spec := certificate.TLSSpec{CommonName: "test.example.com", Organization: "TestOrg"}
		caSecret, _, _, err := selfmanaged.BuildCASigner("default", "test-tls", spec)
		require.NoError(t, err)
		changed, err := selfmanaged.CAContentChanged(caSecret, spec)
		require.NoError(t, err)
		assert.False(t, changed)
	})
}

func TestLeafNeedsChange_SubjectChanged(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		DNSNames:   []string{"test.example.com"},
		Subject:    certificate.CertificateSubject{Countries: []string{"FR"}},
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.Subject.Countries = []string{"US"}
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafSubjectChanged, chg.Reason)
}

func TestLeafNeedsChange_KeyChanged(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		DNSNames:   []string{"test.example.com"},
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.KeySize = 384
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafKeyChanged, chg.Reason)
}

func TestLeafNeedsChange_UsagesChanged(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		DNSNames:   []string{"test.example.com"},
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.Usages = []string{certificate.UsageServerAuth}
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafUsagesChanged, chg.Reason)
}

func TestLeafNeedsChange_SubjectDominatesSAN(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		DNSNames:   []string{"test.example.com"},
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.DNSNames = []string{"test.example.com", "new.example.com"}
	driftSpec.Subject.Countries = []string{"US"}
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafSubjectChanged, chg.Reason)
}

func TestLeafNeedsChange_KeyChanged_Algorithm(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		DNSNames:   []string{"test.example.com"},
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.KeyAlgorithm = certificate.KeyAlgorithmRSA
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafKeyChanged, chg.Reason)
}

func TestLeafNeedsChange_UsagesChanged_KeyUsage(t *testing.T) {
	buildSpec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		DNSNames:   []string{"test.example.com"},
	}
	_, leaf := testSelfManagedObjects(t, buildSpec)
	driftSpec := buildSpec
	driftSpec.KeyUsages = []string{certificate.KeyUsageDigitalSignature}
	chg := leafNeedsChange(t, leaf, driftSpec, time.Now())
	assert.Equal(t, certificate.LeafUsagesChanged, chg.Reason)
}

func TestParseCA_RSAReturnsError(t *testing.T) {
	caSecret, _, _, err := selfmanaged.BuildCASigner("default", "test-tls", certificate.TLSSpec{CommonName: "test.example.com", KeyAlgorithm: certificate.KeyAlgorithmRSA})
	require.NoError(t, err)
	_, _, err = selfmanaged.ParseCA(caSecret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not an EC private key")
}

func TestParseCASigner_PKCS8PrivateKey(t *testing.T) {
	caSecret, signer, caCert, err := selfmanaged.BuildCASigner("default", "test-tls", certificate.TLSSpec{CommonName: "test.example.com"})
	require.NoError(t, err)
	ecKey := signer.(*ecdsa.PrivateKey)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(ecKey)
	require.NoError(t, err)
	caSecret.Data[selfmanaged.CAKeyPrivate] = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})

	parsedCert, parsedSigner, err := selfmanaged.ParseCASigner(caSecret)
	require.NoError(t, err)
	assert.Equal(t, caCert.Raw, parsedCert.Raw)
	_, ok := parsedSigner.(*ecdsa.PrivateKey)
	assert.True(t, ok)
}

func TestParseCASigner_UnknownPEMType(t *testing.T) {
	caSecret, _, _, err := selfmanaged.BuildCASigner("default", "test-tls", certificate.TLSSpec{CommonName: "test.example.com"})
	require.NoError(t, err)
	caSecret.Data[selfmanaged.CAKeyPrivate] = pem.EncodeToMemory(&pem.Block{Type: "FOO KEY", Bytes: []byte("x")})

	_, _, err = selfmanaged.ParseCASigner(caSecret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported private key PEM type")
}
