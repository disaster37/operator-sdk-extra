package certificate_test

import (
	"context"
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixedTLSSpecProvider(spec certificate.TLSSpec) certificate.TLSSpecProvider[*certTestObject] {
	return certificate.TLSSpecProviderFunc[*certTestObject](func(o *certTestObject) certificate.TLSSpec {
		return spec
	})
}

func TestTLSSpecValidator_ValidateCreate(t *testing.T) {
	o := &certTestObject{}

	t.Run("valid dedicated CA", func(t *testing.T) {
		v := certificate.NewTLSSpecValidator[*certTestObject](fixedTLSSpecProvider(certificate.TLSSpec{SecretName: "tls"}))
		warnings, err := v.ValidateCreate(context.Background(), o)
		assert.Nil(t, warnings)
		assert.NoError(t, err)
	})
	t.Run("valid IssuerRef", func(t *testing.T) {
		v := certificate.NewTLSSpecValidator[*certTestObject](fixedTLSSpecProvider(certificate.TLSSpec{SecretName: "tls", IssuerRef: "my-issuer"}))
		_, err := v.ValidateCreate(context.Background(), o)
		assert.NoError(t, err)
	})
	t.Run("valid ClusterIssuerRef", func(t *testing.T) {
		v := certificate.NewTLSSpecValidator[*certTestObject](fixedTLSSpecProvider(certificate.TLSSpec{SecretName: "tls", ClusterIssuerRef: "my-cluster-issuer"}))
		_, err := v.ValidateCreate(context.Background(), o)
		assert.NoError(t, err)
	})
	t.Run("conflicting IssuerRef and ClusterIssuerRef", func(t *testing.T) {
		v := certificate.NewTLSSpecValidator[*certTestObject](fixedTLSSpecProvider(certificate.TLSSpec{SecretName: "tls", IssuerRef: "my-issuer", ClusterIssuerRef: "my-cluster-issuer"}))
		_, err := v.ValidateCreate(context.Background(), o)
		require.ErrorIs(t, err, certificate.ErrConflictingIssuerRefs)
	})
	t.Run("whitespace IssuerRef", func(t *testing.T) {
		v := certificate.NewTLSSpecValidator[*certTestObject](fixedTLSSpecProvider(certificate.TLSSpec{SecretName: "tls", IssuerRef: "   "}))
		_, err := v.ValidateCreate(context.Background(), o)
		require.ErrorIs(t, err, certificate.ErrInvalidIssuerRef)
	})
	t.Run("nil provider", func(t *testing.T) {
		v := certificate.NewTLSSpecValidator[*certTestObject](nil)
		_, err := v.ValidateCreate(context.Background(), o)
		require.ErrorIs(t, err, certificate.ErrNilTLSSpecProvider)
	})
}

func TestTLSSpecValidator_ValidateUpdate(t *testing.T) {
	oldObj := &certTestObject{}
	newObj := &certTestObject{}

	t.Run("valid update", func(t *testing.T) {
		v := certificate.NewTLSSpecValidator[*certTestObject](fixedTLSSpecProvider(certificate.TLSSpec{SecretName: "tls", IssuerRef: "my-issuer"}))
		_, err := v.ValidateUpdate(context.Background(), oldObj, newObj)
		require.NoError(t, err)
	})
	t.Run("invalid update conflicting refs", func(t *testing.T) {
		v := certificate.NewTLSSpecValidator[*certTestObject](fixedTLSSpecProvider(certificate.TLSSpec{SecretName: "tls", IssuerRef: "a", ClusterIssuerRef: "b"}))
		_, err := v.ValidateUpdate(context.Background(), oldObj, newObj)
		require.ErrorIs(t, err, certificate.ErrConflictingIssuerRefs)
	})
	t.Run("nil provider", func(t *testing.T) {
		v := certificate.NewTLSSpecValidator[*certTestObject](nil)
		_, err := v.ValidateUpdate(context.Background(), oldObj, newObj)
		require.ErrorIs(t, err, certificate.ErrNilTLSSpecProvider)
	})
}

func TestTLSSpecValidator_ValidateDelete(t *testing.T) {
	o := &certTestObject{}
	// Even with conflicting refs on the object, deletion is always allowed.
	v := certificate.NewTLSSpecValidator[*certTestObject](fixedTLSSpecProvider(certificate.TLSSpec{SecretName: "tls", IssuerRef: "a", ClusterIssuerRef: "b"}))
	warnings, err := v.ValidateDelete(context.Background(), o)
	assert.Nil(t, warnings)
	assert.NoError(t, err)
}
