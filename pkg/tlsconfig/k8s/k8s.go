// Package k8s resolves TLS material (CA and client certificates) from
// Kubernetes Secrets into tlsconfig.CertificateOptions. It imports
// controller-runtime's client.Client and is kept separate from the
// controller-runtime-free tlsconfig package.
package k8s

import (
	"context"
	"net/http"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/tlsconfig"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resolver resolves TLS material from Kubernetes Secrets via a client.Client.
type Resolver struct {
	client client.Client
}

// New creates a Resolver backed by c.
func New(c client.Client) *Resolver {
	return &Resolver{client: c}
}

// Options selects the Secrets that hold the CA and client certificates.
type Options struct {
	// CASecretName is the Secret holding the CA certificate (key CACertKey,
	// default "ca.crt"). It is required unless ClientCertSecretName is set:
	// Validate fails closed and requires at least one of the two. Note that a
	// skip-verify-only config (InsecureSkipVerify=true with neither secret)
	// is rejected by Validate.
	CASecretName string
	// CACertKey is the data key for the CA certificate (default "ca.crt").
	CACertKey string
	// ClientCertSecretName is the Secret holding the client certificate + key
	// (keys ClientCertKey/ClientKeyKey, default "tls.crt"/"tls.key").
	ClientCertSecretName string
	// ClientCertKey is the data key for the client certificate (default "tls.crt").
	ClientCertKey string
	// ClientKeyKey is the data key for the client private key (default "tls.key").
	ClientKeyKey string
	// InsecureSkipVerify disables server certificate verification.
	InsecureSkipVerify bool
	// ServerName is the expected server name for certificate verification.
	ServerName string
	// MinVersion is the minimum TLS version (tls.VersionTLS*).
	MinVersion uint16
	// MaxVersion is the maximum TLS version (tls.VersionTLS*).
	MaxVersion uint16
}

// Validate checks the option combination. It fails closed: at least one of
// CASecretName or ClientCertSecretName must be set. A skip-verify-only config
// (InsecureSkipVerify=true with neither secret) is therefore rejected.
func (o Options) Validate() error {
	if o.CASecretName == "" && o.ClientCertSecretName == "" {
		return errors.New("at least one of caSecretName or clientCertSecretName is required")
	}
	return nil
}

// Resolve reads the referenced Secrets and returns the equivalent
// tlsconfig.CertificateOptions.
func (r *Resolver) Resolve(ctx context.Context, namespace string, opts Options) (*tlsconfig.CertificateOptions, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("k8s resolver requires a client")
	}
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	out := &tlsconfig.CertificateOptions{
		InsecureSkipVerify: opts.InsecureSkipVerify,
		ServerName:         opts.ServerName,
		MinVersion:         opts.MinVersion,
		MaxVersion:         opts.MaxVersion,
	}

	if opts.CASecretName != "" {
		key := opts.CACertKey
		if key == "" {
			key = "ca.crt"
		}
		secret := &corev1.Secret{}
		if err := r.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: opts.CASecretName}, secret); err != nil {
			return nil, errors.Wrapf(err, "get CA secret %q", opts.CASecretName)
		}
		data, ok := secret.Data[key]
		if !ok || len(data) == 0 {
			return nil, errors.Errorf("CA secret %q missing key %q", opts.CASecretName, key)
		}
		out.CACert = data
	}

	if opts.ClientCertSecretName != "" {
		certKey := opts.ClientCertKey
		if certKey == "" {
			certKey = "tls.crt"
		}
		keyKey := opts.ClientKeyKey
		if keyKey == "" {
			keyKey = "tls.key"
		}
		secret := &corev1.Secret{}
		if err := r.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: opts.ClientCertSecretName}, secret); err != nil {
			return nil, errors.Wrapf(err, "get client certificate secret %q", opts.ClientCertSecretName)
		}
		cert, ok := secret.Data[certKey]
		if !ok || len(cert) == 0 {
			return nil, errors.Errorf("client certificate secret %q missing key %q", opts.ClientCertSecretName, certKey)
		}
		key, ok := secret.Data[keyKey]
		if !ok || len(key) == 0 {
			return nil, errors.Errorf("client certificate secret %q missing key %q", opts.ClientCertSecretName, keyKey)
		}
		out.ClientCert = cert
		out.ClientKey = key
	}

	return out, nil
}

// BuildHTTPTransport resolves the Secrets and builds an *http.Transport in one
// step, applying any tlsconfig.Option (e.g. WithBaseTLSConfig/WithBaseTransport)
// before building.
func (r *Resolver) BuildHTTPTransport(ctx context.Context, namespace string, opts Options, options ...tlsconfig.Option) (*http.Transport, error) {
	certOpts, err := r.Resolve(ctx, namespace, opts)
	if err != nil {
		return nil, err
	}
	for _, opt := range options {
		opt(certOpts)
	}
	return certOpts.BuildHTTPTransport()
}
