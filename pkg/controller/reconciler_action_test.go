package controller

import (
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/tools/record"
)

func (t *ControllerTestSuite) TestReconcilerActionCondition() {
	ra := NewReconcilerAction(t.k8sClient, record.NewFakeRecorder(10), "test")

	assert.Equal(t.T(), "test", ra.Condition().String())

	assert.Panics(t.T(), func() {
		NewReconcilerAction(t.k8sClient, record.NewFakeRecorder(10), "")
	})
}
