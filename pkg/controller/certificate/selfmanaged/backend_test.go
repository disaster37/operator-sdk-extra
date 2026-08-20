package selfmanaged_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com", "test-alt.example.com"},
		Organization: "TestOrg",
		ValidityDays: 90,
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
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		GenerateCRL:  true,
		ValidityDays: 90,
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

func TestNeedRenewal_NilSecret(t *testing.T) {
	need, err := selfmanaged.NeedRenewal(nil, certificate.TLSSpec{}, time.Now())
	require.NoError(t, err)
	assert.True(t, need)
}

func TestNeedRenewal_MissingTLSCrt(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{}}
	need, err := selfmanaged.NeedRenewal(secret, certificate.TLSSpec{}, time.Now())
	require.NoError(t, err)
	assert.True(t, need)
}

func TestNeedRenewal_EmptyTLSCrt(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CertKey: {}}}
	need, err := selfmanaged.NeedRenewal(secret, certificate.TLSSpec{}, time.Now())
	require.NoError(t, err)
	assert.True(t, need)
}

func TestNeedRenewal_MalformedPEM(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CertKey: []byte("not pem")}}
	_, err := selfmanaged.NeedRenewal(secret, certificate.TLSSpec{}, time.Now())
	require.Error(t, err)
}

func TestNeedRenewal_Expired(t *testing.T) {
	now := time.Now()
	certPEM := makeTestCert(t, "test.example.com", now.Add(-48*time.Hour), now.Add(24*time.Hour))
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CertKey: certPEM}}

	need, err := selfmanaged.NeedRenewal(secret, certificate.TLSSpec{RenewalDays: 30}, now.Add(48*time.Hour))
	require.NoError(t, err)
	assert.True(t, need)
}

func TestNeedRenewal_WithinWindow(t *testing.T) {
	now := time.Now()
	certPEM := makeTestCert(t, "test.example.com", now.Add(-1*time.Hour), now.Add(365*24*time.Hour))
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CertKey: certPEM}}

	need, err := selfmanaged.NeedRenewal(secret, certificate.TLSSpec{RenewalDays: 30}, now.Add(355*24*time.Hour))
	require.NoError(t, err)
	assert.True(t, need)
}

func TestNeedRenewal_OutsideWindow(t *testing.T) {
	now := time.Now()
	certPEM := makeTestCert(t, "test.example.com", now.Add(-1*time.Hour), now.Add(365*24*time.Hour))
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CertKey: certPEM}}

	need, err := selfmanaged.NeedRenewal(secret, certificate.TLSSpec{RenewalDays: 30}, now.Add(265*24*time.Hour))
	require.NoError(t, err)
	assert.False(t, need)
}

func TestNeedRenewal_CANearExpiry(t *testing.T) {
	now := time.Now()
	// Leaf far from expiry.
	leafPEM := makeTestCert(t, "test.example.com", now.Add(-1*time.Hour), now.Add(365*24*time.Hour))
	// CA near expiry (within the 30-day window).
	caPEM := makeTestCert(t, "test.example.com-ca", now.Add(-1*time.Hour), now.Add(5*24*time.Hour))
	secret := &corev1.Secret{Data: map[string][]byte{
		selfmanaged.CertKey: leafPEM,
		selfmanaged.CAKey:   caPEM,
	}}

	need, err := selfmanaged.NeedRenewal(secret, certificate.TLSSpec{RenewalDays: 30}, now)
	require.NoError(t, err)
	assert.True(t, need)
}

func TestNeedRenewal_CABadButAbsent(t *testing.T) {
	now := time.Now()
	leafPEM := makeTestCert(t, "test.example.com", now.Add(-1*time.Hour), now.Add(365*24*time.Hour))
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CertKey: leafPEM}}

	need, err := selfmanaged.NeedRenewal(secret, certificate.TLSSpec{RenewalDays: 30}, now)
	require.NoError(t, err)
	assert.False(t, need)
}

func TestNeedRenewal_MalformedCA(t *testing.T) {
	now := time.Now()
	leafPEM := makeTestCert(t, "test.example.com", now.Add(-1*time.Hour), now.Add(365*24*time.Hour))
	secret := &corev1.Secret{Data: map[string][]byte{
		selfmanaged.CertKey: leafPEM,
		selfmanaged.CAKey:   []byte("garbage"),
	}}

	_, err := selfmanaged.NeedRenewal(secret, certificate.TLSSpec{RenewalDays: 30}, now)
	require.Error(t, err)
}

func TestNeedRenewal_NonCertificateBlock(t *testing.T) {
	block := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("x")})
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CertKey: block}}

	_, err := selfmanaged.NeedRenewal(secret, certificate.TLSSpec{}, time.Now())
	require.Error(t, err)
}

func TestNeedRenewal_BadDERCertificate(t *testing.T) {
	block := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad der")})
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CertKey: block}}

	_, err := selfmanaged.NeedRenewal(secret, certificate.TLSSpec{}, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse certificate")
}

// TestNeedRenewal_RenewalDaysOverflowClamped verifies that an absurdly large
// RenewalDays (which would overflow time.Duration and yield a negative
// renewal window, silently disabling renewal) is clamped by
// GetValidRenewalDays so NeedRenewal behaves sanely: a near-expiry cert still
// reports renewal needed (no silent "never renew"), and the call does not
// panic.
func TestNeedRenewal_RenewalDaysOverflowClamped(t *testing.T) {
	now := time.Now()
	// Cert expiring in 1 hour.
	certPEM := makeTestCert(t, "test.example.com", now.Add(-1*time.Hour), now.Add(1*time.Hour))
	secret := &corev1.Secret{Data: map[string][]byte{selfmanaged.CertKey: certPEM}}

	// Near-max int would overflow `RenewalDays * 24 * time.Hour` to a negative
	// window without clamping, making NeedRenewal return false (never renew).
	need, err := selfmanaged.NeedRenewal(secret, certificate.TLSSpec{RenewalDays: 1 << 62}, now)
	require.NoError(t, err)
	assert.True(t, need, "clamped renewal window must still trigger renewal for a near-expiry cert")
}
