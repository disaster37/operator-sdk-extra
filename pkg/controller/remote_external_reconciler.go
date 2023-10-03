package controller

import (
	"reflect"

	"emperror.dev/errors"
	"github.com/disaster37/generic-objectmatcher/patch"
	jsonIterator "github.com/json-iterator/go"
)

// RemoteExternalReconciler is the interface to call the remote API to handler resource
type RemoteExternalReconciler[k8sObject comparable, apiObject comparable] interface {
	Build(o k8sObject) (object apiObject, err error)
	Get(name string) (object apiObject, err error)
	Create(object apiObject) (err error)
	Update(object apiObject) (err error)
	Delete(name string) (err error)
	Diff(currentOject apiObject, expectedObject apiObject, originalObject apiObject, ignoresDiff ...patch.CalculateOption) (patchResult *patch.PatchResult, err error)
	Custom(name string, k8sO k8sObject, apiO apiObject, params ...any) (err error)
}

// BasicRemoteExternalReconciler is the basic implementation of RemoteExternalReconciler
// It only implement the Diff method, because of is generic with 3-way merge patch
type BasicRemoteExternalReconciler[k8sObject comparable, apiObject comparable] struct{}

func (h *BasicRemoteExternalReconciler[k8sObject, apiObject]) Diff(currentOject apiObject, expectedObject apiObject, originalObject apiObject, ignoresDiff ...patch.CalculateOption) (patchResult *patch.PatchResult, err error) {
	if reflect.ValueOf(currentOject).IsNil() {
		expected, err := jsonIterator.ConfigCompatibleWithStandardLibrary.Marshal(expectedObject)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to convert expected object to byte sequence")
		}

		return &patch.PatchResult{
			Patch:    expected,
			Current:  expected,
			Modified: expected,
			Original: nil,
			Patched:  expectedObject,
		}, nil
	}

	return patch.DefaultPatchMaker.Calculate(currentOject, expectedObject, originalObject, ignoresDiff...)
}

func (h *BasicRemoteExternalReconciler[k8sObject, apiObject]) Custom(name string, k8sO k8sObject, apiO apiObject, params ...any) (err error) {
	return nil
}
