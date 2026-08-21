// Package certmanager provides a TLSBackend that emits cert-manager
// Issuer and Certificate custom resources. It relies on cert-manager running
// in the cluster to provision and renew the actual certificate Secret.
//
// This subpackage is optional. Core packages must not import it directly.
// Importing it adds cert-manager API awareness at build time but does not
// require cert-manager to be installed at runtime (SSA will fail gracefully
// if the CRDs are absent).
package certmanager

import (
	"context"
	"crypto/x509/pkix"
	"fmt"

	"emperror.dev/errors"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// certManagerGroup is the API group for cert-manager resources.
	certManagerGroup = "cert-manager.io"

	// certManagerVersion is the API version.
	certManagerVersion = "v1"

	// caIssuerSuffix is appended to the SecretName for the self-signed CA Issuer.
	caIssuerSuffix = "-ca-issuer"
)

var (
	issuerGVK      = schema.GroupVersionKind{Group: certManagerGroup, Version: certManagerVersion, Kind: "Issuer"}
	certificateGVK = schema.GroupVersionKind{Group: certManagerGroup, Version: certManagerVersion, Kind: "Certificate"}
)

// CertManagerBackend is a TLSBackend that emits cert-manager Issuer and
// Certificate custom resources. It supports two modes:
//
//   - Dedicated-CA: creates a self-signed Issuer, a CA Certificate, a CA Issuer,
//     and leaf Certificate(s).
//   - Existing-CA: creates only leaf Certificate(s) referencing an existing
//     Issuer or ClusterIssuer.
type CertManagerBackend[T object.MultiPhaseObject] struct{}

// NewCertManagerBackend creates a new cert-manager backend.
func NewCertManagerBackend[T object.MultiPhaseObject]() *CertManagerBackend[T] {
	return &CertManagerBackend[T]{}
}

// DesiredObjects returns cert-manager Issuer and Certificate objects based on
// the TLSSpec configuration.
func (b *CertManagerBackend[T]) DesiredObjects(ctx context.Context, o T, spec certificate.TLSSpec) ([]client.Object, error) {
	if spec.SecretName == "" {
		return nil, errors.New("cert-manager TLS backend requires a non-empty SecretName in TLSSpec")
	}
	if err := spec.ValidateContent(); err != nil {
		return nil, err
	}

	namespace := o.GetNamespace()
	var objects []client.Object

	if spec.IssuerRef == "" {
		// Dedicated-CA mode: create a self-signed Issuer + CA Certificate + CA Issuer
		caIssuer, err := b.buildSelfSignedIssuer(spec.SecretName+caIssuerSuffix, namespace)
		if err != nil {
			return nil, err
		}
		objects = append(objects, caIssuer)

		caCert, err := b.buildCACertificate(spec, spec.SecretName, spec.SecretName+caIssuerSuffix, namespace)
		if err != nil {
			return nil, err
		}
		objects = append(objects, caCert)

		leaf, err := b.buildLeafCertificate(spec, map[string]interface{}{
			"name": spec.SecretName + "-ca-issuer",
			"kind": "Issuer",
		}, namespace)
		if err != nil {
			return nil, err
		}
		objects = append(objects, leaf)
	} else {
		// Existing-CA mode: leaf Certificate referencing an existing Issuer
		leaf, err := b.buildLeafCertificate(spec, map[string]interface{}{
			"name": spec.IssuerRef,
		}, namespace)
		if err != nil {
			return nil, err
		}
		objects = append(objects, leaf)
	}

	return objects, nil
}

// CertificateSecretName returns the name of the Secret that cert-manager will
// create with the signed certificate.
func (b *CertManagerBackend[T]) CertificateSecretName(o T, spec certificate.TLSSpec) string {
	return spec.SecretName
}

// RequiresRotationSaga returns false — cert-manager handles renewal.
func (b *CertManagerBackend[T]) RequiresRotationSaga() bool {
	return false
}

func (b *CertManagerBackend[T]) buildSelfSignedIssuer(name, namespace string) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(issuerGVK)
	u.SetName(name)
	u.SetNamespace(namespace)
	if err := unstructured.SetNestedField(u.Object, map[string]interface{}{
		"selfSigned": map[string]interface{}{},
	}, "spec"); err != nil {
		return nil, errors.Wrapf(err, "set spec.selfSigned on self-signed Issuer %q", name)
	}
	return u, nil
}

func (b *CertManagerBackend[T]) buildCACertificate(spec certificate.TLSSpec, name, issuerName, namespace string) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(certificateGVK)
	u.SetName(name + "-ca")
	u.SetNamespace(namespace)

	specMap := map[string]interface{}{
		"isCA":       true,
		"commonName": spec.ResolvedCASubject().CommonName,
		"secretName": name + "-ca",
		"issuerRef": map[string]interface{}{
			"name": issuerName,
			"kind": "Issuer",
		},
		"duration": fmtDurationHours(certificate.GetValidCADays(spec)),
	}
	if subject := subjectMap(spec.ResolvedCASubject()); len(subject) > 0 {
		specMap["subject"] = subject
	}

	if err := unstructured.SetNestedField(u.Object, specMap, "spec"); err != nil {
		return nil, errors.Wrapf(err, "set spec on CA Certificate %q", name+"-ca")
	}
	return u, nil
}

func (b *CertManagerBackend[T]) buildLeafCertificate(spec certificate.TLSSpec, issuerRef map[string]interface{}, namespace string) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(certificateGVK)
	u.SetName(spec.SecretName)
	u.SetNamespace(namespace)

	specMap := map[string]interface{}{
		"commonName": spec.LeafSubject().CommonName,
		"secretName": spec.SecretName,
		"issuerRef":  issuerRef,
	}

	if subject := subjectMap(spec.LeafSubject()); len(subject) > 0 {
		specMap["subject"] = subject
	}

	if err := setCommonCertificateSpec(specMap, spec); err != nil {
		return nil, err
	}

	if err := unstructured.SetNestedField(u.Object, specMap, "spec"); err != nil {
		return nil, errors.Wrapf(err, "set spec on leaf Certificate %q", spec.SecretName)
	}
	return u, nil
}

// setCommonCertificateSpec populates the DNS/IP SAN, renewal, validity, usage,
// and private-key fields shared by both leaf Certificate builders onto specMap.
func setCommonCertificateSpec(specMap map[string]interface{}, spec certificate.TLSSpec) error {
	if dnsNames := certificate.DedupStrings(spec.DNSNames); len(dnsNames) > 0 {
		specMap["dnsNames"] = toStringInterfaceSlice(dnsNames)
	}
	if ipAddresses := certificate.DedupStrings(spec.IPAddresses); len(ipAddresses) > 0 {
		specMap["ipAddresses"] = toStringInterfaceSlice(ipAddresses)
	}
	if spec.RenewalDays > 0 {
		specMap["renewBefore"] = fmtDurationHours(certificate.GetValidRenewalDays(spec))
	}
	if spec.LeafValidityDays > 0 {
		// Use the clamped getter so an oversized LeafValidityDays cannot
		// overflow the `days*24` multiplication in fmtDurationHours (the same
		// overflow class MaxValidityDays guards against in the selfmanaged
		// backend).
		specMap["duration"] = fmtDurationHours(certificate.GetValidLeafDays(spec))
	}

	usages := spec.EffectiveUsages()
	mapped := make([]interface{}, 0, len(usages))
	for _, u := range usages {
		cm, ok := certManagerUsageFor(u)
		if !ok {
			return errors.Wrapf(certificate.ErrUnknownUsage, "unknown extended key usage %q", u)
		}
		mapped = append(mapped, cm)
	}
	if len(mapped) > 0 {
		specMap["usages"] = mapped
	}

	size, err := spec.EffectiveKeySize()
	if err != nil {
		return err
	}
	specMap["privateKey"] = map[string]interface{}{
		"algorithm": spec.EffectiveKeyAlgorithm(),
		"size":      int64(size),
	}
	return nil
}

// subjectMap converts a resolved pkix.Name into a cert-manager Certificate
// spec.subject block, omitting empty slices.
func subjectMap(name pkix.Name) map[string]interface{} {
	m := map[string]interface{}{}
	if len(name.Organization) > 0 {
		m["organizations"] = toStringInterfaceSlice(name.Organization)
	}
	if len(name.OrganizationalUnit) > 0 {
		m["organizationalUnits"] = toStringInterfaceSlice(name.OrganizationalUnit)
	}
	if len(name.Country) > 0 {
		m["countries"] = toStringInterfaceSlice(name.Country)
	}
	if len(name.Locality) > 0 {
		m["localities"] = toStringInterfaceSlice(name.Locality)
	}
	if len(name.Province) > 0 {
		m["provinces"] = toStringInterfaceSlice(name.Province)
	}
	if len(name.StreetAddress) > 0 {
		m["streetAddresses"] = toStringInterfaceSlice(name.StreetAddress)
	}
	if len(name.PostalCode) > 0 {
		m["postalCodes"] = toStringInterfaceSlice(name.PostalCode)
	}
	if name.SerialNumber != "" {
		m["serialNumber"] = name.SerialNumber
	}
	return m
}

// certManagerUsageFor maps a Usage* constant to the corresponding cert-manager
// Certificate.spec.usages string.
func certManagerUsageFor(name string) (string, bool) {
	switch name {
	case certificate.UsageServerAuth:
		return "server auth", true
	case certificate.UsageClientAuth:
		return "client auth", true
	case certificate.UsageCodeSigning:
		return "code signing", true
	case certificate.UsageEmailProtection:
		return "email protection", true
	case certificate.UsageTimestamping:
		return "timestamping", true
	case certificate.UsageOCSPSigning:
		return "ocsp signing", true
	default:
		return "", false
	}
}

// toStringInterfaceSlice converts a []string to []interface{} for cert-manager
// Certificate spec fields (unstructured nested values).
func toStringInterfaceSlice(values []string) []interface{} {
	out := make([]interface{}, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

// fmtDurationHours renders a duration in days as a cert-manager duration
// string (e.g. "2160h" for 90 days).
func fmtDurationHours(days int) string {
	return fmt.Sprintf("%dh", days*24)
}
