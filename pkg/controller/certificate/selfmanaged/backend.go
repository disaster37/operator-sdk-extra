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
	"time"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/object"
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

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
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

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate leaf key: %w", err)
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
		DNSNames: spec.DNSNames,
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caTemplate, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create leaf certificate: %w", err)
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
