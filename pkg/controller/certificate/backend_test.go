package certificate_test

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/certificate"
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
