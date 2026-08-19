package controllers

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/multiphase"
	cachecrd "github.com/disaster37/operator-sdk-extra/v3/samples/memcached-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	ConfigmapCondition shared.ConditionName = "ConfigmapReady"
	ConfigmapPhase     shared.PhaseName     = "Configmap"
)

type configMapReconciler struct {
	multiphase.MultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap]
}

func newConfigMapReconciler(c client.Client, recorder record.EventRecorder) multiphase.MultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap] {
	return &configMapReconciler{
		MultiPhaseStepReconcilerAction: multiphase.NewMultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap](
			c,
			ConfigmapPhase,
			ConfigmapCondition,
			recorder,
			"memcached-operator",
		),
	}
}

func (r *configMapReconciler) Read(ctx context.Context, o *cachecrd.Memcached, data map[string]any, logger *logrus.Entry) (read multiphase.MultiPhaseRead[*corev1.ConfigMap], res reconcile.Result, err error) {
	cmList := &corev1.ConfigMapList{}
	read = multiphase.NewMultiPhaseRead[*corev1.ConfigMap]()

	labelSelectors, err := labels.Parse(fmt.Sprintf("name=%s,%s=true", o.GetName(), cachecrd.MemcachedAnnotationKey))
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate label selector")
	}
	if err = r.Client().List(ctx, cmList, &client.ListOptions{Namespace: o.GetNamespace(), LabelSelector: labelSelectors}); err != nil {
		return read, res, errors.Wrapf(err, "Error when read configmaps")
	}

	for i := range cmList.Items {
		read.AddCurrentObject(&cmList.Items[i])
	}

	expectedCms, err := newConfigMapsBuilder(o)
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate expected configMaps")
	}
	for i := range expectedCms {
		read.AddExpectedObject(&expectedCms[i])
	}

	return read, res, nil
}
