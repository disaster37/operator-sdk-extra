// Package pernode provides a TLSBackend that keeps one certificate per node in
// a single Secret (multi-cert transport TLS, e.g. Elasticsearch transport).
package pernode

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"sort"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/selfmanaged"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// NodeCertSuffix is appended to a node name for its certificate Data key.
	NodeCertSuffix = ".crt"
	// NodeKeySuffix is appended to a node name for its private-key Data key.
	NodeKeySuffix = ".key"
)

// NodeSpecProvider supplies per-node certificate parameters. Operator-implemented.
type NodeSpecProvider[T object.MultiPhaseObject] interface {
	// ExpectedNodeNames returns the node names that should have a certificate.
	ExpectedNodeNames(o T) ([]string, error)
	// NodeCertSpec returns the CN/DNS/IP SANs for one node's certificate.
	NodeCertSpec(o T, nodeName string) (cn string, dnsNames []string, ips []string, err error)
}

// PerNodeBackend emits a CA Secret (<secretName>-ca) plus a single Opaque leaf
// Secret (<secretName>) with ca.crt and one <node>.crt/<node>.key pair per node.
type PerNodeBackend[T object.MultiPhaseObject] struct {
	provider NodeSpecProvider[T]
}

// NewPerNodeBackend creates a new per-node backend with the given provider.
func NewPerNodeBackend[T object.MultiPhaseObject](provider NodeSpecProvider[T]) *PerNodeBackend[T] {
	return &PerNodeBackend[T]{provider: provider}
}

// DesiredObjects generates a fresh CA and one cert/key pair per expected node.
func (b *PerNodeBackend[T]) DesiredObjects(ctx context.Context, o T, spec certificate.TLSSpec) ([]client.Object, error) {
	caSecret, caSigner, caCert, err := selfmanaged.BuildCASigner(o.GetNamespace(), spec.SecretName, spec)
	if err != nil {
		return nil, err
	}

	leafSecret, err := b.buildLeafSecret(o, spec, caCert, caSigner, caSecret.Data[selfmanaged.CAKey])
	if err != nil {
		return nil, err
	}

	return []client.Object{caSecret, leafSecret}, nil
}

// CertificateSecretName returns the name of the leaf certificate Secret.
func (b *PerNodeBackend[T]) CertificateSecretName(o T, spec certificate.TLSSpec) string {
	return spec.SecretName
}

// RequiresRotationSaga returns true for the per-node backend.
func (b *PerNodeBackend[T]) RequiresRotationSaga() bool {
	return true
}

// ExpectedNodeNames delegates to the provider.
func (b *PerNodeBackend[T]) ExpectedNodeNames(o T) ([]string, error) {
	return b.provider.ExpectedNodeNames(o)
}

// NodeSecretKeys returns the cert and key Data-key suffixes.
func (b *PerNodeBackend[T]) NodeSecretKeys() (certSuffix, keySuffix string) {
	return NodeCertSuffix, NodeKeySuffix
}

// DesiredLeafWithCA re-issues all expected nodes' certs with the existing CA
// (no new CA), returning the Opaque leaf Secret whose ca.crt equals the CA
// secret's ca.crt.
func (b *PerNodeBackend[T]) DesiredLeafWithCA(ctx context.Context, o T, spec certificate.TLSSpec, caSecret *corev1.Secret) (*corev1.Secret, error) {
	caCert, caSigner, err := selfmanaged.ParseCASigner(caSecret)
	if err != nil {
		return nil, err
	}
	return b.buildLeafSecret(o, spec, caCert, caSigner, caSecret.Data[selfmanaged.CAKey])
}

// LeafNeedsChange reports whether the per-node leaf Secret needs regeneration
// vs spec at now.
func (b *PerNodeBackend[T]) LeafNeedsChange(ctx context.Context, o T, leafSecret *corev1.Secret, spec certificate.TLSSpec, now time.Time) (certificate.LeafChange, error) {
	chg := certificate.LeafChange{}

	if leafSecret == nil {
		chg.Reason = certificate.LeafMissing
		return chg, nil
	}
	if len(leafSecret.Data) == 0 {
		chg.Reason = certificate.LeafMissing
		return chg, nil
	}

	expected, err := b.provider.ExpectedNodeNames(o)
	if err != nil {
		return chg, err
	}
	existing := existingNodeNames(leafSecret)
	chg.NodesAdded, chg.NodesRemoved = nodeSetDiff(expected, existing)

	existingSet := make(map[string]struct{}, len(existing))
	for _, n := range existing {
		existingSet[n] = struct{}{}
	}

	window := time.Duration(certificate.GetValidRenewalDays(spec)) * 24 * time.Hour

	expiring := false
	cnChanged := false
	orgChanged := false
	subjectChanged := false
	keyChangedFlag := false
	usagesChangedFlag := false
	for _, node := range expected {
		if _, ok := existingSet[node]; !ok {
			continue
		}
		cert, err := parseFirstCert(leafSecret.Data[node+NodeCertSuffix])
		if err != nil {
			return chg, errors.Wrapf(err, "parse cert for node %q", node)
		}
		cn, _, _, err := b.provider.NodeCertSpec(o, node)
		if err != nil {
			return chg, err
		}
		if now.After(cert.NotAfter.Add(-window)) {
			expiring = true
		}
		if cert.Subject.CommonName != cn {
			cnChanged = true
		}
		if !selfmanaged.OrganizationsEqual(spec, cert) {
			orgChanged = true
		}
		if !selfmanaged.SubjectRestEqual(spec.LeafSubject(), cert.Subject) {
			subjectChanged = true
		}
		if kc, err := selfmanaged.KeyChanged(spec, cert); err != nil {
			return chg, err
		} else if kc {
			keyChangedFlag = true
		}
		if uc, err := selfmanaged.UsagesChanged(spec, cert); err != nil {
			return chg, err
		} else if uc {
			usagesChangedFlag = true
		}
	}

	switch {
	case expiring:
		chg.Reason = certificate.LeafExpiring
	case cnChanged:
		chg.Reason = certificate.LeafCNChanged
	case orgChanged:
		chg.Reason = certificate.LeafOrgChanged
	case subjectChanged:
		chg.Reason = certificate.LeafSubjectChanged
	case keyChangedFlag:
		chg.Reason = certificate.LeafKeyChanged
	case usagesChangedFlag:
		chg.Reason = certificate.LeafUsagesChanged
	case len(chg.NodesAdded) > 0 || len(chg.NodesRemoved) > 0:
		chg.Reason = certificate.LeafNodesChanged
	default:
		chg.Reason = certificate.LeafNone
	}
	return chg, nil
}

// buildLeafSecret signs one cert/key pair per expected node against caCert/
// caSigner and assembles the Opaque leaf Secret.
func (b *PerNodeBackend[T]) buildLeafSecret(o T, spec certificate.TLSSpec, caCert *x509.Certificate, caSigner crypto.Signer, caPEM []byte) (*corev1.Secret, error) {
	expected, err := b.provider.ExpectedNodeNames(o)
	if err != nil {
		return nil, err
	}
	if len(expected) == 0 {
		return nil, errors.New("per-node TLS backend requires at least one expected node")
	}

	data := map[string][]byte{
		selfmanaged.CAKey: caPEM,
	}
	for _, node := range expected {
		cn, dnsNames, ips, err := b.provider.NodeCertSpec(o, node)
		if err != nil {
			return nil, errors.Wrapf(err, "node cert spec for %q", node)
		}
		nodeSpec := spec
		nodeSpec.CommonName = cn
		nodeSpec.DNSNames = dnsNames
		nodeSpec.IPAddresses = ips
		certPEM, keyPEM, err := selfmanaged.SignLeafSigner(caCert, caSigner, nodeSpec)
		if err != nil {
			return nil, errors.Wrapf(err, "sign leaf for node %q", node)
		}
		data[node+NodeCertSuffix] = certPEM
		data[node+NodeKeySuffix] = keyPEM
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.SecretName,
			Namespace: o.GetNamespace(),
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}, nil
}

// existingNodeNames returns the node names that already have a cert entry in
// the leaf Secret (excluding the fixed ca.crt key), sorted for determinism.
func existingNodeNames(leafSecret *corev1.Secret) []string {
	var names []string
	for k := range leafSecret.Data {
		if k == selfmanaged.CAKey {
			continue
		}
		if strings.HasSuffix(k, NodeCertSuffix) {
			names = append(names, strings.TrimSuffix(k, NodeCertSuffix))
		}
	}
	sort.Strings(names)
	return names
}

// nodeSetDiff computes the order-insensitive set difference between expected
// and existing node names. added = expected - existing, removed = existing - expected.
func nodeSetDiff(expected, existing []string) (added, removed []string) {
	existingSet := make(map[string]struct{}, len(existing))
	for _, n := range existing {
		existingSet[n] = struct{}{}
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, n := range expected {
		expectedSet[n] = struct{}{}
	}
	for _, n := range expected {
		if _, ok := existingSet[n]; !ok {
			added = append(added, n)
		}
	}
	for _, n := range existing {
		if _, ok := expectedSet[n]; !ok {
			removed = append(removed, n)
		}
	}
	return added, removed
}

// parseFirstCert decodes the first CERTIFICATE PEM block and parses it.
func parseFirstCert(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("no certificate found in PEM data")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse certificate")
	}
	return cert, nil
}
