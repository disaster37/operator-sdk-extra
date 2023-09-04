package controllers

import (
	"github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func NewConfigMapsBuilder(o *v1alpha1.Memcached) (configMaps []corev1.ConfigMap, err error) {
	return configMaps, nil
}
