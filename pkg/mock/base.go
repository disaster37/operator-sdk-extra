package mock

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
	h.isFinishedReconcile = false
}

func (h *MockBaseDefault) FinishReconcile() {
	h.isFinishedReconcile = true
}
