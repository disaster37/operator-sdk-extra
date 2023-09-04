package controllers

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/k8s-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/pkg/helper"
	"github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ConfigmapCondition controller.ConditionName = "ConfigmapReady"
	ConfigmapPhase     controller.PhaseName     = "Configmap"
)

type ConfigMapReconciler struct {
	controller.BasicMultiPhaseStepReconciler
}

func NewConfigMapReconciler(client client.Client, logger *logrus.Entry, recorder record.EventRecorder, scheme *runtime.Scheme, ignoresDiff ...patch.CalculateOption) (multiPhaseStepReconciler controller.MultiPhaseStepReconciler, err error) {
	return controller.NewBasicMultiPhaseStepReconciler(
		client,
		ConfigmapPhase,
		ConfigmapCondition,
		logger,
		recorder,
		scheme,
		ignoresDiff...,
	)
}

func (r *ConfigMapReconciler) Read(ctx context.Context, o controller.MultiPhaseObject, data map[string]any) (read controller.MultiPhaseRead, res ctrl.Result, err error) {
	mc := o.(*v1alpha1.Memcached)
	cmList := &corev1.ConfigMapList{}
	read = controller.NewBasicMultiPhaseRead()

	// Read current configmaps
	labelSelectors, err := labels.Parse(fmt.Sprintf("name=%s,%s=true", o.GetName(), v1alpha1.MemcachedAnnotationKey))
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate label selector")
	}
	if err = r.Client.List(ctx, cmList, &client.ListOptions{Namespace: o.GetNamespace(), LabelSelector: labelSelectors}); err != nil {
		return read, res, errors.Wrapf(err, "Error when read configmaps")
	}

	read.SetCurrentObjects(helper.ToSliceOfObject(cmList.Items))

	// Generate expected configmaps
	expectedCms, err := NewConfigMapsBuilder(mc)
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate expected configMaps")
	}
	read.SetExpectedObjects(helper.ToSliceOfObject(expectedCms))

	return read, res, nil
}