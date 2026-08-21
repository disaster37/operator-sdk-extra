// Package tlsconfig provides a generic, app-agnostic client-side TLS
// configuration builder. It has no controller-runtime or client-library
// dependencies (stdlib + emperror.dev/errors only); operators feed the
// resulting *tls.Config or *http.Transport into whichever HTTP client
// library they use.
package tlsconfig

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"

	"emperror.dev/errors"
)

// CertificateOptions describes the desired client-side TLS material: trusted
// CA certificate(s), an optional client certificate + key, and common TLS
// knobs. CA and client material may be supplied inline (PEM bytes) or via file
// paths resolved by Resolve. The zero value means "no customization".
type CertificateOptions struct {
	// CACert is the PEM-encoded trusted CA certificate (or bundle).
	CACert []byte
	// CAPath is a path to a PEM-encoded CA file, resolved by Resolve. Paths
	// must come from operator-controlled configuration only (never untrusted
	// input): Resolve reads whatever file the path points at.
	CAPath string
	// ClientCert is the PEM-encoded client certificate.
	ClientCert []byte
	// ClientKey is the PEM-encoded client private key.
	ClientKey []byte
	// ClientCertPath is a path to the client certificate file, resolved by
	// Resolve. Paths must come from operator-controlled configuration only
	// (never untrusted input).
	ClientCertPath string
	// ClientKeyPath is a path to the client private-key file, resolved by
	// Resolve. Paths must come from operator-controlled configuration only
	// (never untrusted input).
	ClientKeyPath string
	// InsecureSkipVerify disables server certificate verification. When true,
	// Go's TLS stack skips verification entirely, so any CA certificates set
	// via CACert/CAPath are ignored for verification (RootCAs is unused while
	// InsecureSkipVerify is set).
	InsecureSkipVerify bool
	// ServerName is the expected server name for certificate verification.
	ServerName string
	// MinVersion is the minimum TLS version (tls.VersionTLS*).
	MinVersion uint16
	// MaxVersion is the maximum TLS version (tls.VersionTLS*).
	MaxVersion uint16

	baseTLSConfig *tls.Config
	baseTransport *http.Transport
}

// Option mutates a CertificateOptions.
type Option func(*CertificateOptions)

// WithBaseTLSConfig sets the base *tls.Config that BuildTLSConfig clones and
// layers the options onto.
func WithBaseTLSConfig(base *tls.Config) Option {
	return func(o *CertificateOptions) { o.baseTLSConfig = base }
}

// WithBaseTransport sets the base *http.Transport that BuildHTTPTransport
// clones and layers the TLS config onto.
func WithBaseTransport(base *http.Transport) Option {
	return func(o *CertificateOptions) { o.baseTransport = base }
}

// IsZero reports whether the options request no customization. A base TLS
// config or transport (set via WithBaseTLSConfig/WithBaseTransport) counts as
// customization, so BuildTLSConfig/BuildHTTPTransport clone it rather than
// returning (nil, nil).
func (o CertificateOptions) IsZero() bool {
	return len(o.CACert) == 0 && o.CAPath == "" &&
		len(o.ClientCert) == 0 && len(o.ClientKey) == 0 &&
		o.ClientCertPath == "" && o.ClientKeyPath == "" &&
		!o.InsecureSkipVerify && o.ServerName == "" &&
		o.MinVersion == 0 && o.MaxVersion == 0 &&
		o.baseTLSConfig == nil && o.baseTransport == nil
}

// Validate checks the option combination. Nil receiver is valid (no-op).
func (o *CertificateOptions) Validate() error {
	if o == nil {
		return nil
	}
	if o.CAPath != "" && len(o.CACert) > 0 {
		return errors.New("cannot set both caPath and caCert")
	}
	if o.ClientCertPath != "" && len(o.ClientCert) > 0 {
		return errors.New("cannot set both clientCertPath and clientCert")
	}
	if o.ClientKeyPath != "" && len(o.ClientKey) > 0 {
		return errors.New("cannot set both clientKeyPath and clientKey")
	}
	if (len(o.ClientCert) > 0) != (len(o.ClientKey) > 0) {
		return errors.New("clientCert and clientKey must be set together")
	}
	if (o.ClientCertPath != "") != (o.ClientKeyPath != "") {
		return errors.New("clientCertPath and clientKeyPath must be set together")
	}
	if o.MinVersion != 0 && o.MaxVersion != 0 && o.MinVersion > o.MaxVersion {
		return errors.New("minVersion must not exceed maxVersion")
	}
	if err := validateTLSVersion(o.MinVersion); err != nil {
		return err
	}
	if err := validateTLSVersion(o.MaxVersion); err != nil {
		return err
	}
	return nil
}

// validateTLSVersion rejects TLS versions below 1.2 (which are deprecated by
// RFC 8996 and broken) and any unknown version value. 0 means "unset" and is
// allowed (the Go default minimum is TLS 1.2 on current toolchains). This
// prevents a caller from silently downgrading a connection to TLS 1.0/1.1, and
// fails fast on garbage version constants that would otherwise only surface as
// an obscure handshake error at runtime.
func validateTLSVersion(v uint16) error {
	switch v {
	case 0:
		return nil
	case tls.VersionTLS12, tls.VersionTLS13:
		return nil
	default:
		return errors.Errorf("unsupported TLS version 0x%04x: only TLS 1.2 and TLS 1.3 are supported", v)
	}
}

// Resolve loads file-based sources (CAPath, ClientCertPath, ClientKeyPath)
// into their inline counterparts. Nil receiver is a no-op.
func (o *CertificateOptions) Resolve() error {
	if o == nil {
		return nil
	}
	if err := o.Validate(); err != nil {
		return err
	}
	if o.CAPath != "" {
		b, err := os.ReadFile(o.CAPath)
		if err != nil {
			return errors.Wrapf(err, "read CA file %q", o.CAPath)
		}
		o.CACert = b
		o.CAPath = ""
	}
	if o.ClientCertPath != "" {
		b, err := os.ReadFile(o.ClientCertPath)
		if err != nil {
			return errors.Wrapf(err, "read client certificate file %q", o.ClientCertPath)
		}
		o.ClientCert = b
		o.ClientCertPath = ""
	}
	if o.ClientKeyPath != "" {
		b, err := os.ReadFile(o.ClientKeyPath)
		if err != nil {
			return errors.Wrapf(err, "read client key file %q", o.ClientKeyPath)
		}
		o.ClientKey = b
		o.ClientKeyPath = ""
	}
	return nil
}

// BuildTLSConfig builds a *tls.Config from the options. When the options are
// zero (no customization) it returns (nil, nil).
func (o *CertificateOptions) BuildTLSConfig() (*tls.Config, error) {
	if o == nil || o.IsZero() {
		return nil, nil
	}
	if err := o.Validate(); err != nil {
		return nil, err
	}

	var cfg *tls.Config
	if o.baseTLSConfig != nil {
		cfg = o.baseTLSConfig.Clone()
	} else {
		cfg = &tls.Config{}
	}

	if o.InsecureSkipVerify {
		cfg.InsecureSkipVerify = true
	}
	if o.ServerName != "" {
		cfg.ServerName = o.ServerName
	}
	if o.MinVersion != 0 {
		cfg.MinVersion = o.MinVersion
	}
	if o.MaxVersion != 0 {
		cfg.MaxVersion = o.MaxVersion
	}
	if len(o.CACert) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(o.CACert) {
			return nil, errors.New("failed to parse CA certificate")
		}
		cfg.RootCAs = pool
	}
	if len(o.ClientCert) > 0 || len(o.ClientKey) > 0 {
		cert, err := tls.X509KeyPair(o.ClientCert, o.ClientKey)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load client certificate")
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

// BuildHTTPTransport builds an *http.Transport from the options. When the
// options are zero (no customization) it returns (nil, nil).
func (o *CertificateOptions) BuildHTTPTransport() (*http.Transport, error) {
	if o == nil || o.IsZero() {
		return nil, nil
	}
	tlsCfg, err := o.BuildTLSConfig()
	if err != nil {
		return nil, err
	}

	var tr *http.Transport
	if o.baseTransport != nil {
		tr = o.baseTransport.Clone()
	} else {
		tr = &http.Transport{}
	}
	tr.TLSClientConfig = tlsCfg
	return tr, nil
}
