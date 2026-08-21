package rotation_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	apworkflow "github.com/disaster37/operator-sdk-extra/v3/pkg/apis/workflow"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/byo"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/rotation"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/selfmanaged"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/selfmanaged/pernode"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/multiphase"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/workflow"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type rotStatus struct {
	Conditions []metav1.Condition
	PhaseName  shared.PhaseName
	IsOnError  *bool
	LastError  string
	ObsGen     int64
	Ws         *apworkflow.WorkflowStatus
}

func (s *rotStatus) GetWorkflowStatus() *apworkflow.WorkflowStatus {
	if s.Ws == nil {
		s.Ws = &apworkflow.WorkflowStatus{}
	}
	return s.Ws
}

func (s *rotStatus) GetConditions() []metav1.Condition  { return s.Conditions }
func (s *rotStatus) SetConditions(c []metav1.Condition) { s.Conditions = c }
func (s *rotStatus) GetIsOnError() bool                 { return s.IsOnError != nil && *s.IsOnError }
func (s *rotStatus) SetIsOnError(b bool)                { s.IsOnError = &b }
func (s *rotStatus) GetLastErrorMessage() string        { return s.LastError }
func (s *rotStatus) SetLastErrorMessage(m string)       { s.LastError = m }
func (s *rotStatus) GetObservedGeneration() int64       { return s.ObsGen }
func (s *rotStatus) SetObservedGeneration(v int64)      { s.ObsGen = v }
func (s *rotStatus) GetPhaseName() shared.PhaseName     { return s.PhaseName }
func (s *rotStatus) SetPhaseName(n shared.PhaseName)    { s.PhaseName = n }

type rotObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Status rotStatus
}

func (o *rotObject) GetStatus() object.MultiPhaseObjectStatus {
	return &o.Status
}

func (o *rotObject) DeepCopyObject() runtime.Object {
	cp := *o
	cp.ObjectMeta = *o.DeepCopy()
	return &cp
}

var (
	_ object.MultiPhaseObject       = (*rotObject)(nil)
	_ object.MultiPhaseObjectStatus = (*rotStatus)(nil)
	_ workflow.WorkflowStatusGetter = (*rotStatus)(nil)
)

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

var rotGV = schema.GroupVersion{Group: "test.operator-sdk-extra", Version: "v1"}

func newFakeClientWithRot(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	scheme.AddKnownTypes(rotGV, &rotObject{})
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func testLogger() *logrus.Entry {
	return logrus.NewEntry(logrus.New())
}

func newRotObject() *rotObject {
	return &rotObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
}

func testSpecBuilder(o *rotObject) certificate.TLSSpec {
	return certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "test.example.com",
		DNSNames:         []string{"test.example.com"},
		LeafValidityDays: 365,
		RenewalDays:      30,
	}
}

func newStep(c client.Client, backend certificate.TLSBackend[*rotObject], opts ...rotation.Option[*rotObject]) workflow.WorkflowStepReconcilerActionWithDiff[*rotObject, client.Object] {
	return newStepRecorder(c, backend, record.NewFakeRecorder(10), opts...)
}

func newStepRecorder(c client.Client, backend certificate.TLSBackend[*rotObject], recorder record.EventRecorder, opts ...rotation.Option[*rotObject]) workflow.WorkflowStepReconcilerActionWithDiff[*rotObject, client.Object] {
	return rotation.NewTLSStep[*rotObject](
		c, "tls", "TLSCertificatesReady", recorder, "test-manager",
		backend, certificate.TLSSpecProviderFunc[*rotObject](testSpecBuilder), opts...,
	)
}

func newStepProvider(c client.Client, backend certificate.TLSBackend[*rotObject], provider certificate.TLSSpecProvider[*rotObject], opts ...rotation.Option[*rotObject]) workflow.WorkflowStepReconcilerActionWithDiff[*rotObject, client.Object] {
	return rotation.NewTLSStep[*rotObject](
		c, "tls", "TLSCertificatesReady", record.NewFakeRecorder(10), "test-manager",
		backend, provider, opts...,
	)
}

// stubBackend is a configurable TLSBackend for exercising error and edge paths
// that the real backends cannot produce.
type stubBackend struct {
	objs []client.Object
	err  error
	saga bool
}

func (b *stubBackend) DesiredObjects(ctx context.Context, o *rotObject, spec certificate.TLSSpec) ([]client.Object, error) {
	return b.objs, b.err
}

func (b *stubBackend) CertificateSecretName(o *rotObject, spec certificate.TLSSpec) string {
	return "test-tls"
}

func (b *stubBackend) RequiresRotationSaga() bool {
	return b.saga
}

func findSecret(t *testing.T, objs []client.Object, name string) *corev1.Secret {
	t.Helper()
	for _, o := range objs {
		if sec, ok := o.(*corev1.Secret); ok && sec.Name == name {
			return sec
		}
	}
	t.Fatalf("secret %q not found", name)
	return nil
}

func makeCertPEM(t *testing.T, cn string, notBefore, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
		DNSNames:              []string{"test.example.com"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func newSecret(name, namespace string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}

func TestNewTLSStep(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	require.NotNil(t, step)
	assert.Equal(t, shared.PhaseName("tls"), step.GetPhaseName())
}

func TestRead_NoSagaBackend_SingleCycle(t *testing.T) {
	step := newStep(newFakeClient(t), byo.NewBYOBackend[*rotObject]())
	o := newRotObject()
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Empty(t, read.GetExpectedObjects())
	_, ok := data["rotationStartPhase"]
	assert.False(t, ok)
	assert.True(t, step.IsPhaseEmpty(o))
}

func TestRead_SteadyState_NoRenewal(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*rotObject]()
	o := newRotObject()
	spec := testSpecBuilder(o)
	objs, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	require.Len(t, objs, 2)
	caSecret := objs[0].(*corev1.Secret)
	leafSecret := objs[1].(*corev1.Secret)

	c := newFakeClient(t, caSecret, leafSecret)
	step := newStep(c, backend)

	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Len(t, read.GetExpectedObjects(), 2)
	assert.Len(t, read.GetCurrentObjects(), 2)

	_, ok := data["rotationRenewed"]
	assert.False(t, ok)
	pubLeaf := data["tlsSecret"].(*corev1.Secret)
	pubCA := data["caSecret"].(*corev1.Secret)
	assert.NotNil(t, pubLeaf)
	assert.NotNil(t, pubCA)
	assert.NotNil(t, data["leafCert"])
	assert.NotNil(t, data["caCert"])
	// Published copies are sanitized: private keys removed, certs retained.
	assert.NotContains(t, pubLeaf.Data, selfmanaged.KeyKey)
	assert.Contains(t, pubLeaf.Data, selfmanaged.CertKey)
	assert.NotContains(t, pubCA.Data, selfmanaged.CAKeyPrivate)
	assert.Contains(t, pubCA.Data, selfmanaged.CAKey)
	assert.True(t, step.IsPhaseEmpty(o))
}

// TestRead_PublishedSecretsAreSanitized verifies that the Secrets published
// into the data blackboard are deep copies with private-key material removed,
// while the original objects stored in the fake client retain their keys.
func TestRead_PublishedSecretsAreSanitized(t *testing.T) {
	now := time.Now()
	leaf := newSecret("test-tls", "default", map[string][]byte{
		selfmanaged.CertKey: makeCertPEM(t, "test.example.com", now.Add(-1*time.Hour), now.Add(365*24*time.Hour)),
		selfmanaged.KeyKey:  []byte("leaf-private-key"),
		selfmanaged.CAKey:   makeCertPEM(t, "test.example.com-ca", now.Add(-1*time.Hour), now.Add(730*24*time.Hour)),
	})
	ca := newSecret("test-tls-ca", "default", map[string][]byte{
		selfmanaged.CAKey:        makeCertPEM(t, "test.example.com-ca", now.Add(-1*time.Hour), now.Add(730*24*time.Hour)),
		selfmanaged.CAKeyPrivate: []byte("ca-private-key"),
		selfmanaged.CRLKey:       []byte("ca-crl-der"),
	})

	c := newFakeClient(t, leaf, ca)
	step := newStep(c, selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	data := map[string]any{}
	_, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	publishedLeaf, ok := data["tlsSecret"].(*corev1.Secret)
	require.True(t, ok)
	publishedCA, ok := data["caSecret"].(*corev1.Secret)
	require.True(t, ok)

	// Published copies are sanitized: private keys absent, certs retained.
	assert.NotContains(t, publishedLeaf.Data, selfmanaged.KeyKey)
	assert.Contains(t, publishedLeaf.Data, selfmanaged.CertKey)
	assert.Contains(t, publishedLeaf.Data, selfmanaged.CAKey)
	assert.NotContains(t, publishedCA.Data, selfmanaged.CAKeyPrivate)
	assert.Contains(t, publishedCA.Data, selfmanaged.CAKey)
	// Certificate material that is not a private key must survive sanitization.
	assert.Contains(t, publishedCA.Data, selfmanaged.CRLKey)

	// Metadata is preserved on the published copies.
	assert.Equal(t, "test-tls", publishedLeaf.Name)
	assert.Equal(t, "default", publishedLeaf.Namespace)
	assert.Equal(t, "test-tls-ca", publishedCA.Name)
	assert.Equal(t, "default", publishedCA.Namespace)

	// The original objects (as stored by the fake client) still hold the keys.
	gotLeaf := &corev1.Secret{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "test-tls"}, gotLeaf))
	assert.Contains(t, gotLeaf.Data, selfmanaged.KeyKey)
	gotCA := &corev1.Secret{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "test-tls-ca"}, gotCA))
	assert.Contains(t, gotCA.Data, selfmanaged.CAKeyPrivate)
}

func TestRead_RenewalTriggered_MissingSecrets(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	expected := read.GetExpectedObjects()
	require.Len(t, expected, 2)
	newCA := findSecret(t, expected, "test-tls-ca")
	newLeaf := findSecret(t, expected, "test-tls")
	assert.Equal(t, newCA.Data[selfmanaged.CAKey], newLeaf.Data[selfmanaged.CAKey])
	assert.Equal(t, true, data["rotationRenewed"])
	assert.Equal(t, apworkflow.WorkflowPhase(""), data["rotationStartPhase"])
}

func TestRead_RenewalTriggered_WithOldSecret(t *testing.T) {
	now := time.Now()
	oldLeaf := newSecret("test-tls", "default", map[string][]byte{
		selfmanaged.CertKey: makeCertPEM(t, "test.example.com", now.Add(-48*time.Hour), now.Add(-24*time.Hour)),
		selfmanaged.CAKey:   []byte("OLD-CA-PEM"),
	})
	oldCA := newSecret("test-tls-ca", "default", map[string][]byte{
		selfmanaged.CAKey:        makeCertPEM(t, "old-ca", now.Add(-48*time.Hour), now.Add(-24*time.Hour)),
		selfmanaged.CAKeyPrivate: []byte("old-key"),
	})

	step := newStep(newFakeClient(t, oldLeaf, oldCA), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	expected := read.GetExpectedObjects()
	require.Len(t, expected, 2)
	newCA := findSecret(t, expected, "test-tls-ca")
	newLeaf := findSecret(t, expected, "test-tls")

	bundled := append([]byte{}, newCA.Data[selfmanaged.CAKey]...)
	bundled = append(bundled, []byte("OLD-CA-PEM")...)
	assert.Equal(t, bundled, newLeaf.Data[selfmanaged.CAKey])
	assert.Equal(t, true, data["rotationRenewed"])
}

func TestRead_Rotate_Stable(t *testing.T) {
	leaf := newSecret("test-tls", "default", map[string][]byte{
		selfmanaged.CertKey: []byte("x"),
		selfmanaged.CAKey:   []byte("bundle"),
	})
	ca := newSecret("test-tls-ca", "default", map[string][]byte{
		selfmanaged.CAKey: []byte("newCA"),
	})

	step := newStep(newFakeClient(t, leaf, ca), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseRotate}
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Len(t, read.GetExpectedObjects(), 2)
	assert.Len(t, read.GetCurrentObjects(), 2)
	assert.Equal(t, rotation.PhaseRotate, data["rotationStartPhase"])
	assert.Equal(t, rotation.PhaseRotate, step.CurrentPhase(o))
}

func TestRead_Converge_CleanLeaf(t *testing.T) {
	leaf := newSecret("test-tls", "default", map[string][]byte{
		selfmanaged.CertKey: []byte("x"),
		selfmanaged.CAKey:   []byte("newCA-bundle-oldCA"),
	})
	ca := newSecret("test-tls-ca", "default", map[string][]byte{
		selfmanaged.CAKey: []byte("newCA"),
	})

	step := newStep(newFakeClient(t, leaf, ca), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseConverge}
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	expected := read.GetExpectedObjects()
	require.Len(t, expected, 2)
	expectedLeaf := findSecret(t, expected, "test-tls")
	expectedCA := findSecret(t, expected, "test-tls-ca")
	assert.Equal(t, expectedCA.Data[selfmanaged.CAKey], expectedLeaf.Data[selfmanaged.CAKey])
	assert.Equal(t, []byte("newCA"), expectedLeaf.Data[selfmanaged.CAKey])
}

func TestRead_Converge_MissingSecretsFallback(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseConverge}
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	require.Len(t, read.GetExpectedObjects(), 2)
}

func TestRead_UnknownPhase(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: "Weird"}
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Empty(t, read.GetExpectedObjects())
	assert.True(t, step.IsPhaseEmpty(o))
}

type failingClient struct {
	client.Client
	failName string
	err      error
}

func (f *failingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key.Name == f.failName {
		return f.err
	}
	return f.Client.Get(ctx, key, obj, opts...)
}

func TestRead_GetError(t *testing.T) {
	boom := errors.New("boom")

	t.Run("leaf", func(t *testing.T) {
		c := &failingClient{Client: newFakeClient(t), failName: "test-tls", err: boom}
		step := newStep(c, selfmanaged.NewSelfManagedBackend[*rotObject]())
		_, _, err := step.Read(context.Background(), newRotObject(), map[string]any{}, testLogger())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read leaf secret")
	})

	t.Run("ca", func(t *testing.T) {
		c := &failingClient{Client: newFakeClient(t), failName: "test-tls-ca", err: boom}
		step := newStep(c, selfmanaged.NewSelfManagedBackend[*rotObject]())
		_, _, err := step.Read(context.Background(), newRotObject(), map[string]any{}, testLogger())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read CA secret")
	})
}

func TestOnDiff_NoSaga(t *testing.T) {
	step := newStep(newFakeClient(t), byo.NewBYOBackend[*rotObject]())
	res, err := step.OnDiff(context.Background(), newRotObject(), map[string]any{}, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
}

func TestOnDiff_PhaseEmpty(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	data := map[string]any{"rotationStartPhase": apworkflow.WorkflowPhase("")}
	res, err := step.OnDiff(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.True(t, step.IsPhaseEmpty(o))
}

func TestOnDiff_PhaseConverge(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseConverge}
	data := map[string]any{"rotationStartPhase": rotation.PhaseConverge}
	res, err := step.OnDiff(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Equal(t, rotation.PhaseConverge, step.CurrentPhase(o))
}

func TestOnDiff_Rotate_NotConverged(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject](),
		rotation.WithConvergenceCheck(func(ctx context.Context, o *rotObject, data map[string]any) (bool, error) {
			return false, nil
		}))
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseRotate}
	data := map[string]any{"rotationStartPhase": rotation.PhaseRotate}
	res, err := step.OnDiff(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{RequeueAfter: workflow.DefaultRequeueAfter}, res)
	assert.Equal(t, rotation.PhaseRotate, step.CurrentPhase(o))
}

func TestOnDiff_Rotate_Converged(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject](),
		rotation.WithConvergenceCheck(func(ctx context.Context, o *rotObject, data map[string]any) (bool, error) {
			return true, nil
		}))
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseRotate}
	data := map[string]any{"rotationStartPhase": rotation.PhaseRotate}
	res, err := step.OnDiff(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Equal(t, rotation.PhaseConverge, step.CurrentPhase(o))
}

func TestOnDiff_Rotate_NilCheck(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseRotate}
	data := map[string]any{"rotationStartPhase": rotation.PhaseRotate}
	res, err := step.OnDiff(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Equal(t, rotation.PhaseConverge, step.CurrentPhase(o))
}

func TestOnDiff_Rotate_CheckError(t *testing.T) {
	sentinel := errors.New("check failed")
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject](),
		rotation.WithConvergenceCheck(func(ctx context.Context, o *rotObject, data map[string]any) (bool, error) {
			return false, sentinel
		}))
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseRotate}
	data := map[string]any{"rotationStartPhase": rotation.PhaseRotate}
	_, err := step.OnDiff(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, rotation.PhaseRotate, step.CurrentPhase(o))
}

func conditionIsTrue(o *rotObject) bool {
	for _, c := range o.Status.Conditions {
		if c.Type == "TLSCertificatesReady" && c.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func TestOnSuccess_PhaseEmpty_Renewed(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	o.Status.Conditions = []metav1.Condition{{Type: "TLSCertificatesReady", Status: metav1.ConditionFalse}}
	data := map[string]any{
		"rotationStartPhase": apworkflow.WorkflowPhase(""),
		"rotationRenewed":    true,
	}
	res, err := step.OnSuccess(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Equal(t, rotation.PhaseRotate, step.CurrentPhase(o))
	assert.True(t, conditionIsTrue(o))
}

func TestOnSuccess_PhaseEmpty_NotRenewed(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	data := map[string]any{"rotationStartPhase": apworkflow.WorkflowPhase("")}
	res, err := step.OnSuccess(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.True(t, step.IsPhaseEmpty(o))
}

func TestOnSuccess_Converge(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseConverge}
	data := map[string]any{"rotationStartPhase": rotation.PhaseConverge}
	res, err := step.OnSuccess(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.True(t, step.IsPhaseEmpty(o))
}

func TestOnSuccess_NoSaga(t *testing.T) {
	step := newStep(newFakeClient(t), byo.NewBYOBackend[*rotObject]())
	o := newRotObject()
	o.Status.Conditions = []metav1.Condition{{Type: "TLSCertificatesReady", Status: metav1.ConditionFalse}}
	res, err := step.OnSuccess(context.Background(), o, map[string]any{}, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.True(t, step.IsPhaseEmpty(o))
	assert.True(t, conditionIsTrue(o))
}

func TestOnSuccess_InheritedError(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseRotate}
	o.Status.Conditions = []metav1.Condition{{Type: "TLSCertificatesReady", Status: metav1.ConditionFalse}}
	data := map[string]any{"rotationStartPhase": rotation.PhaseRotate}
	res, err := step.OnSuccess(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Equal(t, rotation.PhaseRotate, step.CurrentPhase(o))
	assert.True(t, conditionIsTrue(o))
}

func TestDecorators(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject](),
		rotation.WithLabelsDecorator(func(o *rotObject, obj client.Object) {
			obj.SetLabels(map[string]string{"app": "x"})
		}),
		rotation.WithAnnotationsDecorator(func(o *rotObject, obj client.Object) {
			obj.SetAnnotations(map[string]string{"rot": "1"})
		}))
	o := newRotObject()
	data := map[string]any{}
	read, _, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)

	expected := read.GetExpectedObjects()
	require.Len(t, expected, 2)
	newCA := findSecret(t, expected, "test-tls-ca")
	newLeaf := findSecret(t, expected, "test-tls")
	assert.Equal(t, map[string]string{"app": "x"}, newCA.Labels)
	assert.Equal(t, map[string]string{"rot": "1"}, newCA.Annotations)
	assert.Equal(t, map[string]string{"app": "x"}, newLeaf.Labels)
	assert.Equal(t, map[string]string{"rot": "1"}, newLeaf.Annotations)
}

// reflectApply writes the expected secrets into the fake client, simulating a
// successful SSA apply (the fake client cannot run SSA dry-run).
func reflectApply(t *testing.T, c client.Client, objs []client.Object) {
	t.Helper()
	for _, obj := range objs {
		sec, ok := obj.(*corev1.Secret)
		require.True(t, ok)
		existing := &corev1.Secret{}
		err := c.Get(context.Background(), client.ObjectKeyFromObject(sec), existing)
		if apierrors.IsNotFound(err) {
			require.NoError(t, c.Create(context.Background(), sec))
			continue
		}
		require.NoError(t, err)
		sec.ResourceVersion = existing.ResourceVersion
		require.NoError(t, c.Update(context.Background(), sec))
	}
}

func TestFullProgression(t *testing.T) {
	now := time.Now()
	oldLeaf := newSecret("test-tls", "default", map[string][]byte{
		selfmanaged.CertKey: makeCertPEM(t, "test.example.com", now.Add(-48*time.Hour), now.Add(-24*time.Hour)),
		selfmanaged.CAKey:   []byte("OLD-CA-PEM"),
	})
	oldCA := newSecret("test-tls-ca", "default", map[string][]byte{
		selfmanaged.CAKey:        makeCertPEM(t, "old-ca", now.Add(-48*time.Hour), now.Add(-24*time.Hour)),
		selfmanaged.CAKeyPrivate: []byte("old-key"),
	})

	c := newFakeClient(t, oldLeaf, oldCA)
	step := newStep(c, selfmanaged.NewSelfManagedBackend[*rotObject](),
		rotation.WithConvergenceCheck(func(ctx context.Context, o *rotObject, data map[string]any) (bool, error) {
			return true, nil
		}))
	o := newRotObject()

	// Cycle 1: "" -> Rotate (renewal, bundle old CA into leaf).
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	expected := read.GetExpectedObjects()
	require.Len(t, expected, 2)
	newCA := findSecret(t, expected, "test-tls-ca")
	newLeaf := findSecret(t, expected, "test-tls")
	bundled := append([]byte{}, newCA.Data[selfmanaged.CAKey]...)
	bundled = append(bundled, []byte("OLD-CA-PEM")...)
	assert.Equal(t, bundled, newLeaf.Data[selfmanaged.CAKey])

	_, err = step.OnDiff(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	_, err = step.OnSuccess(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, rotation.PhaseRotate, step.CurrentPhase(o))
	reflectApply(t, c, expected)

	// Cycle 2: Rotate -> Converge (stable, convergence gate).
	data = map[string]any{}
	_, res, err = step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Equal(t, rotation.PhaseRotate, data["rotationStartPhase"])

	_, err = step.OnDiff(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	_, err = step.OnSuccess(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, rotation.PhaseConverge, step.CurrentPhase(o))

	// Cycle 3: Converge -> "" (strip old CA).
	data = map[string]any{}
	read, res, err = step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	expected = read.GetExpectedObjects()
	require.Len(t, expected, 2)
	currentCA := findSecret(t, expected, "test-tls-ca")
	cleanLeaf := findSecret(t, expected, "test-tls")
	assert.Equal(t, currentCA.Data[selfmanaged.CAKey], cleanLeaf.Data[selfmanaged.CAKey])
	assert.NotEqual(t, bundled, cleanLeaf.Data[selfmanaged.CAKey])

	_, err = step.OnSuccess(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.True(t, step.IsPhaseEmpty(o))
	reflectApply(t, c, expected)
}

func TestRead_NoSagaBackend_WithObjectsAndCurrent(t *testing.T) {
	leaf := newSecret("test-tls", "default", map[string][]byte{selfmanaged.CertKey: []byte("x")})
	ca := newSecret("test-tls-ca", "default", map[string][]byte{selfmanaged.CAKey: []byte("y")})
	step := newStep(newFakeClient(t, leaf, ca), &stubBackend{objs: []client.Object{newSecret("extra", "default", nil)}, saga: false})
	o := newRotObject()
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Len(t, read.GetExpectedObjects(), 1)
	// Non-saga backends do not manage the leaf/CA Secrets: they must NOT be
	// registered as current objects, otherwise the SSA diff would delete them
	// as orphans.
	assert.Empty(t, read.GetCurrentObjects())
	_, ok := data["rotationStartPhase"]
	assert.False(t, ok)
}

func TestRead_NoSagaBackend_Error(t *testing.T) {
	boom := errors.New("boom")
	step := newStep(newFakeClient(t), &stubBackend{err: boom, saga: false})
	_, _, err := step.Read(context.Background(), newRotObject(), map[string]any{}, testLogger())
	require.ErrorIs(t, err, boom)
}

func TestRead_Saga_CANeedsRenewalError(t *testing.T) {
	leaf := newSecret("test-tls", "default", map[string][]byte{
		selfmanaged.CertKey: makeCertPEM(t, "test.example.com", time.Now().Add(-1*time.Hour), time.Now().Add(365*24*time.Hour)),
	})
	ca := newSecret("test-tls-ca", "default", map[string][]byte{selfmanaged.CAKey: []byte("not pem")})
	step := newStep(newFakeClient(t, leaf, ca), selfmanaged.NewSelfManagedBackend[*rotObject]())
	_, _, err := step.Read(context.Background(), newRotObject(), map[string]any{}, testLogger())
	require.Error(t, err)
}

func TestRead_Saga_DesiredObjectsError(t *testing.T) {
	boom := errors.New("boom")
	step := newStep(newFakeClient(t), &stubBackend{err: boom, saga: true})
	_, _, err := step.Read(context.Background(), newRotObject(), map[string]any{}, testLogger())
	require.ErrorIs(t, err, boom)
}

func TestRead_Saga_SplitError(t *testing.T) {
	step := newStep(newFakeClient(t), &stubBackend{objs: []client.Object{newSecret("wrong", "default", nil)}, saga: true})
	_, _, err := step.Read(context.Background(), newRotObject(), map[string]any{}, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "saga backend must return CA secret")
}

func TestRead_Converge_DesiredObjectsError(t *testing.T) {
	boom := errors.New("boom")
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseConverge}
	step := newStep(newFakeClient(t), &stubBackend{err: boom, saga: true})
	_, _, err := step.Read(context.Background(), o, map[string]any{}, testLogger())
	require.ErrorIs(t, err, boom)
}

func TestRead_Converge_PartialSecrets(t *testing.T) {
	t.Run("ca only", func(t *testing.T) {
		ca := newSecret("test-tls-ca", "default", map[string][]byte{selfmanaged.CAKey: []byte("y")})
		o := newRotObject()
		o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseConverge}
		step := newStep(newFakeClient(t, ca), selfmanaged.NewSelfManagedBackend[*rotObject]())
		read, _, err := step.Read(context.Background(), o, map[string]any{}, testLogger())
		require.NoError(t, err)
		assert.Len(t, read.GetExpectedObjects(), 2)
		assert.Len(t, read.GetCurrentObjects(), 1)
	})
	t.Run("leaf only", func(t *testing.T) {
		leaf := newSecret("test-tls", "default", map[string][]byte{selfmanaged.CertKey: []byte("x")})
		o := newRotObject()
		o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseConverge}
		step := newStep(newFakeClient(t, leaf), selfmanaged.NewSelfManagedBackend[*rotObject]())
		read, _, err := step.Read(context.Background(), o, map[string]any{}, testLogger())
		require.NoError(t, err)
		assert.Len(t, read.GetExpectedObjects(), 2)
		assert.Len(t, read.GetCurrentObjects(), 1)
	})
}

func TestRead_Converge_EmptyLeafData(t *testing.T) {
	// A Secret that exists but has no data must not panic the Converge path
	// (DeepCopy preserves a nil Data map, so the assignment must be guarded).
	leaf := newSecret("test-tls", "default", nil)
	ca := newSecret("test-tls-ca", "default", map[string][]byte{selfmanaged.CAKey: []byte("y")})
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseConverge}
	step := newStep(newFakeClient(t, leaf, ca), selfmanaged.NewSelfManagedBackend[*rotObject]())
	read, _, err := step.Read(context.Background(), o, map[string]any{}, testLogger())
	require.NoError(t, err)
	expected := read.GetExpectedObjects()
	require.Len(t, expected, 2)
	expectedLeaf := findSecret(t, expected, "test-tls")
	assert.Equal(t, []byte("y"), expectedLeaf.Data[selfmanaged.CAKey])
}

func TestRead_UnknownPhase_WithSecrets(t *testing.T) {
	leaf := newSecret("test-tls", "default", map[string][]byte{selfmanaged.CertKey: []byte("x")})
	ca := newSecret("test-tls-ca", "default", map[string][]byte{selfmanaged.CAKey: []byte("y")})
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: "Weird"}
	step := newStep(newFakeClient(t, leaf, ca), selfmanaged.NewSelfManagedBackend[*rotObject]())
	read, _, err := step.Read(context.Background(), o, map[string]any{}, testLogger())
	require.NoError(t, err)
	assert.Len(t, read.GetExpectedObjects(), 2)
	assert.Len(t, read.GetCurrentObjects(), 2)
	assert.True(t, step.IsPhaseEmpty(o))
}

// TestRead_StuckMidRotation_RecoversToRotate simulates a process crash after
// the bundled leaf was applied but before the "" -> Rotate phase advance was
// persisted: the phase reads "" while the leaf ca.crt is a newCA||oldCA
// bundle. NeedRenewal is false (the new leaf is fresh), so without recovery
// the old CA would stay trusted until the next renewal. The Read must detect
// the stale bundle and advance to Rotate so the saga proceeds to Converge
// and strips the old CA.
func TestRead_StuckMidRotation_RecoversToRotate(t *testing.T) {
	now := time.Now()
	// Fresh leaf cert (so NeedRenewal is false) but ca.crt is a bundle.
	freshLeafPEM := makeCertPEM(t, "test.example.com", now.Add(-1*time.Hour), now.Add(365*24*time.Hour))
	newCAPEM := makeCertPEM(t, "test.example.com-ca", now.Add(-1*time.Hour), now.Add(730*24*time.Hour))
	// Old CA still valid and far from expiry (rotation was triggered by the
	// leaf nearing expiry, not the CA; the CA has 2x validity). NeedRenewal
	// must be false so the steady-state path runs and the recovery fires.
	oldCAPEM := makeCertPEM(t, "old-ca", now.Add(-100*24*time.Hour), now.Add(800*24*time.Hour))
	bundledCA := append([]byte{}, newCAPEM...)
	bundledCA = append(bundledCA, oldCAPEM...)

	leaf := newSecret("test-tls", "default", map[string][]byte{
		selfmanaged.CertKey: freshLeafPEM,
		selfmanaged.CAKey:   bundledCA,
	})
	ca := newSecret("test-tls-ca", "default", map[string][]byte{
		selfmanaged.CAKey: newCAPEM,
	})

	step := newStep(newFakeClient(t, leaf, ca), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	// Steady state: expected == current (no regeneration).
	assert.Len(t, read.GetExpectedObjects(), 2)
	assert.Len(t, read.GetCurrentObjects(), 2)
	// Not renewed this cycle.
	_, renewed := data["rotationRenewed"]
	assert.False(t, renewed, "stuck recovery must not regenerate")
	// Phase advanced to Rotate to resume the saga.
	assert.Equal(t, rotation.PhaseRotate, step.CurrentPhase(o))
}

// TestRead_SteadyState_NoStaleBundle_NoAdvance confirms the recovery logic
// does NOT fire when the leaf ca.crt matches the CA secret ca.crt (steady
// state): the phase must stay "".
func TestRead_SteadyState_NoStaleBundle_NoAdvance(t *testing.T) {
	now := time.Now()
	leafPEM := makeCertPEM(t, "test.example.com", now.Add(-1*time.Hour), now.Add(365*24*time.Hour))
	caPEM := makeCertPEM(t, "test.example.com-ca", now.Add(-1*time.Hour), now.Add(730*24*time.Hour))
	leaf := newSecret("test-tls", "default", map[string][]byte{
		selfmanaged.CertKey: leafPEM,
		selfmanaged.CAKey:   caPEM,
	})
	ca := newSecret("test-tls-ca", "default", map[string][]byte{
		selfmanaged.CAKey: caPEM,
	})

	step := newStep(newFakeClient(t, leaf, ca), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	data := map[string]any{}
	_, _, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.True(t, step.IsPhaseEmpty(o), "phase must stay empty in steady state with no stale bundle")
}

// TestRead_LeafCADiffersButNotBundle_NoAdvance confirms the precise bundle
// detection: a leaf whose ca.crt differs from the CA secret ca.crt but is NOT
// a newCA||oldCA bundle (does not share the CA secret ca.crt as a prefix) must
// not over-trigger recovery. This avoids repairing unrelated corruption via
// the rotation saga.
func TestRead_LeafCADiffersButNotBundle_NoAdvance(t *testing.T) {
	now := time.Now()
	leafPEM := makeCertPEM(t, "test.example.com", now.Add(-1*time.Hour), now.Add(365*24*time.Hour))
	caPEM := makeCertPEM(t, "test.example.com-ca", now.Add(-1*time.Hour), now.Add(730*24*time.Hour))
	otherCAPEM := makeCertPEM(t, "other-ca", now.Add(-1*time.Hour), now.Add(730*24*time.Hour))
	leaf := newSecret("test-tls", "default", map[string][]byte{
		selfmanaged.CertKey: leafPEM,
		// Different ca.crt that is NOT a bundle (no shared prefix).
		selfmanaged.CAKey: otherCAPEM,
	})
	ca := newSecret("test-tls-ca", "default", map[string][]byte{
		selfmanaged.CAKey: caPEM,
	})

	step := newStep(newFakeClient(t, leaf, ca), selfmanaged.NewSelfManagedBackend[*rotObject]())
	o := newRotObject()
	_, _, err := step.Read(context.Background(), o, map[string]any{}, testLogger())
	require.NoError(t, err)
	assert.True(t, step.IsPhaseEmpty(o), "non-bundle ca.crt difference must not trigger recovery")
}

func TestRead_LeafOnly_SANAdd(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*rotObject]()
	o := newRotObject()
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", DNSNames: []string{"test.example.com"}, LeafValidityDays: 365, RenewalDays: 30}
	objs, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	ca := objs[0].(*corev1.Secret)
	leaf := objs[1].(*corev1.Secret)

	c := newFakeClient(t, ca, leaf)
	driftProvider := certificate.TLSSpecProviderFunc[*rotObject](func(o *rotObject) certificate.TLSSpec {
		return certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", DNSNames: []string{"test.example.com", "new.example.com"}, LeafValidityDays: 365, RenewalDays: 30}
	})
	step := newStepProvider(c, backend, driftProvider)
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	expected := read.GetExpectedObjects()
	require.Len(t, expected, 2)
	newLeaf := findSecret(t, expected, "test-tls")
	currentCA := findSecret(t, expected, "test-tls-ca")
	assert.NotEqual(t, leaf.Data[selfmanaged.CertKey], newLeaf.Data[selfmanaged.CertKey])
	assert.Equal(t, ca.Data[selfmanaged.CAKey], currentCA.Data[selfmanaged.CAKey])

	_, renewed := data["rotationRenewed"]
	assert.False(t, renewed, "leaf-only regen must not mark rotationRenewed")
	assert.True(t, step.IsPhaseEmpty(o), "leaf-only regen must not write a phase")

	sig, ok := data["tls.tls"].(*certificate.LayerSignals)
	require.True(t, ok)
	assert.True(t, sig.LeafRegenerated)
	assert.False(t, sig.CARotated)
	assert.Equal(t, []string{"new.example.com"}, sig.LeafChange.SANsAdded)
}

func TestRead_LeafOnly_SANRemove(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*rotObject]()
	o := newRotObject()
	buildSpec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", DNSNames: []string{"test.example.com", "extra.example.com"}, LeafValidityDays: 365, RenewalDays: 30}
	objs, err := backend.DesiredObjects(context.Background(), o, buildSpec)
	require.NoError(t, err)
	ca := objs[0].(*corev1.Secret)
	leaf := objs[1].(*corev1.Secret)

	c := newFakeClient(t, ca, leaf)
	driftProvider := certificate.TLSSpecProviderFunc[*rotObject](func(o *rotObject) certificate.TLSSpec {
		return certificate.TLSSpec{SecretName: "test-tls", CommonName: "test.example.com", DNSNames: []string{"test.example.com"}, LeafValidityDays: 365, RenewalDays: 30}
	})
	step := newStepProvider(c, backend, driftProvider)
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	expected := read.GetExpectedObjects()
	require.Len(t, expected, 2)
	newLeaf := findSecret(t, expected, "test-tls")
	assert.NotEqual(t, leaf.Data[selfmanaged.CertKey], newLeaf.Data[selfmanaged.CertKey])

	sig, ok := data["tls.tls"].(*certificate.LayerSignals)
	require.True(t, ok)
	assert.True(t, sig.LeafRegenerated)
	assert.False(t, sig.CARotated)
	assert.Equal(t, []string{"extra.example.com"}, sig.LeafChange.SANsRemoved)
	assert.False(t, certificate.ShouldRollout(certificate.RolloutOnAdditive, sig))
}

type rotNodeProvider struct {
	expected []string
	err      error
}

func (p *rotNodeProvider) ExpectedNodeNames(o *rotObject) ([]string, error) {
	return p.expected, p.err
}

func (p *rotNodeProvider) NodeCertSpec(o *rotObject, nodeName string) (string, []string, []string, error) {
	return nodeName + ".example.com", []string{nodeName + ".example.com"}, nil, nil
}

func TestRead_PerNode_NodeAdded(t *testing.T) {
	buildProvider := &rotNodeProvider{expected: []string{"node1"}}
	backend := pernode.NewPerNodeBackend[*rotObject](buildProvider)
	o := newRotObject()
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "cluster", Organization: "TestOrg", LeafValidityDays: 365, RenewalDays: 30}
	objs, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	ca := objs[0].(*corev1.Secret)
	leaf := objs[1].(*corev1.Secret)

	c := newFakeClient(t, ca, leaf)
	driftBackend := pernode.NewPerNodeBackend[*rotObject](&rotNodeProvider{expected: []string{"node1", "node2"}})
	specProvider := certificate.TLSSpecProviderFunc[*rotObject](func(o *rotObject) certificate.TLSSpec { return spec })
	step := newStepProvider(c, driftBackend, specProvider)

	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.True(t, step.IsPhaseEmpty(o))

	expected := read.GetExpectedObjects()
	require.Len(t, expected, 2)
	newLeaf := findSecret(t, expected, "test-tls")
	assert.Contains(t, newLeaf.Data, "node2"+pernode.NodeCertSuffix)

	_, renewed := data["rotationRenewed"]
	assert.False(t, renewed)
	_, ok := data["tlsSecret"]
	assert.False(t, ok, "per-node steps must not publish tlsSecret")

	sig, ok := data["tls.tls"].(*certificate.LayerSignals)
	require.True(t, ok)
	assert.True(t, sig.LeafRegenerated)
	assert.False(t, sig.CARotated)
	assert.Equal(t, []string{"node2"}, sig.LeafChange.NodesAdded)
	assert.False(t, certificate.ShouldRollout(certificate.RolloutOnAdditive, sig))
}

func TestRead_PerNode_NodeRemoved(t *testing.T) {
	buildProvider := &rotNodeProvider{expected: []string{"node1", "node2"}}
	backend := pernode.NewPerNodeBackend[*rotObject](buildProvider)
	o := newRotObject()
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "cluster", Organization: "TestOrg", LeafValidityDays: 365, RenewalDays: 30}
	objs, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	ca := objs[0].(*corev1.Secret)
	leaf := objs[1].(*corev1.Secret)

	c := newFakeClient(t, ca, leaf)
	driftBackend := pernode.NewPerNodeBackend[*rotObject](&rotNodeProvider{expected: []string{"node1"}})
	specProvider := certificate.TLSSpecProviderFunc[*rotObject](func(o *rotObject) certificate.TLSSpec { return spec })
	step := newStepProvider(c, driftBackend, specProvider)

	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.True(t, step.IsPhaseEmpty(o))

	expected := read.GetExpectedObjects()
	require.Len(t, expected, 2)
	newLeaf := findSecret(t, expected, "test-tls")
	assert.NotContains(t, newLeaf.Data, "node2"+pernode.NodeCertSuffix)

	sig, ok := data["tls.tls"].(*certificate.LayerSignals)
	require.True(t, ok)
	assert.True(t, sig.LeafRegenerated)
	assert.Equal(t, []string{"node2"}, sig.LeafChange.NodesRemoved)
}

func TestRead_PerNode_Expiring(t *testing.T) {
	buildProvider := &rotNodeProvider{expected: []string{"node1"}}
	backend := pernode.NewPerNodeBackend[*rotObject](buildProvider)
	o := newRotObject()
	spec := certificate.TLSSpec{SecretName: "test-tls", CommonName: "cluster", Organization: "TestOrg", LeafValidityDays: 1, RenewalDays: 30}
	objs, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	ca := objs[0].(*corev1.Secret)
	leaf := objs[1].(*corev1.Secret)

	c := newFakeClient(t, ca, leaf)
	specProvider := certificate.TLSSpecProviderFunc[*rotObject](func(o *rotObject) certificate.TLSSpec { return spec })
	step := newStepProvider(c, backend, specProvider)

	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.True(t, step.IsPhaseEmpty(o))

	expected := read.GetExpectedObjects()
	require.Len(t, expected, 2)
	newLeaf := findSecret(t, expected, "test-tls")
	assert.NotEqual(t, leaf.Data["node1"+pernode.NodeCertSuffix], newLeaf.Data["node1"+pernode.NodeCertSuffix])

	sig, ok := data["tls.tls"].(*certificate.LayerSignals)
	require.True(t, ok)
	assert.True(t, sig.LeafRegenerated)
	assert.Equal(t, certificate.LeafExpiring, sig.LeafChange.Reason)
	assert.True(t, certificate.ShouldRollout(certificate.RolloutOnAdditive, sig))
}

func TestRead_ForceAll_FullSaga(t *testing.T) {
	o := newRotObject()
	o.SetAnnotations(map[string]string{certificate.AnnotationForceRegenerateAll: "true"})
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	require.Len(t, read.GetExpectedObjects(), 2)
	assert.Equal(t, true, data["rotationRenewed"])

	sig, ok := data["tls.tls"].(*certificate.LayerSignals)
	require.True(t, ok)
	assert.True(t, sig.Forced)
	assert.True(t, sig.CARotated)
	assert.True(t, sig.LeafRegenerated)
}

func TestRead_ForceLeaf_LeafOnly(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*rotObject]()
	o := newRotObject()
	spec := testSpecBuilder(o)
	objs, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	ca := objs[0].(*corev1.Secret)
	leaf := objs[1].(*corev1.Secret)

	c := newFakeClient(t, ca, leaf)
	o.SetAnnotations(map[string]string{certificate.AnnotationForceRegenerateLeaf: "true"})
	step := newStep(c, backend)

	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	require.Len(t, read.GetExpectedObjects(), 2)
	assert.True(t, step.IsPhaseEmpty(o))

	_, renewed := data["rotationRenewed"]
	assert.False(t, renewed)

	sig, ok := data["tls.tls"].(*certificate.LayerSignals)
	require.True(t, ok)
	assert.True(t, sig.Forced)
	assert.True(t, sig.LeafRegenerated)
	assert.False(t, sig.CARotated)
	assert.Equal(t, certificate.LeafForceRegen, sig.LeafChange.Reason)
}

func TestRead_ForceLeaf_MissingCA_FallsBackToSaga(t *testing.T) {
	o := newRotObject()
	o.SetAnnotations(map[string]string{certificate.AnnotationForceRegenerateLeaf: "true"})
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	require.Len(t, read.GetExpectedObjects(), 2)
	assert.Equal(t, true, data["rotationRenewed"])

	sig, ok := data["tls.tls"].(*certificate.LayerSignals)
	require.True(t, ok)
	assert.True(t, sig.Forced)
	assert.True(t, sig.CARotated)
}

func TestRead_ForceAllWins_OverForceLeaf(t *testing.T) {
	o := newRotObject()
	o.SetAnnotations(map[string]string{
		certificate.AnnotationForceRegenerateAll:  "true",
		certificate.AnnotationForceRegenerateLeaf: "true",
	})
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	data := map[string]any{}
	_, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Equal(t, true, data["rotationRenewed"])

	sig, ok := data["tls.tls"].(*certificate.LayerSignals)
	require.True(t, ok)
	assert.True(t, sig.Forced)
	assert.True(t, sig.CARotated)
}

func TestRead_ForceIgnored_MidSaga(t *testing.T) {
	o := newRotObject()
	o.Status.Ws = &apworkflow.WorkflowStatus{CurrentPhase: rotation.PhaseRotate}
	o.SetAnnotations(map[string]string{certificate.AnnotationForceRegenerateAll: "true"})
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject]())
	data := map[string]any{}
	_, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Equal(t, "true", o.GetAnnotations()[certificate.AnnotationForceRegenerateAll], "force annotation must be left in place mid-saga")
	_, ok := data["tls.tls"]
	assert.False(t, ok, "no signal must be published mid-saga")
}

func TestRead_NonSaga_ForceUnsupported(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	o := newRotObject()
	o.TypeMeta = metav1.TypeMeta{APIVersion: rotGV.String(), Kind: "RotObject"}
	o.SetAnnotations(map[string]string{certificate.AnnotationForceRegenerateAll: "true"})
	c := newFakeClientWithRot(t, o)
	step := newStepRecorder(c, byo.NewBYOBackend[*rotObject](), recorder)

	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Empty(t, read.GetExpectedObjects())

	_, ok := o.GetAnnotations()[certificate.AnnotationForceRegenerateAll]
	assert.False(t, ok, "force annotation must be removed for non-saga backends")

	select {
	case ev := <-recorder.Events:
		assert.Contains(t, ev, "TLSForceUnsupported")
	default:
		t.Fatal("expected TLSForceUnsupported event")
	}
}

func TestRead_NonSaga_ForceUnsupported_CustomAnnotation(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	o := newRotObject()
	o.TypeMeta = metav1.TypeMeta{APIVersion: rotGV.String(), Kind: "RotObject"}
	o.SetAnnotations(map[string]string{"my.example.com/force": "true"})
	c := newFakeClientWithRot(t, o)
	step := newStepRecorder(c, byo.NewBYOBackend[*rotObject](), recorder,
		rotation.WithForceRegenerateAllAnnotation[*rotObject]("my.example.com/force"))

	data := map[string]any{}
	_, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	_, ok := o.GetAnnotations()["my.example.com/force"]
	assert.False(t, ok)
}

func TestOnSuccess_ForceAll_RemovesAnnotations(t *testing.T) {
	o := newRotObject()
	o.TypeMeta = metav1.TypeMeta{APIVersion: rotGV.String(), Kind: "RotObject"}
	o.SetAnnotations(map[string]string{certificate.AnnotationForceRegenerateAll: "true"})
	c := newFakeClientWithRot(t, o)
	step := newStep(c, selfmanaged.NewSelfManagedBackend[*rotObject]())

	data := map[string]any{
		"rotationStartPhase": apworkflow.WorkflowPhase(""),
		"rotationRenewed":    true,
		"tls.tls":            &certificate.LayerSignals{Forced: true, CARotated: true},
	}
	res, err := step.OnSuccess(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.Equal(t, rotation.PhaseRotate, step.CurrentPhase(o))

	_, ok := o.GetAnnotations()[certificate.AnnotationForceRegenerateAll]
	assert.False(t, ok, "force-all annotation must be removed on success")
}

func TestOnSuccess_ForceLeaf_RemovesAnnotation(t *testing.T) {
	o := newRotObject()
	o.TypeMeta = metav1.TypeMeta{APIVersion: rotGV.String(), Kind: "RotObject"}
	o.SetAnnotations(map[string]string{certificate.AnnotationForceRegenerateLeaf: "true"})
	c := newFakeClientWithRot(t, o)
	step := newStep(c, selfmanaged.NewSelfManagedBackend[*rotObject]())

	data := map[string]any{
		"rotationStartPhase": apworkflow.WorkflowPhase(""),
		"tls.tls":            &certificate.LayerSignals{Forced: true, LeafRegenerated: true},
	}
	res, err := step.OnSuccess(context.Background(), o, data, multiphase.NewMultiPhaseDiff[client.Object](), testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.True(t, step.IsPhaseEmpty(o), "force-leaf must not advance phase")

	_, ok := o.GetAnnotations()[certificate.AnnotationForceRegenerateLeaf]
	assert.False(t, ok, "force-leaf annotation must be removed on success")
}

func TestRead_TwoSteps_NamespacedSignals(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*rotObject]()
	o := newRotObject()
	spec := testSpecBuilder(o)
	objs, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	ca := objs[0].(*corev1.Secret)
	leaf := objs[1].(*corev1.Secret)
	c := newFakeClient(t, ca, leaf)

	forceObj := newRotObject()
	forceObj.SetAnnotations(map[string]string{certificate.AnnotationForceRegenerateAll: "true"})
	provider := certificate.TLSSpecProviderFunc[*rotObject](testSpecBuilder)

	transport := rotation.NewTLSStep[*rotObject](c, "transport", "TLSCertificatesReady", record.NewFakeRecorder(10), "test-manager", backend, provider)
	api := rotation.NewTLSStep[*rotObject](c, "api", "TLSCertificatesReady", record.NewFakeRecorder(10), "test-manager", backend, provider)

	data := map[string]any{}
	_, _, err = transport.Read(context.Background(), forceObj, data, testLogger())
	require.NoError(t, err)
	_, _, err = api.Read(context.Background(), forceObj, data, testLogger())
	require.NoError(t, err)

	_, okTransport := data["tls.transport"].(*certificate.LayerSignals)
	_, okAPI := data["tls.api"].(*certificate.LayerSignals)
	assert.True(t, okTransport, "transport signal must be namespaced")
	assert.True(t, okAPI, "api signal must be namespaced")
}

func TestRead_SagaNoLeafManager_MissingLeaf_FallsBackToSaga(t *testing.T) {
	now := time.Now()
	ca := newSecret("test-tls-ca", "default", map[string][]byte{
		selfmanaged.CAKey: makeCertPEM(t, "test.example.com-ca", now.Add(-1*time.Hour), now.Add(730*24*time.Hour)),
	})
	c := newFakeClient(t, ca)
	stub := &stubBackend{saga: true, objs: []client.Object{
		ca,
		newSecret("test-tls", "default", map[string][]byte{selfmanaged.CertKey: makeCertPEM(t, "test.example.com", now.Add(-1*time.Hour), now.Add(365*24*time.Hour))}),
	}}
	step := newStep(c, stub)

	data := map[string]any{}
	read, res, err := step.Read(context.Background(), newRotObject(), data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	require.Len(t, read.GetExpectedObjects(), 2)
	assert.Equal(t, true, data["rotationRenewed"])
}

func TestRead_WithCertificateCustomizer_MutatesSpec(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*rotObject]()
	customizer := certificate.CertificateCustomizerFunc[*rotObject](func(o *rotObject, base certificate.TLSSpec) (certificate.TLSSpec, error) {
		base.DNSNames = append(base.DNSNames, "customized.example.com")
		return base, nil
	})
	step := newStep(newFakeClient(t), backend, rotation.WithCertificateCustomizer[*rotObject](customizer))
	o := newRotObject()

	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	newLeaf := findSecret(t, read.GetExpectedObjects(), "test-tls")
	block, _ := pem.Decode(newLeaf.Data[selfmanaged.CertKey])
	require.NotNil(t, block)
	leaf, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, []string{"test.example.com", "customized.example.com"}, leaf.DNSNames)
}

func TestRead_CustomizerError(t *testing.T) {
	sentinel := errors.New("customizer boom")
	customizer := certificate.CertificateCustomizerFunc[*rotObject](func(o *rotObject, base certificate.TLSSpec) (certificate.TLSSpec, error) {
		return certificate.TLSSpec{}, sentinel
	})
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject](),
		rotation.WithCertificateCustomizer[*rotObject](customizer))

	_, _, err := step.Read(context.Background(), newRotObject(), map[string]any{}, testLogger())
	require.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "customize certificate content")
}

func TestRead_NilCustomizer(t *testing.T) {
	step := newStep(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject](),
		rotation.WithCertificateCustomizer[*rotObject](nil))
	_, _, err := step.Read(context.Background(), newRotObject(), map[string]any{}, testLogger())
	require.NoError(t, err)
}

func TestRead_CAContentChange_TriggersSaga(t *testing.T) {
	backend := selfmanaged.NewSelfManagedBackend[*rotObject]()
	o := newRotObject()
	buildSpec := certificate.TLSSpec{
		SecretName:       "test-tls",
		CommonName:       "test.example.com",
		DNSNames:         []string{"test.example.com"},
		LeafValidityDays: 365,
		RenewalDays:      30,
	}
	objs, err := backend.DesiredObjects(context.Background(), o, buildSpec)
	require.NoError(t, err)
	ca := objs[0].(*corev1.Secret)
	leaf := objs[1].(*corev1.Secret)

	c := newFakeClient(t, ca, leaf)
	driftProvider := certificate.TLSSpecProviderFunc[*rotObject](func(o *rotObject) certificate.TLSSpec {
		return certificate.TLSSpec{
			SecretName:       "test-tls",
			CommonName:       "test.example.com",
			CACommonName:     "changed-ca",
			DNSNames:         []string{"test.example.com"},
			LeafValidityDays: 365,
			RenewalDays:      30,
		}
	})
	step := newStepProvider(c, backend, driftProvider)

	data := map[string]any{}
	read, res, err := step.Read(context.Background(), o, data, testLogger())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	require.Len(t, read.GetExpectedObjects(), 2)
	assert.Equal(t, true, data["rotationRenewed"])

	sig, ok := data["tls.tls"].(*certificate.LayerSignals)
	require.True(t, ok)
	assert.True(t, sig.CARotated)
	assert.True(t, sig.LeafRegenerated)
}

func TestRead_ComputeSpecValidatesContent(t *testing.T) {
	badProvider := certificate.TLSSpecProviderFunc[*rotObject](func(o *rotObject) certificate.TLSSpec {
		return certificate.TLSSpec{SecretName: "test-tls", KeyAlgorithm: "bogus"}
	})
	step := newStepProvider(newFakeClient(t), selfmanaged.NewSelfManagedBackend[*rotObject](), badProvider)

	_, _, err := step.Read(context.Background(), newRotObject(), map[string]any{}, testLogger())
	require.Error(t, err)
}

func TestRead_BYOBackend_SkipsContentValidation(t *testing.T) {
	// BYO performs no generation and ignores content fields, so invalid
	// content must not fail the cycle (only SecretName is enforced).
	badProvider := certificate.TLSSpecProviderFunc[*rotObject](func(o *rotObject) certificate.TLSSpec {
		return certificate.TLSSpec{SecretName: "my-secret", KeyAlgorithm: "bogus", DNSNames: []string{""}}
	})
	step := newStepProvider(newFakeClient(t), byo.NewBYOBackend[*rotObject](), badProvider)

	read, _, err := step.Read(context.Background(), newRotObject(), map[string]any{}, testLogger())
	require.NoError(t, err)
	assert.Empty(t, read.GetExpectedObjects())
}
