package certmanager_test

import (
	"context"
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
	require.Len(t, objects, 3, "expected self-signed Issuer, CA Certificate, leaf Certificate")

	// First object should be a self-signed Issuer
	issuer, ok := objects[0].(*unstructured.Unstructured)
	require.True(t, ok)
	assert.Equal(t, "Issuer", issuer.GetKind())
	assert.Equal(t, "cert-manager.io/v1", issuer.GetAPIVersion())
	assert.Equal(t, "test-tls-ca-issuer", issuer.GetName())
	assert.Equal(t, "default", issuer.GetNamespace())

	selfSigned, found, err := unstructured.NestedMap(issuer.Object, "spec", "selfSigned")
	require.NoError(t, err)
	assert.True(t, found, "expected spec.selfSigned to exist")
	assert.Empty(t, selfSigned)

	// Second object should be a CA Certificate
	caCert, ok := objects[1].(*unstructured.Unstructured)
	require.True(t, ok)
	assert.Equal(t, "Certificate", caCert.GetKind())
	assert.Equal(t, "test-tls-ca", caCert.GetName())

	isCA, found, err := unstructured.NestedBool(caCert.Object, "spec", "isCA")
	require.NoError(t, err)
	assert.True(t, found)
	assert.True(t, isCA)

	// Third object should be a leaf Certificate
	leaf, ok := objects[2].(*unstructured.Unstructured)
	require.True(t, ok)
	assert.Equal(t, "Certificate", leaf.GetKind())
	assert.Equal(t, "test-tls", leaf.GetName())

	commonName, found, err := unstructured.NestedString(leaf.Object, "spec", "commonName")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "test.example.com", commonName)

	organizations, found, err := unstructured.NestedStringSlice(leaf.Object, "spec", "subject", "organizations")
	require.NoError(t, err)
	assert.True(t, found, "expected spec.subject.organizations to exist")
	assert.Equal(t, []string{"TestOrg"}, organizations)
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
	require.Len(t, objects, 3)

	leaf, ok := objects[2].(*unstructured.Unstructured)
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
	require.Len(t, objects, 3)

	leaf, ok := objects[2].(*unstructured.Unstructured)
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
	require.Len(t, objects, 3)

	leaf, ok := objects[2].(*unstructured.Unstructured)
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
	require.Len(t, objects, 3)

	leaf, ok := objects[2].(*unstructured.Unstructured)
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
	require.Len(t, objects, 3)

	leaf, ok := objects[2].(*unstructured.Unstructured)
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
	require.Len(t, objects, 3)

	leaf, ok := objects[2].(*unstructured.Unstructured)
	require.True(t, ok)
	renewBefore, found, err := unstructured.NestedString(leaf.Object, "spec", "renewBefore")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "876000h", renewBefore)
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
	require.Len(t, objects, 3)
	leaf, ok := objects[2].(*unstructured.Unstructured)
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
	require.Len(t, objects, 3)

	leaf, ok := objects[2].(*unstructured.Unstructured)
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
		require.Len(t, objects, 3)
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
		require.Len(t, objects, 3)
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
	require.Len(t, objects, 3)
	leaf, ok := objects[2].(*unstructured.Unstructured)
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
	require.Len(t, objects, 3)

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
	require.Len(t, objects, 3)

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
	require.Len(t, objects, 3)

	leaf, ok := objects[2].(*unstructured.Unstructured)
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
		leaf, ok := objects[2].(*unstructured.Unstructured)
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
		leaf, ok := objects[2].(*unstructured.Unstructured)
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
		leaf, ok := objects[2].(*unstructured.Unstructured)
		require.True(t, ok)
		alg, found, err := unstructured.NestedString(leaf.Object, "spec", "privateKey", "algorithm")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "ECDSA", alg)
		size, found, err := unstructured.NestedInt64(leaf.Object, "spec", "privateKey", "size")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, int64(256), size)
	})

	t.Run("RSA", func(t *testing.T) {
		spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", KeyAlgorithm: certificate.KeyAlgorithmRSA}
		objects, err := backend.DesiredObjects(context.Background(), o, spec)
		require.NoError(t, err)
		leaf, ok := objects[2].(*unstructured.Unstructured)
		require.True(t, ok)
		alg, found, err := unstructured.NestedString(leaf.Object, "spec", "privateKey", "algorithm")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "RSA", alg)
		size, found, err := unstructured.NestedInt64(leaf.Object, "spec", "privateKey", "size")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, int64(2048), size)
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
	leaf, ok := objects[2].(*unstructured.Unstructured)
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
