package controller

import (
	"context"
	"reflect"
	"time"

	"emperror.dev/errors"
	"github.com/disaster37/k8s-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	v1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RunningPhase   shared.PhaseName     = "running"
	StartingPhase  shared.PhaseName     = "starting"
	ReadyCondition shared.ConditionName = "Ready"
	BaseAnnotation string               = "operator-sdk-extra.webcenter.fr"

	// ShortenError is the historical max length used for truncating error messages.
	// Deprecated: use MaxConditionMessage, MaxEventMessage or MaxStatusMessage instead,
	// combined with UserFacingError to extract the root cause before truncating.
	ShortenError int = 5000

	// MaxConditionMessage is the recommended max length for a metav1.Condition.Message
	// stored in a CRD status. The Kubernetes spec allows up to 32 KiB, but most
	// controllers cap well below 1 KiB to keep `kubectl get` / `kubectl describe` readable.
	MaxConditionMessage = 1024

	// MaxEventMessage is the recommended max length for a corev1.Event message.
	// The Kubernetes event recorder silently drops events whose total size exceeds
	// the etcd object limit (~1.5 MiB), and messages above ~1 KiB are rarely useful
	// when surfaced via `kubectl describe`.
	MaxEventMessage = 512

	// MaxStatusMessage is the recommended max length for the LastErrorMessage
	// field stored in CRD status. Stored in etcd, so kept short to avoid document
	// bloat across repeated failing reconcile loops.
	MaxStatusMessage = 1024
)

// UserFacingError returns a short, human-useful error message from an error chain.
//
// It extracts the deepest (root) cause through errors.Cause — the one produced by
// the underlying library or API — discarding the intermediate wrapping messages
// added by errors.Wrap / errors.Wrapf up the call stack. Those wrapping messages
// remain available via the logger.
//
// The result is truncated to maxLen bytes. When truncation occurs the string is
// suffixed with "..." so the user can see the message was not complete.
//
// If the chain has no extractable cause, the top-level error message is used as-is.
func UserFacingError(err error, maxLen int) string {
	if err == nil {
		return ""
	}

	msg := err.Error()
	if cause := errors.Cause(err); cause != nil && cause != err {
		msg = cause.Error()
	}

	if len(msg) <= maxLen {
		return msg
	}
	if maxLen < 3 {
		return msg[:maxLen]
	}
	return msg[:maxLen-3] + "..."
}

// GetObjectMeta permit to get the metata from client.Object
func GetObjectMeta(r client.Object) metav1.ObjectMeta {
	rt := reflect.TypeOf(r)
	if rt.Kind() != reflect.Ptr {
		panic("Resource must be pointer")
	}
	rv := reflect.ValueOf(r).Elem()
	om := rv.FieldByName("ObjectMeta")
	if !om.IsValid() {
		panic("Resouce must have field ObjectMeta")
	}
	return om.Interface().(metav1.ObjectMeta) //nolint:forcetypeassert // field existence validated above
}

// GetObjectStatus permit to get the status from client.Object
func GetObjectStatus(r client.Object) any {
	rt := reflect.TypeOf(r)
	if rt.Kind() != reflect.Ptr {
		panic("Resource must be pointer")
	}
	rv := reflect.ValueOf(r).Elem()
	om := rv.FieldByName("Status")
	if !om.IsValid() {
		panic("Resouce must have field Status")
	}
	return om.Interface()
}

// MustInjectTypeMeta permit to inject the typeMeta from src to dst
func MustInjectTypeMeta(src, dst client.Object) {
	var rt reflect.Type

	rt = reflect.TypeOf(src)
	if rt.Kind() != reflect.Ptr {
		panic("Resource must be pointer")
	}
	rt = reflect.TypeOf(dst)
	if rt.Kind() != reflect.Ptr {
		panic("Resource must be pointer")
	}

	rvSrc := reflect.ValueOf(src).Elem()
	omSrc := rvSrc.FieldByName("TypeMeta")
	if !omSrc.IsValid() {
		panic("src must have field TypeMeta")
	}
	rvDst := reflect.ValueOf(dst).Elem()
	omDst := rvDst.FieldByName("TypeMeta")
	if !omDst.IsValid() {
		panic("dst must have field TypeMeta")
	}

	omDst.Set(omSrc)
}

// DefaultControllerRateLimiter set rate limiter that is lower agressive than the default
func DefaultControllerRateLimiter[T comparable]() workqueue.TypedRateLimiter[T] {
	return workqueue.NewTypedMaxOfRateLimiter(
		workqueue.NewTypedItemExponentialFailureRateLimiter[T](1*time.Second, 1000*time.Second),
		&workqueue.TypedBucketRateLimiter[T]{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)
}

// EnsureNetworkPolicyForWebhook permit to create / update NetworkPolicy for webhook
func EnsureNetworkPolicyForWebhook(ctx context.Context, c client.Client, logger *logrus.Entry, namespace string, labels map[string]string, podSelecetors map[string]string) error {
	networkPolicy := &networkv1.NetworkPolicy{}
	expectedNetworkPolicy := &networkv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "allow-webhook-access-from-any",
			Labels:    labels,
		},
		Spec: networkv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: podSelecetors,
			},
			Ingress: []networkv1.NetworkPolicyIngressRule{
				{
					From: []networkv1.NetworkPolicyPeer{},
					Ports: []networkv1.NetworkPolicyPort{
						{
							Protocol: ptr.To(v1.ProtocolTCP),
							Port:     ptr.To(intstr.FromInt(9443)),
						},
					},
				},
			},
		},
	}

	if err := c.Get(ctx, types.NamespacedName{Namespace: expectedNetworkPolicy.GetNamespace(), Name: expectedNetworkPolicy.GetName()}, networkPolicy); err != nil {
		// Create
		if k8serrors.IsNotFound(err) {
			// Set diff 3-way annotations
			if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(networkPolicy); err != nil {
				return errors.Wrap(err, "Error when set annotation for 3-way diff on NetworkPolicy for webhook")
			}
			if err = c.Create(ctx, expectedNetworkPolicy); err != nil {
				return errors.Wrap(err, "Error when create NetworkPolicy for webhook")
			}

			logger.Info("Successfully create networkPolicy for webhook")
			return nil
		}
		return errors.Wrap(err, "Error when get NetworkPolicy for webhook")
	}

	// Diff
	MustInjectTypeMeta(networkPolicy, expectedNetworkPolicy)
	patchResult, err := patch.DefaultPatchMaker.Calculate(networkPolicy, expectedNetworkPolicy)
	if err != nil {
		return errors.Wrap(err, "Error when diffing NetworkPolicy for webhook")
	}

	// Update
	if !patchResult.IsEmpty() {
		patchedNP, ok := patchResult.Patched.(*networkv1.NetworkPolicy)
		if !ok {
			return errors.New("unexpected type in patch result: expected *networkv1.NetworkPolicy")
		}
		networkPolicy = patchedNP
		if err = c.Update(ctx, networkPolicy); err != nil {
			return errors.Wrap(err, "Error when update NetworkPolicy for webhook")
		}
		logger.Info("Successfully update networkPolicy for webhook")
	}

	return nil
}
