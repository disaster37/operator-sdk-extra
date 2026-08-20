package rotation

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func makeCertPEMInternal(t *testing.T, cn string, notBefore, notAfter time.Time) []byte {
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

func TestSplitCAAndLeaf(t *testing.T) {
	ca := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "test-tls-ca", Namespace: "default"}}
	leaf := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "test-tls", Namespace: "default"}}

	// Wrong order: split by name.
	gotCA, gotLeaf, err := splitCAAndLeaf([]client.Object{leaf, ca}, "test-tls-ca", "test-tls")
	require.NoError(t, err)
	assert.Same(t, ca, gotCA)
	assert.Same(t, leaf, gotLeaf)

	// Missing leaf.
	_, _, err = splitCAAndLeaf([]client.Object{ca}, "test-tls-ca", "test-tls")
	require.Error(t, err)

	// Non-Secret object ignored; missing leaf still errors.
	_, _, err = splitCAAndLeaf([]client.Object{ca, &corev1.ConfigMap{}}, "test-tls-ca", "test-tls")
	require.Error(t, err)
}

func TestFirstCert(t *testing.T) {
	now := time.Now()
	validPEM := makeCertPEMInternal(t, "test.example.com", now.Add(-1*time.Hour), now.Add(24*time.Hour))

	assert.NotNil(t, firstCert(validPEM))
	assert.Nil(t, firstCert(nil))
	assert.Nil(t, firstCert([]byte("not pem")))
	keyBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("x")})
	assert.Nil(t, firstCert(keyBlock))
	assert.Nil(t, firstCert(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad der")})))
}
