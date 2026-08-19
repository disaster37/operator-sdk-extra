package byo_test

import (
	"context"
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/byo"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type testBYOObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (o *testBYOObject) GetStatus() object.MultiPhaseObjectStatus {
	return nil
}

func (o *testBYOObject) DeepCopyObject() runtime.Object {
	return &testBYOObject{
		TypeMeta:   o.TypeMeta,
		ObjectMeta: *o.ObjectMeta.DeepCopy(),
	}
}

func TestNewBYOBackend(t *testing.T) {
	backend := byo.NewBYOBackend[*testBYOObject]()
	require.NotNil(t, backend)
}

func TestBYOBackendDesiredObjects(t *testing.T) {
	backend := byo.NewBYOBackend[*testBYOObject]()
	o := &testBYOObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	spec := certificate.TLSSpec{SecretName: "my-existing-secret"}
	objects, err := backend.DesiredObjects(context.Background(), o, spec)
	require.NoError(t, err)
	assert.Nil(t, objects, "BYO backend should not create objects")
}

func TestBYOBackendDesiredObjectsEmptySecretName(t *testing.T) {
	backend := byo.NewBYOBackend[*testBYOObject]()
	o := &testBYOObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	spec := certificate.TLSSpec{SecretName: ""}
	_, err := backend.DesiredObjects(context.Background(), o, spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty SecretName")
}

func TestBYOCertificateSecretName(t *testing.T) {
	backend := byo.NewBYOBackend[*testBYOObject]()
	o := &testBYOObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	spec := certificate.TLSSpec{SecretName: "my-secret"}
	name := backend.CertificateSecretName(o, spec)
	assert.Equal(t, "my-secret", name)
}

func TestBYORequiresRotationSaga(t *testing.T) {
	backend := byo.NewBYOBackend[*testBYOObject]()
	assert.False(t, backend.RequiresRotationSaga())
}
