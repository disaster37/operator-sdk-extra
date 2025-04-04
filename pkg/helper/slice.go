package helper

import (
	"reflect"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeleteItemFromSlice is a generic function to remove item from a slice
func DeleteItemFromSlice[t any](x []t, index int) []t {
	if len(x) == 0 {
		return nil
	}

	res := make([]t, 0, len(x)-1)
	for i := 0; i < len(x); i++ {
		if i != index {
			res = append(res, x[i])
		}
	}

	return res
}

// StringToSlice permit to convert string with separator to slice
// Is like strings.Split with trimSpaces each items
func StringToSlice(value, separator string) (result []string) {
	if value == "" {
		return []string{}
	}
	result = strings.Split(value, separator)
	for i, s := range result {
		result[i] = strings.TrimSpace(s)
	}
	return result
}

// ToSliceOfObject permit to convert any slice of object to slice of client.Object
// Slice must not contain pointer
func ToSliceOfObject[srcType client.Object, dstType client.Object](sList []srcType) (res []dstType) {
	res = make([]dstType, 0, len(sList))

	for _, s := range sList {
		res = append(res, reflect.ValueOf(s).Interface().(dstType))
	}

	return res
}

// ToSlice convert slice of pointer object to slice of object
func ToSlice[srcType any](sList []*srcType) (res []srcType) {
	res = make([]srcType, 0, len(sList))
	for _, item := range sList {
		res = append(res, *item)
	}

	return res
}

// ToSlicePtr convert slice of object to slice of pointer object
func ToSlicePtr[srcType any](sList []srcType) (res []*srcType) {
	res = make([]*srcType, 0, len(sList))
	for _, item := range sList {
		res = append(res, &item)
	}

	return res
}
