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

func TestForceAnnotationConstants(t *testing.T) {
	assert.Equal(t, "operator-sdk-extra.webcenter.fr/force-regenerate-tls", certificate.AnnotationForceRegenerateAll)
	assert.Equal(t, "operator-sdk-extra.webcenter.fr/force-regenerate-certificates", certificate.AnnotationForceRegenerateLeaf)
}

func TestLayerSignalsStruct(t *testing.T) {
	lc := &certificate.LeafChange{Reason: certificate.LeafExpiring}
	sig := &certificate.LayerSignals{CARotated: true, LeafRegenerated: true, LeafChange: lc, Forced: true}
	assert.True(t, sig.CARotated)
	assert.True(t, sig.LeafRegenerated)
	assert.True(t, sig.Forced)
	assert.Equal(t, lc, sig.LeafChange)
}

func TestShouldRolloutNilSignal(t *testing.T) {
	assert.False(t, certificate.ShouldRollout(certificate.RolloutAlways, nil))
}

func TestShouldRolloutForcedOverridesNever(t *testing.T) {
	assert.True(t, certificate.ShouldRollout(certificate.RolloutNever, &certificate.LayerSignals{Forced: true}))
}

func TestShouldRolloutAlways(t *testing.T) {
	assert.True(t, certificate.ShouldRollout(certificate.RolloutAlways, &certificate.LayerSignals{CARotated: true}))
	assert.True(t, certificate.ShouldRollout(certificate.RolloutAlways, &certificate.LayerSignals{LeafRegenerated: true}))
	assert.False(t, certificate.ShouldRollout(certificate.RolloutAlways, &certificate.LayerSignals{}))
}

func TestShouldRolloutOnCAChange(t *testing.T) {
	assert.True(t, certificate.ShouldRollout(certificate.RolloutOnCAChange, &certificate.LayerSignals{CARotated: true}))
	assert.False(t, certificate.ShouldRollout(certificate.RolloutOnCAChange, &certificate.LayerSignals{LeafRegenerated: true}))
}

func TestShouldRolloutNever(t *testing.T) {
	assert.False(t, certificate.ShouldRollout(certificate.RolloutNever, &certificate.LayerSignals{CARotated: true}))
	assert.True(t, certificate.ShouldRollout(certificate.RolloutNever, &certificate.LayerSignals{Forced: true}))
}

func TestShouldRolloutOnAdditiveMatrix(t *testing.T) {
	reason := func(r certificate.LeafChangeReason) *certificate.LeafChange {
		return &certificate.LeafChange{Reason: r}
	}
	cases := []struct {
		name string
		sig  *certificate.LayerSignals
		want bool
	}{
		{"CARotated", &certificate.LayerSignals{CARotated: true}, true},
		{"Expiring", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: reason(certificate.LeafExpiring)}, true},
		{"CNChanged", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: reason(certificate.LeafCNChanged)}, true},
		{"OrgChanged", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: reason(certificate.LeafOrgChanged)}, true},
		{"SubjectChanged", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: reason(certificate.LeafSubjectChanged)}, true},
		{"KeyChanged", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: reason(certificate.LeafKeyChanged)}, true},
		{"UsagesChanged", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: reason(certificate.LeafUsagesChanged)}, true},
		{"Missing", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: reason(certificate.LeafMissing)}, true},
		{"LeafForceRegen", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: reason(certificate.LeafForceRegen)}, true},
		{"SANsAdded", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: &certificate.LeafChange{Reason: certificate.LeafSANsChanged, SANsAdded: []string{"a"}}}, true},
		{"SANsRemovedOnly", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: &certificate.LeafChange{Reason: certificate.LeafSANsChanged, SANsRemoved: []string{"a"}}}, false},
		{"IPsAdded", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: &certificate.LeafChange{Reason: certificate.LeafIPsChanged, IPsAdded: []string{"1.2.3.4"}}}, true},
		{"IPsRemovedOnly", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: &certificate.LeafChange{Reason: certificate.LeafIPsChanged, IPsRemoved: []string{"1.2.3.4"}}}, false},
		{"SANsAddedAndRemoved", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: &certificate.LeafChange{Reason: certificate.LeafSANsChanged, SANsAdded: []string{"a"}, SANsRemoved: []string{"b"}}}, true},
		{"NodesChangedOnly", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: &certificate.LeafChange{Reason: certificate.LeafNodesChanged, NodesAdded: []string{"n"}}}, false},
		{"LeafNone", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: reason(certificate.LeafNone)}, false},
		{"nilLeafChange", &certificate.LayerSignals{LeafRegenerated: true, LeafChange: nil}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, certificate.ShouldRollout(certificate.RolloutOnAdditive, tc.sig))
		})
	}
}

func TestRolloutAnnotationRollout(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{"tls.crt": []byte("x")}}
	ann, err := certificate.RolloutAnnotation(true, secret, "old-hash")
	require.NoError(t, err)
	want, err := certificate.SecretHashAnnotation(secret)
	require.NoError(t, err)
	assert.Equal(t, want, ann)
}

func TestRolloutAnnotationKeep(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{"tls.crt": []byte("x")}}
	ann, err := certificate.RolloutAnnotation(false, secret, "abc")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{certificate.AnnotationSecretHash: "abc"}, ann)
}

func TestRolloutAnnotationKeepButEmptyInitializes(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{"tls.crt": []byte("x")}}
	ann, err := certificate.RolloutAnnotation(false, secret, "")
	require.NoError(t, err)
	want, err := certificate.SecretHashAnnotation(secret)
	require.NoError(t, err)
	assert.Equal(t, want, ann)
}
