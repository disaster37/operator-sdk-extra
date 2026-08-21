package tlsconfig_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/tlsconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateCertPEM(t *testing.T, cn string, isCA bool) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}
	if isCA {
		tmpl.KeyUsage = x509.KeyUsageCertSign
	} else {
		tmpl.KeyUsage = x509.KeyUsageDigitalSignature
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

func TestCertificateOptionsIsZero(t *testing.T) {
	assert.True(t, (tlsconfig.CertificateOptions{}).IsZero())
	assert.False(t, (tlsconfig.CertificateOptions{CACert: []byte("x")}).IsZero())
	assert.False(t, (tlsconfig.CertificateOptions{InsecureSkipVerify: true}).IsZero())
	assert.False(t, (tlsconfig.CertificateOptions{ServerName: "s"}).IsZero())
}

func TestCertificateOptionsValidate(t *testing.T) {
	t.Run("nil is valid", func(t *testing.T) {
		var o *tlsconfig.CertificateOptions
		require.NoError(t, o.Validate())
	})
	t.Run("zero is valid", func(t *testing.T) {
		require.NoError(t, (&tlsconfig.CertificateOptions{}).Validate())
	})
	t.Run("ca path and inline conflict", func(t *testing.T) {
		err := (&tlsconfig.CertificateOptions{CAPath: "p", CACert: []byte("x")}).Validate()
		require.Error(t, err)
	})
	t.Run("client cert path and inline conflict", func(t *testing.T) {
		err := (&tlsconfig.CertificateOptions{ClientCertPath: "p", ClientCert: []byte("x")}).Validate()
		require.Error(t, err)
	})
	t.Run("client key path and inline conflict", func(t *testing.T) {
		err := (&tlsconfig.CertificateOptions{ClientKeyPath: "p", ClientKey: []byte("x")}).Validate()
		require.Error(t, err)
	})
	t.Run("inline client cert without key", func(t *testing.T) {
		err := (&tlsconfig.CertificateOptions{ClientCert: []byte("x")}).Validate()
		require.Error(t, err)
	})
	t.Run("inline client key without cert", func(t *testing.T) {
		err := (&tlsconfig.CertificateOptions{ClientKey: []byte("x")}).Validate()
		require.Error(t, err)
	})
	t.Run("path client cert without key", func(t *testing.T) {
		err := (&tlsconfig.CertificateOptions{ClientCertPath: "p"}).Validate()
		require.Error(t, err)
	})
	t.Run("min version exceeds max", func(t *testing.T) {
		err := (&tlsconfig.CertificateOptions{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS12}).Validate()
		require.Error(t, err)
	})
	t.Run("TLS 1.0 min version rejected", func(t *testing.T) {
		err := (&tlsconfig.CertificateOptions{MinVersion: tls.VersionTLS10}).Validate()
		require.Error(t, err)
	})
	t.Run("TLS 1.1 min version rejected", func(t *testing.T) {
		err := (&tlsconfig.CertificateOptions{MinVersion: tls.VersionTLS11}).Validate()
		require.Error(t, err)
	})
	t.Run("unknown version rejected", func(t *testing.T) {
		err := (&tlsconfig.CertificateOptions{MaxVersion: 0xdead}).Validate()
		require.Error(t, err)
	})
	t.Run("TLS 1.2 and 1.3 accepted", func(t *testing.T) {
		require.NoError(t, (&tlsconfig.CertificateOptions{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS13}).Validate())
	})
}

func TestCertificateOptionsResolve(t *testing.T) {
	t.Run("nil is no-op", func(t *testing.T) {
		var o *tlsconfig.CertificateOptions
		require.NoError(t, o.Resolve())
	})
	t.Run("reads files", func(t *testing.T) {
		dir := t.TempDir()
		caPath := filepath.Join(dir, "ca.crt")
		certPath := filepath.Join(dir, "client.crt")
		keyPath := filepath.Join(dir, "client.key")
		require.NoError(t, os.WriteFile(caPath, []byte("ca-data"), 0o600))
		require.NoError(t, os.WriteFile(certPath, []byte("cert-data"), 0o600))
		require.NoError(t, os.WriteFile(keyPath, []byte("key-data"), 0o600))

		o := &tlsconfig.CertificateOptions{CAPath: caPath, ClientCertPath: certPath, ClientKeyPath: keyPath}
		require.NoError(t, o.Resolve())
		assert.Equal(t, []byte("ca-data"), o.CACert)
		assert.Equal(t, []byte("cert-data"), o.ClientCert)
		assert.Equal(t, []byte("key-data"), o.ClientKey)
		assert.Empty(t, o.CAPath)
		assert.Empty(t, o.ClientCertPath)
		assert.Empty(t, o.ClientKeyPath)
	})
	t.Run("read error", func(t *testing.T) {
		o := &tlsconfig.CertificateOptions{CAPath: filepath.Join(t.TempDir(), "missing.crt")}
		require.Error(t, o.Resolve())
	})
	t.Run("client cert read error", func(t *testing.T) {
		o := &tlsconfig.CertificateOptions{ClientCertPath: filepath.Join(t.TempDir(), "missing.crt"), ClientKeyPath: "key"}
		require.Error(t, o.Resolve())
	})
	t.Run("client key read error", func(t *testing.T) {
		dir := t.TempDir()
		certPath := filepath.Join(dir, "client.crt")
		require.NoError(t, os.WriteFile(certPath, []byte("cert-data"), 0o600))
		o := &tlsconfig.CertificateOptions{ClientCertPath: certPath, ClientKeyPath: filepath.Join(dir, "missing.key")}
		require.Error(t, o.Resolve())
	})
	t.Run("validate error", func(t *testing.T) {
		o := &tlsconfig.CertificateOptions{CAPath: "p", CACert: []byte("x")}
		require.Error(t, o.Resolve())
	})
}

func TestBuildTLSConfig(t *testing.T) {
	caPEM, _ := generateCertPEM(t, "ca", true)
	clientCertPEM, clientKeyPEM := generateCertPEM(t, "client", false)

	t.Run("zero value returns nil", func(t *testing.T) {
		cfg, err := (&tlsconfig.CertificateOptions{}).BuildTLSConfig()
		require.NoError(t, err)
		assert.Nil(t, cfg)
	})
	t.Run("nil receiver returns nil", func(t *testing.T) {
		var o *tlsconfig.CertificateOptions
		cfg, err := o.BuildTLSConfig()
		require.NoError(t, err)
		assert.Nil(t, cfg)
	})
	t.Run("CA pool", func(t *testing.T) {
		cfg, err := (&tlsconfig.CertificateOptions{CACert: caPEM}).BuildTLSConfig()
		require.NoError(t, err)
		require.NotNil(t, cfg.RootCAs)
	})
	t.Run("invalid CA returns error", func(t *testing.T) {
		_, err := (&tlsconfig.CertificateOptions{CACert: []byte("not pem")}).BuildTLSConfig()
		require.Error(t, err)
	})
	t.Run("client certificate", func(t *testing.T) {
		cfg, err := (&tlsconfig.CertificateOptions{ClientCert: clientCertPEM, ClientKey: clientKeyPEM}).BuildTLSConfig()
		require.NoError(t, err)
		require.Len(t, cfg.Certificates, 1)
	})
	t.Run("mismatched client certificate", func(t *testing.T) {
		_, otherKey := generateCertPEM(t, "other", false)
		_, err := (&tlsconfig.CertificateOptions{ClientCert: clientCertPEM, ClientKey: otherKey}).BuildTLSConfig()
		require.Error(t, err)
	})
	t.Run("insecure skip verify and server name", func(t *testing.T) {
		cfg, err := (&tlsconfig.CertificateOptions{InsecureSkipVerify: true, ServerName: "svc"}).BuildTLSConfig()
		require.NoError(t, err)
		assert.True(t, cfg.InsecureSkipVerify)
		assert.Equal(t, "svc", cfg.ServerName)
	})
	t.Run("versions", func(t *testing.T) {
		cfg, err := (&tlsconfig.CertificateOptions{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS13}).BuildTLSConfig()
		require.NoError(t, err)
		assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
		assert.Equal(t, uint16(tls.VersionTLS13), cfg.MaxVersion)
	})
	t.Run("validate error", func(t *testing.T) {
		_, err := (&tlsconfig.CertificateOptions{ClientCert: []byte("x")}).BuildTLSConfig()
		require.Error(t, err)
	})
}

func TestBuildTLSConfig_WithBaseTLSConfig(t *testing.T) {
	base := &tls.Config{MinVersion: tls.VersionTLS12}
	o := &tlsconfig.CertificateOptions{ServerName: "svc"}
	tlsconfig.WithBaseTLSConfig(base)(o)
	cfg, err := o.BuildTLSConfig()
	require.NoError(t, err)
	assert.NotSame(t, base, cfg)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.Equal(t, "svc", cfg.ServerName)
}

func TestBuildTLSConfig_WithBaseTLSConfigOnly(t *testing.T) {
	// A base config with no other customization must still be cloned, not
	// silently dropped by the zero-value early return.
	base := &tls.Config{MinVersion: tls.VersionTLS13}
	o := &tlsconfig.CertificateOptions{}
	tlsconfig.WithBaseTLSConfig(base)(o)
	cfg, err := o.BuildTLSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.NotSame(t, base, cfg)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
}

func TestBuildHTTPTransport(t *testing.T) {
	caPEM, _ := generateCertPEM(t, "ca", true)

	t.Run("zero value returns nil", func(t *testing.T) {
		tr, err := (&tlsconfig.CertificateOptions{}).BuildHTTPTransport()
		require.NoError(t, err)
		assert.Nil(t, tr)
	})
	t.Run("builds transport with TLS config", func(t *testing.T) {
		tr, err := (&tlsconfig.CertificateOptions{CACert: caPEM, InsecureSkipVerify: true}).BuildHTTPTransport()
		require.NoError(t, err)
		require.NotNil(t, tr.TLSClientConfig)
		assert.True(t, tr.TLSClientConfig.InsecureSkipVerify)
		assert.NotNil(t, tr.TLSClientConfig.RootCAs)
	})
	t.Run("error propagates", func(t *testing.T) {
		_, err := (&tlsconfig.CertificateOptions{ClientCert: []byte("x")}).BuildHTTPTransport()
		require.Error(t, err)
	})
	t.Run("with base transport", func(t *testing.T) {
		base := &http.Transport{MaxIdleConns: 42}
		o := &tlsconfig.CertificateOptions{CACert: caPEM}
		tlsconfig.WithBaseTransport(base)(o)
		tr, err := o.BuildHTTPTransport()
		require.NoError(t, err)
		assert.NotSame(t, base, tr)
		assert.Equal(t, 42, tr.MaxIdleConns)
		require.NotNil(t, tr.TLSClientConfig)
	})
	t.Run("base transport only", func(t *testing.T) {
		base := &http.Transport{MaxIdleConns: 99}
		o := &tlsconfig.CertificateOptions{}
		tlsconfig.WithBaseTransport(base)(o)
		tr, err := o.BuildHTTPTransport()
		require.NoError(t, err)
		require.NotNil(t, tr)
		assert.NotSame(t, base, tr)
		assert.Equal(t, 99, tr.MaxIdleConns)
	})
}
