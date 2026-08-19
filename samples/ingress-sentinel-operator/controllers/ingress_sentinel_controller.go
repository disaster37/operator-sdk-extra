package controllers

import (
	"context"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/sentinel"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8scontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	targetAnnotation = "sentinel.example.com/target-config"
	controllerName   = "ingress-sentinel"
)

type IngressSentinelReconciler struct {
	controller.Controller
	sentinel.SentinelReconciler[*networkingv1.Ingress]
	sentinel.SentinelReconcilerAction[*networkingv1.Ingress]
	name string
}

func NewIngressSentinelReconciler(c client.Client, logger *logrus.Entry, recorder record.EventRecorder) controller.Controller {
	return &IngressSentinelReconciler{
		Controller: controller.NewController(),
		SentinelReconciler: sentinel.NewSentinelReconciler[*networkingv1.Ingress](
			c,
			controllerName,
			logger,
			recorder,
		),
		SentinelReconcilerAction: newIngressSentinelAction[*networkingv1.Ingress](c, recorder),
		name:                     controllerName,
	}
}

//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *IngressSentinelReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	o := &networkingv1.Ingress{}
	data := map[string]any{}
	return r.SentinelReconciler.Reconcile(ctx, req, o, data, r.SentinelReconcilerAction)
}

func (r *IngressSentinelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Owns(&corev1.ConfigMap{}).
		WithOptions(k8scontroller.Options{
			RateLimiter: controller.DefaultControllerRateLimiter[reconcile.Request](),
		}).
		Complete(r)
}

type ingressSentinelAction[k8sObject client.Object] struct {
	sentinel.SentinelReconcilerAction[k8sObject]
}

func newIngressSentinelAction[k8sObject client.Object](c client.Client, recorder record.EventRecorder) sentinel.SentinelReconcilerAction[k8sObject] {
	return &ingressSentinelAction[k8sObject]{
		SentinelReconcilerAction: sentinel.NewSentinelAction[k8sObject](c, recorder, "ingress-sentinel-operator", false),
	}
}

func (h *ingressSentinelAction[k8sObject]) Read(
	ctx context.Context,
	o k8sObject,
	data map[string]any,
	logger *logrus.Entry,
) (read sentinel.SentinelRead, res reconcile.Result, err error) {
	read = sentinel.NewSentinelRead(h.Client().Scheme())

	if o.GetAnnotations() != nil && o.GetAnnotations()[targetAnnotation] != "" {
		read.AddExpectedObject(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      o.GetName(),
				Namespace: o.GetNamespace(),
			},
			Data: map[string]string{
				"config": o.GetAnnotations()[targetAnnotation],
			},
		})
	}

	currentCM := &corev1.ConfigMap{}
	if err = h.Client().Get(ctx, types.NamespacedName{
		Namespace: o.GetNamespace(),
		Name:      o.GetName(),
	}, currentCM); err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, res, err
		}
		currentCM = nil
	}
	read.AddCurrentObject(currentCM)

	return read, res, nil
}
