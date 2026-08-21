package certificate_test

import (
	"crypto/x509/pkix"
	"net"
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type certTestObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (o *certTestObject) GetStatus() object.MultiPhaseObjectStatus { return nil }

func (o *certTestObject) DeepCopyObject() runtime.Object {
	return &certTestObject{
		TypeMeta:   o.TypeMeta,
		ObjectMeta: *o.DeepCopy(),
	}
}

var _ object.MultiPhaseObject = (*certTestObject)(nil)

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

func TestTLSSpecOrganizations(t *testing.T) {
	t.Run("subject organizations win", func(t *testing.T) {
		spec := certificate.TLSSpec{Organization: "legacy", Subject: certificate.CertificateSubject{Organizations: []string{"a", "b"}}}
		assert.Equal(t, []string{"a", "b"}, spec.Organizations())
	})
	t.Run("legacy organization fallback", func(t *testing.T) {
		spec := certificate.TLSSpec{Organization: "legacy"}
		assert.Equal(t, []string{"legacy"}, spec.Organizations())
	})
	t.Run("both empty", func(t *testing.T) {
		spec := certificate.TLSSpec{}
		assert.Nil(t, spec.Organizations())
	})
	t.Run("empty legacy organization dropped", func(t *testing.T) {
		spec := certificate.TLSSpec{Organization: ""}
		assert.Nil(t, spec.Organizations())
	})
}

func TestTLSSpecLeafSubject(t *testing.T) {
	t.Run("full RDN mapping", func(t *testing.T) {
		spec := certificate.TLSSpec{
			CommonName: "cn",
			Subject: certificate.CertificateSubject{
				Organizations:       []string{"org"},
				OrganizationalUnits: []string{"ou"},
				Countries:           []string{"FR"},
				Localities:          []string{"loc"},
				Provinces:           []string{"prov"},
				StreetAddresses:     []string{"street"},
				PostalCodes:         []string{"75000"},
				SerialNumber:        "sn",
			},
		}
		want := pkix.Name{
			CommonName:         "cn",
			Organization:       []string{"org"},
			OrganizationalUnit: []string{"ou"},
			Country:            []string{"FR"},
			Locality:           []string{"loc"},
			Province:           []string{"prov"},
			StreetAddress:      []string{"street"},
			PostalCode:         []string{"75000"},
			SerialNumber:       "sn",
		}
		assert.Equal(t, want, spec.LeafSubject())
	})
	t.Run("legacy organization resolved", func(t *testing.T) {
		spec := certificate.TLSSpec{CommonName: "cn", Organization: "legacy"}
		assert.Equal(t, []string{"legacy"}, spec.LeafSubject().Organization)
		assert.Equal(t, "cn", spec.LeafSubject().CommonName)
	})
	t.Run("empty yields CN only", func(t *testing.T) {
		spec := certificate.TLSSpec{CommonName: "cn"}
		assert.Equal(t, pkix.Name{CommonName: "cn"}, spec.LeafSubject())
	})
}

func TestTLSSpecResolvedCASubject(t *testing.T) {
	t.Run("default CN", func(t *testing.T) {
		spec := certificate.TLSSpec{CommonName: "cn", Organization: "legacy"}
		got := spec.ResolvedCASubject()
		assert.Equal(t, "cn-ca", got.CommonName)
		assert.Equal(t, []string{"legacy"}, got.Organization)
	})
	t.Run("explicit CACommonName", func(t *testing.T) {
		spec := certificate.TLSSpec{CommonName: "cn", CACommonName: "my-ca"}
		assert.Equal(t, "my-ca", spec.ResolvedCASubject().CommonName)
	})
	t.Run("explicit CASubject", func(t *testing.T) {
		spec := certificate.TLSSpec{
			CommonName:   "cn",
			CACommonName: "my-ca",
			CASubject: &certificate.CertificateSubject{
				Organizations: []string{"ca-org"},
				SerialNumber:  "ca-sn",
			},
		}
		got := spec.ResolvedCASubject()
		assert.Equal(t, "my-ca", got.CommonName)
		assert.Equal(t, []string{"ca-org"}, got.Organization)
		assert.Equal(t, "ca-sn", got.SerialNumber)
	})
}

func TestTLSSpecEffectiveKeyAlgorithm(t *testing.T) {
	assert.Equal(t, certificate.KeyAlgorithmECDSA, (certificate.TLSSpec{}).EffectiveKeyAlgorithm())
	assert.Equal(t, certificate.KeyAlgorithmECDSA, (certificate.TLSSpec{KeyAlgorithm: certificate.KeyAlgorithmECDSA}).EffectiveKeyAlgorithm())
	assert.Equal(t, certificate.KeyAlgorithmRSA, (certificate.TLSSpec{KeyAlgorithm: certificate.KeyAlgorithmRSA}).EffectiveKeyAlgorithm())
}

func TestTLSSpecEffectiveKeySize(t *testing.T) {
	t.Run("ECDSA default", func(t *testing.T) {
		size, err := (certificate.TLSSpec{}).EffectiveKeySize()
		require.NoError(t, err)
		assert.Equal(t, 256, size)
	})
	t.Run("ECDSA from KeySize", func(t *testing.T) {
		size, err := (certificate.TLSSpec{KeySize: 384}).EffectiveKeySize()
		require.NoError(t, err)
		assert.Equal(t, 384, size)
	})
	t.Run("ECDSA from Curve", func(t *testing.T) {
		size, err := (certificate.TLSSpec{Curve: certificate.CurveP521}).EffectiveKeySize()
		require.NoError(t, err)
		assert.Equal(t, 521, size)
	})
	t.Run("KeySize and Curve conflict", func(t *testing.T) {
		_, err := (certificate.TLSSpec{KeySize: 384, Curve: certificate.CurveP256}).EffectiveKeySize()
		require.ErrorIs(t, err, certificate.ErrInvalidKeySize)
	})
	t.Run("RSA default", func(t *testing.T) {
		size, err := (certificate.TLSSpec{KeyAlgorithm: certificate.KeyAlgorithmRSA}).EffectiveKeySize()
		require.NoError(t, err)
		assert.Equal(t, 2048, size)
	})
	t.Run("RSA explicit", func(t *testing.T) {
		size, err := (certificate.TLSSpec{KeyAlgorithm: certificate.KeyAlgorithmRSA, KeySize: 4096}).EffectiveKeySize()
		require.NoError(t, err)
		assert.Equal(t, 4096, size)
	})
}

func TestTLSSpecEffectiveUsages(t *testing.T) {
	assert.Equal(t, []string{certificate.UsageServerAuth, certificate.UsageClientAuth}, (certificate.TLSSpec{}).EffectiveUsages())
	custom := certificate.TLSSpec{Usages: []string{certificate.UsageServerAuth, certificate.UsageCodeSigning}}
	assert.Equal(t, custom.Usages, custom.EffectiveUsages())
}

func TestTLSSpecEffectiveKeyUsages(t *testing.T) {
	assert.Equal(t, []string{certificate.KeyUsageDigitalSignature, certificate.KeyUsageKeyEncipherment}, (certificate.TLSSpec{}).EffectiveKeyUsages())
	custom := certificate.TLSSpec{KeyUsages: []string{certificate.KeyUsageDigitalSignature}}
	assert.Equal(t, custom.KeyUsages, custom.EffectiveKeyUsages())
}

func TestTLSSpecValidateContent(t *testing.T) {
	t.Run("zero value is valid", func(t *testing.T) {
		require.NoError(t, (certificate.TLSSpec{}).ValidateContent())
	})
	t.Run("valid all fields", func(t *testing.T) {
		spec := certificate.TLSSpec{
			KeyAlgorithm: certificate.KeyAlgorithmRSA,
			KeySize:      2048,
			Usages:       []string{certificate.UsageServerAuth},
			KeyUsages:    []string{certificate.KeyUsageDigitalSignature},
			DNSNames:     []string{"a.example.com"},
			IPAddresses:  []string{"10.0.0.1"},
		}
		require.NoError(t, spec.ValidateContent())
	})
	t.Run("unknown algorithm", func(t *testing.T) {
		err := (certificate.TLSSpec{KeyAlgorithm: "Ed25519"}).ValidateContent()
		require.ErrorIs(t, err, certificate.ErrUnsupportedKeyAlgorithm)
	})
	t.Run("bad ECDSA size", func(t *testing.T) {
		err := (certificate.TLSSpec{KeySize: 1024}).ValidateContent()
		require.ErrorIs(t, err, certificate.ErrInvalidKeySize)
	})
	t.Run("bad RSA size", func(t *testing.T) {
		err := (certificate.TLSSpec{KeyAlgorithm: certificate.KeyAlgorithmRSA, KeySize: 1024}).ValidateContent()
		require.ErrorIs(t, err, certificate.ErrInvalidKeySize)
	})
	t.Run("RSA size too large", func(t *testing.T) {
		// An unbounded RSA key size would let a misconfiguration drive
		// rsa.GenerateKey to synthesize an enormous key (DoS). Must be rejected.
		err := (certificate.TLSSpec{KeyAlgorithm: certificate.KeyAlgorithmRSA, KeySize: certificate.MaxRSAKeySize + 1}).ValidateContent()
		require.ErrorIs(t, err, certificate.ErrInvalidKeySize)
	})
	t.Run("RSA size at max is valid", func(t *testing.T) {
		err := (certificate.TLSSpec{KeyAlgorithm: certificate.KeyAlgorithmRSA, KeySize: certificate.MaxRSAKeySize}).ValidateContent()
		require.NoError(t, err)
	})
	t.Run("KeySize and Curve conflict", func(t *testing.T) {
		err := (certificate.TLSSpec{KeySize: 384, Curve: certificate.CurveP256}).ValidateContent()
		require.ErrorIs(t, err, certificate.ErrInvalidKeySize)
	})
	t.Run("unknown usage", func(t *testing.T) {
		err := (certificate.TLSSpec{Usages: []string{"bogus"}}).ValidateContent()
		require.ErrorIs(t, err, certificate.ErrUnknownUsage)
	})
	t.Run("unknown key usage", func(t *testing.T) {
		err := (certificate.TLSSpec{KeyUsages: []string{"bogus"}}).ValidateContent()
		require.ErrorIs(t, err, certificate.ErrUnknownUsage)
	})
	t.Run("empty DNS string", func(t *testing.T) {
		err := (certificate.TLSSpec{DNSNames: []string{""}}).ValidateContent()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DNS name must not be empty")
	})
	t.Run("whitespace DNS string", func(t *testing.T) {
		err := (certificate.TLSSpec{DNSNames: []string{"   "}}).ValidateContent()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DNS name must not be empty")
	})
	t.Run("invalid IP SAN", func(t *testing.T) {
		err := (certificate.TLSSpec{IPAddresses: []string{"not-an-ip"}}).ValidateContent()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid IP SAN")
	})
	t.Run("valid IP SAN accepted", func(t *testing.T) {
		require.NoError(t, (certificate.TLSSpec{IPAddresses: []string{"10.0.0.1", "2001:db8::1"}}).ValidateContent())
	})
	t.Run("unknown curve", func(t *testing.T) {
		err := (certificate.TLSSpec{Curve: "P-999"}).ValidateContent()
		require.ErrorIs(t, err, certificate.ErrUnknownCurve)
	})
	t.Run("known curves accepted", func(t *testing.T) {
		require.NoError(t, (certificate.TLSSpec{Curve: certificate.CurveP256}).ValidateContent())
		require.NoError(t, (certificate.TLSSpec{Curve: certificate.CurveP384}).ValidateContent())
		require.NoError(t, (certificate.TLSSpec{Curve: certificate.CurveP521}).ValidateContent())
	})
	t.Run("leaf certSign reserved for CA", func(t *testing.T) {
		err := (certificate.TLSSpec{KeyUsages: []string{certificate.KeyUsageCertSign}}).ValidateContent()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved for the CA")
	})
	t.Run("leaf crlSign reserved for CA", func(t *testing.T) {
		err := (certificate.TLSSpec{KeyUsages: []string{certificate.KeyUsageCRLSign}}).ValidateContent()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved for the CA")
	})
}

func TestDedupStrings(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, certificate.DedupStrings([]string{"a", "b", "a", "c", "b"}))
	assert.Nil(t, certificate.DedupStrings(nil))
	assert.Equal(t, []string{"only"}, certificate.DedupStrings([]string{"only"}))
}

func TestDedupIPs(t *testing.T) {
	ips := []net.IP{net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.2"), net.ParseIP("10.0.0.1")}
	got := certificate.DedupIPs(ips)
	require.Len(t, got, 2)
	assert.True(t, got[0].Equal(net.ParseIP("10.0.0.1")))
	assert.True(t, got[1].Equal(net.ParseIP("10.0.0.2")))
	assert.Nil(t, certificate.DedupIPs(nil))
}

func TestDefaultCertificateCustomizer(t *testing.T) {
	o := &certTestObject{ObjectMeta: metav1.ObjectMeta{Name: "my-object"}}
	customizer := certificate.DefaultCertificateCustomizer[*certTestObject]()

	t.Run("fills empty CN", func(t *testing.T) {
		spec, err := customizer.CustomizeCertificate(o, certificate.TLSSpec{SecretName: "tls"})
		require.NoError(t, err)
		assert.Equal(t, "my-object", spec.CommonName)
		assert.Equal(t, "tls", spec.SecretName)
	})
	t.Run("leaves non-empty CN", func(t *testing.T) {
		spec, err := customizer.CustomizeCertificate(o, certificate.TLSSpec{CommonName: "explicit"})
		require.NoError(t, err)
		assert.Equal(t, "explicit", spec.CommonName)
	})
	t.Run("identity otherwise", func(t *testing.T) {
		base := certificate.TLSSpec{CommonName: "cn", DNSNames: []string{"a"}}
		spec, err := customizer.CustomizeCertificate(o, base)
		require.NoError(t, err)
		assert.Equal(t, base, spec)
	})
}

func TestCertificateCustomizerFunc(t *testing.T) {
	called := false
	var f certificate.CertificateCustomizerFunc[*certTestObject] = func(o *certTestObject, base certificate.TLSSpec) (certificate.TLSSpec, error) {
		called = true
		return base, nil
	}
	o := &certTestObject{}
	spec, err := f.CustomizeCertificate(o, certificate.TLSSpec{})
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, certificate.TLSSpec{}, spec)
}
