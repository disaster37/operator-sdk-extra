package certificate_test

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/stretchr/testify/assert"
)

func TestGetValidTLSDaysDefault(t *testing.T) {
	spec := certificate.TLSSpec{}
	days := certificate.GetValidTLSDays(spec)
	assert.Equal(t, 365, days)
}

func TestGetValidTLSDaysCustom(t *testing.T) {
	spec := certificate.TLSSpec{ValidityDays: 90}
	days := certificate.GetValidTLSDays(spec)
	assert.Equal(t, 90, days)
}

func TestGetValidTLSDaysZero(t *testing.T) {
	spec := certificate.TLSSpec{ValidityDays: 0}
	days := certificate.GetValidTLSDays(spec)
	assert.Equal(t, 365, days)
}

func TestGetValidTLSDaysNegative(t *testing.T) {
	spec := certificate.TLSSpec{ValidityDays: -1}
	days := certificate.GetValidTLSDays(spec)
	assert.Equal(t, 365, days)
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
