package mock

import "github.com/sirupsen/logrus"

type MockBase interface {
	IsFinishedReconcile() bool
	StartReconcile()
	FinishReconcile()
}

type MockBaseDefault struct {
	isFinishedReconcile bool
}

func NewMockDefault() MockBase {
	return &MockBaseDefault{
		isFinishedReconcile: false,
	}
}

func (h *MockBaseDefault) IsFinishedReconcile() bool {
	return h.isFinishedReconcile
}

func (h *MockBaseDefault) StartReconcile() {
	logrus.Debug("call StartReconcile")
	h.isFinishedReconcile = false
}

func (h *MockBaseDefault) FinishReconcile() {
	logrus.Debug("call FinishReconcile")
	h.isFinishedReconcile = true
}
