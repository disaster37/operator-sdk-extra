// Package byo provides a TLSBackend that references an existing user-managed
// Secret. It does not emit any child objects — it simply validates that the
// Secret exists and returns its name for consumer reference.
package byo

import (
	"context"
	"fmt"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BYOBackend is a TLSBackend that references an existing user-managed Secret.
// It does not generate or manage any certificates. The user is responsible
// for creating and rotating the Secret.
type BYOBackend[T object.MultiPhaseObject] struct{}

// NewBYOBackend creates a new bring-your-own-secret backend.
func NewBYOBackend[T object.MultiPhaseObject]() *BYOBackend[T] {
	return &BYOBackend[T]{}
}

// DesiredObjects returns nil — the BYO backend does not create any objects.
// The user is expected to have created the Secret referenced by
// spec.SecretName out of band.
func (b *BYOBackend[T]) DesiredObjects(ctx context.Context, o T, spec certificate.TLSSpec) ([]client.Object, error) {
	if spec.SecretName == "" {
		return nil, fmt.Errorf("BYO TLS backend requires a non-empty SecretName in TLSSpec")
	}
	return nil, nil
}

// CertificateSecretName returns the name of the user-managed Secret.
func (b *BYOBackend[T]) CertificateSecretName(o T, spec certificate.TLSSpec) string {
	return spec.SecretName
}

// RequiresRotationSaga returns false — the user manages rotation out of band.
func (b *BYOBackend[T]) RequiresRotationSaga() bool {
	return false
}
