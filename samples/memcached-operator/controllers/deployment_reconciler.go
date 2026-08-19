package controllers

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/multiphase"
	cachecrd "github.com/disaster37/operator-sdk-extra/v3/samples/memcached-operator/api/v1alpha1"
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
			true,
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

// OnDiff demonstrates the pre-update hook pattern.
// In a real operator, you would perform pre-tasks here (e.g. drain nodes,
// scale down, pause traffic) before SSA apply runs.
func (r *deploymentReconciler) OnDiff(ctx context.Context, o *cachecrd.Memcached, data map[string]any, diff multiphase.MultiPhaseDiff[*appv1.Deployment], logger *logrus.Entry) (res reconcile.Result, err error) {
	if diff.NeedUpdate() {
		logger.Infof("Pre-update task: deployment change detected for %d object(s)", len(diff.GetObjectsToUpdate()))
		for _, dpl := range diff.GetObjectsToUpdate() {
			logger.Debugf("Deployment '%s' will be updated", dpl.GetName())
		}
	}
	return reconcile.Result{}, nil
}
