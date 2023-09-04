package controllers

import (
	"github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	appv1 "k8s.io/api/apps/v1"
)

func NewDeploymentsBuilder(o *v1alpha1.Memcached) (deployments *appv1.Deployment, err error) {

	return deployments, nil
}
