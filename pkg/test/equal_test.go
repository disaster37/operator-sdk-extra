package test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestEqualFromYamlFile(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: "test",
		},
		Data: map[string]string{
			"fu": "bar",
		},
	}

	EqualFromYamlFile[*corev1.ConfigMap](t, "testdata/configmap.yaml", cm, scheme.Scheme)
}
