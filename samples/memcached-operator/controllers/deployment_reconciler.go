package controllers

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/multiphase"
	cachecrd "github.com/disaster37/operator-sdk-extra/v2/samples/memcached-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	appv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	DeploymentCondition shared.ConditionName = "DeploymentReady"
	DeploymentPhase     shared.PhaseName     = "Deployment"
)

type deploymentReconciler struct {
	multiphase.MultiPhaseStepReconcilerAction[*cachecrd.Memcached, *appv1.Deployment]
}

func newDeploymentReconciler(c client.Client, recorder record.EventRecorder) multiphase.MultiPhaseStepReconcilerAction[*cachecrd.Memcached, *appv1.Deployment] {
	return &deploymentReconciler{
		MultiPhaseStepReconcilerAction: multiphase.NewMultiPhaseStepReconcilerAction[*cachecrd.Memcached, *appv1.Deployment](
			c,
			DeploymentPhase,
			DeploymentCondition,
			recorder,
			"memcached-operator",
		),
	}
}

func (r *deploymentReconciler) Read(ctx context.Context, o *cachecrd.Memcached, data map[string]any, logger *logrus.Entry) (read multiphase.MultiPhaseRead[*appv1.Deployment], res reconcile.Result, err error) {
	deploymentList := &appv1.DeploymentList{}
	read = multiphase.NewMultiPhaseRead[*appv1.Deployment]()

	labelSelectors, err := labels.Parse(fmt.Sprintf("name=%s,%s=true", o.GetName(), cachecrd.MemcachedAnnotationKey))
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate label selector")
	}
	if err = r.Client().List(ctx, deploymentList, &client.ListOptions{Namespace: o.GetNamespace(), LabelSelector: labelSelectors}); err != nil {
		return read, res, errors.Wrapf(err, "Error when read deployments")
	}

	for i := range deploymentList.Items {
		read.AddCurrentObject(&deploymentList.Items[i])
	}

	expectedDeployments, err := newDeploymentsBuilder(o)
	if err != nil {
		return read, res, errors.Wrap(err, "Error when generate expected deployments")
	}
	for i := range expectedDeployments {
		read.AddExpectedObject(&expectedDeployments[i])
	}

	return read, res, nil
}
