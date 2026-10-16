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
	"time"

	"emperror.dev/errors"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// certManagerGroup is the API group for cert-manager resources.
	certManagerGroup = "cert-manager.io"

	// certManagerVersion is the API version.
	certManagerVersion = "v1"

	// issuerKind is the Kind for namespace-scoped cert-manager Issuers.
	issuerKind = "Issuer"

	// clusterIssuerKind is the Kind for cluster-scoped cert-manager ClusterIssuers.
	clusterIssuerKind = "ClusterIssuer"

	// selfSignedIssuerSuffix is appended to the SecretName for the bootstrap self-signed Issuer.
	selfSignedIssuerSuffix = "-selfsigned-issuer"

	// caSecretSuffix is appended to the SecretName for the CA Certificate and its Secret.
	caSecretSuffix = "-ca"

	// caIssuerSuffix is appended to the SecretName for the CA-backed Issuer.
	caIssuerSuffix = "-ca-issuer"

	// conditionTypeIssuing is the cert-manager Certificate status condition
	// that triggers certificate re-issuance when set to "True".
	conditionTypeIssuing = "Issuing"

	// reasonManuallyTriggered matches cmctl renew's reason string.
	reasonManuallyTriggered = "ManuallyTriggered"

	// messageManuallyTriggered matches cmctl renew's message string.
	messageManuallyTriggered = "Certificate re-issuance manually triggered"
)

var (
	// IssuerGVK is the GroupVersionKind for cert-manager Issuer resources.
	IssuerGVK = schema.GroupVersionKind{Group: certManagerGroup, Version: certManagerVersion, Kind: issuerKind}

	// CertificateGVK is the GroupVersionKind for cert-manager Certificate resources.
	CertificateGVK = schema.GroupVersionKind{Group: certManagerGroup, Version: certManagerVersion, Kind: "Certificate"}

	// ErrClientRequired is returned when ForceRegenerate is called on a
	// CertManagerBackend that was constructed without WithClient.
	ErrClientRequired = errors.Sentinel("cert-manager backend requires a Kubernetes client (WithClient) to force certificate regeneration")

	// certManagerUsageMap maps certificate.Usage* constants to cert-manager
	// Certificate.spec.usages strings. ValidateContent guarantees every usage
	// in spec.EffectiveUsages() is a key in this map.
	certManagerUsageMap = map[string]string{
		certificate.UsageServerAuth:      "server auth",
		certificate.UsageClientAuth:      "client auth",
		certificate.UsageCodeSigning:     "code signing",
		certificate.UsageEmailProtection: "email protection",
		certificate.UsageTimestamping:    "timestamping",
		certificate.UsageOCSPSigning:     "ocsp signing",
	}
)

// NewIssuerObject returns an empty unstructured cert-manager v1 Issuer
// prototype suitable for passing to builder.Owns(...) in SetupWithManager.
func NewIssuerObject() *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(IssuerGVK)
	return u
}

// NewCertificateObject returns an empty unstructured cert-manager v1
// Certificate prototype suitable for passing to builder.Owns(...) in
// SetupWithManager.
func NewCertificateObject() *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(CertificateGVK)
	return u
}

// Option configures a CertManagerBackend.
type Option[T object.MultiPhaseObject] func(*CertManagerBackend[T])

// WithClient configures the Kubernetes client used by ForceRegenerate to set
// the Issuing=True status condition on cert-manager Certificate CRs.
func WithClient[T object.MultiPhaseObject](c client.Client) Option[T] {
	return func(b *CertManagerBackend[T]) {
		b.client = c
	}
}

// CertManagerBackend is a TLSBackend that emits cert-manager Issuer and
// Certificate custom resources. It supports two modes:
//
//   - Dedicated-CA: creates a self-signed bootstrap Issuer, a CA Certificate,
//     a CA Issuer, and a leaf Certificate.
//   - Existing-CA: creates only a leaf Certificate referencing an existing
//     Issuer (TLSSpec.IssuerRef) or ClusterIssuer (TLSSpec.ClusterIssuerRef).
type CertManagerBackend[T object.MultiPhaseObject] struct {
	client client.Client
}

var _ certificate.ForceRegenerator[object.MultiPhaseObject] = (*CertManagerBackend[object.MultiPhaseObject])(nil)

// NewCertManagerBackend creates a new cert-manager backend. Pass
// WithClient(c) to enable ForceRegenerate annotation support.
func NewCertManagerBackend[T object.MultiPhaseObject](opts ...Option[T]) *CertManagerBackend[T] {
	b := &CertManagerBackend[T]{}
	for _, opt := range opts {
		opt(b)
	}
	return b
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
	pkMap, err := buildPrivateKeyMap(spec)
	if err != nil {
		return nil, err
	}

	namespace := o.GetNamespace()
	var objects []client.Object

	if spec.IssuerRef == "" && spec.ClusterIssuerRef == "" {
		// Dedicated-CA mode:
		// 1. Self-signed bootstrap Issuer (<SecretName>-selfsigned-issuer)
		// 2. CA Certificate (<SecretName>-ca, secretName: <SecretName>-ca) signed by bootstrap Issuer
		// 3. CA Issuer (<SecretName>-ca-issuer, spec.ca.secretName: <SecretName>-ca)
		// 4. Leaf Certificate (<SecretName>, secretName: <SecretName>) signed by CA Issuer
		selfSignedIssuerName := spec.SecretName + selfSignedIssuerSuffix
		caSecretName := spec.SecretName + caSecretSuffix
		caIssuerName := spec.SecretName + caIssuerSuffix

		objects = append(objects,
			b.buildSelfSignedIssuer(selfSignedIssuerName, namespace),
			b.buildCACertificate(spec, caSecretName, selfSignedIssuerName, namespace, pkMap),
			b.buildCAIssuer(caIssuerName, caSecretName, namespace),
			b.buildLeafCertificate(spec, issuerRefMap(caIssuerName, issuerKind), namespace, pkMap),
		)
	} else {
		// Existing-CA mode: leaf Certificate referencing an existing Issuer or ClusterIssuer
		refName := spec.IssuerRef
		refKind := issuerKind
		if spec.ClusterIssuerRef != "" {
			refName = spec.ClusterIssuerRef
			refKind = clusterIssuerKind
		}
		objects = append(objects, b.buildLeafCertificate(spec, issuerRefMap(refName, refKind), namespace, pkMap))
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

// ForceRegenerate implements certificate.ForceRegenerator by setting the
// Issuing=True status condition (Reason: ManuallyTriggered, matching cmctl
// renew) on the target cert-manager Certificate resource(s).
//
//   - forceAll in dedicated-CA mode (both IssuerRef and ClusterIssuerRef empty)
//     triggers re-issuance on both <SecretName>-ca and <SecretName>.
//   - forceAll in existing-CA mode, or forceLeaf in any mode, triggers
//     re-issuance on the leaf Certificate (<SecretName>) only.
//   - If a target Certificate does not exist yet (NotFound), it is skipped
//     because the subsequent SSA Apply will create it and trigger initial issuance.
func (b *CertManagerBackend[T]) ForceRegenerate(ctx context.Context, o T, spec certificate.TLSSpec, forceAll, forceLeaf bool) error {
	if !forceAll && !forceLeaf {
		return nil
	}
	if b.client == nil {
		return ErrClientRequired
	}

	namespace := o.GetNamespace()
	if forceAll && spec.IssuerRef == "" && spec.ClusterIssuerRef == "" {
		if err := b.triggerCertificateIssuing(ctx, namespace, spec.SecretName+caSecretSuffix); err != nil {
			return err
		}
	}
	return b.triggerCertificateIssuing(ctx, namespace, spec.SecretName)
}

func (b *CertManagerBackend[T]) triggerCertificateIssuing(ctx context.Context, namespace, certName string) error {
	cert := NewCertificateObject()
	err := b.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: certName}, cert)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "get cert-manager Certificate %q", certName)
	}

	statusMap, _, _ := unstructured.NestedMap(cert.Object, "status")
	if statusMap == nil {
		statusMap = map[string]interface{}{}
	}
	rawConditions, _, _ := unstructured.NestedSlice(statusMap, "conditions")

	nowStr := metav1.Now().UTC().Format(time.RFC3339)
	newCond := map[string]interface{}{
		"type":               conditionTypeIssuing,
		"status":             "True",
		"reason":             reasonManuallyTriggered,
		"message":            messageManuallyTriggered,
		"lastTransitionTime": nowStr,
		"observedGeneration": cert.GetGeneration(),
	}

	updated := false
	for i, elem := range rawConditions {
		condMap, ok := elem.(map[string]interface{})
		if ok && condMap["type"] == conditionTypeIssuing {
			if condMap["status"] == "True" {
				return nil
			}
			rawConditions[i] = newCond
			updated = true
			break
		}
	}
	if !updated {
		rawConditions = append(rawConditions, newCond)
	}

	statusMap["conditions"] = rawConditions
	cert.Object["status"] = statusMap

	if err := b.client.Status().Update(ctx, cert); err != nil {
		return errors.Wrapf(err, "update status condition Issuing=True on Certificate %q", certName)
	}
	return nil
}

func (b *CertManagerBackend[T]) buildSelfSignedIssuer(name, namespace string) *unstructured.Unstructured {
	u := NewIssuerObject()
	u.SetName(name)
	u.SetNamespace(namespace)
	u.Object["spec"] = map[string]interface{}{
		"selfSigned": map[string]interface{}{},
	}
	return u
}

func (b *CertManagerBackend[T]) buildCAIssuer(name, caSecretName, namespace string) *unstructured.Unstructured {
	u := NewIssuerObject()
	u.SetName(name)
	u.SetNamespace(namespace)
	u.Object["spec"] = map[string]interface{}{
		"ca": map[string]interface{}{
			"secretName": caSecretName,
		},
	}
	return u
}

func (b *CertManagerBackend[T]) buildCACertificate(spec certificate.TLSSpec, caCertName, issuerName, namespace string, pkMap map[string]interface{}) *unstructured.Unstructured {
	u := NewCertificateObject()
	u.SetName(caCertName)
	u.SetNamespace(namespace)

	specMap := map[string]interface{}{
		"isCA":       true,
		"commonName": spec.ResolvedCASubject().CommonName,
		"secretName": caCertName,
		"issuerRef":  issuerRefMap(issuerName, issuerKind),
		"duration":   fmtDurationHours(certificate.GetValidCADays(spec)),
		"privateKey": pkMap,
	}
	if subject := subjectMap(spec.ResolvedCASubject()); len(subject) > 0 {
		specMap["subject"] = subject
	}

	u.Object["spec"] = specMap
	return u
}

func (b *CertManagerBackend[T]) buildLeafCertificate(spec certificate.TLSSpec, issuerRef map[string]interface{}, namespace string, pkMap map[string]interface{}) *unstructured.Unstructured {
	u := NewCertificateObject()
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

	setCommonCertificateSpec(specMap, spec, pkMap)

	u.Object["spec"] = specMap
	return u
}

// setCommonCertificateSpec populates the DNS/IP SAN, renewal, validity, usage,
// and private-key fields onto specMap.
//
// renewBefore applies to the leaf Certificate only. CARenewalDays is not
// mapped: the CA is owned by the Issuer (or is self-signed per Certificate).
func setCommonCertificateSpec(specMap map[string]interface{}, spec certificate.TLSSpec, pkMap map[string]interface{}) {
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
		specMap["duration"] = fmtDurationHours(certificate.GetValidLeafDays(spec))
	}

	usages := spec.EffectiveUsages()
	mapped := make([]interface{}, len(usages))
	for i, u := range usages {
		mapped[i] = certManagerUsageMap[u]
	}
	specMap["usages"] = mapped
	specMap["privateKey"] = pkMap
}

// issuerRefMap builds a cert-manager issuerRef block with explicit name, kind,
// and group ("cert-manager.io").
func issuerRefMap(name, kind string) map[string]interface{} {
	return map[string]interface{}{
		"name":  name,
		"kind":  kind,
		"group": certManagerGroup,
	}
}

// buildPrivateKeyMap builds the cert-manager Certificate spec.privateKey block
// and validates cert-manager-specific key size constraints.
//
// Precondition: spec.ValidateContent() has already succeeded, so
// EffectiveKeyAlgorithm() is either ECDSA (size 256, 384, or 521) or RSA
// (size in [2048, 8192]), and EffectiveKeySize() will not error.
func buildPrivateKeyMap(spec certificate.TLSSpec) (map[string]interface{}, error) {
	alg := spec.EffectiveKeyAlgorithm()
	size, _ := spec.EffectiveKeySize()
	if alg == certificate.KeyAlgorithmRSA && size != 2048 && size != 4096 && size != 8192 {
		return nil, errors.Wrapf(
			certificate.ErrInvalidKeySize,
			"invalid RSA key size %d for cert-manager: want 2048, 4096 or 8192",
			size,
		)
	}
	return map[string]interface{}{
		"algorithm": alg,
		"size":      int64(size),
	}, nil
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
