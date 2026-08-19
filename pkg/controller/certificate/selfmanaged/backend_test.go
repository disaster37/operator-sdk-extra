package selfmanaged_test

import (
	"context"
	"testing"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/certificate/selfmanaged"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type testSelfManagedObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (o *testSelfManagedObject) GetStatus() object.MultiPhaseObjectStatus {
	return nil
}

func (o *testSelfManagedObject) DeepCopyObject() runtime.Object {
	return &testSelfManagedObject{
		TypeMeta:   o.TypeMeta,
		ObjectMeta: *o.ObjectMeta.DeepCopy(),
	}
}

func TestNewSelfManagedBackend(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	require.NotNil(t, backend)
}

func TestSelfManagedBackendDesiredObjects(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com", "test-alt.example.com"},
		Organization: "TestOrg",
		ValidityDays: 90,
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 2, "expected CA secret and leaf secret")
}

func TestSelfManagedBackendDesiredObjectsDefaults(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
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
	require.Len(t, objects, 2)
}

func TestSelfManagedBackendGoldenFiles(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
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
	require.Len(t, objects, 2)

	// CA secret
	caSecret, ok := objects[0].(*corev1.Secret)
	require.True(t, ok, "first object should be a Secret")
	assert.Equal(t, "test-tls-ca", caSecret.Name)
	assert.Equal(t, "default", caSecret.Namespace)
	assert.Contains(t, string(caSecret.Data["ca.crt"]), "BEGIN CERTIFICATE")
	assert.Contains(t, string(caSecret.Data["ca.key"]), "BEGIN EC PRIVATE KEY")

	// Leaf secret
	leafSecret, ok := objects[1].(*corev1.Secret)
	require.True(t, ok, "second object should be a Secret")
	assert.Equal(t, "test-tls", leafSecret.Name)
	assert.Equal(t, "default", leafSecret.Namespace)
	assert.Equal(t, corev1.SecretTypeTLS, leafSecret.Type)
	assert.Contains(t, string(leafSecret.Data["tls.crt"]), "BEGIN CERTIFICATE")
	assert.Contains(t, string(leafSecret.Data["tls.key"]), "BEGIN EC PRIVATE KEY")
	assert.Contains(t, string(leafSecret.Data["ca.crt"]), "BEGIN CERTIFICATE")
}

func TestCertificateSecretName(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	spec := certificate.TLSSpec{SecretName: "my-tls"}
	name := backend.CertificateSecretName(o, spec)
	assert.Equal(t, "my-tls", name)
}

func TestRequiresRotationSaga(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	assert.True(t, backend.RequiresRotationSaga())
}

func TestSelfManagedBackendWithDNSNames(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*testSelfManagedObject]()
	o := &testSelfManagedObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	spec := certificate.TLSSpec{
		SecretName:   "test-tls",
		CommonName:   "test.example.com",
		DNSNames:     []string{"test.example.com", "*.example.com"},
		Organization: "TestOrg",
	}

	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objects, 2)
}
