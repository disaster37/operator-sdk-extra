package controller

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	BaseAnnotation string = "operator-sdk-extra.webcenter.fr"
	ShortenError   int    = 100
	RunningPhase PhaseName = "running"
	StartingPhase PhaseName = "starting"
)

// PhaseName is the the current phase name (step) on controller
type PhaseName string

// String return the phase name as string
func (o PhaseName) String() string {
	return string(o)
}

// Condition is the condition name
type ConditionName string

// String return the condition name as string
func (o ConditionName) String() string {
	return string(o)
}

// FinalizerName is the finalizer name
type FinalizerName string

// String return the finalizer name as string
func (o FinalizerName) String() string {
	return string(o)
}

func getObjectMeta(r client.Object) metav1.ObjectMeta {
	rt := reflect.TypeOf(r)
	if rt.Kind() != reflect.Ptr {
		panic("Resource must be pointer")
	}
	rv := reflect.ValueOf(r).Elem()
	om := rv.FieldByName("ObjectMeta")
	if !om.IsValid() {
		panic("Resouce must have field ObjectMeta")
	}
	return om.Interface().(metav1.ObjectMeta)
}

func getObjectStatus(r client.Object) any {
	rt := reflect.TypeOf(r)
	if rt.Kind() != reflect.Ptr {
		panic("Resource must be pointer")
	}
	rv := reflect.ValueOf(r).Elem()
	om := rv.FieldByName("Status")
	if !om.IsValid() {
		return nil
	}
	return om.Interface()
}
