// Package rotation provides a reusable multi-cycle TLS rotation saga step
// built on workflow.WorkflowStepReconcilerActionWithDiff.
//
// The saga phases are:
//
//	""       -> Rotate   : emit new CA + leaf with ca.crt = newCA||oldCA bundle.
//	Rotate   -> Converge : hold desired state stable, gate on an operator-injected
//	                       convergence check, then advance.
//	Converge -> ""       : emit leaf with ca.crt = newCA only (strip old CA).
//
// Backends with RequiresRotationSaga() == false degrade to a single-cycle emit
// of DesiredObjects with no phase writes.
package rotation

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	apworkflow "github.com/disaster37/operator-sdk-extra/v3/pkg/apis/workflow"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/certificate/selfmanaged"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/multiphase"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/workflow"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// PhaseRotate is the saga phase during which the new CA + bundled leaf
	// have been applied and the operator waits for convergence.
	PhaseRotate apworkflow.WorkflowPhase = "Rotate"
	// PhaseConverge is the saga phase that strips the old CA from the leaf.
	PhaseConverge apworkflow.WorkflowPhase = "Converge"
)

// ConvergenceCheck is invoked during the Rotate phase to decide whether the
// system has absorbed the new CA bundle (e.g., all pods have rolled). Return
// true to advance to Converge; return false to requeue.
type ConvergenceCheck[T object.MultiPhaseObject] func(ctx context.Context, o T, data map[string]any) (bool, error)

// LabelsDecorator mutates labels on each expected object.
type LabelsDecorator[T object.MultiPhaseObject] func(o T, obj client.Object)

// AnnotationsDecorator mutates annotations on each expected object.
type AnnotationsDecorator[T object.MultiPhaseObject] func(o T, obj client.Object)

// Option configures a TLS saga step.
type Option[T object.MultiPhaseObject] func(*tlsStep[T])

// WithConvergenceCheck injects the Rotate-phase convergence gate. If not set,
// the Rotate phase advances to Converge immediately (no waiting).
func WithConvergenceCheck[T object.MultiPhaseObject](f ConvergenceCheck[T]) Option[T] {
	return func(s *tlsStep[T]) { s.convergenceCheck = f }
}

// WithLabelsDecorator applies labels to every expected object.
func WithLabelsDecorator[T object.MultiPhaseObject](f LabelsDecorator[T]) Option[T] {
	return func(s *tlsStep[T]) { s.labelsDecorator = f }
}

// WithAnnotationsDecorator applies annotations to every expected object.
func WithAnnotationsDecorator[T object.MultiPhaseObject](f AnnotationsDecorator[T]) Option[T] {
	return func(s *tlsStep[T]) { s.annotationsDecorator = f }
}

// WithForceRegenerateAllAnnotation overrides the force-all annotation name.
func WithForceRegenerateAllAnnotation[T object.MultiPhaseObject](name string) Option[T] {
	return func(s *tlsStep[T]) { s.forceAllAnnotation = name }
}

// WithForceRegenerateLeafAnnotation overrides the force-leaf annotation name.
func WithForceRegenerateLeafAnnotation[T object.MultiPhaseObject](name string) Option[T] {
	return func(s *tlsStep[T]) { s.forceLeafAnnotation = name }
}

// tlsStep is the concrete saga step. It embeds the default workflow WithDiff
// action (for Diff/Apply/Configure/OnError/phase helpers) and overrides
// Read, OnDiff, OnSuccess.
type tlsStep[T object.MultiPhaseObject] struct {
	*workflow.DefaultWorkflowStepReconcilerActionWithDiff[T, client.Object]
	backend              certificate.TLSBackend[T]
	provider             certificate.TLSSpecProvider[T]
	convergenceCheck     ConvergenceCheck[T]
	labelsDecorator      LabelsDecorator[T]
	annotationsDecorator AnnotationsDecorator[T]
	forceAllAnnotation   string
	forceLeafAnnotation  string
}

// NewTLSStep creates a reusable TLS rotation saga step.
//
//   - phaseName/conditionName/recorder/fieldManager: standard step wiring.
//   - backend: the TLSBackend (selfmanaged → saga; byo/certmanager → single cycle).
//   - provider: supplies the computed TLSSpec for the reconciled object.
//   - opts: WithConvergenceCheck, WithLabelsDecorator, WithAnnotationsDecorator,
//     WithForceRegenerateAllAnnotation, WithForceRegenerateLeafAnnotation.
//
// The returned step is a WorkflowStepReconcilerActionWithDiff[T, client.Object].
func NewTLSStep[T object.MultiPhaseObject](
	c client.Client,
	phaseName shared.PhaseName,
	conditionName shared.ConditionName,
	recorder record.EventRecorder,
	fieldManager string,
	backend certificate.TLSBackend[T],
	provider certificate.TLSSpecProvider[T],
	opts ...Option[T],
) workflow.WorkflowStepReconcilerActionWithDiff[T, client.Object] {
	s := &tlsStep[T]{
		DefaultWorkflowStepReconcilerActionWithDiff: workflow.NewWorkflowStepReconcilerActionWithDiff[T, client.Object](
			c, phaseName, conditionName, recorder, fieldManager,
		).(*workflow.DefaultWorkflowStepReconcilerActionWithDiff[T, client.Object]),
		backend:             backend,
		provider:            provider,
		forceAllAnnotation:  certificate.AnnotationForceRegenerateAll,
		forceLeafAnnotation: certificate.AnnotationForceRegenerateLeaf,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// signalKey returns the data-map key for this step's LayerSignals. It is
// namespaced by phase name so transport and API TLS steps never collide.
func (s *tlsStep[T]) signalKey() string { return "tls." + s.GetPhaseName().String() }

// forceFlags reads the two force annotations (== "true"). force-all wins.
func (s *tlsStep[T]) forceFlags(o T) (forceAll, forceLeaf bool) {
	ann := o.GetAnnotations()
	if ann == nil {
		return false, false
	}
	forceAll = ann[s.forceAllAnnotation] == "true"
	forceLeaf = ann[s.forceLeafAnnotation] == "true"
	if forceAll && forceLeaf {
		forceLeaf = false
	}
	return forceAll, forceLeaf
}

// removeForceAnnotations deletes the honored annotation(s) and persists via
// client.Update. No requeue: the STS step still runs in this cycle.
func (s *tlsStep[T]) removeForceAnnotations(ctx context.Context, o T, forceAll, forceLeaf bool) error {
	ann := o.GetAnnotations()
	if len(ann) == 0 {
		return nil
	}
	if forceAll {
		delete(ann, s.forceAllAnnotation)
		delete(ann, s.forceLeafAnnotation) // force-all wins: clear both
	} else if forceLeaf {
		delete(ann, s.forceLeafAnnotation)
	} else {
		return nil
	}
	return s.Client().Update(ctx, o)
}

func (s *tlsStep[T]) Read(ctx context.Context, o T, data map[string]any, logger *logrus.Entry) (multiphase.MultiPhaseRead[client.Object], reconcile.Result, error) {
	read := multiphase.NewMultiPhaseRead[client.Object]()
	spec := s.provider.TLSSpec(o)
	namespace := o.GetNamespace()
	leafName := s.backend.CertificateSecretName(o, spec)
	caName := leafName + selfmanaged.CASecretSuffix

	// Load current secrets (NotFound is acceptable).
	currentLeaf := &corev1.Secret{}
	leafErr := s.Client().Get(ctx, client.ObjectKey{Namespace: namespace, Name: leafName}, currentLeaf)
	if leafErr != nil && !apierrors.IsNotFound(leafErr) {
		return nil, reconcile.Result{}, fmt.Errorf("read leaf secret %q: %w", leafName, leafErr)
	}
	currentCA := &corev1.Secret{}
	caErr := s.Client().Get(ctx, client.ObjectKey{Namespace: namespace, Name: caName}, currentCA)
	if caErr != nil && !apierrors.IsNotFound(caErr) {
		return nil, reconcile.Result{}, fmt.Errorf("read CA secret %q: %w", caName, caErr)
	}
	leafExists := leafErr == nil
	caExists := caErr == nil

	// Publish current data (best-effort parse). The Secrets are published as
	// sanitized deep copies with private-key material removed so accidental
	// debug-logging of the data map cannot leak key material. Per-node
	// backends omit tlsSecret/leafCert entirely: their private keys use
	// dynamic <node>.key names that sanitizedSecret would not strip.
	_, isPerNode := s.backend.(certificate.NodeSetTLSBackend[T])
	if leafExists && !isPerNode {
		data["tlsSecret"] = sanitizedSecret(currentLeaf, selfmanaged.KeyKey)
		if c := firstCert(currentLeaf.Data[selfmanaged.CertKey]); c != nil {
			data["leafCert"] = c
		}
	}
	if caExists {
		data["caSecret"] = sanitizedSecret(currentCA, selfmanaged.CAKeyPrivate)
		if c := firstCert(currentCA.Data[selfmanaged.CAKey]); c != nil {
			data["caCert"] = c
		}
	}

	// Non-saga backend: single-cycle emit, no phase logic.
	// The backend's DesiredObjects are the only managed objects; the leaf/CA
	// Secrets are NOT added as current objects because non-saga backends (byo,
	// certmanager) do not manage them as their own output. Registering the
	// Secrets as current would cause the SSA diff to classify them as orphans
	// and delete the user's (byo) or cert-manager's Secrets.
	if !s.backend.RequiresRotationSaga() {
		forceAll, forceLeaf := s.forceFlags(o)
		if forceAll || forceLeaf {
			s.Recorder().Event(o, corev1.EventTypeWarning, "TLSForceUnsupported",
				"force-regenerate annotations are not supported by this TLS backend; they were removed")
			if err := s.removeForceAnnotations(ctx, o, forceAll, forceLeaf); err != nil {
				return nil, reconcile.Result{}, err
			}
		}
		if err := s.emitDesired(ctx, o, spec, read); err != nil {
			return nil, reconcile.Result{}, err
		}
		return read, reconcile.Result{}, nil
	}

	startPhase := s.CurrentPhase(o)
	data["rotationStartPhase"] = startPhase

	switch startPhase {
	case "":
		now := time.Now()

		// 1. Force annotations (honored at phase "" only).
		forceAll, forceLeaf := s.forceFlags(o)

		// 2. CA need.
		caNeed := !caExists || forceAll
		if caExists && !forceAll {
			need, err := selfmanaged.CANeedsRenewal(currentCA, spec, now)
			if err != nil {
				return nil, reconcile.Result{}, err
			}
			caNeed = need
		}

		// 3. Leaf change (drift/expiry). LeafManager treats nil leaf as Missing.
		leafChg := certificate.LeafChange{}
		var leafArg *corev1.Secret
		if leafExists {
			leafArg = currentLeaf
		}
		if lm, ok := s.backend.(certificate.LeafManager[T]); ok {
			var err error
			leafChg, err = lm.LeafNeedsChange(ctx, o, leafArg, spec, now)
			if err != nil {
				return nil, reconcile.Result{}, err
			}
		} else if !leafExists {
			leafChg = certificate.LeafChange{Reason: certificate.LeafMissing}
		}

		// 4. force-leaf overrides drift; fall back to full saga if CA missing.
		if forceLeaf && !forceAll {
			leafChg = certificate.LeafChange{Reason: certificate.LeafForceRegen}
		}
		if forceLeaf && caNeed {
			forceAll = true
			forceLeaf = false
		}

		switch {
		case caNeed:
			// FULL CA SAGA (unchanged bundle logic) + signals.
			if err := s.runCASaga(ctx, o, spec, read, data, currentCA, currentLeaf, caExists, leafExists, caName, leafName, leafChg, forceAll); err != nil {
				return nil, reconcile.Result{}, err
			}

		case !leafChg.IsZero():
			// LEAF-ONLY (single-leaf OR per-node). No phase writes.
			if lm, ok := s.backend.(certificate.LeafManager[T]); ok {
				newLeaf, err := lm.DesiredLeafWithCA(ctx, o, spec, currentCA)
				if err != nil {
					return nil, reconcile.Result{}, err
				}
				s.decorate(o, newLeaf)
				read.AddExpectedObject(newLeaf)
				if leafExists {
					read.AddCurrentObject(currentLeaf)
				}
				if caExists {
					read.AddExpectedObject(currentCA)
					read.AddCurrentObject(currentCA)
				}
				data[s.signalKey()] = &certificate.LayerSignals{
					LeafRegenerated: true, LeafChange: &leafChg, Forced: forceLeaf,
				}
			} else {
				// Saga backend without LeafManager: fall back to full CA saga.
				if err := s.runCASaga(ctx, o, spec, read, data, currentCA, currentLeaf, caExists, leafExists, caName, leafName, leafChg, forceAll); err != nil {
					return nil, reconcile.Result{}, err
				}
			}

		default:
			// STEADY (unchanged, incl. stale-bundle recovery to Rotate).
			//
			// Recover from a stuck mid-rotation: a process crash after the
			// bundled leaf was applied but before the "" -> Rotate phase
			// advance was persisted leaves the leaf's ca.crt as a
			// newCA||oldCA bundle while the phase reads "". The renewal checks
			// are false (the new leaf is fresh), so the saga would otherwise
			// sit at "" with the old CA trusted until the next renewal (which
			// re-bundles rather than strips). The old CA's private key has
			// been destroyed by the new CA secret, but if it was compromised
			// before rotation the stale bundle extends the attacker's trust
			// window. Detect the stale bundle (leaf ca.crt != CA secret
			// ca.crt; in steady state they are byte-equal) and advance to
			// Rotate: the bundle is already applied, so Rotate's stable
			// state holds and the saga proceeds to Converge to strip the
			// old CA. This is a no-op in the happy path, where the phase is
			// already Rotate after a successful rotation.
			if leafExists && caExists && staleBundle(currentLeaf, currentCA) {
				if _, err := s.AdvancePhase(ctx, o, PhaseRotate, logger); err != nil {
					return nil, reconcile.Result{}, fmt.Errorf("advance phase to %q: %w", PhaseRotate, err)
				}
			}
			addStableObjects(read, currentCA, currentLeaf, caExists, leafExists)
		}

	case PhaseRotate:
		// Stable: do NOT regenerate. Expected == current (already applied in "").
		addStableObjects(read, currentCA, currentLeaf, caExists, leafExists)

	case PhaseConverge:
		// Clean leaf: ca.crt = current CA only (strip old CA). Do NOT regenerate.
		if caExists && leafExists {
			cleanLeaf := currentLeaf.DeepCopy()
			if cleanLeaf.Data == nil {
				cleanLeaf.Data = map[string][]byte{}
			}
			cleanLeaf.Data[selfmanaged.CAKey] = currentCA.Data[selfmanaged.CAKey]
			s.decorate(o, cleanLeaf)
			read.AddExpectedObject(cleanLeaf)
			read.AddCurrentObject(currentLeaf)
			read.AddExpectedObject(currentCA)
			read.AddCurrentObject(currentCA)
		} else {
			// Should not happen (we applied both in ""), but degrade gracefully:
			// fall back to a fresh DesiredObjects emit.
			if err := s.emitDesired(ctx, o, spec, read); err != nil {
				return nil, reconcile.Result{}, err
			}
			addCurrentObjects(read, currentCA, currentLeaf, caExists, leafExists)
		}

	default:
		// Unknown phase: treat as "" (re-evaluate).
		if _, err := s.AdvancePhase(ctx, o, "", logger); err != nil {
			return nil, reconcile.Result{}, fmt.Errorf("advance phase to empty: %w", err)
		}
		// Recurse-free: just set expected == current and let next cycle handle.
		addStableObjects(read, currentCA, currentLeaf, caExists, leafExists)
	}

	return read, reconcile.Result{}, nil
}

// runCASaga executes a full CA rotation: it generates a fresh CA + leaf via
// DesiredObjects, bundles the old CA into the leaf's ca.crt when a current
// leaf exists (so consumers trust both CAs during the transition), decorates
// and registers both objects, and publishes the rotationRenewed + LayerSignals
// data. It is shared by the CA-renewal branch and the fallback used when a
// saga backend lacks the LeafManager capability.
func (s *tlsStep[T]) runCASaga(
	ctx context.Context,
	o T,
	spec certificate.TLSSpec,
	read multiphase.MultiPhaseRead[client.Object],
	data map[string]any,
	currentCA, currentLeaf *corev1.Secret,
	caExists, leafExists bool,
	caName, leafName string,
	leafChg certificate.LeafChange,
	forceAll bool,
) error {
	objs, err := s.backend.DesiredObjects(ctx, o, spec)
	if err != nil {
		return err
	}
	newCA, newLeaf, err := splitCAAndLeaf(objs, caName, leafName)
	if err != nil {
		return err
	}
	if leafExists {
		if oldCA, ok := currentLeaf.Data[selfmanaged.CAKey]; ok && len(oldCA) > 0 {
			bundled := append([]byte{}, newCA.Data[selfmanaged.CAKey]...)
			bundled = append(bundled, oldCA...)
			newLeaf.Data[selfmanaged.CAKey] = bundled
		}
	}
	s.decorate(o, newCA)
	s.decorate(o, newLeaf)
	read.AddExpectedObject(newCA)
	read.AddExpectedObject(newLeaf)
	addCurrentObjects(read, currentCA, currentLeaf, caExists, leafExists)
	data["rotationRenewed"] = true
	data[s.signalKey()] = &certificate.LayerSignals{
		CARotated: true, LeafRegenerated: true, LeafChange: &leafChg, Forced: forceAll,
	}
	return nil
}

// splitCAAndLeaf finds the CA and leaf Secret in objs by name.
func splitCAAndLeaf(objs []client.Object, caName, leafName string) (*corev1.Secret, *corev1.Secret, error) {
	var ca, leaf *corev1.Secret
	for _, obj := range objs {
		sec, ok := obj.(*corev1.Secret)
		if !ok {
			continue
		}
		switch sec.Name {
		case caName:
			ca = sec
		case leafName:
			leaf = sec
		}
	}
	if ca == nil || leaf == nil {
		return nil, nil, fmt.Errorf("saga backend must return CA secret %q and leaf secret %q", caName, leafName)
	}
	return ca, leaf, nil
}

// sanitizedSecret returns a deep copy of secret with keyToStrip removed from
// its Data map. The original secret is never mutated. It is used when
// publishing Secrets into the reconcile data blackboard so that private-key
// material does not leak via debug logging or injected convergence checks.
func sanitizedSecret(secret *corev1.Secret, keyToStrip string) *corev1.Secret {
	cp := secret.DeepCopy()
	if cp.Data == nil {
		return cp
	}
	delete(cp.Data, keyToStrip)
	return cp
}

// firstCert parses the first CERTIFICATE PEM block. Returns nil on any failure
// (best-effort publication).
func firstCert(pemBytes []byte) *x509.Certificate {
	if len(pemBytes) == 0 {
		return nil
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	return c
}

// addCurrentObjects registers the present CA and leaf secrets as current.
func addCurrentObjects(read multiphase.MultiPhaseRead[client.Object], ca, leaf *corev1.Secret, caExists, leafExists bool) {
	if caExists {
		read.AddCurrentObject(ca)
	}
	if leafExists {
		read.AddCurrentObject(leaf)
	}
}

// addStableObjects registers the present CA and leaf secrets as both expected
// and current, producing a no-op apply that keeps the applied state stable.
func addStableObjects(read multiphase.MultiPhaseRead[client.Object], ca, leaf *corev1.Secret, caExists, leafExists bool) {
	if caExists {
		read.AddExpectedObject(ca)
		read.AddCurrentObject(ca)
	}
	if leafExists {
		read.AddExpectedObject(leaf)
		read.AddCurrentObject(leaf)
	}
}

// staleBundle reports whether the leaf's ca.crt is a leftover newCA||oldCA
// bundle from a Rotate whose phase advance was lost. In steady state the
// leaf ca.crt and the CA secret ca.crt are byte-equal (DesiredObjects sets
// both to the same single CA cert, and Converge resets the leaf to the CA
// secret's ca.crt). A bundle is detected precisely: the leaf ca.crt has the
// CA secret ca.crt as a prefix and is strictly longer, so unrelated
// corruption (which would not share the prefix) does not over-trigger.
func staleBundle(leaf, ca *corev1.Secret) bool {
	leafCA := leaf.Data[selfmanaged.CAKey]
	caCA := ca.Data[selfmanaged.CAKey]
	return len(leafCA) > len(caCA) && bytes.HasPrefix(leafCA, caCA)
}

func (s *tlsStep[T]) decorate(o T, obj client.Object) {
	if s.labelsDecorator != nil {
		s.labelsDecorator(o, obj)
	}
	if s.annotationsDecorator != nil {
		s.annotationsDecorator(o, obj)
	}
}

// emitDesired generates the backend's desired objects, decorates them, and adds
// them as expected objects to read.
func (s *tlsStep[T]) emitDesired(ctx context.Context, o T, spec certificate.TLSSpec, read multiphase.MultiPhaseRead[client.Object]) error {
	objs, err := s.backend.DesiredObjects(ctx, o, spec)
	if err != nil {
		return err
	}
	for _, obj := range objs {
		s.decorate(o, obj)
		read.AddExpectedObject(obj)
	}
	return nil
}

func (s *tlsStep[T]) OnDiff(ctx context.Context, o T, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (reconcile.Result, error) {
	if !s.backend.RequiresRotationSaga() {
		return reconcile.Result{}, nil
	}
	startPhase, _ := data["rotationStartPhase"].(apworkflow.WorkflowPhase)
	if startPhase != PhaseRotate {
		return reconcile.Result{}, nil
	}
	if s.convergenceCheck == nil {
		// No gate injected: advance immediately.
		if _, err := s.AdvancePhase(ctx, o, PhaseConverge, logger); err != nil {
			return reconcile.Result{}, fmt.Errorf("advance phase to %q: %w", PhaseConverge, err)
		}
		return reconcile.Result{}, nil
	}
	converged, err := s.convergenceCheck(ctx, o, data)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !converged {
		return reconcile.Result{RequeueAfter: workflow.DefaultRequeueAfter}, nil
	}
	if _, err := s.AdvancePhase(ctx, o, PhaseConverge, logger); err != nil {
		return reconcile.Result{}, fmt.Errorf("advance phase to %q: %w", PhaseConverge, err)
	}
	return reconcile.Result{}, nil
}

func (s *tlsStep[T]) OnSuccess(ctx context.Context, o T, data map[string]any, diff multiphase.MultiPhaseDiff[client.Object], logger *logrus.Entry) (reconcile.Result, error) {
	// Remove honored force annotations (only reached after a successful Apply).
	// This runs BEFORE the inherited condition handling because client.Update
	// deserializes the API response back into `o`, overwriting any in-memory
	// status mutation (the ready condition) made earlier in this cycle.
	if s.backend.RequiresRotationSaga() {
		if sig, _ := data[s.signalKey()].(*certificate.LayerSignals); sig != nil && sig.Forced {
			forceAll, forceLeaf := s.forceFlags(o) // annotations still present on o
			if forceAll || forceLeaf {
				if err := s.removeForceAnnotations(ctx, o, forceAll, forceLeaf); err != nil {
					return reconcile.Result{}, fmt.Errorf("remove force-regenerate annotation: %w", err)
				}
			}
		}
	}

	// Inherited condition handling (sets condition True on success).
	res, err := s.DefaultWorkflowStepReconcilerActionWithDiff.OnSuccess(ctx, o, data, diff, logger)
	if err != nil {
		return res, err
	}
	if !s.backend.RequiresRotationSaga() {
		return res, nil // non-saga force removal already happened in Read
	}

	startPhase, _ := data["rotationStartPhase"].(apworkflow.WorkflowPhase)
	switch startPhase {
	case "":
		if renewed, _ := data["rotationRenewed"].(bool); renewed {
			if _, err := s.AdvancePhase(ctx, o, PhaseRotate, logger); err != nil {
				return res, fmt.Errorf("advance phase to %q: %w", PhaseRotate, err)
			}
		}
	case PhaseConverge:
		if _, err := s.AdvancePhase(ctx, o, "", logger); err != nil {
			return res, fmt.Errorf("advance phase to empty: %w", err)
		}
	}
	return res, nil
}
