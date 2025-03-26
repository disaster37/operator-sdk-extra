package multiphase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBasicMultiPhaseDiffDiff(t *testing.T) {
	// With default object
	o := NewMultiPhaseDiff[*corev1.ConfigMap]()

	assert.False(t, o.IsDiff())
	assert.Empty(t, o.Diff())

	// When diff
	o.AddDiff("test")
	assert.True(t, o.IsDiff())
	assert.Contains(t, o.Diff(), "test")
}

func TestBasicMultiPhaseDiffCreate(t *testing.T) {
	// With default object
	o := NewMultiPhaseDiff[*corev1.ConfigMap]()

	assert.False(t, o.NeedCreate())
	assert.Empty(t, o.GetObjectsToCreate())

	// When set a list of object when empty
	o = NewMultiPhaseDiff[*corev1.ConfigMap]()
	objects := []*corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}
	o.SetObjectsToCreate(objects)
	assert.True(t, o.NeedCreate())
	assert.Equal(t, objects, o.GetObjectsToCreate())

	// When set a list not empty
	o = NewMultiPhaseDiff[*corev1.ConfigMap]()
	objects = []*corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}
	o.SetObjectsToCreate(objects)
	o.SetObjectsToCreate(objects)
	assert.True(t, o.NeedCreate())
	assert.Equal(t, 2, len(o.GetObjectsToCreate()))

	// When add object
	o = NewMultiPhaseDiff[*corev1.ConfigMap]()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	objects = []*corev1.ConfigMap{cm}

	o.AddObjectToCreate(cm)
	assert.True(t, o.NeedCreate())
	assert.Equal(t, objects, o.GetObjectsToCreate())
}

func TestBasicMultiPhaseDiffUpdate(t *testing.T) {
	// With default object
	o := NewMultiPhaseDiff[*corev1.ConfigMap]()

	assert.False(t, o.NeedUpdate())
	assert.Empty(t, o.GetObjectsToUpdate())

	// When set a list of object when empty
	o = NewMultiPhaseDiff[*corev1.ConfigMap]()
	objects := []*corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}

	o.SetObjectsToUpdate(objects)
	assert.True(t, o.NeedUpdate())
	assert.Equal(t, objects, o.GetObjectsToUpdate())

	// When set a list not empty
	o = NewMultiPhaseDiff[*corev1.ConfigMap]()
	objects = []*corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}
	o.SetObjectsToUpdate(objects)
	o.SetObjectsToUpdate(objects)
	assert.True(t, o.NeedUpdate())
	assert.Equal(t, 2, len(o.GetObjectsToUpdate()))

	// When add object
	o = NewMultiPhaseDiff[*corev1.ConfigMap]()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	objects = []*corev1.ConfigMap{cm}

	o.AddObjectToUpdate(cm)
	assert.True(t, o.NeedUpdate())
	assert.Equal(t, objects, o.GetObjectsToUpdate())
}

func TestBasicMultiPhaseDiffDelete(t *testing.T) {
	// With default object
	o := NewMultiPhaseDiff[*corev1.ConfigMap]()

	assert.False(t, o.NeedDelete())
	assert.Empty(t, o.GetObjectsToDelete())

	// When need to delete object
	o = NewMultiPhaseDiff[*corev1.ConfigMap]()
	objects := []*corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}
	o.SetObjectsToDelete(objects)
	assert.True(t, o.NeedDelete())
	assert.Equal(t, objects, o.GetObjectsToDelete())

	// When set a list not empty
	o = NewMultiPhaseDiff[*corev1.ConfigMap]()
	objects = []*corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}
	o.SetObjectsToDelete(objects)
	o.SetObjectsToDelete(objects)
	assert.True(t, o.NeedDelete())
	assert.Equal(t, 2, len(o.GetObjectsToDelete()))

	// When add object
	o = NewMultiPhaseDiff[*corev1.ConfigMap]()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	objects = []*corev1.ConfigMap{cm}

	o.AddObjectToDelete(cm)
	assert.True(t, o.NeedDelete())
	assert.Equal(t, objects, o.GetObjectsToDelete())
}
