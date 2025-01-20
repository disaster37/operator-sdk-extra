package multiphase_test

import (
	"context"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/multiphase"
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
	name               string               = "test"
	finalizer          shared.FinalizerName = "test.operator.webcenter.fr/finalizer"
	ConfigmapCondition shared.ConditionName = "ConfigmapReady"
	ConfigmapPhase     shared.PhaseName     = "Configmap"
)

/*************
* Controller
 */
type TestReconciler struct {
	controller.Controller
	multiphase.MultiPhaseReconciler[*MultiPhaseObject]
	multiphase.MultiPhaseReconcilerAction[*MultiPhaseObject]
	name            string
	stepReconcilers []multiphase.MultiPhaseStepReconcilerAction[*MultiPhaseObject, client.Object]
}

func NewTestReconciler(c client.Client, logger *logrus.Entry, recorder record.EventRecorder) (multiPhaseReconciler controller.Controller) {
	return &TestReconciler{
		Controller: controller.NewController(),
		MultiPhaseReconciler: multiphase.NewMultiPhaseReconciler[*MultiPhaseObject](
			c,
			name,
			finalizer,
			logger,
			recorder,
		),
		MultiPhaseReconcilerAction: multiphase.NewMultiPhaseReconcilerAction[*MultiPhaseObject](
			c,
			controller.ReadyCondition,
			recorder,
		),
		name: name,
		stepReconcilers: []multiphase.MultiPhaseStepReconcilerAction[*MultiPhaseObject, client.Object]{
			newConfiMapReconciler(c, recorder),
		},
	}
}

func (r *TestReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	o := &MultiPhaseObject{}
	data := map[string]any{}

	return r.MultiPhaseReconciler.Reconcile(
		ctx,
		req,
		o,
		data,
		r.MultiPhaseReconcilerAction,
		r.stepReconcilers...,
	)
}

func (h *TestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&MultiPhaseObject{}).
		Owns(&corev1.ConfigMap{}).
		WithOptions(k8scontroller.Options{
			RateLimiter: controller.DefaultControllerRateLimiter[reconcile.Request](),
		}).
		WithOptions(k8scontroller.TypedOptions[reconcile.Request]{
			MaxConcurrentReconciles: 1,
		}).
		Complete(h)
}

/************
* Step reconciler
 */

type configMapReconciler struct {
	multiphase.MultiPhaseStepReconcilerAction[*MultiPhaseObject, client.Object]
}

func newConfiMapReconciler(c client.Client, recorder record.EventRecorder) (multiPhaseStepReconcilerAction multiphase.MultiPhaseStepReconcilerAction[*MultiPhaseObject, client.Object]) {
	return &configMapReconciler{
		MultiPhaseStepReconcilerAction: multiphase.NewMultiPhaseStepReconcilerAction[*MultiPhaseObject, client.Object](
			c,
			ConfigmapPhase,
			ConfigmapCondition,
			recorder,
		),
	}
}

func (r *configMapReconciler) Read(ctx context.Context, o *MultiPhaseObject, data map[string]any, logger *logrus.Entry) (read multiphase.MultiPhaseRead[client.Object], res reconcile.Result, err error) {
	cm := &corev1.ConfigMap{}
	read = multiphase.NewMultiPhaseRead[client.Object]()

	if err = r.Client().Get(ctx, types.NamespacedName{Namespace: o.Namespace, Name: o.Name}, cm); err != nil {
		if !k8serrors.IsNotFound(err) {
			return read, res, errors.Wrap(err, "Error when read config maps")
		}
		cm = nil
	}
	if cm != nil {
		read.AddCurrentObject(cm)
	}

	read.AddExpectedObject(&corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      o.Name,
			Namespace: o.Namespace,
			Labels:    o.Labels,
		},
		Data: map[string]string{
			"foo": "bar",
		},
	})

	return read, res, nil
}
