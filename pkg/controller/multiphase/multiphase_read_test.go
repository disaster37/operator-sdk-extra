package multiphase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBasicMultiPhaseReadCurrent(t *testing.T) {
	// With default object
	o := NewMultiPhaseRead[*corev1.ConfigMap]()
	assert.Empty(t, o.GetCurrentObjects())

	// When set a list
	o = NewMultiPhaseRead[*corev1.ConfigMap]()
	objects := []*corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}
	o.SetCurrentObjects(objects)
	assert.Equal(t, objects, o.GetCurrentObjects())

	// When set a list with existing list
	o = NewMultiPhaseRead[*corev1.ConfigMap]()
	objects = []*corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}
	o.SetCurrentObjects(objects)
	o.SetCurrentObjects(objects)
	assert.Equal(t, 2, len(o.GetCurrentObjects()))

	// When add pne object
	o = NewMultiPhaseRead[*corev1.ConfigMap]()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	objects = []*corev1.ConfigMap{cm}
	o.AddCurrentObject(cm)
	assert.Equal(t, objects, o.GetCurrentObjects())
}

func TestBasicMultiPhaseReadExpected(t *testing.T) {
	// With default object
	o := NewMultiPhaseRead[*corev1.ConfigMap]()
	assert.Empty(t, o.GetExpectedObjects())

	// When set a list
	o = NewMultiPhaseRead[*corev1.ConfigMap]()
	objects := []*corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}
	o.SetExpectedObjects(objects)
	assert.Equal(t, objects, o.GetExpectedObjects())

	// When set a list with existing list
	o = NewMultiPhaseRead[*corev1.ConfigMap]()
	objects = []*corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}
	o.SetExpectedObjects(objects)
	o.SetExpectedObjects(objects)
	assert.Equal(t, 2, len(o.GetExpectedObjects()))

	// When add pne object
	o = NewMultiPhaseRead[*corev1.ConfigMap]()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	objects = []*corev1.ConfigMap{cm}
	o.AddExpectedObject(cm)
	assert.Equal(t, objects, o.GetExpectedObjects())
}
