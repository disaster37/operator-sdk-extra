package controllers

import (
	"github.com/disaster37/operator-sdk-extra/v2/testdata/memcached-operator/api/v1alpha1"
	"github.com/thoas/go-funk"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newConfigMapsBuilder(o *v1alpha1.Memcached) (configMaps []corev1.ConfigMap, err error) {
	configMaps = make([]corev1.ConfigMap, 0, 1)

	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      o.Name,
			Namespace: o.Namespace,
			Labels: funk.UnionStringMap(
				map[string]string{
					"name":                          o.GetName(),
					v1alpha1.MemcachedAnnotationKey: "true",
				},
				o.Labels,
			),
		},
		Data: map[string]string{
			"INSTANCE_NAME": o.Name,
		},
	}

	configMaps = append(configMaps, *cm)

	return configMaps, nil
}
