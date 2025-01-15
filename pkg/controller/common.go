package controller

import (
	"reflect"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RunningPhase   shared.PhaseName     = "running"
	StartingPhase  shared.PhaseName     = "starting"
	ReadyCondition shared.ConditionName = "Ready"
	BaseAnnotation string               = "operator-sdk-extra.webcenter.fr"
	ShortenError   int                  = 5000
)

// GetObjectMeta permit to get the metata from client.Object
func GetObjectMeta(r client.Object) metav1.ObjectMeta {
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

// GetObjectStatus permit to get the status from client.Object
func GetObjectStatus(r client.Object) any {
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

// MustInjectTypeMeta permit to inject the typeMeta from src to dst
func MustInjectTypeMeta(src, dst client.Object) {
	var (
		rt reflect.Type
	)

	rt = reflect.TypeOf(src)
	if rt.Kind() != reflect.Ptr {
		panic("Resource must be pointer")
	}
	rt = reflect.TypeOf(dst)
	if rt.Kind() != reflect.Ptr {
		panic("Resource must be pointer")
	}

	rvSrc := reflect.ValueOf(src).Elem()
	omSrc := rvSrc.FieldByName("TypeMeta")
	if !omSrc.IsValid() {
		panic("src must have field TypeMeta")
	}
	rvDst := reflect.ValueOf(dst).Elem()
	omDst := rvDst.FieldByName("TypeMeta")
	if !omDst.IsValid() {
		panic("dst must have field TypeMeta")
	}

	omDst.Set(omSrc)
}
