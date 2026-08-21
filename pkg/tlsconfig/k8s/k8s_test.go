package k8s_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/tlsconfig"
	tlsk8s "github.com/disaster37/operator-sdk-extra/v3/pkg/tlsconfig/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func caCertPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func newClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func secret(name, namespace string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data:       data,
	}
}

func TestNew(t *testing.T) {
	r := tlsk8s.New(newClient(t))
	require.NotNil(t, r)
}

func TestOptionsValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		require.NoError(t, (tlsk8s.Options{CASecretName: "ca"}).Validate())
	})
	t.Run("client cert only is valid", func(t *testing.T) {
		require.NoError(t, (tlsk8s.Options{ClientCertSecretName: "client"}).Validate())
	})
	t.Run("empty errors", func(t *testing.T) {
		require.Error(t, (tlsk8s.Options{}).Validate())
	})
	t.Run("skip-verify-only rejected", func(t *testing.T) {
		require.Error(t, (tlsk8s.Options{InsecureSkipVerify: true}).Validate())
	})
}

func TestResolverResolve(t *testing.T) {
	ctx := context.Background()
	namespace := "default"

	t.Run("CA secret with default key", func(t *testing.T) {
		c := newClient(t, secret("ca-secret", namespace, map[string][]byte{"ca.crt": []byte("ca-data")}))
		r := tlsk8s.New(c)
		out, err := r.Resolve(ctx, namespace, tlsk8s.Options{CASecretName: "ca-secret"})
		require.NoError(t, err)
		assert.Equal(t, []byte("ca-data"), out.CACert)
	})
	t.Run("CA secret with custom key", func(t *testing.T) {
		c := newClient(t, secret("ca-secret", namespace, map[string][]byte{"custom": []byte("ca-data")}))
		r := tlsk8s.New(c)
		out, err := r.Resolve(ctx, namespace, tlsk8s.Options{CASecretName: "ca-secret", CACertKey: "custom"})
		require.NoError(t, err)
		assert.Equal(t, []byte("ca-data"), out.CACert)
	})
	t.Run("client cert secret", func(t *testing.T) {
		c := newClient(t, secret("client-secret", namespace, map[string][]byte{"tls.crt": []byte("cert"), "tls.key": []byte("key")}))
		r := tlsk8s.New(c)
		out, err := r.Resolve(ctx, namespace, tlsk8s.Options{ClientCertSecretName: "client-secret"})
		require.NoError(t, err)
		assert.Equal(t, []byte("cert"), out.ClientCert)
		assert.Equal(t, []byte("key"), out.ClientKey)
	})
	t.Run("missing secret", func(t *testing.T) {
		r := tlsk8s.New(newClient(t))
		_, err := r.Resolve(ctx, namespace, tlsk8s.Options{CASecretName: "nope"})
		require.Error(t, err)
	})
	t.Run("missing key", func(t *testing.T) {
		c := newClient(t, secret("ca-secret", namespace, map[string][]byte{"other": []byte("x")}))
		r := tlsk8s.New(c)
		_, err := r.Resolve(ctx, namespace, tlsk8s.Options{CASecretName: "ca-secret"})
		require.Error(t, err)
	})
	t.Run("empty client cert key", func(t *testing.T) {
		c := newClient(t, secret("client-secret", namespace, map[string][]byte{"tls.key": []byte("key")}))
		r := tlsk8s.New(c)
		_, err := r.Resolve(ctx, namespace, tlsk8s.Options{ClientCertSecretName: "client-secret"})
		require.Error(t, err)
	})
	t.Run("empty client key key", func(t *testing.T) {
		c := newClient(t, secret("client-secret", namespace, map[string][]byte{"tls.crt": []byte("cert")}))
		r := tlsk8s.New(c)
		_, err := r.Resolve(ctx, namespace, tlsk8s.Options{ClientCertSecretName: "client-secret"})
		require.Error(t, err)
	})
	t.Run("missing client secret", func(t *testing.T) {
		r := tlsk8s.New(newClient(t))
		_, err := r.Resolve(ctx, namespace, tlsk8s.Options{ClientCertSecretName: "nope"})
		require.Error(t, err)
	})
	t.Run("nil resolver", func(t *testing.T) {
		var r *tlsk8s.Resolver
		_, err := r.Resolve(ctx, namespace, tlsk8s.Options{CASecretName: "ca"})
		require.Error(t, err)
	})
	t.Run("empty options", func(t *testing.T) {
		r := tlsk8s.New(newClient(t))
		_, err := r.Resolve(ctx, namespace, tlsk8s.Options{})
		require.Error(t, err)
	})
}

func TestResolverBuildHTTPTransport(t *testing.T) {
	ctx := context.Background()
	namespace := "default"
	caSecret := secret("ca-secret", namespace, map[string][]byte{"ca.crt": caCertPEM(t)})

	t.Run("builds transport", func(t *testing.T) {
		r := tlsk8s.New(newClient(t, caSecret))
		tr, err := r.BuildHTTPTransport(ctx, namespace, tlsk8s.Options{CASecretName: "ca-secret", InsecureSkipVerify: true})
		require.NoError(t, err)
		require.NotNil(t, tr.TLSClientConfig)
		assert.True(t, tr.TLSClientConfig.InsecureSkipVerify)
	})
	t.Run("applies options", func(t *testing.T) {
		base := &http.Transport{MaxIdleConns: 7}
		r := tlsk8s.New(newClient(t, caSecret))
		tr, err := r.BuildHTTPTransport(ctx, namespace, tlsk8s.Options{CASecretName: "ca-secret"}, tlsconfig.WithBaseTransport(base))
		require.NoError(t, err)
		assert.Equal(t, 7, tr.MaxIdleConns)
	})
	t.Run("error propagates", func(t *testing.T) {
		r := tlsk8s.New(newClient(t))
		_, err := r.BuildHTTPTransport(ctx, namespace, tlsk8s.Options{CASecretName: "nope"})
		require.Error(t, err)
	})
}

func TestResolverBuildHTTPTransport_NoClient(t *testing.T) {
	r := tlsk8s.New(nil)
	_, err := r.BuildHTTPTransport(context.Background(), "default", tlsk8s.Options{CASecretName: "ca"})
	require.Error(t, err)
}
