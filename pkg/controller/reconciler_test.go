package controller

import (
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/tools/record"
)

func (t *ControllerTestSuite) TestReconciler() {
	logger := logrus.NewEntry(logrus.New())
	r := NewReconciler(t.k8sClient, record.NewFakeRecorder(10), "test", logger)

	assert.Equal(t.T(), "test", r.Finalizer().String())
	assert.Equal(t.T(), logger, r.Logger())

	assert.Panics(t.T(), func() {
		NewReconciler(t.k8sClient, record.NewFakeRecorder(10), "test", nil)
	})

	r = NewReconciler(t.k8sClient, record.NewFakeRecorder(10), "", logger)
	assert.Equal(t.T(), "", r.Finalizer().String())
}
