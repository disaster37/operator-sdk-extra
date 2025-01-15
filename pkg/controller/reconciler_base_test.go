package controller

import (
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/tools/record"
)

func (t *ControllerTestSuite) TestReconcilerBase() {
	recorder := record.NewFakeRecorder(10)
	rc := NewBaseReconciler(t.k8sClient, recorder)

	assert.Equal(t.T(), t.k8sClient, rc.Client())
	assert.Equal(t.T(), recorder, rc.Recorder())

	assert.Panics(t.T(), func() {
		NewBaseReconciler(nil, recorder)
	})

	assert.Panics(t.T(), func() {
		NewBaseReconciler(t.k8sClient, nil)
	})

}
