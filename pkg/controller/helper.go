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
	ShortenError   int                  = 5000
)

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
	return om.Interface().(metav1.ObjectMeta)
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
func EnsureNetworkPolicyForWebhook(c client.Client, logger *logrus.Entry, namespace string, labels map[string]string, podSelecetors map[string]string) error {
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

	if err := c.Get(context.Background(), types.NamespacedName{Namespace: expectedNetworkPolicy.GetNamespace(), Name: expectedNetworkPolicy.GetName()}, networkPolicy); err != nil {
		// Create
		if k8serrors.IsNotFound(err) {
			// Set diff 3-way annotations
			if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(networkPolicy); err != nil {
				return errors.Wrap(err, "Error when set annotation for 3-way diff on NetworkPolicy for webhook")
			}
			if err = c.Create(context.Background(), expectedNetworkPolicy); err != nil {
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
		networkPolicy = patchResult.Patched.(*networkv1.NetworkPolicy)
		if err = c.Update(context.Background(), networkPolicy); err != nil {
			return errors.Wrap(err, "Error when update NetworkPolicy for webhook")
		}
		logger.Info("Successfully update networkPolicy for webhook")
	}

	return nil
}
