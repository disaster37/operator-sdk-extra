package sentinel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetItems(t *testing.T) {

	objects := &corev1.ConfigMapList{
		Items: []corev1.ConfigMap{
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
			},
		},
	}

	expected := []*corev1.ConfigMap{
		{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
		},
	}

	assert.Equal(t, expected, GetItems[*corev1.ConfigMapList, *corev1.ConfigMap](objects))

	assert.Panics(t, func() {
		GetItems[*corev1.ConfigMapList, *corev1.ConfigMap](nil)
	})

	var test *struct {
		client.ObjectList
	} = &struct{ client.ObjectList }{}

	assert.Panics(t, func() {
		GetItems[client.ObjectList, *corev1.ConfigMap](test)
	})

}
