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
	appv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DeploymentCondition shared.ConditionName = "DeploymentReady"
	DeploymentPhase     shared.PhaseName     = "Deployment"
)

type DeploymentReconciler struct {
	controller.MultiPhaseStepReconcilerAction
	BaseReconciler
}

func NewDeploymentReconciler(client client.Client, logger *logrus.Entry, recorder record.EventRecorder, scheme *runtime.Scheme) (multiPhaseStepReconcilerAction *DeploymentReconciler) {

	return &DeploymentReconciler{
		MultiPhaseStepReconcilerAction: controller.NewBasicMultiPhaseStepReconcilerAction(
			client,
			DeploymentPhase,
			DeploymentCondition,
			logger,
			recorder,
			scheme,
		),
		BaseReconciler: BaseReconciler{
			client:   client,
			recorder: recorder,
			logger:   logger,
		},
	}
}

func (r *DeploymentReconciler) Read(ctx context.Context, o object.MultiPhaseObject, data map[string]any) (read controller.MultiPhaseRead, res ctrl.Result, err error) {
	mc := o.(*v1alpha1.Memcached)
	deploymentList := &appv1.DeploymentList{}
	read = controller.NewBasicMultiPhaseRead()

	// Read current configmaps
	labelSelectors, err := labels.Parse(fmt.Sprintf("name=%s,%s=true", o.GetName(), v1alpha1.MemcachedAnnotationKey))
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate label selector")
	}
	if err = r.client.List(ctx, deploymentList, &client.ListOptions{Namespace: o.GetNamespace(), LabelSelector: labelSelectors}); err != nil {
		return read, res, errors.Wrapf(err, "Error when read deployments")
	}

	read.SetCurrentObjects(helper.ToSliceOfObject(deploymentList.Items))

	// Generate expected configmaps
	expectedDeployments, err := NewDeploymentsBuilder(mc)
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate expected deployments")
	}
	read.SetExpectedObjects(helper.ToSliceOfObject(expectedDeployments))

	return read, res, nil
}
