package sentinel_test

import (
	"context"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/sentinel"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8scontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	name       = "test"
	annotation = "test.operator.webcenter.fr/sentinel"
)

/*************
* Controller
 */
type TestReconciler struct {
	controller.Controller
	sentinel.SentinelReconciler[*corev1.Namespace]
	sentinel.SentinelReconcilerAction[*corev1.Namespace]
	name string
}

func NewTestReconciler(c client.Client, logger *logrus.Entry, recorder record.EventRecorder) (multiPhaseReconciler controller.Controller) {
	return &TestReconciler{
		Controller: controller.NewController(),
		SentinelReconciler: sentinel.NewSentinelReconciler[*corev1.Namespace](
			c,
			name,
			logger,
			recorder,
		),
		SentinelReconcilerAction: newTemplateAnnotationsReconciler[*corev1.Namespace](
			c,
			recorder,
		),
		name: name,
	}
}

func (r *TestReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	o := &corev1.Namespace{}
	data := map[string]any{}

	return r.SentinelReconciler.Reconcile(
		ctx,
		req,
		o,
		data,
		r.SentinelReconcilerAction,
	)
}

func (h *TestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		WithOptions(k8scontroller.Options{
			RateLimiter: controller.DefaultControllerRateLimiter[reconcile.Request](),
		}).
		WithOptions(k8scontroller.TypedOptions[reconcile.Request]{
			MaxConcurrentReconciles: 1,
		}).
		Complete(h)
}

/************
* Template reconciler
 */

type templateAnnotationsReconciler[k8sObject client.Object] struct {
	sentinel.SentinelReconcilerAction[k8sObject]
}

func newTemplateAnnotationsReconciler[k8sObject client.Object](c client.Client, recorder record.EventRecorder) sentinel.SentinelReconcilerAction[k8sObject] {
	return &templateAnnotationsReconciler[k8sObject]{
		SentinelReconcilerAction: sentinel.NewSentinelAction[k8sObject](
			c,
			recorder,
		),
	}
}

func (h *templateAnnotationsReconciler[k8sObject]) Read(ctx context.Context, o k8sObject, data map[string]any, logger *logrus.Entry) (read sentinel.SentinelRead, res reconcile.Result, err error) {
	read = sentinel.NewSentinelRead(h.Client().Scheme())
	var (
		cm *corev1.ConfigMap
		s  *corev1.Secret
	)

	if o.GetAnnotations() != nil && o.GetAnnotations()[annotation] != "" {
		// Compute expecting objects
		read.AddExpectedObject(&corev1.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:      o.GetName(),
				Namespace: o.GetName(),
			},
			Data: map[string]string{
				"val": o.GetAnnotations()[annotation],
			},
		})

		read.AddExpectedObject(&corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      o.GetName(),
				Namespace: o.GetName(),
			},
			Data: map[string][]byte{
				"val": []byte(o.GetAnnotations()[annotation]),
			},
		})
	}

	// Read current objects
	cm = &corev1.ConfigMap{}
	if err = h.Client().Get(ctx, types.NamespacedName{Namespace: o.GetName(), Name: o.GetName()}, cm); err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, res, err
		}
		cm = nil
	}
	read.AddCurrentObject(cm)

	s = &corev1.Secret{}
	if err = h.Client().Get(ctx, types.NamespacedName{Namespace: o.GetName(), Name: o.GetName()}, s); err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, res, err
		}
		s = nil
	}
	read.AddCurrentObject(s)

	return read, res, nil
}
