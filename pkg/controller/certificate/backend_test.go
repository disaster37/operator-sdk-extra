package certificate_test

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/stretchr/testify/assert"
)

func TestGetValidLeafDaysDefault(t *testing.T) {
	spec := certificate.TLSSpec{}
	days := certificate.GetValidLeafDays(spec)
	assert.Equal(t, 365, days)
}

func TestGetValidLeafDaysCustom(t *testing.T) {
	spec := certificate.TLSSpec{LeafValidityDays: 90}
	days := certificate.GetValidLeafDays(spec)
	assert.Equal(t, 90, days)
}

func TestGetValidLeafDaysZero(t *testing.T) {
	spec := certificate.TLSSpec{LeafValidityDays: 0}
	days := certificate.GetValidLeafDays(spec)
	assert.Equal(t, 365, days)
}

func TestGetValidLeafDaysNegative(t *testing.T) {
	spec := certificate.TLSSpec{LeafValidityDays: -1}
	days := certificate.GetValidLeafDays(spec)
	assert.Equal(t, 365, days)
}

func TestGetValidCADaysDefault(t *testing.T) {
	spec := certificate.TLSSpec{}
	days := certificate.GetValidCADays(spec)
	assert.Equal(t, 730, days)
}

func TestGetValidCADaysCustom(t *testing.T) {
	spec := certificate.TLSSpec{CAValidityDays: 500}
	days := certificate.GetValidCADays(spec)
	assert.Equal(t, 500, days)
}

func TestGetValidCADaysZero(t *testing.T) {
	spec := certificate.TLSSpec{LeafValidityDays: 90, CAValidityDays: 0}
	days := certificate.GetValidCADays(spec)
	assert.Equal(t, 180, days)
}

func TestGetValidCADaysNegative(t *testing.T) {
	spec := certificate.TLSSpec{LeafValidityDays: 30, CAValidityDays: -1}
	days := certificate.GetValidCADays(spec)
	assert.Equal(t, 60, days)
}

func TestGetValidLeafDaysClampsOverflow(t *testing.T) {
	// Values above MaxValidityDays are clamped to prevent time.Duration
	// overflow in `LeafValidityDays * 24 * time.Hour`.
	spec := certificate.TLSSpec{LeafValidityDays: certificate.MaxValidityDays + 1}
	days := certificate.GetValidLeafDays(spec)
	assert.Equal(t, certificate.MaxValidityDays, days)

	// A wildly large value (near max int) must also clamp, not overflow.
	spec = certificate.TLSSpec{LeafValidityDays: 1 << 62}
	days = certificate.GetValidLeafDays(spec)
	assert.Equal(t, certificate.MaxValidityDays, days)
}

func TestGetValidCADaysClampsOverflow(t *testing.T) {
	// Explicit CAValidityDays above the cap must clamp.
	spec := certificate.TLSSpec{CAValidityDays: certificate.MaxValidityDays + 1}
	days := certificate.GetValidCADays(spec)
	assert.Equal(t, certificate.MaxValidityDays, days)

	// Default 2× leaf must also clamp when the leaf is at the cap.
	spec = certificate.TLSSpec{LeafValidityDays: certificate.MaxValidityDays}
	days = certificate.GetValidCADays(spec)
	assert.Equal(t, certificate.MaxValidityDays, days)

	// A wildly large explicit value must clamp, not overflow the 2× path.
	spec = certificate.TLSSpec{CAValidityDays: 1 << 62}
	days = certificate.GetValidCADays(spec)
	assert.Equal(t, certificate.MaxValidityDays, days)
}

func TestGetValidRenewalDaysDefault(t *testing.T) {
	spec := certificate.TLSSpec{}
	days := certificate.GetValidRenewalDays(spec)
	assert.Equal(t, 30, days)
}

func TestGetValidRenewalDaysCustom(t *testing.T) {
	spec := certificate.TLSSpec{RenewalDays: 45}
	days := certificate.GetValidRenewalDays(spec)
	assert.Equal(t, 45, days)
}

func TestGetValidRenewalDaysZero(t *testing.T) {
	spec := certificate.TLSSpec{RenewalDays: 0}
	days := certificate.GetValidRenewalDays(spec)
	assert.Equal(t, 30, days)
}

func TestGetValidRenewalDaysNegative(t *testing.T) {
	spec := certificate.TLSSpec{RenewalDays: -5}
	days := certificate.GetValidRenewalDays(spec)
	assert.Equal(t, 30, days)
}

func TestGetValidRenewalDaysClampsOverflow(t *testing.T) {
	// Values above MaxRenewalDays are clamped to MaxRenewalDays to prevent
	// time.Duration overflow in `RenewalDays * 24 * time.Hour`.
	spec := certificate.TLSSpec{RenewalDays: certificate.MaxRenewalDays + 1}
	days := certificate.GetValidRenewalDays(spec)
	assert.Equal(t, certificate.MaxRenewalDays, days)

	// A wildly large value (near max int) must also clamp, not overflow.
	spec = certificate.TLSSpec{RenewalDays: 1 << 62}
	days = certificate.GetValidRenewalDays(spec)
	assert.Equal(t, certificate.MaxRenewalDays, days)
}

func TestGetValidRenewalDaysAtMax(t *testing.T) {
	spec := certificate.TLSSpec{RenewalDays: certificate.MaxRenewalDays}
	days := certificate.GetValidRenewalDays(spec)
	assert.Equal(t, certificate.MaxRenewalDays, days)
}

func TestCurveConstants(t *testing.T) {
	assert.Equal(t, "P-256", certificate.CurveP256)
	assert.Equal(t, "P-384", certificate.CurveP384)
	assert.Equal(t, "P-521", certificate.CurveP521)
	assert.Equal(t, 30, certificate.DefaultRenewalDays)
}
