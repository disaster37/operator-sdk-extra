package controllers

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/pkg/test"
	cachecrd "github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestBuildDeployment(t *testing.T) {
	var (
		o    *cachecrd.Memcached
		err  error
		dpls []appv1.Deployment
	)

	// When no target elasticsearch
	o = &cachecrd.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: cachecrd.MemcachedSpec{
			Size:          1,
			ContainerPort: 8080,
		},
	}

	dpls, err = newDeploymentsBuilder(o)
	assert.NoError(t, err)
	test.EqualFromYamlFile[*appv1.Deployment](t, "testdata/deployment.yml", &dpls[0], scheme.Scheme)
}
