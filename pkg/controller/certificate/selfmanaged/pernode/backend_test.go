package pernode_test

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"
	"time"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/selfmanaged"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/selfmanaged/pernode"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type testPerNodeObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (o *testPerNodeObject) GetStatus() object.MultiPhaseObjectStatus {
	return nil
}

func (o *testPerNodeObject) DeepCopyObject() runtime.Object {
	return &testPerNodeObject{
		TypeMeta:   o.TypeMeta,
		ObjectMeta: *o.DeepCopy(),
	}
}

type stubNodeSpecProvider struct {
	expected []string
	err      error
	specFunc func(node string) (cn string, dns []string, ips []string, err error)
}

func (p *stubNodeSpecProvider) ExpectedNodeNames(o *testPerNodeObject) ([]string, error) {
	return p.expected, p.err
}

func (p *stubNodeSpecProvider) NodeCertSpec(o *testPerNodeObject, nodeName string) (string, []string, []string, error) {
	if p.specFunc != nil {
		return p.specFunc(nodeName)
	}
	return nodeName + ".example.com", []string{nodeName + ".example.com"}, nil, nil
}

func newStubProvider(nodes ...string) *stubNodeSpecProvider {
	return &stubNodeSpecProvider{expected: nodes}
}

func newPerNodeObject() *testPerNodeObject {
	return &testPerNodeObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}
}

func newPerNodeBackend(p *stubNodeSpecProvider) *pernode.PerNodeBackend[*testPerNodeObject] {
	return pernode.NewPerNodeBackend[*testPerNodeObject](p)
}

func parseNodeCert(t *testing.T, leaf *corev1.Secret, node string) *x509.Certificate {
	t.Helper()
	certs, err := parsePEMCerts(leaf.Data[node+pernode.NodeCertSuffix])
	require.NoError(t, err)
	return certs[0]
}

// parsePEMCerts decodes the first CERTIFICATE PEM block and parses it.
func parsePEMCerts(pemBytes []byte) ([]*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("no certificate found in PEM data")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}
	return []*x509.Certificate{cert}, nil
}

func TestNewPerNodeBackend(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1"))
	require.NotNil(t, backend)
}

func TestPerNodeDesiredObjects(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1", "node2"))
	o := newPerNodeObject()
	spec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "cluster",
		Organization: "TestOrg",
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 2)

	caSecret, ok := objects[0].(*corev1.Secret)
	require.True(t, ok)
	assert.Equal(t, "test-tls-ca", caSecret.Name)

	leafSecret, ok := objects[1].(*corev1.Secret)
	require.True(t, ok)
	assert.Equal(t, "test-tls", leafSecret.Name)
	assert.Equal(t, corev1.SecretTypeOpaque, leafSecret.Type)
	assert.Contains(t, leafSecret.Data, selfmanaged.CAKey)
	assert.Contains(t, leafSecret.Data, "node1"+pernode.NodeCertSuffix)
	assert.Contains(t, leafSecret.Data, "node1"+pernode.NodeKeySuffix)
	assert.Contains(t, leafSecret.Data, "node2"+pernode.NodeCertSuffix)
	assert.Contains(t, leafSecret.Data, "node2"+pernode.NodeKeySuffix)
}

func TestPerNodeDesiredLeafWithCA(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1", "node2"))
	o := newPerNodeObject()
	spec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "cluster",
		Organization: "TestOrg",
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	caSecret := objects[0].(*corev1.Secret)
	priorLeaf := objects[1].(*corev1.Secret)

	newLeaf, err := backend.DesiredLeafWithCA(context.Background(), o, spec, caSecret)
	require.NoError(t, err)
	assert.Equal(t, corev1.SecretTypeOpaque, newLeaf.Type)
	assert.Equal(t, caSecret.Data[selfmanaged.CAKey], newLeaf.Data[selfmanaged.CAKey])

	caCert, _, err := selfmanaged.ParseCA(caSecret)
	require.NoError(t, err)
	for _, node := range []string{"node1", "node2"} {
		cert := parseNodeCert(t, newLeaf, node)
		require.NoError(t, cert.CheckSignatureFrom(caCert))
		assert.NotEqual(t, priorLeaf.Data[node+pernode.NodeKeySuffix], newLeaf.Data[node+pernode.NodeKeySuffix])
	}
}

func TestPerNodeDesiredLeafWithCA_MissingCAKey(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1"))
	o := newPerNodeObject()
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "cluster"}

	caSecret, _, _, err := selfmanaged.BuildCA("default", spec.SecretName, spec)
	require.NoError(t, err)
	delete(caSecret.Data, selfmanaged.CAKeyPrivate)

	_, err = backend.DesiredLeafWithCA(context.Background(), o, spec, caSecret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing ca.key")
}

func TestPerNodeLeafNeedsChange_NilSecret(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1"))
	chg, err := backend.LeafNeedsChange(context.Background(), newPerNodeObject(), nil, certificate.TLSSpec{SecretName: "test-tls"}, time.Now())
	require.NoError(t, err)
	assert.Equal(t, certificate.LeafMissing, chg.Reason)
}

func TestPerNodeLeafNeedsChange_MissingSecret(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1"))
	chg, err := backend.LeafNeedsChange(context.Background(), newPerNodeObject(), &corev1.Secret{}, certificate.TLSSpec{SecretName: "test-tls"}, time.Now())
	require.NoError(t, err)
	assert.Equal(t, certificate.LeafMissing, chg.Reason)
}

func TestPerNodeLeafNeedsChange_NodeAdded(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1"))
	o := newPerNodeObject()
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "cluster", Organization: "TestOrg"}
	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	leaf := objects[1].(*corev1.Secret)

	// Expect node1 + node2, but only node1 present.
	driftBackend := newPerNodeBackend(newStubProvider("node1", "node2"))
	chg, err := driftBackend.LeafNeedsChange(context.Background(), o, leaf, spec, time.Now())
	require.NoError(t, err)
	assert.Equal(t, certificate.LeafNodesChanged, chg.Reason)
	assert.Equal(t, []string{"node2"}, chg.NodesAdded)
	assert.Empty(t, chg.NodesRemoved)
}

func TestPerNodeLeafNeedsChange_NodeRemoved(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1", "node2"))
	o := newPerNodeObject()
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "cluster", Organization: "TestOrg"}
	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	leaf := objects[1].(*corev1.Secret)

	// Expect only node1, but node2 still present.
	backend = newPerNodeBackend(newStubProvider("node1"))
	chg, err := backend.LeafNeedsChange(context.Background(), o, leaf, spec, time.Now())
	require.NoError(t, err)
	assert.Equal(t, certificate.LeafNodesChanged, chg.Reason)
	assert.Empty(t, chg.NodesAdded)
	assert.Equal(t, []string{"node2"}, chg.NodesRemoved)
}

func TestPerNodeLeafNeedsChange_NodeAddAndRemove(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1", "node2"))
	o := newPerNodeObject()
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "cluster", Organization: "TestOrg"}
	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	leaf := objects[1].(*corev1.Secret)

	backend = newPerNodeBackend(newStubProvider("node2", "node3"))
	chg, err := backend.LeafNeedsChange(context.Background(), o, leaf, spec, time.Now())
	require.NoError(t, err)
	assert.Equal(t, certificate.LeafNodesChanged, chg.Reason)
	assert.Equal(t, []string{"node3"}, chg.NodesAdded)
	assert.Equal(t, []string{"node1"}, chg.NodesRemoved)
}

func TestPerNodeLeafNeedsChange_NoChange(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1", "node2"))
	o := newPerNodeObject()
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "cluster", Organization: "TestOrg"}
	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	leaf := objects[1].(*corev1.Secret)

	chg, err := backend.LeafNeedsChange(context.Background(), o, leaf, spec, time.Now())
	require.NoError(t, err)
	assert.True(t, chg.IsZero())
	assert.Equal(t, certificate.LeafNone, chg.Reason)
}

func TestPerNodeLeafNeedsChange_NodeExpiring(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1"))
	o := newPerNodeObject()
	spec := certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "cluster",
		Organization:     "TestOrg",
		LeafValidityDays: 365,
	}
	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	leaf := objects[1].(*corev1.Secret)

	chg, err := backend.LeafNeedsChange(context.Background(), o, leaf, spec, time.Now().Add(340*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, certificate.LeafExpiring, chg.Reason)
}

func TestPerNodeLeafNeedsChange_NodeCNOrgDrift(t *testing.T) {
	buildProvider := newStubProvider("node1")
	backend := newPerNodeBackend(buildProvider)
	o := newPerNodeObject()
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "cluster", Organization: "TestOrg"}
	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	leaf := objects[1].(*corev1.Secret)

	t.Run("CN", func(t *testing.T) {
		p := newStubProvider("node1")
		p.specFunc = func(node string) (string, []string, []string, error) {
			return "wrong.example.com", []string{"node1.example.com"}, nil, nil
		}
		chg, err := newPerNodeBackend(p).LeafNeedsChange(context.Background(), o, leaf, spec, time.Now())
		require.NoError(t, err)
		assert.Equal(t, certificate.LeafCNChanged, chg.Reason)
	})

	t.Run("Org", func(t *testing.T) {
		p := newStubProvider("node1")
		p.specFunc = func(node string) (string, []string, []string, error) {
			return "node1.example.com", []string{"node1.example.com"}, nil, nil
		}
		driftSpec := spec
		driftSpec.Organization = "OtherOrg"
		chg, err := newPerNodeBackend(p).LeafNeedsChange(context.Background(), o, leaf, driftSpec, time.Now())
		require.NoError(t, err)
		assert.Equal(t, certificate.LeafOrgChanged, chg.Reason)
	})
}

func TestPerNodeExpectedNodeNames(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1", "node2"))
	names, err := backend.ExpectedNodeNames(newPerNodeObject())
	require.NoError(t, err)
	assert.Equal(t, []string{"node1", "node2"}, names)
}

func TestPerNodeNodeSecretKeys(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1"))
	certSuffix, keySuffix := backend.NodeSecretKeys()
	assert.Equal(t, pernode.NodeCertSuffix, certSuffix)
	assert.Equal(t, pernode.NodeKeySuffix, keySuffix)
}

func TestPerNodeRequiresRotationSaga(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1"))
	assert.True(t, backend.RequiresRotationSaga())
}

func TestPerNodeCertificateSecretName(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1"))
	assert.Equal(t, "test-tls", backend.CertificateSecretName(newPerNodeObject(), certificate.TLSSpec{SecretName: "test-tls"}))
}

func TestPerNodeDesiredObjects_NoNodes(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider())
	_, err := backend.DesiredObjects(context.Background(), newPerNodeObject(), certificate.TLSSpec{SecretName: "test-tls", CommonName: "cluster"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one expected node")
}

func TestPerNodeLeafNeedsChange_MalformedCert(t *testing.T) {
	backend := newPerNodeBackend(newStubProvider("node1"))
	o := newPerNodeObject()
	leaf := &corev1.Secret{Data: map[string][]byte{
		selfmanaged.CAKey:                []byte("ca"),
		"node1" + pernode.NodeCertSuffix: []byte("not pem"),
	}}
	_, err := backend.LeafNeedsChange(context.Background(), o, leaf, certificate.TLSSpec{SecretName: "test-tls", CommonName: "cluster"}, time.Now())
	require.Error(t, err)
}

func TestPerNodeLeafNeedsChange_ExpectedNodeNamesError(t *testing.T) {
	p := newStubProvider()
	p.err = fmt.Errorf("boom")
	backend := newPerNodeBackend(p)
	_, err := backend.LeafNeedsChange(context.Background(), newPerNodeObject(), &corev1.Secret{Data: map[string][]byte{"x": {}}}, certificate.TLSSpec{SecretName: "test-tls"}, time.Now())
	require.Error(t, err)
}
