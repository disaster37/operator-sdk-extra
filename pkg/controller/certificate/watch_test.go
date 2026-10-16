package certificate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type certTestObjectList struct {
	metav1.TypeMeta
	metav1.ListMeta
	Items []certTestObject
}

func (l *certTestObjectList) DeepCopyObject() runtime.Object {
	if l == nil {
		return nil
	}
	cp := &certTestObjectList{
		TypeMeta: l.TypeMeta,
		ListMeta: l.ListMeta,
	}
	if l.Items != nil {
		cp.Items = make([]certTestObject, len(l.Items))
		for i := range l.Items {
			cp.Items[i] = *l.Items[i].DeepCopyObject().(*certTestObject)
		}
	}
	return cp
}

var certTestGV = schema.GroupVersion{Group: "test.operator-sdk-extra", Version: "v1"}

func certTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	scheme.AddKnownTypes(certTestGV, &certTestObject{}, &certTestObjectList{})
	return scheme
}

func certTestSpecProvider(secretName string) certificate.TLSSpecProvider[*certTestObject] {
	return certificate.TLSSpecProviderFunc[*certTestObject](func(o *certTestObject) certificate.TLSSpec {
		return certificate.TLSSpec{SecretName: secretName}
	})
}

func TestExtractTLSSecretNames(t *testing.T) {
	t.Run("indexes leaf and CA names", func(t *testing.T) {
		extract := certificate.ExtractTLSSecretNames(certTestSpecProvider("app-tls"))
		got := extract(&certTestObject{ObjectMeta: metav1.ObjectMeta{Name: "obj"}})
		assert.Equal(t, []string{"app-tls", "app-tls-ca"}, got)
	})
	t.Run("empty SecretName yields nil", func(t *testing.T) {
		extract := certificate.ExtractTLSSecretNames(certTestSpecProvider(""))
		assert.Nil(t, extract(&certTestObject{ObjectMeta: metav1.ObjectMeta{Name: "obj"}}))
	})
	t.Run("nil provider yields nil", func(t *testing.T) {
		extract := certificate.ExtractTLSSecretNames[*certTestObject](nil)
		assert.Nil(t, extract(&certTestObject{ObjectMeta: metav1.ObjectMeta{Name: "obj"}}))
	})
	t.Run("wrong object type yields nil", func(t *testing.T) {
		extract := certificate.ExtractTLSSecretNames(certTestSpecProvider("app-tls"))
		assert.Nil(t, extract(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret"}}))
	})
}

type recordingFieldIndexer struct {
	field    string
	obj      client.Object
	extract  client.IndexerFunc
	called   bool
	indexErr error
}

func (f *recordingFieldIndexer) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	f.called = true
	f.field = field
	f.obj = obj
	f.extract = extractValue
	return f.indexErr
}

type recordingManager struct {
	ctrl.Manager
	indexer client.FieldIndexer
}

func (m *recordingManager) GetFieldIndexer() client.FieldIndexer { return m.indexer }

func TestSetupTLSSecretIndexer(t *testing.T) {
	t.Run("default field fallback", func(t *testing.T) {
		fi := &recordingFieldIndexer{}
		mgr := &recordingManager{indexer: fi}
		prototype := &certTestObject{}
		require.NoError(t, certificate.SetupTLSSecretIndexer(prototype, "", certTestSpecProvider("app-tls"))(mgr))
		assert.True(t, fi.called)
		assert.Equal(t, certificate.TLSSecretNameIndexField, fi.field)
		assert.Same(t, prototype, fi.obj)
		assert.Equal(t, []string{"app-tls", "app-tls-ca"}, fi.extract(prototype))
	})
	t.Run("custom field name", func(t *testing.T) {
		fi := &recordingFieldIndexer{}
		mgr := &recordingManager{indexer: fi}
		require.NoError(t, certificate.SetupTLSSecretIndexer(&certTestObject{}, "custom.field", certTestSpecProvider("app-tls"))(mgr))
		assert.Equal(t, "custom.field", fi.field)
	})
	t.Run("returns indexer error", func(t *testing.T) {
		boom := errors.New("index boom")
		fi := &recordingFieldIndexer{indexErr: boom}
		mgr := &recordingManager{indexer: fi}
		require.ErrorIs(t, certificate.SetupTLSSecretIndexer(&certTestObject{}, "", certTestSpecProvider("app-tls"))(mgr), boom)
	})
}

func TestSecretToTLSObjectsMapFunc(t *testing.T) {
	provider := certificate.TLSSpecProviderFunc[*certTestObject](func(o *certTestObject) certificate.TLSSpec {
		switch o.GetName() {
		case "cluster-a":
			return certificate.TLSSpec{SecretName: "a-tls"}
		case "cluster-b":
			return certificate.TLSSpec{SecretName: "b-tls"}
		default:
			return certificate.TLSSpec{}
		}
	})
	c := fake.NewClientBuilder().
		WithScheme(certTestScheme()).
		WithIndex(&certTestObject{}, certificate.TLSSecretNameIndexField, certificate.ExtractTLSSecretNames(provider)).
		WithObjects(
			&certTestObject{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a", Namespace: "default"}},
			&certTestObject{ObjectMeta: metav1.ObjectMeta{Name: "cluster-b", Namespace: "default"}},
		).
		Build()

	newListFunc := func() client.ObjectList { return &certTestObjectList{} }
	mapFunc := certificate.SecretToTLSObjectsMapFunc(c, newListFunc, "")

	t.Run("matches leaf secret", func(t *testing.T) {
		reqs := mapFunc(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "a-tls", Namespace: "default"}})
		assert.Equal(t, []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: "default", Name: "cluster-a"}}}, reqs)
	})
	t.Run("matches CA secret", func(t *testing.T) {
		reqs := mapFunc(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "a-tls-ca", Namespace: "default"}})
		assert.Equal(t, []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: "default", Name: "cluster-a"}}}, reqs)
	})
	t.Run("custom field name", func(t *testing.T) {
		custom := fake.NewClientBuilder().
			WithScheme(certTestScheme()).
			WithIndex(&certTestObject{}, "custom.field", certificate.ExtractTLSSecretNames(provider)).
			WithObjects(&certTestObject{ObjectMeta: metav1.ObjectMeta{Name: "cluster-b", Namespace: "default"}}).
			Build()
		customFunc := certificate.SecretToTLSObjectsMapFunc(custom, newListFunc, "custom.field")
		reqs := customFunc(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "b-tls", Namespace: "default"}})
		assert.Equal(t, []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: "default", Name: "cluster-b"}}}, reqs)
	})
	t.Run("no match", func(t *testing.T) {
		reqs := mapFunc(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "other-secret", Namespace: "default"}})
		assert.Empty(t, reqs)
	})
	t.Run("nil secret yields nil", func(t *testing.T) {
		assert.Nil(t, mapFunc(context.Background(), nil))
	})
	t.Run("unnamed secret yields nil", func(t *testing.T) {
		assert.Nil(t, mapFunc(context.Background(), &corev1.Secret{}))
	})
	t.Run("list error returns nil", func(t *testing.T) {
		failing := &failingListClient{Client: c, err: errors.New("list boom")}
		reqs := certificate.SecretToTLSObjectsMapFunc(failing, newListFunc, "")(context.Background(),
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "a-tls", Namespace: "default"}})
		assert.Nil(t, reqs)
	})
	t.Run("non-list object returns nil", func(t *testing.T) {
		noop := &noopListClient{Client: c}
		reqs := certificate.SecretToTLSObjectsMapFunc(noop, func() client.ObjectList { return &noItemsList{} }, "")(context.Background(),
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "a-tls", Namespace: "default"}})
		assert.Nil(t, reqs)
	})
}

type failingListClient struct {
	client.Client
	err error
}

func (c *failingListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.err
}

type noopListClient struct {
	client.Client
}

func (c *noopListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

type noItemsList struct {
	metav1.TypeMeta
	metav1.ListMeta
}

func (l *noItemsList) DeepCopyObject() runtime.Object {
	if l == nil {
		return nil
	}
	cp := *l
	return &cp
}
