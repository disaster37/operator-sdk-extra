package certmanager_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/certmanager"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type testCMObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (o *testCMObject) GetStatus() object.MultiPhaseObjectStatus {
	return nil
}

func (o *testCMObject) DeepCopyObject() runtime.Object {
	return &testCMObject{
		TypeMeta:   o.TypeMeta,
		ObjectMeta: *o.DeepCopy(),
	}
}

func TestNewCertManagerBackend(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	require.NotNil(t, backend)
}

func TestCertManagerBackendDedicatedCA(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
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
	require.Len(t, objects, 4, "expected self-signed Issuer, CA Certificate, CA Issuer, leaf Certificate")

	// First object should be a self-signed bootstrap Issuer
	issuer, ok := objects[0].(*unstructured.Unstructured)
	require.True(t, ok)
	assert.Equal(t, "Issuer", issuer.GetKind())
	assert.Equal(t, "cert-manager.io/v1", issuer.GetAPIVersion())
	assert.Equal(t, "test-tls-selfsigned-issuer", issuer.GetName())
	assert.Equal(t, "default", issuer.GetNamespace())

	selfSigned, found, err := unstructured.NestedMap(issuer.Object, "spec", "selfSigned")
	require.NoError(t, err)
	assert.True(t, found, "expected spec.selfSigned to exist")
	assert.Empty(t, selfSigned)

	// Second object should be a CA Certificate signed by the bootstrap Issuer
	caCert, ok := objects[1].(*unstructured.Unstructured)
	require.True(t, ok)
	assert.Equal(t, "Certificate", caCert.GetKind())
	assert.Equal(t, "test-tls-ca", caCert.GetName())

	isCA, found, err := unstructured.NestedBool(caCert.Object, "spec", "isCA")
	require.NoError(t, err)
	assert.True(t, found)
	assert.True(t, isCA)

	caSecretName, found, err := unstructured.NestedString(caCert.Object, "spec", "secretName")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "test-tls-ca", caSecretName)

	caIssuerRef, found, err := unstructured.NestedMap(caCert.Object, "spec", "issuerRef")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, map[string]interface{}{
		"name":  "test-tls-selfsigned-issuer",
		"kind":  "Issuer",
		"group": "cert-manager.io",
	}, caIssuerRef)

	assertPrivateKey(t, caCert, "ECDSA", int64(256))

	// Third object should be the CA-backed Issuer
	caIssuer, ok := objects[2].(*unstructured.Unstructured)
	require.True(t, ok)
	assert.Equal(t, "Issuer", caIssuer.GetKind())
	assert.Equal(t, "test-tls-ca-issuer", caIssuer.GetName())
	caIssuerSecret, found, err := unstructured.NestedString(caIssuer.Object, "spec", "ca", "secretName")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "test-tls-ca", caIssuerSecret)

	// Fourth object should be a leaf Certificate signed by the CA Issuer
	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)
	assert.Equal(t, "Certificate", leaf.GetKind())
	assert.Equal(t, "test-tls", leaf.GetName())

	commonName, found, err := unstructured.NestedString(leaf.Object, "spec", "commonName")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "test.example.com", commonName)

	leafIssuerRef, found, err := unstructured.NestedMap(leaf.Object, "spec", "issuerRef")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, map[string]interface{}{
		"name":  "test-tls-ca-issuer",
		"kind":  "Issuer",
		"group": "cert-manager.io",
	}, leafIssuerRef)

	assertPrivateKey(t, leaf, "ECDSA", int64(256))

	organizations, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "subject", "organizations")
	require.NoError(t, err)
	assert.True(t, found, "expected spec.subject.organizations to exist")
	assert.Equal(t, []string{"TestOrg"}, organizations)
}

// assertPrivateKey asserts the algorithm and size of an unstructured
// Certificate's spec.privateKey block.
func assertPrivateKey(t *testing.T, cert *unstructured.Unstructured, algorithm string, size int64) {
	t.Helper()
	alg, found, err := unstructured.NestedString(cert.Object, "spec", "privateKey", "algorithm")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, algorithm, alg)
	gotSize, found, err := unstructured.NestedInt64(cert.Object, "spec", "privateKey", "size")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, size, gotSize)
}

func TestCertManagerBackendExistingCA(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		DNSNames:   []string{"test.example.com"},
		IssuerRef:  "my-cluster-issuer",
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 1, "expected only leaf Certificate in existing-CA mode")

	leaf, ok := objects[0].(*unstructured.Unstructured)
	require.True(t, ok)
	assert.Equal(t, "Certificate", leaf.GetKind())
	assert.Equal(t, "test-tls", leaf.GetName())

	issuerRef, found, err := unstructured.NestedString(leaf.Object, "spec", "issuerRef", "name")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "my-cluster-issuer", issuerRef)

	issuerKind, found, err := unstructured.NestedString(leaf.Object, "spec", "issuerRef", "kind")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "Issuer", issuerKind)

	issuerGroup, found, err := unstructured.NestedString(leaf.Object, "spec", "issuerRef", "group")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "cert-manager.io", issuerGroup)
}

func TestCertManagerBackendEmptySecretName(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{SecretName: ""}
	_, err := backend.DesiredObjects(context.Background(), o, spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty SecretName")
}

func TestCertManagerCertificateSecretName(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	spec := certificate.TLSSpec{SecretName: "my-tls"}
	name := backend.CertificateSecretName(o, spec)
	assert.Equal(t, "my-tls", name)
}

func TestCertManagerRequiresRotationSaga(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	assert.False(t, backend.RequiresRotationSaga())
}

func TestCertManagerBackendIPSANsDedicatedCA(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:  "test-tls",
		CommonName:  "test.example.com",
		IPAddresses: []string{"10.0.0.1"},
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 4)

	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)
	ips, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "ipAddresses")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []string{"10.0.0.1"}, ips)
}

func TestCertManagerBackendIPSANsExistingCA(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:  "test-tls",
		CommonName:  "test.example.com",
		IssuerRef:   "my-cluster-issuer",
		IPAddresses: []string{"10.0.0.1"},
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 1)

	leaf, ok := objects[0].(*unstructured.Unstructured)
	require.True(t, ok)
	ips, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "ipAddresses")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []string{"10.0.0.1"}, ips)
}

func TestCertManagerBackendDedupSANs(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:  "test-tls",
		CommonName:  "test.example.com",
		IssuerRef:   "my-cluster-issuer",
		DNSNames:    []string{"a.example.com", "b.example.com", "a.example.com"},
		IPAddresses: []string{"10.0.0.1", "10.0.0.2", "10.0.0.1"},
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 1)

	leaf, ok := objects[0].(*unstructured.Unstructured)
	require.True(t, ok)
	dns, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "dnsNames")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []string{"a.example.com", "b.example.com"}, dns)

	ips, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "ipAddresses")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, ips)
}

func TestCertManagerBackendIPSANsAbsentByDefault(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
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
	require.Len(t, objects, 4)

	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)
	_, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "ipAddresses")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestCertManagerBackendRenewBefore(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:  "test-tls",
		CommonName:  "test.example.com",
		RenewalDays: 20,
	}

	// Dedicated-CA mode.
	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 4)

	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)
	renewBefore, found, err := unstructured.NestedString(leaf.Object, "spec", "renewBefore")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "480h", renewBefore)

	// Existing-CA mode.
	spec.IssuerRef = "my-cluster-issuer"
	objects, err = backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 1)

	leaf, ok = objects[0].(*unstructured.Unstructured)
	require.True(t, ok)
	renewBefore, found, err = unstructured.NestedString(leaf.Object, "spec", "renewBefore")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "480h", renewBefore)
}

func TestCertManagerBackendRenewBeforeAbsent(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:  "test-tls",
		CommonName:  "test.example.com",
		RenewalDays: 0,
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 4)

	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)
	_, found, err := unstructured.NestedString(leaf.Object, "spec", "renewBefore")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestCertManagerBackendSubjectOrganizationsAbsent(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
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
	require.Len(t, objects, 4)

	caCert, ok := objects[1].(*unstructured.Unstructured)
	require.True(t, ok)
	_, caFound, err := unstructured.NestedStringSlice(caCert.Object, "spec", "subject", "organizations")
	require.NoError(t, err)
	assert.False(t, caFound, "CA spec.subject.organizations should be absent when Organization is empty")

	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)
	_, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "subject", "organizations")
	require.NoError(t, err)
	assert.False(t, found, "spec.subject.organizations should be absent when Organization is empty")
}

func TestCertManagerBackendRenewBeforeClamped(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	// A huge RenewalDays must be clamped to MaxRenewalDays instead of
	// overflowing `RenewalDays * 24` (int overflow would corrupt renewBefore).
	spec := certificate.TLSSpec{
		SecretName:  "test-tls",
		CommonName:  "test.example.com",
		RenewalDays: certificate.MaxRenewalDays + 1000000,
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 4)

	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)
	renewBefore, found, err := unstructured.NestedString(leaf.Object, "spec", "renewBefore")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "876000h", renewBefore)
}

func TestCertManagerBackendCARenewalDaysIgnored(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	// CARenewalDays must not be mapped: renewBefore stays on the leaf window
	// (20d -> 480h); the CA is owned by the Issuer.
	spec := certificate.TLSSpec{
		SecretName:    "test-tls",
		CommonName:    "test.example.com",
		RenewalDays:   20,
		CARenewalDays: 90,
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 4)

	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)
	renewBefore, found, err := unstructured.NestedString(leaf.Object, "spec", "renewBefore")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "480h", renewBefore, "CARenewalDays must not change the leaf renewBefore")
}

func TestCertManagerBackendLeafValidityDays(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "test.example.com",
		LeafValidityDays: 90,
	}

	// Dedicated-CA mode.
	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 4)
	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)
	duration, found, err := unstructured.NestedString(leaf.Object, "spec", "duration")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "2160h", duration)

	// Existing-CA mode.
	spec.IssuerRef = "my-cluster-issuer"
	objects, err = backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 1)
	leaf, ok = objects[0].(*unstructured.Unstructured)
	require.True(t, ok)
	duration, found, err = unstructured.NestedString(leaf.Object, "spec", "duration")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "2160h", duration)
}

func TestCertManagerBackendLeafValidityClamped(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	// A huge LeafValidityDays must be clamped to MaxValidityDays instead of
	// overflowing the `LeafValidityDays * 24` multiplication in the duration.
	spec := certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "test.example.com",
		LeafValidityDays: certificate.MaxValidityDays + 1000000,
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 4)

	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)
	duration, found, err := unstructured.NestedString(leaf.Object, "spec", "duration")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "876000h", duration)
}

func TestCertManagerBackendCADuration(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	t.Run("default", func(t *testing.T) {
		spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com"}
		objects, err := backend.DesiredObjects(context.Background(), o, spec)
		require.NoError(t, err)
		require.Len(t, objects, 4)
		caCert, ok := objects[1].(*unstructured.Unstructured)
		require.True(t, ok)
		duration, found, err := unstructured.NestedString(caCert.Object, "spec", "duration")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, fmt.Sprintf("%dh", certificate.GetValidCADays(spec)*24), duration)
	})

	t.Run("custom", func(t *testing.T) {
		spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", CAValidityDays: 500}
		objects, err := backend.DesiredObjects(context.Background(), o, spec)
		require.NoError(t, err)
		require.Len(t, objects, 4)
		caCert, ok := objects[1].(*unstructured.Unstructured)
		require.True(t, ok)
		duration, found, err := unstructured.NestedString(caCert.Object, "spec", "duration")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, fmt.Sprintf("%dh", certificate.GetValidCADays(spec)*24), duration)
	})
}

func TestCertManagerBackendLeafValidityAbsentByDefault(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "test.example.com",
		LeafValidityDays: 0,
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 4)
	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)
	_, found, err := unstructured.NestedString(leaf.Object, "spec", "duration")
	require.NoError(t, err)
	assert.False(t, found, "spec.duration must be absent when LeafValidityDays is 0")
}

func TestCertManagerBackendCASubject(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		CACommonName: "my-ca",
		CASubject: &certificate.CertificateSubject{
			Organizations:       []string{"ca-org"},
			OrganizationalUnits: []string{"ca-ou"},
		},
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 4)

	caCert, ok := objects[1].(*unstructured.Unstructured)
	require.True(t, ok)
	cn, found, err := unstructured.NestedString(caCert.Object, "spec", "commonName")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "my-ca", cn)

	orgs, found, err := unstructured.NestedStringSlice(caCert.Object, "spec", "subject", "organizations")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []string{"ca-org"}, orgs)

	ous, found, err := unstructured.NestedStringSlice(caCert.Object, "spec", "subject", "organizationalUnits")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []string{"ca-ou"}, ous)
}

func TestCertManagerBackendCAOrganizationsNotHardcoded(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		Organization: "TestOrg",
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 4)

	caCert, ok := objects[1].(*unstructured.Unstructured)
	require.True(t, ok)
	orgs, found, err := unstructured.NestedStringSlice(caCert.Object, "spec", "subject", "organizations")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []string{"TestOrg"}, orgs, "CA organizations must follow spec, not be hardcoded")
}

func TestCertManagerBackendLeafSubjectFullRDN(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		Subject: certificate.CertificateSubject{
			Organizations:       []string{"org"},
			OrganizationalUnits: []string{"ou"},
			Countries:           []string{"FR"},
			Localities:          []string{"loc"},
			Provinces:           []string{"prov"},
			StreetAddresses:     []string{"street"},
			PostalCodes:         []string{"75000"},
			SerialNumber:        "sn",
		},
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 4)

	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)

	orgs, _, err := unstructured.NestedStringSlice(leaf.Object, "spec", "subject", "organizations")
	require.NoError(t, err)
	assert.Equal(t, []string{"org"}, orgs)
	ous, _, err := unstructured.NestedStringSlice(leaf.Object, "spec", "subject", "organizationalUnits")
	require.NoError(t, err)
	assert.Equal(t, []string{"ou"}, ous)
	countries, _, err := unstructured.NestedStringSlice(leaf.Object, "spec", "subject", "countries")
	require.NoError(t, err)
	assert.Equal(t, []string{"FR"}, countries)
	serial, _, err := unstructured.NestedString(leaf.Object, "spec", "subject", "serialNumber")
	require.NoError(t, err)
	assert.Equal(t, "sn", serial)
}

func TestCertManagerBackendUsagesMapping(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	t.Run("defaults", func(t *testing.T) {
		spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com"}
		objects, err := backend.DesiredObjects(context.Background(), o, spec)
		require.NoError(t, err)
		leaf, ok := objects[3].(*unstructured.Unstructured)
		require.True(t, ok)
		usages, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "usages")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, []string{"server auth", "client auth"}, usages)
	})

	t.Run("custom", func(t *testing.T) {
		spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", Usages: []string{certificate.UsageServerAuth, certificate.UsageCodeSigning}}
		objects, err := backend.DesiredObjects(context.Background(), o, spec)
		require.NoError(t, err)
		leaf, ok := objects[3].(*unstructured.Unstructured)
		require.True(t, ok)
		usages, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "usages")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, []string{"server auth", "code signing"}, usages)
	})
}

func TestCertManagerBackendPrivateKey(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	t.Run("default ECDSA", func(t *testing.T) {
		spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com"}
		objects, err := backend.DesiredObjects(context.Background(), o, spec)
		require.NoError(t, err)
		caCert, ok := objects[1].(*unstructured.Unstructured)
		require.True(t, ok)
		assertPrivateKey(t, caCert, "ECDSA", int64(256))
		leaf, ok := objects[3].(*unstructured.Unstructured)
		require.True(t, ok)
		assertPrivateKey(t, leaf, "ECDSA", int64(256))
	})

	t.Run("RSA", func(t *testing.T) {
		spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", KeyAlgorithm: certificate.KeyAlgorithmRSA}
		objects, err := backend.DesiredObjects(context.Background(), o, spec)
		require.NoError(t, err)
		caCert, ok := objects[1].(*unstructured.Unstructured)
		require.True(t, ok)
		assertPrivateKey(t, caCert, "RSA", int64(2048))
		leaf, ok := objects[3].(*unstructured.Unstructured)
		require.True(t, ok)
		assertPrivateKey(t, leaf, "RSA", int64(2048))
	})
}

func TestCertManagerBackendAllUsages(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		Usages: []string{
			certificate.UsageServerAuth,
			certificate.UsageClientAuth,
			certificate.UsageCodeSigning,
			certificate.UsageEmailProtection,
			certificate.UsageTimestamping,
			certificate.UsageOCSPSigning,
		},
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	leaf, ok := objects[3].(*unstructured.Unstructured)
	require.True(t, ok)
	usages, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "usages")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []string{
		"server auth",
		"client auth",
		"code signing",
		"email protection",
		"timestamping",
		"ocsp signing",
	}, usages)
}

func TestCertManagerBackendExistingCAFullSpec(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName: "test-tls",
		CommonName: "test.example.com",
		IssuerRef:  "my-cluster-issuer",
		Subject: certificate.CertificateSubject{
			Organizations: []string{"org"},
		},
		Usages:       []string{certificate.UsageServerAuth},
		KeyUsages:    []string{certificate.KeyUsageDigitalSignature},
		KeyAlgorithm: certificate.KeyAlgorithmRSA,
		KeySize:      2048,
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 1)

	leaf, ok := objects[0].(*unstructured.Unstructured)
	require.True(t, ok)

	issuerRef, found, err := unstructured.NestedString(leaf.Object, "spec", "issuerRef", "name")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "my-cluster-issuer", issuerRef)

	issuerKind, found, err := unstructured.NestedString(leaf.Object, "spec", "issuerRef", "kind")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "Issuer", issuerKind)

	issuerGroup, found, err := unstructured.NestedString(leaf.Object, "spec", "issuerRef", "group")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "cert-manager.io", issuerGroup)

	orgs, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "subject", "organizations")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []string{"org"}, orgs)

	usages, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "usages")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []string{"server auth"}, usages)

	alg, found, err := unstructured.NestedString(leaf.Object, "spec", "privateKey", "algorithm")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "RSA", alg)
}

func TestCertManagerBackendInvalidContent(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", Usages: []string{"bogus"}}
	_, err := backend.DesiredObjects(context.Background(), o, spec)
	require.Error(t, err)
}

func TestCertManagerPrototypeObjects(t *testing.T) {
	issuer := certmanager.NewIssuerObject()
	assert.Equal(t, certmanager.IssuerGVK, issuer.GroupVersionKind())

	cert := certmanager.NewCertificateObject()
	assert.Equal(t, certmanager.CertificateGVK, cert.GroupVersionKind())
}

func TestCertManagerBackendClusterIssuerRef(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}

	spec := certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "test.example.com",
		DNSNames:         []string{"test.example.com"},
		ClusterIssuerRef: "letsencrypt-prod",
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 1)

	leaf, ok := objects[0].(*unstructured.Unstructured)
	require.True(t, ok)

	issuerRef, found, err := unstructured.NestedMap(leaf.Object, "spec", "issuerRef")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, map[string]interface{}{
		"name":  "letsencrypt-prod",
		"kind":  "ClusterIssuer",
		"group": "cert-manager.io",
	}, issuerRef)

	assertPrivateKey(t, leaf, "ECDSA", int64(256))
}

func TestCertManagerBackendConflictingIssuerRefs(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}

	spec := certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "test.example.com",
		IssuerRef:        "ns-issuer",
		ClusterIssuerRef: "cluster-issuer",
	}

	_, err := backend.DesiredObjects(context.Background(), o, spec)
	require.ErrorIs(t, err, certificate.ErrConflictingIssuerRefs)
}

func TestCertManagerBackendWhitespaceIssuerRefs(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}

	t.Run("whitespace IssuerRef", func(t *testing.T) {
		spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", IssuerRef: "  "}
		_, err := backend.DesiredObjects(context.Background(), o, spec)
		require.ErrorIs(t, err, certificate.ErrInvalidIssuerRef)
	})
	t.Run("whitespace ClusterIssuerRef", func(t *testing.T) {
		spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", ClusterIssuerRef: " \t "}
		_, err := backend.DesiredObjects(context.Background(), o, spec)
		require.ErrorIs(t, err, certificate.ErrInvalidIssuerRef)
	})
}

func TestCertManagerBackendPrivateKeyCurvesAndSizes(t *testing.T) {
	backend := certmanager.NewCertManagerBackend[*testCMObject]()
	o := &testCMObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}

	assertBoth := func(t *testing.T, spec certificate.TLSSpec, algorithm string, size int64) {
		t.Helper()
		objects, err := backend.DesiredObjects(context.Background(), o, spec)
		require.NoError(t, err)
		require.Len(t, objects, 4)
		caCert, ok := objects[1].(*unstructured.Unstructured)
		require.True(t, ok)
		assertPrivateKey(t, caCert, algorithm, size)
		leaf, ok := objects[3].(*unstructured.Unstructured)
		require.True(t, ok)
		assertPrivateKey(t, leaf, algorithm, size)
	}

	t.Run("ECDSA P-384", func(t *testing.T) {
		assertBoth(t, certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", Curve: certificate.CurveP384}, "ECDSA", int64(384))
	})
	t.Run("ECDSA P-521", func(t *testing.T) {
		assertBoth(t, certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", Curve: certificate.CurveP521}, "ECDSA", int64(521))
	})
	t.Run("RSA 4096", func(t *testing.T) {
		assertBoth(t, certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", KeyAlgorithm: certificate.KeyAlgorithmRSA, KeySize: 4096}, "RSA", int64(4096))
	})
	t.Run("RSA 8192", func(t *testing.T) {
		assertBoth(t, certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", KeyAlgorithm: certificate.KeyAlgorithmRSA, KeySize: 8192}, "RSA", int64(8192))
	})
	t.Run("RSA unsupported size 3072 rejected", func(t *testing.T) {
		spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", KeyAlgorithm: certificate.KeyAlgorithmRSA, KeySize: 3072}
		_, err := backend.DesiredObjects(context.Background(), o, spec)
		require.ErrorIs(t, err, certificate.ErrInvalidKeySize)
	})
}

func newCMCertificate(name, namespace string, conditions []interface{}) *unstructured.Unstructured {
	cert := certmanager.NewCertificateObject()
	cert.SetName(name)
	cert.SetNamespace(namespace)
	if conditions != nil {
		_ = unstructured.SetNestedSlice(cert.Object, conditions, "status", "conditions")
	}
	return cert
}

func cmCondition(t *testing.T, c client.Client, name, condType string) map[string]interface{} {
	t.Helper()
	cert := certmanager.NewCertificateObject()
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: name}, cert))
	conds, found, err := unstructured.NestedSlice(cert.Object, "status", "conditions")
	require.NoError(t, err)
	if !found {
		return nil
	}
	for _, raw := range conds {
		m, ok := raw.(map[string]interface{})
		if ok && m["type"] == condType {
			return m
		}
	}
	return nil
}

type failingGetClient struct {
	client.Client
	failName string
	err      error
}

func (c *failingGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key.Name == c.failName {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

type failingStatusClient struct {
	client.Client
	err error
}

func (c *failingStatusClient) Status() client.SubResourceWriter {
	return &failingStatusWriter{err: c.err}
}

type failingStatusWriter struct {
	client.SubResourceWriter
	err error
}

func (w *failingStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return w.err
}

func TestCertManagerBackendForceRegenerate(t *testing.T) {
	ctx := context.Background()
	o := &testCMObject{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}
	dedicatedSpec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com"}

	newBackend := func(c client.Client) *certmanager.CertManagerBackend[*testCMObject] {
		return certmanager.NewCertManagerBackend[*testCMObject](certmanager.WithClient[*testCMObject](c))
	}

	t.Run("no force flags is no-op", func(t *testing.T) {
		backend := certmanager.NewCertManagerBackend[*testCMObject]()
		require.NoError(t, backend.ForceRegenerate(ctx, o, dedicatedSpec, false, false))
	})

	t.Run("nil client returns ErrClientRequired", func(t *testing.T) {
		backend := certmanager.NewCertManagerBackend[*testCMObject]()
		err := backend.ForceRegenerate(ctx, o, dedicatedSpec, true, false)
		require.ErrorIs(t, err, certmanager.ErrClientRequired)
	})

	t.Run("forceAll dedicated mode triggers CA and leaf", func(t *testing.T) {
		ca := newCMCertificate("test-tls-ca", "default", []interface{}{
			map[string]interface{}{"type": "Ready", "status": "True"},
			map[string]interface{}{"type": "Issuing", "status": "False"},
		})
		leaf := newCMCertificate("test-tls", "default", nil)
		c := fake.NewClientBuilder().WithStatusSubresource(certmanager.NewCertificateObject()).WithObjects(ca, leaf).Build()

		require.NoError(t, newBackend(c).ForceRegenerate(ctx, o, dedicatedSpec, true, false))

		caIssuing := cmCondition(t, c, "test-tls-ca", "Issuing")
		require.NotNil(t, caIssuing)
		assert.Equal(t, "True", caIssuing["status"])
		assert.Equal(t, "ManuallyTriggered", caIssuing["reason"])
		caReady := cmCondition(t, c, "test-tls-ca", "Ready")
		require.NotNil(t, caReady)
		assert.Equal(t, "True", caReady["status"])

		leafIssuing := cmCondition(t, c, "test-tls", "Issuing")
		require.NotNil(t, leafIssuing)
		assert.Equal(t, "True", leafIssuing["status"])
		assert.Equal(t, "ManuallyTriggered", leafIssuing["reason"])
	})

	t.Run("forceLeaf dedicated mode triggers leaf only", func(t *testing.T) {
		ca := newCMCertificate("test-tls-ca", "default", []interface{}{
			map[string]interface{}{"type": "Ready", "status": "True"},
		})
		leaf := newCMCertificate("test-tls", "default", nil)
		c := fake.NewClientBuilder().WithStatusSubresource(certmanager.NewCertificateObject()).WithObjects(ca, leaf).Build()

		require.NoError(t, newBackend(c).ForceRegenerate(ctx, o, dedicatedSpec, false, true))

		leafIssuing := cmCondition(t, c, "test-tls", "Issuing")
		require.NotNil(t, leafIssuing)
		assert.Equal(t, "True", leafIssuing["status"])
		assert.Nil(t, cmCondition(t, c, "test-tls-ca", "Issuing"))
	})

	t.Run("forceAll with ClusterIssuerRef triggers leaf only", func(t *testing.T) {
		leaf := newCMCertificate("test-tls", "default", nil)
		c := fake.NewClientBuilder().WithStatusSubresource(certmanager.NewCertificateObject()).WithObjects(leaf).Build()

		spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", ClusterIssuerRef: "cluster-ca"}
		require.NoError(t, newBackend(c).ForceRegenerate(ctx, o, spec, true, false))

		leafIssuing := cmCondition(t, c, "test-tls", "Issuing")
		require.NotNil(t, leafIssuing)
		assert.Equal(t, "True", leafIssuing["status"])
	})

	t.Run("already Issuing=True is idempotent no-op", func(t *testing.T) {
		leaf := newCMCertificate("test-tls", "default", []interface{}{
			map[string]interface{}{"type": "Issuing", "status": "True", "reason": "Existing"},
		})
		c := fake.NewClientBuilder().WithStatusSubresource(certmanager.NewCertificateObject()).WithObjects(leaf).Build()

		require.NoError(t, newBackend(c).ForceRegenerate(ctx, o, dedicatedSpec, false, true))

		leafIssuing := cmCondition(t, c, "test-tls", "Issuing")
		require.NotNil(t, leafIssuing)
		assert.Equal(t, "Existing", leafIssuing["reason"])
	})

	t.Run("NotFound Certificate is ignored", func(t *testing.T) {
		c := fake.NewClientBuilder().WithStatusSubresource(certmanager.NewCertificateObject()).Build()
		require.NoError(t, newBackend(c).ForceRegenerate(ctx, o, dedicatedSpec, true, false))
	})

	t.Run("Get error propagates on CA", func(t *testing.T) {
		boom := errors.New("get boom")
		ca := newCMCertificate("test-tls-ca", "default", nil)
		leaf := newCMCertificate("test-tls", "default", nil)
		c := fake.NewClientBuilder().WithStatusSubresource(certmanager.NewCertificateObject()).WithObjects(ca, leaf).Build()
		failing := &failingGetClient{Client: c, failName: "test-tls-ca", err: boom}

		err := newBackend(failing).ForceRegenerate(ctx, o, dedicatedSpec, true, false)
		require.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "get cert-manager Certificate")
	})

	t.Run("Get error propagates on leaf", func(t *testing.T) {
		boom := errors.New("get boom")
		ca := newCMCertificate("test-tls-ca", "default", nil)
		leaf := newCMCertificate("test-tls", "default", nil)
		c := fake.NewClientBuilder().WithStatusSubresource(certmanager.NewCertificateObject()).WithObjects(ca, leaf).Build()
		failing := &failingGetClient{Client: c, failName: "test-tls", err: boom}

		err := newBackend(failing).ForceRegenerate(ctx, o, dedicatedSpec, true, false)
		require.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "get cert-manager Certificate")
	})

	t.Run("Status Update error propagates", func(t *testing.T) {
		boom := errors.New("update boom")
		leaf := newCMCertificate("test-tls", "default", nil)
		c := fake.NewClientBuilder().WithStatusSubresource(certmanager.NewCertificateObject()).WithObjects(leaf).Build()
		failing := &failingStatusClient{Client: c, err: boom}

		err := newBackend(failing).ForceRegenerate(ctx, o, dedicatedSpec, false, true)
		require.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "update status condition Issuing=True")
	})
}
