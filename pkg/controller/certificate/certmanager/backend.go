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
	"fmt"

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
		return nil, fmt.Errorf("cert-manager TLS backend requires a non-empty SecretName in TLSSpec")
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

		caCert, err := b.buildCACertificate(spec.SecretName, spec.SecretName+caIssuerSuffix, namespace)
		if err != nil {
			return nil, err
		}
		objects = append(objects, caCert)

		leaf, err := b.buildLeafCertificate(spec, spec.SecretName+"-ca-issuer", namespace)
		if err != nil {
			return nil, err
		}
		objects = append(objects, leaf)
	} else {
		// Existing-CA mode: leaf Certificate referencing an existing Issuer
		leaf, err := b.buildLeafCertificateWithIssuer(spec, spec.IssuerRef, namespace)
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
		return nil, fmt.Errorf("set spec.selfSigned on self-signed Issuer %q: %w", name, err)
	}
	return u, nil
}

func (b *CertManagerBackend[T]) buildCACertificate(name, issuerName, namespace string) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(certificateGVK)
	u.SetName(name + "-ca")
	u.SetNamespace(namespace)
	if err := unstructured.SetNestedField(u.Object, map[string]interface{}{
		"isCA":       true,
		"commonName": name + "-ca",
		"secretName": name + "-ca",
		"issuerRef": map[string]interface{}{
			"name": issuerName,
			"kind": "Issuer",
		},
		"subject": map[string]interface{}{
			"organizations": []interface{}{"operator-sdk-extra"},
		},
	}, "spec"); err != nil {
		return nil, fmt.Errorf("set spec on CA Certificate %q: %w", name+"-ca", err)
	}
	return u, nil
}

func (b *CertManagerBackend[T]) buildLeafCertificate(spec certificate.TLSSpec, caIssuerName, namespace string) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(certificateGVK)
	u.SetName(spec.SecretName)
	u.SetNamespace(namespace)

	specMap := map[string]interface{}{
		"commonName": spec.CommonName,
		"secretName": spec.SecretName,
		"issuerRef": map[string]interface{}{
			"name": caIssuerName,
			"kind": "Issuer",
		},
	}

	if spec.Organization != "" {
		specMap["subject"] = map[string]interface{}{
			"organizations": []interface{}{spec.Organization},
		}
	}

	setCommonCertificateSpec(specMap, spec)

	if spec.ValidityDays > 0 {
		specMap["duration"] = fmt.Sprintf("%dh", spec.ValidityDays*24)
	}

	if err := unstructured.SetNestedField(u.Object, specMap, "spec"); err != nil {
		return nil, fmt.Errorf("set spec on leaf Certificate %q: %w", spec.SecretName, err)
	}
	return u, nil
}

func (b *CertManagerBackend[T]) buildLeafCertificateWithIssuer(spec certificate.TLSSpec, issuerRef, namespace string) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(certificateGVK)
	u.SetName(spec.SecretName)
	u.SetNamespace(namespace)

	specMap := map[string]interface{}{
		"commonName": spec.CommonName,
		"secretName": spec.SecretName,
		"issuerRef": map[string]interface{}{
			"name": issuerRef,
		},
	}

	setCommonCertificateSpec(specMap, spec)

	if err := unstructured.SetNestedField(u.Object, specMap, "spec"); err != nil {
		return nil, fmt.Errorf("set spec on leaf Certificate %q: %w", spec.SecretName, err)
	}
	return u, nil
}

// setCommonCertificateSpec populates the DNS/IP SAN and renewal fields shared
// by both leaf Certificate builders onto specMap.
func setCommonCertificateSpec(specMap map[string]interface{}, spec certificate.TLSSpec) {
	if len(spec.DNSNames) > 0 {
		specMap["dnsNames"] = toStringInterfaceSlice(spec.DNSNames)
	}
	if len(spec.IPAddresses) > 0 {
		specMap["ipAddresses"] = toStringInterfaceSlice(spec.IPAddresses)
	}
	if spec.RenewalDays > 0 {
		specMap["renewBefore"] = fmt.Sprintf("%dh", certificate.GetValidRenewalDays(spec)*24)
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
