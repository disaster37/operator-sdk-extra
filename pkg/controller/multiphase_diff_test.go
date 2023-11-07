package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestBasicMultiPhaseDiffDiff(t *testing.T) {

	// With default object
	o := &BasicMultiPhaseDiff{}

	assert.False(t, o.IsDiff())
	assert.Empty(t, o.Diff())

	// When diff
	o.AddDiff("test")
	assert.True(t, o.IsDiff())
	assert.Contains(t, o.Diff(), "test")

}

func TestBasicMultiPhaseDiffCreate(t *testing.T) {

	// With default object
	o := &BasicMultiPhaseDiff{}

	assert.False(t, o.NeedCreate())
	assert.Empty(t, o.GetObjectsToCreate())

	objects := []client.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}

	// When need to create object
	o.SetObjectsToCreate(objects)

	assert.True(t, o.NeedCreate())
	assert.Equal(t, objects, o.GetObjectsToCreate())
}

func TestBasicMultiPhaseDiffUpdate(t *testing.T) {

	// With default object
	o := &BasicMultiPhaseDiff{}

	assert.False(t, o.NeedUpdate())
	assert.Empty(t, o.GetObjectsToUpdate())

	objects := []client.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}

	// When need to update object
	o.SetObjectsToUpdate(objects)

	assert.True(t, o.NeedUpdate())
	assert.Equal(t, objects, o.GetObjectsToUpdate())
}

func TestBasicMultiPhaseDiffDelete(t *testing.T) {

	// With default object
	o := &BasicMultiPhaseDiff{}

	assert.False(t, o.NeedDelete())
	assert.Empty(t, o.GetObjectsToDelete())

	objects := []client.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		},
	}

	// When need to delete object
	o.SetObjectsToDelete(objects)

	assert.True(t, o.NeedDelete())
	assert.Equal(t, objects, o.GetObjectsToDelete())
}
