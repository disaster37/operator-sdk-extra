package multiphase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBasicMultiPhaseReadCurrent(t *testing.T) {

	// With default object
	o := &DefaultMultiPhaseRead[*corev1.ConfigMap]{}

	assert.Empty(t, o.GetCurrentObjects())

	// When current objects
	objects := []*corev1.ConfigMap{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}

	o.SetCurrentObjects(objects)

	assert.Equal(t, objects, o.GetCurrentObjects())
}

func TestBasicMultiPhaseReadExpected(t *testing.T) {

	// With default object
	o := &DefaultMultiPhaseRead[*corev1.ConfigMap]{}

	assert.Empty(t, o.GetExpectedObjects())

	// When current objects
	objects := []*corev1.ConfigMap{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}

	o.SetExpectedObjects(objects)

	assert.Equal(t, objects, o.GetExpectedObjects())
}
