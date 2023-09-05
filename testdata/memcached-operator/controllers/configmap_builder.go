package controllers

import (
	"github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewConfigMapsBuilder(o *v1alpha1.Memcached) (configMaps []corev1.ConfigMap, err error) {
	configMaps = make([]corev1.ConfigMap, 0, 1)

	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: o.Name,
		},
		Data: map[string]string{
			"INSTANCE_NAME": o.Name,
		},
	}

	configMaps = append(configMaps, *cm)

	return configMaps, nil
}
