package certificate_test

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestSecretHashNil(t *testing.T) {
	_, err := certificate.SecretHash(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret must not be nil")
}

func TestSecretHashDeterministic(t *testing.T) {
	secret := &corev1.Secret{
		Data: map[string][]byte{
			"tls.crt": []byte("cert-data"),
			"tls.key": []byte("key-data"),
			"ca.crt":  []byte("ca-data"),
		},
	}

	hash1, err := certificate.SecretHash(secret)
	require.NoError(t, err)

	hash2, err := certificate.SecretHash(secret)
	require.NoError(t, err)

	assert.Equal(t, hash1, hash2, "hash must be deterministic")
}

func TestSecretHashChangesOnDataChange(t *testing.T) {
	secret1 := &corev1.Secret{
		Data: map[string][]byte{
			"tls.crt": []byte("cert-data-v1"),
			"tls.key": []byte("key-data-v1"),
		},
	}

	secret2 := &corev1.Secret{
		Data: map[string][]byte{
			"tls.crt": []byte("cert-data-v2"),
			"tls.key": []byte("key-data-v1"),
		},
	}

	hash1, err := certificate.SecretHash(secret1)
	require.NoError(t, err)

	hash2, err := certificate.SecretHash(secret2)
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2, "hash must change when data changes")
}

func TestSecretHashEmptyData(t *testing.T) {
	secret := &corev1.Secret{
		Data: map[string][]byte{},
	}

	hash, err := certificate.SecretHash(secret)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestSecretHashAnnotation(t *testing.T) {
	secret := &corev1.Secret{
		Data: map[string][]byte{
			"tls.crt": []byte("cert-data"),
		},
	}

	annotations, err := certificate.SecretHashAnnotation(secret)
	require.NoError(t, err)

	hash, ok := annotations[certificate.AnnotationSecretHash]
	assert.True(t, ok)
	assert.NotEmpty(t, hash)
}

func TestSecretHashAnnotationNil(t *testing.T) {
	_, err := certificate.SecretHashAnnotation(nil)
	assert.Error(t, err)
}

func TestAnnotationSecretHashConstant(t *testing.T) {
	assert.Equal(t, "operator-sdk-extra.webcenter.fr/certificate-hash", certificate.AnnotationSecretHash)
}
