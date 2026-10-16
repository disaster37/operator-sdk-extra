package certificate

import (
	"context"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ErrNilTLSSpecProvider is returned when TLSSpecValidator is constructed or
// invoked with a nil TLSSpecProvider.
var ErrNilTLSSpecProvider = errors.Sentinel("TLS spec provider must not be nil")

// TLSSpecValidator validates a MultiPhaseObject's computed TLSSpec on admission
// CREATE and UPDATE operations by delegating to TLSSpec.ValidateContent().
// It satisfies sigs.k8s.io/controller-runtime/pkg/webhook/admission.Validator[T].
//
// Operators that already implement admission.Validator[T] for other CRD fields
// can either call validator.ValidateCreate / ValidateUpdate from their own
// validator or call spec.ValidateContent() directly.
type TLSSpecValidator[T object.MultiPhaseObject] struct {
	provider TLSSpecProvider[T]
}

var _ admission.Validator[object.MultiPhaseObject] = (*TLSSpecValidator[object.MultiPhaseObject])(nil)

// NewTLSSpecValidator creates a new admission validator backed by provider.
func NewTLSSpecValidator[T object.MultiPhaseObject](provider TLSSpecProvider[T]) *TLSSpecValidator[T] {
	return &TLSSpecValidator[T]{provider: provider}
}

// ValidateCreate validates the computed TLSSpec on resource creation.
func (v *TLSSpecValidator[T]) ValidateCreate(ctx context.Context, obj T) (admission.Warnings, error) {
	if v.provider == nil {
		return nil, ErrNilTLSSpecProvider
	}
	return nil, v.provider.TLSSpec(obj).ValidateContent()
}

// ValidateUpdate validates the computed TLSSpec on resource update.
func (v *TLSSpecValidator[T]) ValidateUpdate(ctx context.Context, oldObj, newObj T) (admission.Warnings, error) {
	if v.provider == nil {
		return nil, ErrNilTLSSpecProvider
	}
	return nil, v.provider.TLSSpec(newObj).ValidateContent()
}

// ValidateDelete always allows deletion.
func (v *TLSSpecValidator[T]) ValidateDelete(ctx context.Context, obj T) (admission.Warnings, error) {
	return nil, nil
}
