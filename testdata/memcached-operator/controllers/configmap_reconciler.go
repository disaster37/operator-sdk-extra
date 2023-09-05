package controllers

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/pkg/helper"
	"github.com/disaster37/operator-sdk-extra/pkg/object"
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
	ConfigmapCondition shared.ConditionName = "ConfigmapReady"
	ConfigmapPhase     shared.PhaseName     = "Configmap"
)

type ConfigMapReconciler struct {
	controller.MultiPhaseStepReconcilerAction
}

func NewConfigMapReconcilerAction(client client.Client, logger *logrus.Entry, recorder record.EventRecorder, scheme *runtime.Scheme) (multiPhaseStepReconciler controller.MultiPhaseStepReconcilerAction, err error) {
	basicMultiPhaseStepAction, err := controller.NewBasicMultiPhaseStepReconcilerAction(
		client,
		ConfigmapPhase,
		ConfigmapCondition,
		logger,
		recorder,
		scheme,
	)
	if err != nil {
		return nil, errors.Wrap(err, "Error when create basicMultiPhaseStep reconciler")
	}

	return &ConfigMapReconciler{
		MultiPhaseStepReconcilerAction: basicMultiPhaseStepAction,
	}, nil
}

func (r *ConfigMapReconciler) Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (read controller.MultiPhaseRead, res ctrl.Result, err error) {
	mc := o.(*v1alpha1.Memcached)
	cmList := &corev1.ConfigMapList{}
	read = controller.NewBasicMultiPhaseRead()

	// Read current configmaps
	labelSelectors, err := labels.Parse(fmt.Sprintf("name=%s,%s=true", o.GetName(), v1alpha1.MemcachedAnnotationKey))
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate label selector")
	}
	if err = r.GetClient().List(ctx, cmList, &client.ListOptions{Namespace: o.GetNamespace(), LabelSelector: labelSelectors}); err != nil {
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
