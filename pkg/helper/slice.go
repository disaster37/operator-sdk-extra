package helper

import (
	"reflect"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeleteItemFromSlice is a generic function to remove item from a slice
func DeleteItemFromSlice(x any, index int) any {
	if x == nil || reflect.ValueOf(x).IsNil() {
		return x
	}
	xValue := reflect.ValueOf(x)
	xType := xValue.Type()
	if xType.Kind() != reflect.Slice {
		panic("First parameter must be a slice")
	}

	expectedSlice := reflect.MakeSlice(reflect.SliceOf(xType.Elem()), 0, xValue.Len()-1)

	for i := 0; i < xValue.Len(); i++ {
		if i != index {
			expectedSlice = reflect.Append(expectedSlice, xValue.Index(i))
		}
	}

	return expectedSlice.Interface()
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
