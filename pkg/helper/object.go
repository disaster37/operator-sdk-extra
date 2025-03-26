package helper

import (
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ToObject permit to convert object type
func ToObject[srcType client.Object, dstType client.Object](o srcType) (res dstType) {
	return reflect.ValueOf(o).Interface().(dstType)
}
