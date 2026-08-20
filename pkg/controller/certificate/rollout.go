package certificate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

const (
	// AnnotationSecretHash is the annotation key used to store a stable
	// hash of the certificate Secret contents on pod templates. When the
	// Secret changes, the hash changes, triggering a natural rolling restart.
	AnnotationSecretHash = "operator-sdk-extra.webcenter.fr/certificate-hash"

	// AnnotationForceRegenerateAll triggers a full CA+leaf rotation + rollout.
	// Read as == "true" (the ignoreReconcile convention). Overridable via
	// rotation.WithForceRegenerateAllAnnotation.
	AnnotationForceRegenerateAll = "operator-sdk-extra.webcenter.fr/force-regenerate-tls"

	// AnnotationForceRegenerateLeaf triggers a leaf-only regen + rollout.
	AnnotationForceRegenerateLeaf = "operator-sdk-extra.webcenter.fr/force-regenerate-certificates"
)

// LayerSignals is the per-layer change-delta signal a TLS step publishes under
// data["tls.<phaseName>"] for the STS step to gate rollout.
type LayerSignals struct {
	CARotated       bool        // a CA-saga started this cycle
	LeafRegenerated bool        // leaf regenerated (CA saga OR leaf-only)
	LeafChange      *LeafChange // drift/expiry detail (SAN/IP/node deltas)
	Forced          bool        // a force annotation was honored this cycle
}

// RolloutPolicy selects which change-delta signals trigger a rollout.
type RolloutPolicy string

const (
	RolloutAlways     RolloutPolicy = "Always"
	RolloutOnCAChange RolloutPolicy = "OnCAChange"
	RolloutOnAdditive RolloutPolicy = "OnAdditive" // recommended default
	RolloutNever      RolloutPolicy = "Never"
)

// ShouldRollout decides whether a pod rollout is needed for one layer's
// signals. Forced overrides everything, including RolloutNever.
func ShouldRollout(policy RolloutPolicy, sig *LayerSignals) bool {
	if sig == nil {
		return false
	}
	if sig.Forced {
		return true
	}
	switch policy {
	case RolloutAlways:
		return sig.CARotated || sig.LeafRegenerated
	case RolloutOnCAChange:
		return sig.CARotated
	case RolloutNever:
		return false
	default: // RolloutOnAdditive (unknown values treated as additive)
		if sig.CARotated {
			return true
		}
		lc := sig.LeafChange
		if lc == nil {
			return false
		}
		switch lc.Reason {
		case LeafExpiring, LeafCNChanged, LeafOrgChanged, LeafMissing, LeafForceRegen:
			return true
		case LeafSANsChanged, LeafIPsChanged:
			return len(lc.SANsAdded) > 0 || len(lc.IPsAdded) > 0
		default: // LeafNone, LeafNodesChanged
			return false
		}
	}
}

// RolloutAnnotation builds the pod-template annotation map entry.
//   - shouldRollout=true: fresh hash of secret (SecretHashAnnotation).
//   - shouldRollout=false and currentHash!="": keep currentHash (no restart).
//   - shouldRollout=false and currentHash=="" (first run): initialize hash.
func RolloutAnnotation(shouldRollout bool, secret *corev1.Secret, currentHash string) (map[string]string, error) {
	if !shouldRollout && currentHash != "" {
		return map[string]string{AnnotationSecretHash: currentHash}, nil
	}
	return SecretHashAnnotation(secret)
}

// SecretHash computes a stable SHA-256 hash of a Secret's data fields.
// This hash is used as a pod-template annotation so that any change to the
// certificate Secret forces a rolling restart of the consuming workload.
//
// This decouples "certificate changed" from "must restart" without bespoke
// polling of StatefulSet.Status.CurrentReplicas.
func SecretHash(secret *corev1.Secret) (string, error) {
	if secret == nil {
		return "", fmt.Errorf("secret must not be nil")
	}

	h := sha256.New()
	for _, key := range sortedKeys(secret.Data) {
		h.Write([]byte(key))
		h.Write([]byte{0})
		h.Write(secret.Data[key])
		h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// SecretHashAnnotation builds the annotation map entry for the certificate
// hash. The returned map is suitable for merging into a pod template's
// metadata.annotations.
func SecretHashAnnotation(secret *corev1.Secret) (map[string]string, error) {
	hash, err := SecretHash(secret)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		AnnotationSecretHash: hash,
	}, nil
}

// sortedKeys returns a sorted slice of map keys for deterministic hashing.
// The sort is lexicographic, which is sufficient for stable hashing since the
// Secret data keys are short strings.
func sortedKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort for small maps (Secrets typically have 2-4 keys).
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
