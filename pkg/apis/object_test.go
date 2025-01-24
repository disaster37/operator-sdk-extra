package apis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestObjectStatusCondition(t *testing.T) {
	o := &DefaultObjectStatus{}

	assert.Empty(t, o.GetConditions())

	condition := v1.Condition{
		Type:    "test",
		Message: "test",
	}

	o.SetConditions([]v1.Condition{condition})

	assert.Equal(t, []v1.Condition{condition}, o.GetConditions())
}

func TestObjectStatusError(t *testing.T) {
	o := &DefaultObjectStatus{}

	assert.False(t, o.GetIsOnError())

	o.SetIsOnError(true)
	assert.True(t, o.GetIsOnError())
}

func TestObjectStatusErrorMessage(t *testing.T) {
	o := &DefaultObjectStatus{}

	assert.Empty(t, o.GetLastErrorMessage())

	o.SetLastErrorMessage("test")
	assert.Equal(t, "test", o.GetLastErrorMessage())
}

func TestObjectStatusObservedGeneration(t *testing.T) {
	o := &DefaultObjectStatus{}

	assert.Equal(t, int64(0), o.GetObservedGeneration())

	o.SetObservedGeneration(10)
	assert.Equal(t, int64(10), o.GetObservedGeneration())
}
