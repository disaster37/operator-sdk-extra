package multiphase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBasicMultiPhaseDiffDiff(t *testing.T) {
	o := NewMultiPhaseDiff[*corev1.ConfigMap]()

	assert.False(t, o.IsDiff())
	assert.Empty(t, o.Diff())

	o.AddDiff("test")
	assert.True(t, o.IsDiff())
	assert.Contains(t, o.Diff(), "test")
}

func TestBasicMultiPhaseDiffCreate(t *testing.T) {
	o := NewMultiPhaseDiff[*corev1.ConfigMap]()

	assert.False(t, o.NeedCreate())
	assert.Empty(t, o.GetObjectsToCreate())

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

	o = NewMultiPhaseDiff[*corev1.ConfigMap]()
	o.SetObjectsToCreate([]*corev1.ConfigMap{})
	assert.False(t, o.NeedCreate())
}

func TestBasicMultiPhaseDiffUpdate(t *testing.T) {
	o := NewMultiPhaseDiff[*corev1.ConfigMap]()

	assert.False(t, o.NeedUpdate())
	assert.Empty(t, o.GetObjectsToUpdate())

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

	o = NewMultiPhaseDiff[*corev1.ConfigMap]()
	o.SetObjectsToUpdate([]*corev1.ConfigMap{})
	assert.False(t, o.NeedUpdate())
}

func TestBasicMultiPhaseDiffDelete(t *testing.T) {
	o := NewMultiPhaseDiff[*corev1.ConfigMap]()

	assert.False(t, o.NeedDelete())
	assert.Empty(t, o.GetObjectsToDelete())

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

	o = NewMultiPhaseDiff[*corev1.ConfigMap]()
	o.SetObjectsToDelete([]*corev1.ConfigMap{})
	assert.False(t, o.NeedDelete())
}

func TestBasicMultiPhaseDiffGetObjectsToApply(t *testing.T) {
	o := NewMultiPhaseDiff[*corev1.ConfigMap]()

	assert.Empty(t, o.GetObjectsToApply())

	createObj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "create",
		},
	}
	updateObj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "update",
		},
	}

	o.AddObjectToCreate(createObj)
	o.AddObjectToUpdate(updateObj)

	applyList := o.GetObjectsToApply()
	assert.Len(t, applyList, 2)
	assert.Equal(t, "create", applyList[0].GetName())
	assert.Equal(t, "update", applyList[1].GetName())
}
