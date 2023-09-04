package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestObjectGetStatus(t *testing.T) {
	status := BasicObjectStatus{
		LastErrorMessage: "fake",
	}

	o := &BasicObject{
		Status: status,
	}

	assert.Equal(t, &status, o.GetStatus())
}

func TestBasicObjectStatusConditions(t *testing.T) {

	// With default object
	o := &BasicObjectStatus{}

	assert.Empty(t, o.GetConditions())

	conditions := []metav1.Condition{
		{
			Type:    "TestCondition",
			Status:  metav1.ConditionTrue,
			Reason:  "Test",
			Message: "It's a test",
		},
	}
	o.SetConditions(conditions)
	assert.Equal(t, conditions, o.GetConditions())

}

func TestBasicObjectError(t *testing.T) {
	// With default object
	o := &BasicObjectStatus{}

	assert.False(t, o.GetIsOnError())

	// When no error
	o.SetIsOnError(false)
	assert.False(t, o.GetIsOnError())

	// When error
	o.SetIsOnError(true)
	assert.True(t, o.GetIsOnError())
}

func TestBasicObjectStatusLastErrorMessage(t *testing.T) {
	// With default object
	o := &BasicObjectStatus{}

	assert.Empty(t, o.GetLastErrorMessage())

	// When error message
	o.SetLastErrorMessage("fake error")
	assert.Equal(t, "fake error", o.GetLastErrorMessage())
}
