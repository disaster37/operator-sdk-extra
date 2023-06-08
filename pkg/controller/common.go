package controller

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	Base_annotation = "operator-sdk-extra.webcenter.fr"
)

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
