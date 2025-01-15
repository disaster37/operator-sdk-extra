package sentinel

import (
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/multiphase"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SentinelRead is the interface to store the result of read from sentinel reconciler
// objectType is the key if you need to handle some type of object to not shuffle them
type SentinelRead interface {

	// GetAllCurrentObjects premit to get the map of current objects, mapped by type
	GetReads() map[string]multiphase.MultiPhaseRead[client.Object]

	// SetCurrentObjects will add object on the right list
	SetCurrentObjects(objects []client.Object)

	// AddCurrentObject will add object on the right list
	AddCurrentObject(o client.Object)

	// AddExpectedObject will add object on the right list
	AddExpectedObject(o client.Object)

	// SetExpectedObjects will add object on the right list
	SetExpectedObjects(objects []client.Object)
}

// DefaultSentinelRead is the default implementation of SentinelRead
type DefaultSentinelRead struct {
	reads  map[string]multiphase.MultiPhaseRead[client.Object]
	scheme runtime.ObjectTyper
}

// NewSentinelRead is the default implementation of SentinelRead interface
func NewSentinelRead(scheme runtime.ObjectTyper) SentinelRead {
	return &DefaultSentinelRead{
		reads:  map[string]multiphase.MultiPhaseRead[client.Object]{},
		scheme: scheme,
	}
}

func (h *DefaultSentinelRead) GetReads() map[string]multiphase.MultiPhaseRead[client.Object] {
	return h.reads
}

func (h *DefaultSentinelRead) SetCurrentObjects(objects []client.Object) {
	for _, o := range objects {
		h.AddCurrentObject(o)
	}
}

func (h *DefaultSentinelRead) AddCurrentObject(o client.Object) {
	o = GetObjectWithMeta(o, h.scheme)
	t := GetObjectType(o.GetObjectKind())
	if h.reads[t] == nil {
		h.reads[t] = multiphase.NewMultiPhaseRead[client.Object]()
	}

	h.reads[t].AddCurrentObject(o)

}

func (h *DefaultSentinelRead) SetExpectedObjects(objects []client.Object) {
	for _, o := range objects {
		h.AddExpectedObject(o)
	}
}

func (h *DefaultSentinelRead) AddExpectedObject(o client.Object) {
	o = GetObjectWithMeta(o, h.scheme)
	t := GetObjectType(o.GetObjectKind())
	if h.reads[t] == nil {
		h.reads[t] = multiphase.NewMultiPhaseRead[client.Object]()
	}

	h.reads[t].AddExpectedObject(o)
}
