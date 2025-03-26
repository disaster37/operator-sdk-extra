package controllers

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/test"
	cachecrd "github.com/disaster37/operator-sdk-extra/v2/testdata/memcached-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestBuildConfigMap(t *testing.T) {
	var (
		o          *cachecrd.Memcached
		err        error
		configMaps []corev1.ConfigMap
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

	configMaps, err = newConfigMapsBuilder(o)
	assert.NoError(t, err)
	test.EqualFromYamlFile[*corev1.ConfigMap](t, "testdata/configmap.yml", &configMaps[0], scheme.Scheme)
}
