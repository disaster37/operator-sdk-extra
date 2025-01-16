package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetObjectMeta(t *testing.T) {
	meta := v1.ObjectMeta{
		Name:      "test",
		Namespace: "default",
	}

	o := &corev1.ConfigMap{
		ObjectMeta: meta,
	}

	assert.Equal(t, meta, GetObjectMeta(o))

	assert.Panics(t, func() {
		GetObjectMeta(nil)
	})
}

func TestGetObjectStatus(t *testing.T) {
	status := corev1.PodStatus{
		Message: "test",
	}
	o := &corev1.Pod{
		Status: status,
	}

	assert.Equal(t, status, GetObjectStatus(o))

	assert.Panics(t, func() {
		GetObjectStatus(nil)
	})
}

func TestMustInjectTypeMeta(t *testing.T) {
	meta := v1.TypeMeta{
		Kind: "ConfigMap",
	}
	src := &corev1.ConfigMap{
		TypeMeta: meta,
	}

	dst := &corev1.ConfigMap{}

	MustInjectTypeMeta(src, dst)
	assert.Equal(t, src, dst)

	assert.Panics(t, func() {
		MustInjectTypeMeta(nil, nil)
	})
}
