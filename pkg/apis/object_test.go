package apis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestDefaultObjectStatusGetConditions(t *testing.T) {
	status := &DefaultObjectStatus{
		Conditions: []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue},
			{Type: "Progressing", Status: metav1.ConditionFalse},
		},
	}

	conditions := status.GetConditions()
	assert.NotNil(t, conditions)
	assert.Equal(t, 2, len(conditions))
	assert.Equal(t, metav1.ConditionTrue, conditions[0].Status)
}

func TestDefaultObjectStatusSetConditions(t *testing.T) {
	status := &DefaultObjectStatus{}
	newConditions := []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
	}

	status.SetConditions(newConditions)
	assert.Equal(t, newConditions, status.Conditions)
}

func TestDefaultObjectStatusGetIsOnErrorNil(t *testing.T) {
	status := &DefaultObjectStatus{}

	result := status.GetIsOnError()
	assert.False(t, result)
}

func TestDefaultObjectStatusGetIsOnErrorFalse(t *testing.T) {
	falseVal := ptr.To(false)
	status := &DefaultObjectStatus{
		IsOnError: falseVal,
	}

	result := status.GetIsOnError()
	assert.False(t, result)
}

func TestDefaultObjectStatusGetIsOnErrorTrue(t *testing.T) {
	trueVal := ptr.To(true)
	status := &DefaultObjectStatus{
		IsOnError: trueVal,
	}

	result := status.GetIsOnError()
	assert.True(t, result)
}

func TestDefaultObjectStatusSetIsOnErrorFalse(t *testing.T) {
	status := &DefaultObjectStatus{}
	status.SetIsOnError(false)
	assert.NotNil(t, status.IsOnError)
	assert.False(t, *status.IsOnError)
}

func TestDefaultObjectStatusSetIsOnErrorTrue(t *testing.T) {
	status := &DefaultObjectStatus{}
	status.SetIsOnError(true)
	assert.NotNil(t, status.IsOnError)
	assert.True(t, *status.IsOnError)
}

func TestDefaultObjectStatusGetLastErrorMessageEmpty(t *testing.T) {
	status := &DefaultObjectStatus{
		LastErrorMessage: "",
	}

	result := status.GetLastErrorMessage()
	assert.Empty(t, result)
}

func TestDefaultObjectStatusGetLastErrorMessageWithValue(t *testing.T) {
	expectedMessage := "something went wrong"
	status := &DefaultObjectStatus{
		LastErrorMessage: expectedMessage,
	}

	result := status.GetLastErrorMessage()
	assert.Equal(t, expectedMessage, result)
}

func TestDefaultObjectStatusSetLastErrorMessage(t *testing.T) {
	status := &DefaultObjectStatus{}
	newMessage := "error occurred"

	status.SetLastErrorMessage(newMessage)
	assert.Equal(t, newMessage, status.LastErrorMessage)
}

func TestDefaultObjectStatusGetObservedGenerationZero(t *testing.T) {
	status := &DefaultObjectStatus{}

	result := status.GetObservedGeneration()
	assert.Equal(t, int64(0), result)
}

func TestDefaultObjectStatusGetObservedGenerationWithValue(t *testing.T) {
	status := &DefaultObjectStatus{
		ObservedGeneration: 42,
	}

	result := status.GetObservedGeneration()
	assert.Equal(t, int64(42), result)
}

func TestDefaultObjectStatusSetObservedGeneration(t *testing.T) {
	status := &DefaultObjectStatus{}
	status.SetObservedGeneration(100)
	assert.Equal(t, int64(100), status.ObservedGeneration)
}
